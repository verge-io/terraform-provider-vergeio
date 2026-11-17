package acctest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestReferencesSweptObject(t *testing.T) {
	vms := map[int]string{9: ResourcePrefix + "vm"}
	networks := map[int]string{4: ResourcePrefix + "net"}
	users := map[int]string{7: ResourcePrefix + "user"}
	groups := map[int]string{3: ResourcePrefix + "group"}

	if !referencesSweptObject("vms/9", vms, networks, users, groups) {
		t.Fatal("vm reference should match")
	}
	if !referencesSweptObject("vnets/4", vms, networks, users, groups) {
		t.Fatal("network reference should match")
	}
	if !referencesSweptObject("users/7", vms, networks, users, groups) {
		t.Fatal("user reference should match")
	}
	if !referencesSweptObject("groups/3", vms, networks, users, groups) {
		t.Fatal("group reference should match")
	}
	if referencesSweptObject("vms/10", vms, networks, users, groups) {
		t.Fatal("unrelated vm reference must not match")
	}
	if referencesSweptObject("production", vms, networks, users, groups) {
		t.Fatal("bare names must not match")
	}
	if ownerVMID("vms/9") != 9 {
		t.Fatal("owner vm id was not parsed")
	}
	if ownerVMID("vnets/4") != 0 {
		t.Fatal("non-vm owner must be ignored")
	}

	userName := ResourcePrefix + "user"
	groupName := ResourcePrefix + "group"
	if !permissionBelongsToSweep(vergeos.Permission{Identity: 7, Table: "vms"}, users, groups) {
		t.Fatal("permission on a swept user key should match")
	}
	if !permissionBelongsToSweep(vergeos.Permission{Identity: 99, IdentityDisplay: groupName, Table: "vms"}, users, groups) {
		t.Fatal("permission whose display name is a swept group should match")
	}
	if !permissionBelongsToSweep(vergeos.Permission{Identity: 1, Table: "groups", Row: 3}, users, groups) {
		t.Fatal("permission on a swept group row should match")
	}
	if !permissionBelongsToSweep(vergeos.Permission{Identity: 1, Table: "users", Row: 7}, users, groups) {
		t.Fatal("permission on a swept user row should match")
	}
	if permissionBelongsToSweep(vergeos.Permission{Identity: 1, IdentityDisplay: "admin", Table: "vms", Row: 0}, users, groups) {
		t.Fatal("unrelated table grant must not match")
	}
	if userName == "" || groupName == "" {
		t.Fatal("names should be set")
	}
}

func TestSnapshotBelongsToSweep(t *testing.T) {
	parents := map[int]string{70: ResourcePrefix + "vm"}
	if !snapshotBelongsToSweep(ResourcePrefix+"snap", 0, parents) {
		t.Fatal("prefixed snapshot name should match")
	}
	if !snapshotBelongsToSweep("snapshot-20060102-150405", 70, parents) {
		t.Fatal("snapshot of a swept parent should match")
	}
	if snapshotBelongsToSweep("nightly", 3, parents) {
		t.Fatal("unrelated snapshot must not match")
	}
	if snapshotBelongsToSweep("nightly", 0, parents) {
		t.Fatal("snapshot with no parent must not match")
	}
}

func TestDeleteNodesOfTenantDeletesHighestNodeIDFirst(t *testing.T) {
	var mu sync.Mutex
	var deleted []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenant_nodes":
			// List order is the lower nodeid first. VergeOS still requires
			// the highest nodeid to be deleted first.
			_, _ = w.Write([]byte(`[{"$key":7,"tenant":44,"name":"a","machine":0,"enabled":true,"nodeid":1},{"$key":11,"tenant":44,"name":"b","machine":0,"enabled":true,"nodeid":2}]`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/tenant_nodes/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v4/tenant_nodes/")
			switch id {
			case "7":
				_, _ = w.Write([]byte(`{"$key":7,"tenant":44,"name":"a","machine":0,"enabled":true,"nodeid":1}`))
			case "11":
				_, _ = w.Write([]byte(`{"$key":11,"tenant":44,"name":"b","machine":0,"enabled":true,"nodeid":2}`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/tenant_nodes/"):
			id := strings.TrimPrefix(r.URL.Path, "/api/v4/tenant_nodes/")
			mu.Lock()
			deleted = append(deleted, id)
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
		default:
			http.Error(w, "unexpected "+r.Method+" "+r.URL.RequestURI(), http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	sdk, err := vergeos.NewClient(
		vergeos.WithBaseURL(server.URL),
		vergeos.WithCredentials("user", "pass"),
		vergeos.WithInsecureTLS(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := deleteNodesOfTenant(context.Background(), sdk, 44); err != nil {
		t.Fatal(err)
	}
	if strings.Join(deleted, ",") != "11,7" {
		t.Fatalf("delete order = %#v, want highest nodeid first", deleted)
	}
}

// TestSweep removes prefixed leftovers. It stays out of a normal acceptance
// run, including one that sets TF_ACC and tests packages in parallel.
// Run it alone with TF_ACC=1 and TF_ACC_SWEEP=1. Unit CI never contacts a
// VergeOS system.
func TestSweep(t *testing.T) {
	if os.Getenv("TF_ACC") == "" || os.Getenv("TF_ACC_SWEEP") == "" {
		t.Skip("acceptance sweep skipped unless TF_ACC=1 and TF_ACC_SWEEP=1")
	}
	PreCheck(t)
	if err := Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
}
