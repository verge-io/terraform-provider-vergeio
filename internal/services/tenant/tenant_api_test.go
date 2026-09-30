// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestTenantPoweredOn(t *testing.T) {
	if tenantPoweredOn(nil) {
		t.Fatal("missing status is off")
	}
	cases := []struct {
		name   string
		status *vergeos.TenantStatus
		on     bool
	}{
		{name: "offline", status: &vergeos.TenantStatus{Status: "offline"}, on: false},
		{name: "stopping", status: &vergeos.TenantStatus{Status: "stopping", Stopping: true, Running: true}, on: false},
		{name: "running", status: &vergeos.TenantStatus{Running: true, Status: "online"}, on: true},
		{name: "starting", status: &vergeos.TenantStatus{Starting: true, Status: "starting"}, on: true},
		{name: "migrating", status: &vergeos.TenantStatus{Status: "migrating"}, on: true},
		{name: "error", status: &vergeos.TenantStatus{Status: "error"}, on: false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantPoweredOn(tt.status); got != tt.on {
				t.Fatalf("tenantPoweredOn = %v, want %v", got, tt.on)
			}
		})
	}
}

func TestTenantResourceSchema(t *testing.T) {
	r := NewTenantResource()
	meta := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	name := resp.Schema.Attributes["name"]
	if name == nil || !name.IsRequired() {
		t.Fatal("name should be required")
	}
	password := resp.Schema.Attributes["password"]
	if password == nil || !password.IsSensitive() {
		t.Fatal("password should be sensitive")
	}
	ui := resp.Schema.Attributes["ui_address"]
	if ui == nil || !ui.IsComputed() || ui.IsRequired() {
		t.Fatal("ui_address should be computed")
	}
	power := resp.Schema.Attributes["powerstate"]
	if power == nil || !power.IsOptional() || !power.IsComputed() {
		t.Fatal("powerstate should be optional and computed")
	}
	if resp.Schema.MarkdownDescription == "" || !containsAll(t, resp.Schema.MarkdownDescription, "need_fw_apply", "vnet_cidrs", "second Terraform") {
		t.Fatalf("description should document the address gap and the second configuration: %s", resp.Schema.MarkdownDescription)
	}
}

func TestChangePasswordReplaceOnlyWhenConfigured(t *testing.T) {
	resp := &resource.SchemaResponse{}
	NewTenantResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	attr, ok := resp.Schema.Attributes["change_password"].(schema.BoolAttribute)
	if !ok {
		t.Fatal("change_password should be a bool")
	}
	var replace planmodifier.Bool
	for _, mod := range attr.PlanModifiers {
		if strings.Contains(mod.Description(context.Background()), "configured") {
			replace = mod
			break
		}
	}
	if replace == nil {
		t.Fatal("change_password should replace only when the configuration sets it")
	}
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})

	omitted := &planmodifier.BoolResponse{PlanValue: types.BoolUnknown()}
	replace.PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		StateValue:  types.BoolValue(false),
		PlanValue:   types.BoolUnknown(),
		ConfigValue: types.BoolNull(),
		State:       tfsdk.State{Raw: raw},
		Plan:        tfsdk.Plan{Raw: raw},
	}, omitted)
	if omitted.RequiresReplace {
		t.Fatal("omitted change_password planned a replacement")
	}

	changed := &planmodifier.BoolResponse{PlanValue: types.BoolValue(true)}
	replace.PlanModifyBool(context.Background(), planmodifier.BoolRequest{
		StateValue:  types.BoolValue(false),
		PlanValue:   types.BoolValue(true),
		ConfigValue: types.BoolValue(true),
		State:       tfsdk.State{Raw: raw},
		Plan:        tfsdk.Plan{Raw: raw},
	}, changed)
	if !changed.RequiresReplace {
		t.Fatal("configured change_password change did not plan a replacement")
	}
}

func TestTenantConfigure(t *testing.T) {
	r := &TenantResource{}
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
	r.Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: vergeio.NewClient("https://example.test", "user", "pass", true),
	}, ok)
	if ok.Diagnostics.HasError() {
		t.Fatal(ok.Diagnostics)
	}
	if r.api == nil || r.api.Name() != "Tenant Api" {
		t.Fatalf("api = %#v", r.api)
	}
}

