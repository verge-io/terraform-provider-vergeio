// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestTenantCopySchema(t *testing.T) {
	item := NewTenantCopyResource()
	meta := &resource.MetadataResponse{}
	item.Metadata(t.Context(), resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_copy" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &resource.SchemaResponse{}
	item.Schema(t.Context(), resource.SchemaRequest{}, resp)
	for _, name := range []string{"tenant_id", "name"} {
		attr := resp.Schema.Attributes[name]
		if attr == nil || !attr.IsRequired() {
			t.Fatalf("%s should be required", name)
		}
	}
	isolate := resp.Schema.Attributes["isolate"]
	if isolate == nil || !isolate.IsOptional() || !isolate.IsComputed() {
		t.Fatal("isolate should be optional and computed")
	}
	if resp.Schema.Attributes["snapshot_id"] != nil || resp.Schema.Attributes["snapshot_name"] != nil {
		t.Fatal("snapshot fields should not be arguments")
	}
	if !strings.Contains(resp.Schema.MarkdownDescription, "current state") || !strings.Contains(resp.Schema.MarkdownDescription, "does not expose") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestCreateTenantCopyClonesCurrentStateAndIsolatesTheCopy(t *testing.T) {
	poll, timeout := tenantCopyPoll, tenantCopyTimeout
	tenantCopyPoll = time.Millisecond
	tenantCopyTimeout = 2 * time.Second
	t.Cleanup(func() {
		tenantCopyPoll = poll
		tenantCopyTimeout = timeout
	})

	var actions []map[string]any
	cloned := false
	isolated := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v4/tenant_snapshots"):
			t.Errorf("tenant copy touched a snapshot: %s %s", r.Method, r.URL.Path)
			http.Error(w, "snapshot", http.StatusMethodNotAllowed)
		case r.URL.Path == "/api/v4/tenants" && r.Method == http.MethodGet:
			if !cloned {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = w.Write([]byte(`[{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":false,"vnet":0}]`))
		case r.URL.Path == "/api/v4/tenants/40" && r.Method == http.MethodGet:
			isolate := "false"
			if isolated {
				isolate = "true"
			}
			_, _ = w.Write([]byte(`{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":` + isolate + `,"vnet":0}`))
		case r.URL.Path == "/api/v4/tenants/7" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":7,"name":"source","is_snapshot":false,"isolate":false,"vnet":4}`))
		case r.URL.Path == "/api/v4/tenant_actions" && r.Method == http.MethodPost:
			body := readTenantAction(t, r)
			actions = append(actions, body)
			switch body["action"] {
			case "clone":
				cloned = true
			case "isolateon":
				if body["tenant"] != float64(40) {
					t.Errorf("isolateon tenant = %v", body["tenant"])
				}
				isolated = true
			case "restore", "isolateoff":
				t.Errorf("forbidden action %#v", body)
			}
			_, _ = w.Write([]byte(`{"$key":1}`))
		case r.URL.Path == "/api/v4/tenant_status":
			_, _ = w.Write([]byte(`[]`))
		case r.URL.Path == "/api/v4/tenants/40" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testTenantAPI(t, server.URL)
	data := &TenantCopyResourceModel{
		TenantID: types.StringValue("7"),
		Name:     types.StringValue("sandbox"),
		Isolate:  types.BoolValue(true),
	}
	if err := api.createTenantCopy(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() != "40" {
		t.Fatalf("copy = %#v", data)
	}
	if len(actions) != 2 || actions[0]["action"] != "clone" || actions[0]["tenant"] != float64(7) {
		t.Fatalf("actions = %#v", actions)
	}
	params, _ := actions[0]["params"].(map[string]any)
	if params["name"] != "sandbox" {
		t.Fatalf("clone params = %#v", params)
	}
	if _, ok := params["snapshot"]; ok {
		t.Fatalf("clone params included snapshot: %#v", params)
	}
	if actions[1]["action"] != "isolateon" || actions[1]["tenant"] != float64(40) {
		t.Fatalf("isolate = %#v", actions[1])
	}

	if err := api.deleteTenantCopy(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	data.Id = types.StringValue("7")
	if err := api.deleteTenantCopy(t.Context(), data); err == nil {
		t.Fatal("deleted the source tenant")
	}
}

func TestCreateTenantCopyDeletesCopyWhenIsolateFails(t *testing.T) {
	for _, tc := range []struct {
		name    string
		isolate bool
		want    string
	}{
		{name: "action error", isolate: false, want: "isolate tenant 40"},
		{name: "still connected", isolate: true, want: "is not isolated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			poll, timeout := tenantCopyPoll, tenantCopyTimeout
			tenantCopyPoll = time.Millisecond
			tenantCopyTimeout = 2 * time.Second
			t.Cleanup(func() {
				tenantCopyPoll = poll
				tenantCopyTimeout = timeout
			})

			var deleted []string
			cloned := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				case r.URL.Path == "/api/v4/tenants" && r.Method == http.MethodGet:
					if !cloned {
						_, _ = w.Write([]byte(`[]`))
						return
					}
					_, _ = w.Write([]byte(`[{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":false,"vnet":0}]`))
				case r.URL.Path == "/api/v4/tenants/40" && r.Method == http.MethodGet:
					_, _ = w.Write([]byte(`{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":false,"vnet":0}`))
				case r.URL.Path == "/api/v4/tenants/7" && r.Method == http.MethodGet:
					_, _ = w.Write([]byte(`{"$key":7,"name":"source","is_snapshot":false,"isolate":false,"vnet":4}`))
				case r.URL.Path == "/api/v4/tenant_actions" && r.Method == http.MethodPost:
					body := readTenantAction(t, r)
					switch body["action"] {
					case "clone":
						cloned = true
						_, _ = w.Write([]byte(`{"$key":1}`))
					case "isolateon":
						if !tc.isolate {
							http.Error(w, "isolate refused", http.StatusInternalServerError)
							return
						}
						_, _ = w.Write([]byte(`{"$key":1}`))
					default:
						t.Errorf("action %#v", body)
					}
				case r.URL.Path == "/api/v4/tenant_status":
					_, _ = w.Write([]byte(`[]`))
				case r.URL.Path == "/api/v4/tenants/40" && r.Method == http.MethodDelete:
					deleted = append(deleted, r.URL.Path)
					_, _ = w.Write([]byte(`{}`))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			api := testTenantAPI(t, server.URL)
			data := &TenantCopyResourceModel{
				TenantID: types.StringValue("7"),
				Name:     types.StringValue("sandbox"),
				Isolate:  types.BoolValue(true),
			}
			err := api.createTenantCopy(t.Context(), data)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v", err)
			}
			if !data.Id.IsNull() {
				t.Fatalf("id stored after failed isolate: %s", data.Id)
			}
			if len(deleted) != 1 || deleted[0] != "/api/v4/tenants/40" {
				t.Fatalf("deleted = %#v", deleted)
			}
		})
	}
}

func TestReadTenantCopyAfterImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.URL.Path == "/api/v4/tenants/40" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":true}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testTenantAPI(t, server.URL)
	data := &TenantCopyResourceModel{Id: types.StringValue("40")}
	if err := api.readTenantCopy(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Name.ValueString() != "sandbox" || !data.TenantID.IsNull() || data.Isolate.IsNull() || !data.Isolate.ValueBool() {
		t.Fatalf("read = %#v", data)
	}
}

func TestReadTenantCopyStoresLiveIsolation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.URL.Path == "/api/v4/tenants/40" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":false}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	api := testTenantAPI(t, server.URL)
	data := &TenantCopyResourceModel{Id: types.StringValue("40")}
	if err := api.readTenantCopy(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Isolate.IsNull() || data.Isolate.ValueBool() {
		t.Fatalf("isolate = %#v, want false from the live tenant", data.Isolate)
	}
}

func TestTenantCopyImportState(t *testing.T) {
	ctx := context.Background()
	item := &TenantCopyResource{}
	schemaResp := &resource.SchemaResponse{}
	item.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	resp := &resource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	item.ImportState(ctx, resource.ImportStateRequest{ID: "7/40"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TenantCopyResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Id.ValueString() != "40" || got.TenantID.ValueString() != "7" || got.Isolate.IsNull() || !got.Isolate.ValueBool() {
		t.Fatalf("imported = %#v", got)
	}
}

func testTenantAPI(t *testing.T, host string) *API {
	t.Helper()
	api, err := NewAPI(vergeio.NewClient(host, "user", "pass", true))
	if err != nil {
		t.Fatal(err)
	}
	return api
}

func readTenantAction(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("body %s: %v", body, err)
	}
	return decoded
}
