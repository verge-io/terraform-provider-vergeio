package vm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

func TestSyncDisksRenamePutsInPlace(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_drives/46":
			if !strings.Contains(string(body), `"name":"data2"`) {
				t.Errorf("PUT body = %s, want name data2", body)
			}
			if strings.Contains(string(body), "46") {
				t.Errorf("PUT body sent the drive key: %s", body)
			}
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/46":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"data2","disksize":1073741824,"interface":"virtio","enabled":true,"orderid":0}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Key:       types.StringValue("46"),
		Name:      types.StringValue("data2"),
		Interface: types.StringValue("virtio"),
		Enabled:   types.BoolValue(true),
		DiskSize:  types.Float64Value(1),
	}}
	state := []*diskResourceModel{{
		Key:       types.StringValue("46"),
		Name:      types.StringValue("data"),
		Interface: types.StringValue("virtio"),
		Enabled:   types.BoolValue(true),
		DiskSize:  types.Float64Value(1),
		Media:     types.StringValue("disk"),
	}}
	// The plan keeps media from state. A rename must not look like a media change.
	plan[0].Media = state[0].Media

	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if len(state) != 1 || state[0].Key.ValueString() != "46" || state[0].Name.ValueString() != "data2" {
		t.Fatalf("state after rename = key %q name %q", state[0].Key.ValueString(), state[0].Name.ValueString())
	}
	if len(calls) != 2 || calls[0] != "PUT /api/v4/machine_drives/46" || calls[1] != "GET /api/v4/machine_drives/46" {
		t.Fatalf("calls = %#v, want PUT then GET of drive 46", calls)
	}
}

func TestSyncDisksMediaChangeDoesNotRecreate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Key:   types.StringValue("46"),
		Name:  types.StringValue("data"),
		Media: types.StringValue("cdrom"),
	}}
	state := []*diskResourceModel{{
		Key:   types.StringValue("46"),
		Name:  types.StringValue("data"),
		Media: types.StringValue("disk"),
	}}
	err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7"))
	if err == nil {
		t.Fatal("expected media change to be refused")
	}
	if state[0].Key.ValueString() != "46" {
		t.Fatalf("drive key changed to %q", state[0].Key.ValueString())
	}
}

func TestSyncDisksNameFallbackUpdatesExistingKey(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == http.MethodPut && r.URL.Path == "/api/v4/machine_drives/11" {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_drives/11" {
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"name":"os","description":"next"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusInternalServerError)
	}))
	defer server.Close()

	api := &DiskApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	plan := []*diskResourceModel{{
		Name:        types.StringValue("os"),
		Description: types.StringValue("next"),
	}}
	state := []*diskResourceModel{{
		Key:         types.StringValue("11"),
		Name:        types.StringValue("os"),
		Description: types.StringValue("prev"),
	}}
	if err := api.syncDisks(t.Context(), &plan, &state, types.Int32Value(1), types.StringValue("7")); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "PUT /api/v4/machine_drives/11" {
		t.Fatalf("calls = %#v, want PUT of the existing drive", calls)
	}
	if state[0].Key.ValueString() != "11" {
		t.Fatalf("key = %q, want 11", state[0].Key.ValueString())
	}
}
