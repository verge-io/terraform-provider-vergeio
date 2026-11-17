// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
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

func TestTenantSnapshotActionSchema(t *testing.T) {
	item := NewTenantSnapshotAction()
	meta := &action.MetadataResponse{}
	item.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_snapshot" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	if diags := schemaResp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	if !schemaResp.Schema.Attributes["tenant_id"].IsRequired() {
		t.Fatal("tenant_id should be required")
	}
	for _, name := range []string{"name", "description", "retention_seconds"} {
		attr := schemaResp.Schema.Attributes[name]
		if attr.IsRequired() || !attr.IsOptional() {
			t.Fatalf("%s should be optional", name)
		}
	}
}

func TestTenantSnapshotActionValidateConfig(t *testing.T) {
	item := NewTenantSnapshotAction()
	if diags := validateTenantSnapshot(t, item, tenantSnapshotModel("3")); diags.HasError() {
		t.Fatal(diags)
	}
	unknown := tenantSnapshotModel("3")
	unknown.TenantID = types.StringUnknown()
	if diags := validateTenantSnapshot(t, item, unknown); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := validateTenantSnapshot(t, item, tenantSnapshotModel("0")); !diags.HasError() {
		t.Fatal("tenant_id 0 was accepted")
	}
}

func TestTenantSnapshotActionInvoke(t *testing.T) {
	var post string
	server := tenantSnapshotServer(t, func(body string) { post = body })
	item := configuredTenantSnapshot(t, server.URL)
	var progress []string
	resp := &action.InvokeResponse{SendProgress: func(event action.InvokeProgressEvent) {
		progress = append(progress, event.Message)
	}}
	model := tenantSnapshotModel("8")
	model.Name = types.StringValue("before-change")
	model.Description = types.StringValue("safety net")
	model.RetentionSeconds = types.Int64Value(3600)
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantSnapshotConfig(t, item, model)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	for _, want := range []string{`"tenant":8`, `"name":"before-change"`, `"description":"safety net"`, `"expires":`} {
		if !strings.Contains(post, want) {
			t.Fatalf("post %s missing %s", post, want)
		}
	}
	if len(progress) == 0 || !strings.Contains(progress[len(progress)-1], "before-change") {
		t.Fatalf("progress = %#v", progress)
	}
}

func TestTenantSnapshotActionRequiresClient(t *testing.T) {
	item := &TenantSnapshotAction{}
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: tenantSnapshotConfig(t, item, tenantSnapshotModel("8"))}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a missing client diagnostic")
	}
}

func tenantSnapshotModel(id string) *tenantSnapshotActionModel {
	return &tenantSnapshotActionModel{
		TenantID:         types.StringValue(id),
		Name:             types.StringNull(),
		Description:      types.StringNull(),
		RetentionSeconds: types.Int64Null(),
	}
}

func validateTenantSnapshot(t *testing.T, item action.Action, model *tenantSnapshotActionModel) diag.Diagnostics {
	t.Helper()
	withValidate := item.(action.ActionWithValidateConfig)
	resp := &action.ValidateConfigResponse{}
	withValidate.ValidateConfig(context.Background(), action.ValidateConfigRequest{Config: tenantSnapshotConfig(t, item, model)}, resp)
	return resp.Diagnostics
}

func tenantSnapshotConfig(t *testing.T, item action.Action, model *tenantSnapshotActionModel) tfsdk.Config {
	t.Helper()
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	return tfsdk.Config{Raw: plan.Raw, Schema: schemaResp.Schema}
}

func configuredTenantSnapshot(t *testing.T, host string) *TenantSnapshotAction {
	t.Helper()
	item := &TenantSnapshotAction{}
	resp := &action.ConfigureResponse{}
	item.Configure(context.Background(), action.ConfigureRequest{ProviderData: vergeio.NewClient(host, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}

func tenantSnapshotServer(t *testing.T, record func(string)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/tenant_snapshots":
			body, _ := io.ReadAll(r.Body)
			record(string(body))
			_, _ = w.Write([]byte(`{"$key":15}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenant_snapshots/15":
			_, _ = w.Write([]byte(`{"$key":15,"tenant":8,"name":"before-change","description":"safety net"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
