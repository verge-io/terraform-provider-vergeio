// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestReadVMNilClientDoesNotPanic(t *testing.T) {
	err := (*VMApi)(nil).readVM(t.Context(), &VMResourceModel{Id: types.StringValue("4")})
	if err == nil {
		t.Fatal("nil VM api should return an error")
	}
	err = (&VMApi{}).readVM(t.Context(), &VMResourceModel{Id: types.StringValue("4")})
	if err == nil {
		t.Fatal("unconfigured VM api should return an error")
	}
}

func TestReadVMEmptyResponseDoesNotPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms/4":
			_, _ = w.Write([]byte(`null`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true))
	err := api.readVM(t.Context(), &VMResourceModel{Id: types.StringValue("4")})
	if err == nil {
		t.Fatal("empty VM response should be an error")
	}
}

func TestReadVMAppliesSnapshotProfile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms/4":
			_, _ = w.Write([]byte(`{"$key":4,"machine":3,"name":"web","snapshot_profile":12}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true))
	data := &VMResourceModel{Id: types.StringValue("4")}
	if err := api.readVM(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Name.ValueString() != "web" || data.SnapshotProfile.ValueInt32() != 12 || data.Machine.ValueInt32() != 3 {
		t.Fatalf("vm = name %q profile %d machine %d", data.Name.ValueString(), data.SnapshotProfile.ValueInt32(), data.Machine.ValueInt32())
	}
}

// TestReadVMRetriesSDKSetup is the snapshot acceptance failure: the VM API
// was built while govergeos setup failed, then Read dereferenced a nil SDK.
func TestReadVMRetriesSDKSetup(t *testing.T) {
	var versionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			versionCalls++
			if versionCalls == 1 {
				http.Error(w, "unavailable", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms/4":
			_, _ = w.Write([]byte(`{"$key":4,"machine":3,"name":"web","snapshot_profile":12}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	vergeClient := vergeio.NewClient(server.URL, "user", "pass", true)
	api := NewVMApi(vergeClient)
	if api.sdk != nil {
		t.Fatal("first SDK setup should fail")
	}
	data := &VMResourceModel{Id: types.StringValue("4")}
	if err := api.readVM(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.SnapshotProfile.ValueInt32() != 12 {
		t.Fatalf("snapshot_profile = %d, want 12", data.SnapshotProfile.ValueInt32())
	}
}

// TestReadVMReusesSDKClient covers a later Configure whose own version check
// fails after the snapshot profile resource already connected.
func TestReadVMReusesSDKClient(t *testing.T) {
	var versionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			versionCalls++
			if versionCalls > 1 {
				http.Error(w, "unavailable", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms/4":
			_, _ = w.Write([]byte(`{"$key":4,"machine":3,"name":"web","snapshot_profile":12}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	vergeClient := vergeio.NewClient(server.URL, "user", "pass", true)
	if _, err := vergeClient.SDK(); err != nil {
		t.Fatal(err)
	}
	api := &VMApi{client: vergeClient}
	data := &VMResourceModel{Id: types.StringValue("4")}
	if err := api.readVM(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if versionCalls != 1 {
		t.Fatalf("version checks = %d, want 1", versionCalls)
	}
	if data.Name.ValueString() != "web" || data.SnapshotProfile.ValueInt32() != 12 {
		t.Fatalf("vm = name %q profile %d", data.Name.ValueString(), data.SnapshotProfile.ValueInt32())
	}
}

// TestReadVMPrefersMachineRunning is a UI shutdown that leaves the powerstate
// column true. Refresh has to store the machine running flag, or the next
// plan does not show the drift.
func TestReadVMPrefersMachineRunning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms/4":
			_, _ = w.Write([]byte(`{"$key":4,"machine":3,"name":"web","powerstate":true,"running":false,"status":"stopped"}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true))
	data := &VMResourceModel{Id: types.StringValue("4")}
	if err := api.readVM(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.PowerState.IsNull() || data.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want false from machine running", data.PowerState)
	}
}

// TestReadVMKeepsPowerStateAlias is a Get that already aliased
// machine#status#running as powerstate and did not include a separate
// running field.
func TestReadVMKeepsPowerStateAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms/4":
			_, _ = w.Write([]byte(`{"$key":4,"machine":3,"name":"web","powerstate":true}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true))
	data := &VMResourceModel{Id: types.StringValue("4")}
	if err := api.readVM(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.PowerState.IsNull() || !data.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want true", data.PowerState)
	}
}