func TestCreateTenantPowersOnAndResolvesUIAddress(t *testing.T) {
	fake := newFake(t)
	fake.uiIP = "203.0.113.10"
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:          types.StringValue("customer-a"),
		Description:   types.StringValue("Customer A"),
		Password:      types.StringValue("secret-pass"),
		PowerState:    types.BoolValue(true),
		PreferredNode: types.Int32Value(4),
	}
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() == "" {
		t.Fatal("missing id")
	}
	if data.Password.ValueString() != "secret-pass" {
		t.Fatalf("password was cleared: %#v", data.Password)
	}
	if data.PreferredNode.ValueInt32() != 4 {
		t.Fatalf("preferred_node = %#v", data.PreferredNode)
	}
	if !data.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v", data.PowerState)
	}
	if data.UIAddress.ValueString() != "203.0.113.10" {
		t.Fatalf("ui_address = %#v", data.UIAddress)
	}
	if data.UIAddressID.ValueInt32() != 15 {
		t.Fatalf("ui_address_id = %#v", data.UIAddressID)
	}
	if data.UUID.ValueString() == "" || data.VNet.ValueInt32() != 9 {
		t.Fatalf("uuid/vnet = %s %v", data.UUID.ValueString(), data.VNet)
	}
	creates := fake.bodiesFor(httpMethodPost, "/api/v4/tenants")
	if len(creates) != 1 || creates[0]["password"] != "secret-pass" || creates[0]["name"] != "customer-a" {
		t.Fatalf("create body = %#v", creates)
	}
	actions := fake.recordedActions()
	if len(actions) != 1 || actions[0]["action"] != "poweron" {
		t.Fatalf("actions = %#v", actions)
	}
	params, _ := actions[0]["params"].(map[string]any)
	if intField(params["preferred_node"]) != 4 {
		t.Fatalf("power params = %#v", actions[0])
	}
}

