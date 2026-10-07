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

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestTenantCloneActionSchema(t *testing.T) {
	item := NewTenantCloneAction()
	meta := &action.MetadataResponse{}
	item.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_clone" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	if diags := schemaResp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	for _, name := range []string{"tenant_id", "name"} {
		if !schemaResp.Schema.Attributes[name].IsRequired() {
			t.Fatalf("%s should be required", name)
		}
	}
	for _, name := range []string{"no_vnet", "no_storage", "no_nodes"} {
		attr := schemaResp.Schema.Attributes[name]
		if attr.IsRequired() || !attr.IsOptional() {
			t.Fatalf("%s should be optional", name)
		}
	}
}

func TestTenantCloneActionValidateConfig(t *testing.T) {
	item := NewTenantCloneAction()
	if diags := validateTenantAction(t, item, tenantCloneModel("4", "copy")); diags.HasError() {
		t.Fatal(diags)
	}
	unknown := tenantCloneModel("4", "copy")
	unknown.TenantID = types.StringUnknown()
	unknown.Name = types.StringUnknown()
	unknown.NoVNet = types.BoolUnknown()
	if diags := validateTenantAction(t, item, unknown); diags.HasError() {
		t.Fatal(diags)
	}
	for _, model := range []*tenantCloneActionModel{
		tenantCloneModel("0", "copy"),
		tenantCloneModel("4", "  "),
		tenantCloneModel("nope", "copy"),
	} {
		if diags := validateTenantAction(t, item, model); !diags.HasError() {
			t.Fatalf("accepted %#v", model)
		}
	}
}

func TestTenantCloneActionInvoke(t *testing.T) {
	var post string
	server := tenantActionServer(t, func(body string) { post = body })
	item := configuredTenantAction(t, server.URL, &TenantCloneAction{})
	model := tenantCloneModel("8", "  sandbox  ")
	model.NoVNet = types.BoolValue(true)
	model.NoStorage = types.BoolValue(false)
	model.NoNodes = types.BoolValue(true)
	var progress []string
	resp := &action.InvokeResponse{SendProgress: func(event action.InvokeProgressEvent) {
		progress = append(progress, event.Message)
	}}
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantActionConfig(t, item, model)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	body := decodeActionPost(t, post)
	if body["action"] != "clone" || body["tenant"] != float64(8) {
		t.Fatalf("post = %s", post)
	}
	params, ok := body["params"].(map[string]any)
	if !ok {
		t.Fatalf("params = %#v", body["params"])
	}
	if params["name"] != "sandbox" || params["no_vnet"] != true || params["no_storage"] != false || params["no_nodes"] != true {
		t.Fatalf("params = %#v", params)
	}
	if len(progress) == 0 || !strings.Contains(progress[len(progress)-1], "sandbox") {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestTenantCloneActionOmittedFlagsCopyEverything(t *testing.T) {
	var post string
	server := tenantActionServer(t, func(body string) { post = body })
	item := configuredTenantAction(t, server.URL, &TenantCloneAction{})
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantActionConfig(t, item, tenantCloneModel("8", "copy"))}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	params := decodeActionPost(t, post)["params"].(map[string]any)
	for _, name := range []string{"no_vnet", "no_storage", "no_nodes"} {
		if params[name] != false {
			t.Fatalf("%s = %#v, want false", name, params[name])
		}
	}
}

func TestTenantCloneActionRequiresClient(t *testing.T) {
	item := &TenantCloneAction{}
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantActionConfig(t, item, tenantCloneModel("8", "copy"))}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a missing client diagnostic")
	}
}

func TestTenantResetActionInvoke(t *testing.T) {
	itemType := NewTenantResetAction()
	meta := &action.MetadataResponse{}
	itemType.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_reset" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	var post string
	server := tenantActionServer(t, func(body string) { post = body })
	item := configuredTenantAction(t, server.URL, &TenantResetAction{})
	if diags := validateTenantAction(t, item, &tenantResetActionModel{TenantID: types.StringValue("0")}); !diags.HasError() {
		t.Fatal("tenant_id 0 was accepted")
	}
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantActionConfig(t, item, &tenantResetActionModel{TenantID: types.StringValue("8")})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	body := decodeActionPost(t, post)
	if body["action"] != "reset" || body["tenant"] != float64(8) {
		t.Fatalf("post = %s", post)
	}
}

