// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestTenantLayer2NetworkSchema(t *testing.T) {
	r := NewTenantLayer2NetworkResource()
	meta := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_layer2_network" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, name := range []string{"tenant_id", "network_id"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok || !attr.IsRequired() {
			t.Fatalf("%s should be required", name)
		}
		if !stringRequiresReplace(t, attr.PlanModifiers) {
			t.Fatalf("%s should require replace", name)
		}
	}
	enabled, ok := resp.Schema.Attributes["enabled"].(schema.BoolAttribute)
	if !ok || !enabled.IsOptional() || !enabled.IsComputed() || enabled.Default == nil {
		t.Fatal("enabled should be optional and computed with a default")
	}
	if len(enabled.PlanModifiers) != 0 {
		t.Fatal("enabled should update in place")
	}
	id := resp.Schema.Attributes["id"]
	if id == nil || !id.IsComputed() || id.IsOptional() || id.IsRequired() {
		t.Fatal("id should be computed")
	}
	if resp.Schema.MarkdownDescription == "" || !containsAll(t, resp.Schema.MarkdownDescription, "26.0", "disables", "tenant-side configuration", "block a later recreation") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestTenantLayer2NetworkConfigure(t *testing.T) {
	r := &TenantLayer2NetworkResource{}
	r.Configure(context.Background(), resource.ConfigureRequest{}, &resource.ConfigureResponse{})
	if r.api != nil {
		t.Fatal("nil provider data should skip configure")
	}
	resp := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("invalid provider data should error")
	}
	ok := &resource.ConfigureResponse{}
	r.Configure(context.Background(), resource.ConfigureRequest{ProviderData: tenantTestClient(t)}, ok)
	if ok.Diagnostics.HasError() {
		t.Fatal(ok.Diagnostics)
	}
	if r.api == nil {
		t.Fatal("api was not configured")
	}
}

func TestCreateTenantLayer2Network(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", true)
	if err := api.createTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() == "" {
		t.Fatal("missing id")
	}
	if !data.Enabled.ValueBool() || data.TenantID.ValueString() != "7" || data.NetworkID.ValueString() != "10" {
		t.Fatalf("model = %#v", data)
	}
	creates := fake.bodiesFor(http.MethodPost, "/api/v4/tenant_layer2_vnets")
	if len(creates) != 1 {
		t.Fatalf("create bodies = %#v", creates)
	}
	if intField(creates[0]["tenant"]) != 7 || intField(creates[0]["vnet"]) != 10 || creates[0]["enabled"] != true {
		t.Fatalf("create body = %#v", creates[0])
	}
	if err := api.readTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.Enabled.ValueBool() {
		t.Fatal("read cleared enabled")
	}
}

func TestCreateTenantLayer2NetworkDisabled(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", false)
	if err := api.createTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Enabled.ValueBool() {
		t.Fatal("created enabled")
	}
	creates := fake.bodiesFor(http.MethodPost, "/api/v4/tenant_layer2_vnets")
	if len(creates) != 1 || creates[0]["enabled"] != false {
		t.Fatalf("create body = %#v", creates)
	}
}

func TestReadTenantLayer2NetworkRejectsReusedKey(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", true)
	if err := api.createTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	data.TenantID = types.StringValue("8")
	if err := api.readTenantLayer2Network(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused tenant err = %v", err)
	}
	data.TenantID = types.StringValue("7")
	data.NetworkID = types.StringValue("11")
	if err := api.readTenantLayer2Network(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused network err = %v", err)
	}
}

