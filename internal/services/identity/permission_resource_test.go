// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestPermissionResourceMetadataAndSchema(t *testing.T) {
	meta := &fwresource.MetadataResponse{}
	NewPermissionResource().Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_permission" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &fwresource.SchemaResponse{}
	NewPermissionResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	for _, name := range []string{"user_id", "group_id", "object_id", "list", "read", "create", "modify", "delete"} {
		attr, ok := resp.Schema.Attributes[name]
		if !ok || !attr.IsOptional() {
			t.Fatalf("%s should be optional", name)
		}
	}
	table, ok := resp.Schema.Attributes["table"]
	if !ok || !table.IsRequired() {
		t.Fatal("table should be required")
	}
	if _, ok := resp.Schema.Attributes["mask"]; ok {
		t.Fatal("rights must not be a bit mask")
	}
}

func TestPermissionConfigure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	resource := &PermissionResource{}
	resp := &fwresource.ConfigureResponse{}
	resource.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() || resource.api == nil || resource.api.Name() != "Permission Api" {
		t.Fatalf("configure failed: %v", resp.Diagnostics)
	}

	resource = &PermissionResource{}
	resp = &fwresource.ConfigureResponse{}
	resource.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: 1}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected invalid client error")
	}
}

func TestPermissionValidateConfigRejectsReadWithoutList(t *testing.T) {
	ctx := context.Background()
	resource := NewPermissionResource().(*PermissionResource)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	config := permissionConfig(t, schemaResp, &PermissionResourceModel{
		Id:       types.StringNull(),
		UserID:   types.StringValue("7"),
		GroupID:  types.StringNull(),
		Table:    types.StringValue("vms"),
		ObjectID: types.Int64Null(),
		List:     types.BoolValue(false),
		Read:     types.BoolValue(true),
		Create:   types.BoolNull(),
		Modify:   types.BoolNull(),
		Delete:   types.BoolNull(),
	})
	resp := &fwresource.ValidateConfigResponse{}
	resource.ValidateConfig(ctx, fwresource.ValidateConfigRequest{Config: config}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("read true with list false should be rejected")
	}

	config = permissionConfig(t, schemaResp, &PermissionResourceModel{
		Id:       types.StringNull(),
		UserID:   types.StringValue("7"),
		GroupID:  types.StringNull(),
		Table:    types.StringValue("vms"),
		ObjectID: types.Int64Null(),
		List:     types.BoolNull(),
		Read:     types.BoolValue(true),
		Create:   types.BoolNull(),
		Modify:   types.BoolNull(),
		Delete:   types.BoolNull(),
	})
	resp = &fwresource.ValidateConfigResponse{}
	resource.ValidateConfig(ctx, fwresource.ValidateConfigRequest{Config: config}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("omitted list should be allowed: %v", resp.Diagnostics)
	}
}

func permissionConfig(t *testing.T, schemaResp *fwresource.SchemaResponse, model *PermissionResourceModel) tfsdk.Config {
	t.Helper()
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	return tfsdk.Config{Raw: plan.Raw, Schema: schemaResp.Schema}
}

func TestPermissionCreateKeepsIDWhenRefreshFails(t *testing.T) {
	var permissionGets atomic.Int32
	server := permissionServer(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/permissions" {
			_, _ = w.Write([]byte(`{"$key":5}`))
			return true
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/permissions/5" {
			if permissionGets.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"$key":5,"identity":9,"table":"vms","row":0,"list":true,"read":true,"create":false,"modify":false,"delete":false}`))
				return true
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"err":"unavailable"}`))
			return true
		}
		return false
	})

	ctx := context.Background()
	permission := &PermissionResource{}
	configure := &fwresource.ConfigureResponse{}
	permission.Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &fwresource.SchemaResponse{}
	permission.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &PermissionResourceModel{
		Id:       types.StringNull(),
		UserID:   types.StringNull(),
		GroupID:  types.StringValue("4"),
		Table:    types.StringValue("vms"),
		ObjectID: types.Int64Null(),
		List:     types.BoolValue(true),
		Read:     types.BoolValue(true),
		Create:   types.BoolValue(false),
		Modify:   types.BoolValue(false),
		Delete:   types.BoolValue(false),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	permission.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the follow-up read to fail")
	}
	var got PermissionResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.Id.ValueString() != "5" {
		t.Fatalf("id = %q, want 5", got.Id.ValueString())
	}
}
