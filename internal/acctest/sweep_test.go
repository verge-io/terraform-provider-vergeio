package acctest

import (
	"context"
	"os"
	"testing"

	"github.com/verge-io/govergeos"
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

// TestSweep removes prefixed leftovers. It is skipped unless TF_ACC and lab
// credentials are set, so unit CI never contacts a VergeOS system.
func TestSweep(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance sweep skipped unless TF_ACC=1")
	}
	PreCheck(t)
	if err := Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
}