func TestReadTenantLayer2NetworkNotFound(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", true)
	data.Id = types.StringValue("99")
	err := api.readTenantLayer2Network(context.Background(), data)
	if !vergeos.IsNotFoundError(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateTenantLayer2NetworkEnabled(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", true)
	if err := api.createTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	path := "/api/v4/tenant_layer2_vnets/" + data.Id.ValueString()
	data.Enabled = types.BoolValue(false)
	if err := api.updateTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Enabled.ValueBool() {
		t.Fatal("update left the assignment enabled")
	}
	updates := fake.bodiesFor(http.MethodPut, path)
	if len(updates) != 1 || updates[0]["enabled"] != false {
		t.Fatalf("update bodies = %#v", updates)
	}
	if err := api.readTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Enabled.ValueBool() {
		t.Fatal("read restored enabled")
	}
	data.Enabled = types.BoolValue(true)
	if err := api.updateTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	updates = fake.bodiesFor(http.MethodPut, path)
	if len(updates) != 2 || updates[1]["enabled"] != true {
		t.Fatalf("update bodies = %#v", updates)
	}
}

func TestDeleteTenantLayer2NetworkDisablesFirst(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", true)
	if err := api.createTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	id := data.Id.ValueString()
	path := "/api/v4/tenant_layer2_vnets/" + id
	if err := api.deleteTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	updates := fake.bodiesFor(http.MethodPut, path)
	if len(updates) != 1 || updates[0]["enabled"] != false {
		t.Fatalf("disable bodies = %#v", updates)
	}
	if fake.callCount(http.MethodDelete, path) != 1 {
		t.Fatalf("delete calls = %d", fake.callCount(http.MethodDelete, path))
	}
	assertCallOrder(t, fake.recordedCalls(), "PUT "+path, "DELETE "+path)
	if err := api.readTenantLayer2Network(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("assignment still readable: %v", err)
	}
	if err := api.deleteTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestDeleteTenantLayer2NetworkDisablesWhenAlreadyDisabled(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", false)
	if err := api.createTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	path := "/api/v4/tenant_layer2_vnets/" + data.Id.ValueString()
	if err := api.deleteTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	updates := fake.bodiesFor(http.MethodPut, path)
	if len(updates) != 1 || updates[0]["enabled"] != false {
		t.Fatalf("disable bodies = %#v", updates)
	}
	assertCallOrder(t, fake.recordedCalls(), "PUT "+path, "DELETE "+path)
}

func TestDeleteTenantLayer2NetworkStopsWhenDisableFails(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", true)
	if err := api.createTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.layer2DisableStatus = http.StatusConflict
	fake.layer2DisableMessage = "disable refused"
	fake.mu.Unlock()
	path := "/api/v4/tenant_layer2_vnets/" + data.Id.ValueString()
	err := api.deleteTenantLayer2Network(context.Background(), data)
	if err == nil || !containsAll(t, err.Error(), "disable", "before delete") {
		t.Fatalf("err = %v", err)
	}
	if fake.callCount(http.MethodDelete, path) != 0 {
		t.Fatal("delete ran after disable failed")
	}
	if err := api.readTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.Enabled.ValueBool() {
		t.Fatal("failed disable cleared enabled")
	}
}

func TestDeleteTenantLayer2NetworkReportsDeleteFailureAfterDisable(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := layer2Model("7", "10", true)
	if err := api.createTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.layer2DeleteStatus = http.StatusConflict
	fake.layer2DeleteMessage = "tenant-side components remain"
	fake.mu.Unlock()
	err := api.deleteTenantLayer2Network(context.Background(), data)
	if err == nil || !containsAll(t, err.Error(), "then delete failed", "tenant-side components remain") {
		t.Fatalf("err = %v", err)
	}
	if err := api.readTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Enabled.ValueBool() {
		t.Fatal("assignment stayed enabled after the disable that preceded the failed delete")
	}
	fake.mu.Lock()
	fake.layer2DeleteStatus = 0
	fake.mu.Unlock()
	if err := api.deleteTenantLayer2Network(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantLayer2Network(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("assignment still readable: %v", err)
	}
}

func layer2Model(tenantID, networkID string, enabled bool) *TenantLayer2NetworkResourceModel {
	return &TenantLayer2NetworkResourceModel{
		TenantID:  types.StringValue(tenantID),
		NetworkID: types.StringValue(networkID),
		Enabled:   types.BoolValue(enabled),
	}
}

func assertCallOrder(t *testing.T, calls []string, first, second string) {
	t.Helper()
	firstAt, secondAt := -1, -1
	for i, call := range calls {
		if call == first && firstAt < 0 {
			firstAt = i
		}
		if call == second {
			secondAt = i
		}
	}
	if firstAt < 0 || secondAt < 0 || firstAt > secondAt {
		t.Fatalf("calls = %v, want %s before %s", calls, first, second)
	}
}