func TestCreateTenantOffDoesNotPower(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:       types.StringValue("customer-b"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.PowerState.ValueBool() {
		t.Fatal("tenant should stay off")
	}
	if !data.UIAddress.IsNull() {
		t.Fatalf("ui_address = %#v", data.UIAddress)
	}
	if actions := fake.recordedActions(); len(actions) != 0 {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestUpdateTenantSendsChangedFieldsAndPreservesPassword(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	state := &TenantResourceModel{
		Name:        types.StringValue("customer-a"),
		Description: types.StringValue("old"),
		Password:    types.StringValue("secret-pass"),
		PowerState:  types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	plan := *state
	plan.Description = types.StringValue("new")
	if err := api.updateTenant(context.Background(), &plan, state); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Description.ValueString() != "new" {
		t.Fatalf("description = %s", plan.Description.ValueString())
	}
	if plan.Password.ValueString() != "secret-pass" {
		t.Fatalf("password = %#v", plan.Password)
	}
	updates := fake.bodiesFor(httpMethodPut, "/api/v4/tenants/"+plan.Id.ValueString())
	if len(updates) != 1 {
		t.Fatalf("updates = %#v", updates)
	}
	if updates[0]["description"] != "new" {
		t.Fatalf("update body = %#v", updates[0])
	}
	if _, ok := updates[0]["password"]; ok {
		t.Fatalf("unchanged password was sent: %#v", updates[0])
	}
	if _, ok := updates[0]["name"]; ok {
		t.Fatalf("unchanged name was sent: %#v", updates[0])
	}
}

func TestUpdateTenantPowerOff(t *testing.T) {
	fake := newFake(t)
	fake.uiIP = "203.0.113.20"
	api := fake.api(t)
	state := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(true),
	}
	if err := api.createTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	plan := *state
	plan.PowerState = types.BoolValue(false)
	if err := api.updateTenant(context.Background(), &plan, state); err != nil {
		t.Fatal(err)
	}
	actions := fake.recordedActions()
	if fake.callCount(httpMethodPut, "/api/v4/tenants/"+state.Id.ValueString()) != 0 {
		t.Fatal("power-only update should not PUT the tenant")
	}
	if len(actions) != 2 || actions[1]["action"] != "poweroff" {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestDeleteRunningTenantPowersOffFirst(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(true),
	}
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.deleteTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	actions := fake.recordedActions()
	if len(actions) < 2 || actions[len(actions)-1]["action"] != "poweroff" {
		t.Fatalf("actions = %#v", actions)
	}
	if fake.callCount(httpMethodDelete, "/api/v4/tenants/"+data.Id.ValueString()) != 1 {
		t.Fatal("tenant was not deleted")
	}
	if err := api.readTenant(context.Background(), data); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("read after delete = %v", err)
	}
}

func TestDeleteOfflineTenantSkipsPowerOff(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.deleteTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if actions := fake.recordedActions(); len(actions) != 0 {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestPowerTimeoutLeavesTenantImportable(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = 20 * time.Millisecond
	tenantPowerInterval = time.Millisecond

	fake := newFake(t)
	fake.holdPower = true
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(true),
	}
	err := api.createTenant(context.Background(), data)
	if err == nil || !containsAll(t, err.Error(), "timed out", "can be imported") {
		t.Fatalf("err = %v", err)
	}
	if data.Id.ValueString() == "" {
		t.Fatal("created tenant id was lost")
	}
}

func TestTenantNodeCreateUpdateDelete(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantNodeResourceModel{
		TenantID:    types.StringValue("7"),
		Name:        types.StringValue("node1"),
		CPUCores:    types.Int32Value(4),
		RAM:         types.Int32Value(8192),
		Enabled:     types.BoolValue(true),
		OnPowerLoss: types.StringValue("last_state"),
	}
	if err := api.createTenantNode(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantNode(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.CPUCores.ValueInt32() != 4 || data.RAM.ValueInt32() != 8192 || data.TenantID.ValueString() != "7" {
		t.Fatalf("node = %#v", data)
	}
	state := *data
	plan := *data
	plan.RAM = types.Int32Value(16384)
	if err := api.updateTenantNode(context.Background(), &plan, &state); err != nil {
		t.Fatal(err)
	}
	updates := fake.bodiesFor(httpMethodPut, "/api/v4/tenant_nodes/"+data.Id.ValueString())
	if len(updates) != 1 || intField(updates[0]["ram"]) != 16384 {
		t.Fatalf("update = %#v", updates)
	}
	if _, ok := updates[0]["cpu_cores"]; ok {
		t.Fatalf("unchanged cpu_cores was sent: %#v", updates[0])
	}
	if err := api.deleteTenantNode(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantNode(context.Background(), data); err == nil {
		t.Fatal("node still exists")
	}
}

func TestTenantStorageCreateUpdateDelete(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantStorageResourceModel{
		TenantID:    types.StringValue("7"),
		Tier:        types.Int32Value(1),
		Provisioned: types.Int64Value(107374182400),
	}
	if err := api.createTenantStorage(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantStorage(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Tier.ValueInt32() != 1 || data.Provisioned.ValueInt64() != 107374182400 {
		t.Fatalf("storage = %#v", data)
	}
	if data.Used.ValueInt64() != 0 {
		t.Fatalf("used = %#v", data.Used)
	}
	state := *data
	plan := *data
	plan.Provisioned = types.Int64Value(214748364800)
	if err := api.updateTenantStorage(context.Background(), &plan, &state); err != nil {
		t.Fatal(err)
	}
	updates := fake.bodiesFor(httpMethodPut, "/api/v4/tenant_storage/"+data.Id.ValueString())
	if len(updates) != 1 || int64Field(updates[0]["provisioned"]) != 214748364800 {
		t.Fatalf("update = %#v", updates)
	}
	if err := api.deleteTenantStorage(context.Background(), data); err != nil {
		t.Fatal(err)
	}
}

func TestTenantsDataSourceListsAndFilters(t *testing.T) {
	fake := newFake(t)
	fake.seedTenant(1, "customer-a", "203.0.113.10", true)
	fake.seedTenant(2, "customer-b", "", false)
	api := fake.api(t)

	all := &TenantsDataSourceModel{}
	if err := api.readTenants(context.Background(), all); err != nil {
		t.Fatal(err)
	}
	if len(all.Tenants) != 2 {
		t.Fatalf("tenants = %d", len(all.Tenants))
	}

	filtered := &TenantsDataSourceModel{FilterName: types.StringValue("customer-a")}
	if err := api.readTenants(context.Background(), filtered); err != nil {
		t.Fatal(err)
	}
	if len(filtered.Tenants) != 1 {
		t.Fatalf("filtered = %#v", filtered.Tenants)
	}
	got := filtered.Tenants[0]
	if got.Name.ValueString() != "customer-a" || got.UIAddress.ValueString() != "203.0.113.10" || !got.PowerState.ValueBool() {
		t.Fatalf("tenant = %#v", got)
	}

	miss := &TenantsDataSourceModel{FilterName: types.StringValue("missing")}
	if err := api.readTenants(context.Background(), miss); err != nil {
		t.Fatal(err)
	}
	if len(miss.Tenants) != 0 {
		t.Fatalf("missing filter returned %#v", miss.Tenants)
	}
}

func containsAll(t *testing.T, s string, parts ...string) bool {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}

const (
	httpMethodPost   = "POST"
	httpMethodPut    = "PUT"
	httpMethodDelete = "DELETE"
)
