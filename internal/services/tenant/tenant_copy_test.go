// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"encoding/json"
	"fmt"
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
	created := resp.Schema.Attributes["created"]
	if created == nil || !created.IsComputed() {
		t.Fatal("created should be computed")
	}
	if !strings.Contains(resp.Schema.MarkdownDescription, "current state") || !strings.Contains(resp.Schema.MarkdownDescription, "does not expose") || !strings.Contains(resp.Schema.MarkdownDescription, "keeps that key in state") {
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
	created := time.Now().Unix()
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
			_, _ = fmt.Fprintf(w, `{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":%s,"vnet":0,"created":%d}`, isolate, created)
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
	if data.Id.ValueString() != "40" || data.Created.ValueInt64() != created {
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
	if data.Name.ValueString() != "sandbox" || !data.TenantID.IsNull() || data.Isolate.IsNull() || !data.Isolate.ValueBool() {
		t.Fatalf("read = %#v", data)
	}
}

func TestReadTenantCopyKeepsConfiguredIsolate(t *testing.T) {
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
	data := &TenantCopyResourceModel{
		Id:      types.StringValue("40"),
		Isolate: types.BoolValue(false),
	}
	if err := api.readTenantCopy(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Isolate.IsNull() || data.Isolate.ValueBool() {
		t.Fatalf("isolate = %#v, want the configured false kept", data.Isolate)
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

func TestCreateTenantCopyKeepsIDWhenWaitFails(t *testing.T) {
	fresh := time.Now().Unix()
	for _, tc := range []struct {
		name      string
		cancel    bool
		timeout   time.Duration
		hideFirst bool
		row       string
		wantID    string
		wantErr   string
	}{
		{
			name:      "timeout",
			timeout:   0,
			hideFirst: true,
			row:       fmt.Sprintf(`{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":false,"vnet":0,"created":%d}`, fresh),
			wantID:    "40",
			wantErr:   "was not created",
		},
		{
			name:      "canceled",
			cancel:    true,
			timeout:   2 * time.Second,
			hideFirst: true,
			row:       fmt.Sprintf(`{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":false,"vnet":0,"created":%d}`, fresh),
			wantID:    "40",
			wantErr:   "context canceled",
		},
		{
			name:      "older row",
			timeout:   0,
			hideFirst: true,
			row:       `{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":false,"vnet":0,"created":1000}`,
			wantErr:   "was not created",
		},
		{
			name:      "source key",
			timeout:   0,
			hideFirst: true,
			row:       fmt.Sprintf(`{"$key":7,"name":"sandbox","is_snapshot":false,"isolate":false,"vnet":0,"created":%d}`, fresh),
			wantErr:   "was not created",
		},
		{
			name:    "missing",
			timeout: 0,
			wantErr: "was not created",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			poll, timeout := tenantCopyPoll, tenantCopyTimeout
			tenantCopyPoll = time.Hour
			tenantCopyTimeout = tc.timeout
			t.Cleanup(func() {
				tenantCopyPoll = poll
				tenantCopyTimeout = timeout
			})

			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			var deleted []string
			cloned := false
			lookups := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				case r.URL.Path == "/api/v4/tenants" && r.Method == http.MethodGet:
					if !cloned || tc.row == "" {
						_, _ = w.Write([]byte(`[]`))
						return
					}
					lookups++
					if tc.hideFirst && lookups == 1 {
						if tc.cancel {
							cancel()
						}
						_, _ = w.Write([]byte(`[]`))
						return
					}
					_, _ = fmt.Fprintf(w, "[%s]", tc.row)
				case r.Method == http.MethodGet && (r.URL.Path == "/api/v4/tenants/40" || r.URL.Path == "/api/v4/tenants/7"):
					if tc.row == "" {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte(tc.row))
				case r.URL.Path == "/api/v4/tenant_actions" && r.Method == http.MethodPost:
					body := readTenantAction(t, r)
					if body["action"] == "clone" {
						cloned = true
					}
					if body["action"] == "isolateon" || body["action"] == "restore" || body["action"] == "isolateoff" {
						t.Errorf("action %#v", body)
					}
					_, _ = w.Write([]byte(`{"$key":1}`))
				case r.Method == http.MethodDelete:
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
			err := api.createTenantCopy(ctx, data)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v", err)
			}
			gotID := ""
			if !data.Id.IsNull() && !data.Id.IsUnknown() {
				gotID = data.Id.ValueString()
			}
			if gotID != tc.wantID {
				t.Fatalf("id = %q, want %q; err = %v", gotID, tc.wantID, err)
			}
			if tc.wantID != "" && (data.Created.IsNull() || data.Created.ValueInt64() == 0) {
				t.Fatal("kept copy has no created time")
			}
			if len(deleted) != 0 {
				t.Fatalf("deleted = %#v", deleted)
			}
		})
	}
}

func TestCreateTenantCopyStoresStateWhenWaitTimesOut(t *testing.T) {
	poll, timeout := tenantCopyPoll, tenantCopyTimeout
	tenantCopyPoll = time.Hour
	tenantCopyTimeout = 0
	t.Cleanup(func() {
		tenantCopyPoll = poll
		tenantCopyTimeout = timeout
	})

	created := time.Now().Unix()
	row := fmt.Sprintf(`{"$key":40,"name":"sandbox","is_snapshot":false,"isolate":false,"vnet":0,"created":%d}`, created)
	cloned := false
	lookups := 0
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
			lookups++
			if lookups == 1 {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			_, _ = fmt.Fprintf(w, "[%s]", row)
		case r.URL.Path == "/api/v4/tenants/40":
			_, _ = w.Write([]byte(row))
		case r.URL.Path == "/api/v4/tenant_actions":
			cloned = true
			_, _ = w.Write([]byte(`{"$key":1}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	item := &TenantCopyResource{}
	configure := &resource.ConfigureResponse{}
	item.Configure(ctx, resource.ConfigureRequest{ProviderData: vergeio.NewClient(server.URL, "user", "pass", true)}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &resource.SchemaResponse{}
	item.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &TenantCopyResourceModel{
		TenantID: types.StringValue("7"),
		Name:     types.StringValue("sandbox"),
		Isolate:  types.BoolValue(true),
		Created:  types.Int64Unknown(),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	item.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("create succeeded while the copy was not ready")
	}
	var got TenantCopyResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.Id.ValueString() != "40" || got.Name.ValueString() != "sandbox" || got.Created.ValueInt64() != created || !got.Isolate.ValueBool() {
		t.Fatalf("state = id %s name %s created %s isolate %s", got.Id, got.Name, got.Created, got.Isolate)
	}
	if got.Created.IsUnknown() {
		t.Fatal("state still has an unknown created time")
	}
}

func TestDeleteTenantCopyLeavesReusedKey(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		stateName  string
		created    int64
		wantDelete bool
		wantErr    bool
	}{
		{name: "different created", status: http.StatusOK, body: `{"$key":40,"name":"sandbox","is_snapshot":false,"vnet":0,"created":1800000000}`, stateName: "sandbox", created: 1700000000},
		{name: "different name", status: http.StatusOK, body: `{"$key":40,"name":"other","is_snapshot":false,"vnet":0,"created":1700000000}`, stateName: "sandbox", created: 1700000000},
		{name: "same", status: http.StatusOK, body: `{"$key":40,"name":"sandbox","is_snapshot":false,"vnet":0,"created":1700000000}`, stateName: "sandbox", created: 1700000000, wantDelete: true},
		{name: "name only", status: http.StatusOK, body: `{"$key":40,"name":"sandbox","is_snapshot":false,"vnet":0,"created":1800000000}`, stateName: "sandbox", wantDelete: true},
		{name: "missing", status: http.StatusNotFound, stateName: "sandbox", created: 1700000000},
		{name: "read error", status: http.StatusInternalServerError, stateName: "sandbox", created: 1700000000, wantErr: true},
		{name: "snapshot", status: http.StatusOK, body: `{"$key":40,"name":"sandbox","is_snapshot":true,"vnet":0,"created":1700000000}`, stateName: "sandbox", created: 1700000000, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var deleted int
			var actions int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenants/40":
					w.WriteHeader(tc.status)
					if tc.body != "" {
						_, _ = w.Write([]byte(tc.body))
					}
				case r.URL.Path == "/api/v4/tenant_status":
					_, _ = w.Write([]byte(`[]`))
				case r.Method == http.MethodPost && r.URL.Path == "/api/v4/tenant_actions":
					actions++
					_, _ = w.Write([]byte(`{}`))
				case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/tenants/40":
					deleted++
					_, _ = w.Write([]byte(`{}`))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			data := &TenantCopyResourceModel{
				Id:       types.StringValue("40"),
				TenantID: types.StringValue("7"),
				Name:     types.StringValue(tc.stateName),
				Isolate:  types.BoolValue(true),
			}
			if tc.created != 0 {
				data.Created = types.Int64Value(tc.created)
			}
			err := testTenantAPI(t, server.URL).deleteTenantCopy(t.Context(), data)
			if tc.wantErr {
				if err == nil {
					t.Fatal("delete succeeded")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if tc.wantDelete && deleted != 1 {
				t.Fatalf("deleted = %d", deleted)
			}
			if !tc.wantDelete && (deleted != 0 || actions != 0) {
				t.Fatalf("deleted = %d actions = %d", deleted, actions)
			}
		})
	}
}

func TestReadTenantCopyDropsReusedKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/tenants/40":
			_, _ = w.Write([]byte(`{"$key":40,"name":"other","is_snapshot":false,"created":1800000000}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	item := &TenantCopyResource{}
	configure := &resource.ConfigureResponse{}
	item.Configure(ctx, resource.ConfigureRequest{ProviderData: vergeio.NewClient(server.URL, "user", "pass", true)}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &resource.SchemaResponse{}
	item.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &TenantCopyResourceModel{
		Id:       types.StringValue("40"),
		TenantID: types.StringValue("7"),
		Name:     types.StringValue("sandbox"),
		Isolate:  types.BoolValue(true),
		Created:  types.Int64Value(1700000000),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &resource.ReadResponse{State: state}
	item.Read(ctx, resource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("a reused tenant key stayed in state")
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