func TestTenantNodeMigrateActionInvoke(t *testing.T) {
	itemType := NewTenantNodeMigrateAction()
	meta := &action.MetadataResponse{}
	itemType.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_node_migrate" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	var post string
	server := tenantNodeActionServer(t, func(body string) { post = body })
	item := configuredTenantAction(t, server.URL, &TenantNodeMigrateAction{})
	if diags := validateTenantAction(t, item, &tenantNodeMigrateActionModel{
		TenantNodeID: types.StringValue("5"),
		TargetNode:   types.StringValue("0"),
	}); !diags.HasError() {
		t.Fatal("target_node 0 was accepted")
	}
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantActionConfig(t, item, &tenantNodeMigrateActionModel{
		TenantNodeID: types.StringValue("5"),
		TargetNode:   types.StringValue("3"),
	})}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	body := decodeActionPost(t, post)
	if body["action"] != "migrate" || body["tenant_node"] != float64(5) {
		t.Fatalf("post = %s", post)
	}
	params, ok := body["params"].(map[string]any)
	if !ok || params["node"] != float64(3) {
		t.Fatalf("params = %#v", body["params"])
	}
}

func TestTenantNodePowerActionInvoke(t *testing.T) {
	itemType := NewTenantNodePowerAction()
	meta := &action.MetadataResponse{}
	itemType.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_node_power" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	schemaResp := &action.SchemaResponse{}
	itemType.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	if !strings.Contains(schemaResp.Schema.MarkdownDescription, "poweroffmaintenance") {
		t.Fatal("schema should record that poweroffmaintenance is not offered")
	}
	for _, operation := range []string{tenantNodeKill, tenantNodeReset} {
		t.Run(operation, func(t *testing.T) {
			var post string
			server := tenantNodeActionServer(t, func(body string) { post = body })
			item := configuredTenantAction(t, server.URL, &TenantNodePowerAction{})
			resp := &action.InvokeResponse{}
			item.Invoke(context.Background(), action.InvokeRequest{Config: tenantActionConfig(t, item, &tenantNodePowerActionModel{
				TenantNodeID: types.StringValue("5"),
				Operation:    types.StringValue(operation),
			})}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			body := decodeActionPost(t, post)
			if body["action"] != operation || body["tenant_node"] != float64(5) {
				t.Fatalf("post = %s", post)
			}
		})
	}
}

func TestTenantNodePowerActionRejectsMaintenance(t *testing.T) {
	var posts int
	server := tenantNodeActionServer(t, func(string) { posts++ })
	item := configuredTenantAction(t, server.URL, &TenantNodePowerAction{})
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantActionConfig(t, item, &tenantNodePowerActionModel{
		TenantNodeID: types.StringValue("5"),
		Operation:    types.StringValue("poweroffmaintenance"),
	})}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("poweroffmaintenance was accepted")
	}
	if posts != 0 {
		t.Fatalf("posted %d actions", posts)
	}
}

func TestTenantNodePowerActionRequiresClient(t *testing.T) {
	item := &TenantNodePowerAction{}
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantActionConfig(t, item, &tenantNodePowerActionModel{
		TenantNodeID: types.StringValue("5"),
		Operation:    types.StringValue(tenantNodeKill),
	})}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a missing client diagnostic")
	}
}

func tenantCloneModel(id, name string) *tenantCloneActionModel {
	return &tenantCloneActionModel{
		TenantID:  types.StringValue(id),
		Name:      types.StringValue(name),
		NoVNet:    types.BoolNull(),
		NoStorage: types.BoolNull(),
		NoNodes:   types.BoolNull(),
	}
}

func validateTenantAction(t *testing.T, item action.Action, model any) diag.Diagnostics {
	t.Helper()
	withValidate := item.(action.ActionWithValidateConfig)
	resp := &action.ValidateConfigResponse{}
	withValidate.ValidateConfig(context.Background(), action.ValidateConfigRequest{Config: tenantActionConfig(t, item, model)}, resp)
	return resp.Diagnostics
}

func tenantActionConfig(t *testing.T, item action.Action, model any) tfsdk.Config {
	t.Helper()
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	return tfsdk.Config{Raw: plan.Raw, Schema: schemaResp.Schema}
}

func configuredTenantAction(t *testing.T, host string, item interface {
	Configure(context.Context, action.ConfigureRequest, *action.ConfigureResponse)
	action.Action
}) action.Action {
	t.Helper()
	resp := &action.ConfigureResponse{}
	item.Configure(context.Background(), action.ConfigureRequest{ProviderData: vergeio.NewClient(host, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}

func tenantActionServer(t *testing.T, record func(string)) *httptest.Server {
	t.Helper()
	return actionHTTPServer(t, "/api/v4/tenant_actions", record)
}

func tenantNodeActionServer(t *testing.T, record func(string)) *httptest.Server {
	t.Helper()
	return actionHTTPServer(t, "/api/v4/tenant_node_actions", record)
}

func actionHTTPServer(t *testing.T, actionPath string, record func(string)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == actionPath:
			body, _ := io.ReadAll(r.Body)
			record(string(body))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func decodeActionPost(t *testing.T, body string) map[string]any {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	return obj
}
