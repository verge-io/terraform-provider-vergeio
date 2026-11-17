// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestVMSnapshotActionSchema(t *testing.T) {
	item := NewVMSnapshotAction()
	meta := &action.MetadataResponse{}
	item.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_vm_snapshot" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	if diags := schemaResp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	vmID, ok := schemaResp.Schema.Attributes["vm_id"].(interface{ IsRequired() bool })
	if !ok || !vmID.IsRequired() {
		t.Fatal("vm_id should be required")
	}
	for _, name := range []string{"name", "retention_seconds", "quiesce"} {
		attr, exists := schemaResp.Schema.Attributes[name]
		if !exists || attr.IsRequired() || !attr.IsOptional() {
			t.Fatalf("%s should be optional", name)
		}
	}
}

func TestVMSnapshotActionValidateConfig(t *testing.T) {
	item := NewVMSnapshotAction()
	if diags := validateAction(t, item, &vmSnapshotActionModel{
		VMID:             types.StringValue("12"),
		Name:             types.StringNull(),
		RetentionSeconds: types.Int64Null(),
		Quiesce:          types.BoolNull(),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := validateAction(t, item, &vmSnapshotActionModel{
		VMID:             types.StringUnknown(),
		Name:             types.StringNull(),
		RetentionSeconds: types.Int64Null(),
		Quiesce:          types.BoolNull(),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	for _, id := range []string{"", "0", "-3", "vm"} {
		diags := validateAction(t, item, &vmSnapshotActionModel{
			VMID:             types.StringValue(id),
			Name:             types.StringNull(),
			RetentionSeconds: types.Int64Null(),
			Quiesce:          types.BoolNull(),
		})
		if !diags.HasError() {
			t.Fatalf("vm_id %q was accepted", id)
		}
	}
}

func TestVMSnapshotActionInvoke(t *testing.T) {
	var mu sync.Mutex
	var posts []string
	server := actionTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			_, _ = w.Write([]byte(`{"$key":7,"machine":70,"powerstate":true}`))
			return true
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_snapshots":
			mu.Lock()
			posts = append(posts, string(body))
			mu.Unlock()
			_, _ = w.Write([]byte(`{"$key":9}`))
			return true
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_snapshots/9":
			_, _ = w.Write([]byte(`{"$key":9,"machine":70,"name":"before-change","expires":1}`))
			return true
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			mu.Lock()
			posts = append(posts, string(body))
			mu.Unlock()
			_, _ = w.Write([]byte(`{}`))
			return true
		default:
			return false
		}
	})
	item := configuredSnapshotAction(t, server.URL)
	var progress []string
	resp := &action.InvokeResponse{SendProgress: func(event action.InvokeProgressEvent) {
		progress = append(progress, event.Message)
	}}
	item.Invoke(context.Background(), action.InvokeRequest{Config: actionModelConfig(t, item, &vmSnapshotActionModel{
		VMID:             types.StringValue("7"),
		Name:             types.StringValue("before-change"),
		RetentionSeconds: types.Int64Value(3600),
		Quiesce:          types.BoolValue(false),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(posts) != 1 {
		t.Fatalf("posts = %#v, want one machine_snapshots create", posts)
	}
	payload := decodeJSONMap(t, []byte(posts[0]))
	if payload["name"] != "before-change" || payload["machine"] != float64(70) {
		t.Fatalf("create = %#v", payload)
	}
	if _, ok := payload["quiesce"]; ok {
		t.Fatalf("quiesce was sent: %#v", payload)
	}
	if len(progress) == 0 || !strings.Contains(progress[len(progress)-1], "before-change") {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestVMSnapshotActionQuiescePostsGuestAction(t *testing.T) {
	var posts []string
	server := actionTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			_, _ = w.Write([]byte(`{"$key":7,"machine":70}`))
			return true
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_snapshots":
			posts = append(posts, string(body))
			_, _ = w.Write([]byte(`{"$key":9}`))
			return true
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_snapshots/9":
			_, _ = w.Write([]byte(`{"$key":9,"name":"q"}`))
			return true
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			posts = append(posts, string(body))
			_, _ = w.Write([]byte(`{}`))
			return true
		default:
			return false
		}
	})
	item := configuredSnapshotAction(t, server.URL)
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: actionModelConfig(t, item, &vmSnapshotActionModel{
		VMID:             types.StringValue("7"),
		Name:             types.StringNull(),
		RetentionSeconds: types.Int64Null(),
		Quiesce:          types.BoolValue(true),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if len(posts) != 2 || !strings.Contains(posts[0], `"quiesce":true`) || !strings.Contains(posts[1], `"quiesce_snapshot"`) {
		t.Fatalf("posts = %#v", posts)
	}
}

func TestVMSnapshotActionRequiresClient(t *testing.T) {
	item := &VMSnapshotAction{}
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: actionModelConfig(t, item, &vmSnapshotActionModel{
		VMID:             types.StringValue("7"),
		Name:             types.StringNull(),
		RetentionSeconds: types.Int64Null(),
		Quiesce:          types.BoolNull(),
	})}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a missing client diagnostic")
	}
}

func TestVMPowerActionValidateConfig(t *testing.T) {
	item := NewVMPowerAction()
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	if diags := schemaResp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	ok := &vmPowerActionModel{
		VMID:           types.StringValue("7"),
		Operation:      types.StringValue(vmPowerShutdown),
		TimeoutSeconds: types.Int64Value(30),
		Force:          types.BoolValue(true),
	}
	if diags := validateAction(t, item, ok); diags.HasError() {
		t.Fatal(diags)
	}
	rejected := &vmPowerActionModel{
		VMID:           types.StringValue("7"),
		Operation:      types.StringValue(vmPowerOn),
		TimeoutSeconds: types.Int64Value(30),
		Force:          types.BoolValue(true),
	}
	diags := validateAction(t, item, rejected)
	if !diags.HasError() {
		t.Fatal("power_on accepted shutdown options")
	}
	unknown := &vmPowerActionModel{
		VMID:           types.StringUnknown(),
		Operation:      types.StringUnknown(),
		TimeoutSeconds: types.Int64Value(30),
		Force:          types.BoolValue(true),
	}
	if diags := validateAction(t, item, unknown); diags.HasError() {
		t.Fatal(diags)
	}
}

func TestVMPowerActionInvoke(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		force     bool
		timeout   int64
		want      string
	}{
		{name: "shutdown", operation: vmPowerShutdown, want: `"action":"poweroff"`},
		{name: "reset", operation: vmPowerReset, want: `"action":"reset"`},
		{name: "power on", operation: vmPowerOn, want: `"action":"poweron"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var posts []string
			powered := test.operation == vmPowerShutdown
			server := actionTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
				body, _ := io.ReadAll(r.Body)
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
					flag := "false"
					if powered {
						flag = "true"
					}
					_, _ = w.Write([]byte(`{"$key":7,"powerstate":` + flag + `}`))
					return true
				case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
					posts = append(posts, string(body))
					powered = test.operation == vmPowerOn
					_, _ = w.Write([]byte(`{}`))
					return true
				default:
					return false
				}
			})
			item := configuredPowerAction(t, server.URL)
			model := &vmPowerActionModel{
				VMID:           types.StringValue("7"),
				Operation:      types.StringValue(test.operation),
				TimeoutSeconds: types.Int64Null(),
				Force:          types.BoolNull(),
			}
			if test.timeout > 0 {
				model.TimeoutSeconds = types.Int64Value(test.timeout)
			}
			if test.force {
				model.Force = types.BoolValue(true)
			}
			resp := &action.InvokeResponse{}
			item.Invoke(context.Background(), action.InvokeRequest{Config: actionModelConfig(t, item, model)}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			if len(posts) != 1 || !strings.Contains(posts[0], test.want) {
				t.Fatalf("posts = %#v, want %s", posts, test.want)
			}
		})
	}
}

func TestVMPowerActionPowerOnIgnoresStalePowerState(t *testing.T) {
	var posts []string
	running := false
	server := actionTestServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		body, _ := io.ReadAll(r.Body)
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vms/7":
			flag := "false"
			if running {
				flag = "true"
			}
			_, _ = w.Write([]byte(`{"$key":7,"powerstate":true,"running":` + flag + `}`))
			return true
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vm_actions":
			posts = append(posts, string(body))
			running = true
			_, _ = w.Write([]byte(`{}`))
			return true
		default:
			return false
		}
	})
	item := configuredPowerAction(t, server.URL)
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: actionModelConfig(t, item, &vmPowerActionModel{
		VMID:           types.StringValue("7"),
		Operation:      types.StringValue(vmPowerOn),
		TimeoutSeconds: types.Int64Null(),
		Force:          types.BoolNull(),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if len(posts) != 1 || !strings.Contains(posts[0], `"action":"poweron"`) {
		t.Fatalf("posts = %#v, want one poweron", posts)
	}
}

func TestVMPowerActionConfigureRejectsBadClient(t *testing.T) {
	item := &VMPowerAction{}
	resp := &action.ConfigureResponse{}
	item.Configure(context.Background(), action.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || item.sdk != nil {
		t.Fatalf("unconfigured provider: sdk=%v diags=%v", item.sdk, resp.Diagnostics)
	}
	resp = &action.ConfigureResponse{}
	item.Configure(context.Background(), action.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a type diagnostic")
	}
	resp = &action.ConfigureResponse{}
	item.Configure(context.Background(), action.ConfigureRequest{ProviderData: versionClient(t, "25.0.0")}, resp)
	if !resp.Diagnostics.HasError() || item.sdk != nil {
		t.Fatalf("old server: sdk=%v diags=%v", item.sdk, resp.Diagnostics)
	}
}

func validateAction(t *testing.T, item action.Action, model any) diag.Diagnostics {
	t.Helper()
	withValidate, ok := item.(action.ActionWithValidateConfig)
	if !ok {
		t.Fatal("action does not validate config")
	}
	resp := &action.ValidateConfigResponse{}
	withValidate.ValidateConfig(context.Background(), action.ValidateConfigRequest{Config: actionModelConfig(t, item, model)}, resp)
	return resp.Diagnostics
}

func actionModelConfig(t *testing.T, item action.Action, model any) tfsdk.Config {
	t.Helper()
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	return tfsdk.Config{Raw: plan.Raw, Schema: schemaResp.Schema}
}

func configuredSnapshotAction(t *testing.T, host string) *VMSnapshotAction {
	t.Helper()
	item := &VMSnapshotAction{}
	resp := &action.ConfigureResponse{}
	item.Configure(context.Background(), action.ConfigureRequest{ProviderData: vergeio.NewClient(host, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}

func configuredPowerAction(t *testing.T, host string) *VMPowerAction {
	t.Helper()
	item := &VMPowerAction{}
	resp := &action.ConfigureResponse{}
	item.Configure(context.Background(), action.ConfigureRequest{ProviderData: vergeio.NewClient(host, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}

func actionTestServer(t *testing.T, handle func(http.ResponseWriter, *http.Request) bool) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		if r.URL.Path == "/version.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if !handle(w, r) {
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func decodeJSONMap(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return obj
}
