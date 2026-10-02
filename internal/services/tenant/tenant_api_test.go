// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"
)

func TestTenantPoweredOn(t *testing.T) {
	if tenantPoweredOn(nil) {
		t.Fatal("missing status is off")
	}
	if !tenantPoweredOff(nil) {
		t.Fatal("missing status is off")
	}
	cases := []struct {
		name   string
		status *vergeos.TenantStatus
		on     bool
		off    bool
	}{
		{name: "offline", status: &vergeos.TenantStatus{Status: "offline"}, on: false, off: true},
		{name: "stopping", status: &vergeos.TenantStatus{Status: "stopping", Stopping: true, Running: true}, on: false, off: false},
		{name: "running", status: &vergeos.TenantStatus{Running: true, Status: "online"}, on: true, off: false},
		{name: "starting", status: &vergeos.TenantStatus{Starting: true, Status: "starting"}, on: false, off: false},
		{name: "provisioning", status: &vergeos.TenantStatus{Status: "provisioning"}, on: false, off: false},
		{name: "migrating", status: &vergeos.TenantStatus{Status: "migrating"}, on: true, off: false},
		{name: "error", status: &vergeos.TenantStatus{Status: "error"}, on: false, off: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tenantPoweredOn(tt.status); got != tt.on {
				t.Fatalf("tenantPoweredOn = %v, want %v", got, tt.on)
			}
			if got := tenantPoweredOff(tt.status); got != tt.off {
				t.Fatalf("tenantPoweredOff = %v, want %v", got, tt.off)
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
		ProviderData: tenantTestClient(t),
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
	if err := powerTenant(t, api, data); err != nil {
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
	if err := powerTenant(t, api, data); err != nil {
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
	if _, err := api.updateTenant(context.Background(), &plan, state); err != nil {
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
	if err := powerTenant(t, api, state); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	plan := *state
	plan.PowerState = types.BoolValue(false)
	if _, err := api.updateTenant(context.Background(), &plan, state); err != nil {
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
	if err := powerTenant(t, api, data); err != nil {
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

func TestDeleteOfflineTenantStopsRunningVNet(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = time.Second
	tenantPowerInterval = time.Millisecond

	fake := newFake(t)
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	// Offline tenant whose vnet is still running: the #205 destroy race and
	// the leftover state after a create power timeout (#207).
	fake.mu.Lock()
	fake.status[id] = statusObject(id, false)
	vnetID := intField(fake.tenants[id]["vnet"])
	fake.vnets[vnetID] = map[string]any{"$key": vnetID, "name": "tenant_customer-a", "running": true}
	fake.mu.Unlock()

	if err := api.deleteTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if actions := fake.recordedActions(); len(actions) != 0 {
		t.Fatalf("tenant actions = %#v, want none for already-offline", actions)
	}
	vnetActions := fake.recordedVNetActions()
	if len(vnetActions) != 1 || vnetActions[0]["action"] != "kill" {
		t.Fatalf("vnet actions = %#v, want one kill", vnetActions)
	}
	if fake.callCount(httpMethodDelete, "/api/v4/tenants/"+data.Id.ValueString()) != 1 {
		t.Fatal("tenant was not deleted")
	}
}

func TestDeleteOfflineTenantWithStoppedVNetNeedsNoKill(t *testing.T) {
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
		t.Fatalf("tenant actions = %#v", actions)
	}
	if vnetActions := fake.recordedVNetActions(); len(vnetActions) != 0 {
		t.Fatalf("vnet actions = %#v, want none when already stopped", vnetActions)
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
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	err := powerTenant(t, api, data)
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

func TestReadTenantStorageRejectsForeignTenant(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantStorageResourceModel{
		TenantID:    types.StringValue("7"),
		Tier:        types.Int32Value(1),
		Provisioned: types.Int64Value(1073741824),
	}
	if err := api.createTenantStorage(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	id, err := parseID(data.Id, "tenant storage")
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.storage[id]["tenant"] = 99
	fake.mu.Unlock()

	err = api.readTenantStorage(context.Background(), data)
	if err == nil || !vergeos.IsNotFoundError(err) {
		t.Fatalf("read foreign key = %v, want NotFound", err)
	}
	if data.TenantID.ValueString() != "7" {
		t.Fatalf("state tenant_id was overwritten to %q", data.TenantID.ValueString())
	}
}

func TestReadTenantStorageAllowsMatchingTenant(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantStorageResourceModel{
		TenantID:    types.StringValue("7"),
		Tier:        types.Int32Value(1),
		Provisioned: types.Int64Value(1073741824),
	}
	if err := api.createTenantStorage(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenantStorage(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.TenantID.ValueString() != "7" {
		t.Fatalf("tenant_id = %q", data.TenantID.ValueString())
	}
}

func TestReadTenantStorageSkipsOwnershipOnImport(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	created := &TenantStorageResourceModel{
		TenantID:    types.StringValue("7"),
		Tier:        types.Int32Value(1),
		Provisioned: types.Int64Value(1073741824),
	}
	if err := api.createTenantStorage(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	data := &TenantStorageResourceModel{Id: created.Id}
	if err := api.readTenantStorage(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.TenantID.ValueString() != "7" {
		t.Fatalf("imported tenant_id = %q", data.TenantID.ValueString())
	}
}

func TestReadTenantNodeRejectsForeignTenant(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantNodeResourceModel{
		TenantID: types.StringValue("7"),
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	id, err := parseID(data.Id, "tenant node")
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.nodes[id]["tenant"] = 99
	fake.mu.Unlock()

	err = api.readTenantNode(context.Background(), data)
	if err == nil || !vergeos.IsNotFoundError(err) {
		t.Fatalf("read foreign key = %v, want NotFound", err)
	}
	if data.TenantID.ValueString() != "7" {
		t.Fatalf("state tenant_id was overwritten to %q", data.TenantID.ValueString())
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

func TestCreateKeepsIDWhenPowerFails(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = 20 * time.Millisecond
	tenantPowerInterval = time.Millisecond

	fake := newFake(t)
	fake.holdPower = true
	// Seed a node so create does not defer power-on (#207); holdPower then
	// times out waiting for terminal online.
	fake.mu.Lock()
	fake.nodes[1] = map[string]any{"$key": 1, "tenant": 1, "name": "n1", "machine": 81}
	fake.mu.Unlock()
	ctx := context.Background()
	resp := createTenantResource(t, ctx, fake, TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		Password:   types.StringValue("Tf-acc-tenant-password"),
		PowerState: types.BoolValue(true),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected power to fail")
	}
	if !containsAll(t, diagnosticText(resp.Diagnostics), "was created", "timed out") {
		t.Fatalf("diagnostics = %s", diagnosticText(resp.Diagnostics))
	}
	if id := tenantStateID(t, ctx, resp.State); id == "" {
		t.Fatal("power failure dropped the tenant id")
	}
}

func TestCreateDefersPowerOnWithoutNodesKeepsPlannedTrue(t *testing.T) {
	fake := newFake(t)
	ctx := context.Background()
	resp := createTenantResource(t, ctx, fake, TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		Password:   types.StringValue("Tf-acc-tenant-password"),
		PowerState: types.BoolValue(true),
	})
	if resp.Diagnostics.HasError() {
		t.Fatalf("diagnostics = %s", diagnosticText(resp.Diagnostics))
	}
	if actions := fake.recordedActions(); len(actions) != 0 {
		t.Fatalf("actions = %#v, want deferred power-on with no orphan vnet", actions)
	}
	id := tenantStateID(t, ctx, resp.State)
	if id == "" {
		t.Fatal("missing tenant id")
	}
	var stored TenantResourceModel
	diags := resp.State.Get(ctx, &stored)
	if diags.HasError() {
		t.Fatalf("state get: %s", diagnosticText(diags))
	}
	if !stored.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want planned true kept after deferred create", stored.PowerState)
	}
}

func TestCreateKeepsIDWhenReadFails(t *testing.T) {
	fake := newFake(t)
	// Create reads the tenant back once, ensureTenantVNetStopped reads it
	// again, then Create's follow-up read should fail (#205 path).
	fake.failTenantGet = 2
	ctx := context.Background()
	resp := createTenantResource(t, ctx, fake, TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	})
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the follow-up read to fail")
	}
	if !strings.Contains(diagnosticText(resp.Diagnostics), "Error reading tenant") {
		t.Fatalf("diagnostics = %s", diagnosticText(resp.Diagnostics))
	}
	if id := tenantStateID(t, ctx, resp.State); id == "" {
		t.Fatal("read failure dropped the tenant id")
	}
}

func TestReadKeepsConfiguredChangePassword(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:           types.StringValue("customer-a"),
		Password:       types.StringValue("secret-pass"),
		ChangePassword: types.BoolValue(true),
		PowerState:     types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.tenants[id]["change_password"] = false
	fake.mu.Unlock()
	if err := api.readTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ChangePassword.IsNull() || !data.ChangePassword.ValueBool() {
		t.Fatalf("change_password refreshed from API: %#v", data.ChangePassword)
	}
	if data.Password.ValueString() != "secret-pass" {
		t.Fatalf("password = %#v", data.Password)
	}
}

func TestWaitPowerIgnoresStartingAndStopping(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = 30 * time.Millisecond
	tenantPowerInterval = time.Millisecond

	fake := newFake(t)
	api := fake.api(t)
	fake.mu.Lock()
	fake.status[7] = statusObjectTransitional(7, true)
	fake.mu.Unlock()
	if err := api.waitPower(context.Background(), 7, true); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("starting should not settle online: %v", err)
	}

	fake.mu.Lock()
	fake.status[8] = statusObjectTransitional(8, false)
	fake.mu.Unlock()
	if err := api.waitPower(context.Background(), 8, false); err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("stopping should not settle offline: %v", err)
	}
}

func TestReconcilePowerWaitsThroughTransitions(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = time.Second
	tenantPowerInterval = time.Millisecond

	fake := newFake(t)
	fake.transitionPower = true
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(true),
	}
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if err := powerTenant(t, api, data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if !data.PowerState.ValueBool() || data.Status.ValueString() != "online" {
		t.Fatalf("after power on: powerstate=%v status=%s", data.PowerState.ValueBool(), data.Status.ValueString())
	}

	plan := *data
	plan.PowerState = types.BoolValue(false)
	if _, err := api.updateTenant(context.Background(), &plan, data); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.PowerState.ValueBool() || plan.Status.ValueString() != "offline" {
		t.Fatalf("after power off: powerstate=%v status=%s", plan.PowerState.ValueBool(), plan.Status.ValueString())
	}
}

func TestDeleteTenantNodePowerOffsRunningNodeOnly(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = time.Second
	tenantPowerInterval = time.Millisecond

	fake := newFake(t)
	api := fake.api(t)
	tenant := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(true),
	}
	if err := api.createTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	if err := powerTenant(t, api, tenant); err != nil {
		t.Fatal(err)
	}
	running := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	stopped := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node2"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), running); err != nil {
		t.Fatal(err)
	}
	if err := api.createTenantNode(context.Background(), stopped); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	runningID, _ := parseID(running.Id, "tenant node")
	machineID := intField(fake.nodes[runningID]["machine"])
	fake.machines[machineID]["running"] = true
	fake.machines[machineID]["status"] = "running"
	fake.mu.Unlock()

	beforeTenantActions := len(fake.recordedActions())
	if err := api.deleteTenantNode(context.Background(), running); err != nil {
		t.Fatal(err)
	}
	if actions := fake.recordedActions(); len(actions) != beforeTenantActions {
		t.Fatalf("tenant power actions = %#v, want unchanged (siblings stay up)", actions[beforeTenantActions:])
	}
	nodeActions := fake.recordedNodeActions()
	if len(nodeActions) != 1 || nodeActions[0]["action"] != "poweroff" {
		t.Fatalf("node actions = %#v, want one poweroff (no kill)", nodeActions)
	}
	if fake.callCount(httpMethodDelete, "/api/v4/tenant_nodes/"+running.Id.ValueString()) != 1 {
		t.Fatal("running node was not deleted")
	}

	// Stopped sibling deletes with no power action while tenant stays online.
	if err := api.deleteTenantNode(context.Background(), stopped); err != nil {
		t.Fatal(err)
	}
	if nodeActions := fake.recordedNodeActions(); len(nodeActions) != 1 {
		t.Fatalf("node actions after stopped delete = %#v", nodeActions)
	}
	status, err := api.tenantStatus(context.Background(), mustParseID(t, tenant.Id))
	if err != nil {
		t.Fatal(err)
	}
	if !tenantPoweredOn(status) {
		t.Fatalf("tenant status = %#v, want still online after node deletes", status)
	}
}

func TestDeleteTenantNodeKillsWhenPowerOffStuck(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = 20 * time.Millisecond
	tenantPowerInterval = 5 * time.Millisecond

	fake := newFake(t)
	fake.nodePowerOffStuck = true
	api := fake.api(t)
	tenant := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(true),
	}
	if err := api.createTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	if err := powerTenant(t, api, tenant); err != nil {
		t.Fatal(err)
	}
	node := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	nodeID, _ := parseID(node.Id, "tenant node")
	machineID := intField(fake.nodes[nodeID]["machine"])
	fake.machines[machineID]["running"] = true
	fake.machines[machineID]["status"] = "running"
	fake.mu.Unlock()

	if err := api.deleteTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	nodeActions := fake.recordedNodeActions()
	if len(nodeActions) != 2 {
		t.Fatalf("node actions = %#v, want poweroff then kill", nodeActions)
	}
	if nodeActions[0]["action"] != "poweroff" || nodeActions[1]["action"] != "kill" {
		t.Fatalf("node actions = %#v, want poweroff then kill", nodeActions)
	}
	if fake.callCount(httpMethodDelete, "/api/v4/tenant_nodes/"+node.Id.ValueString()) != 1 {
		t.Fatal("node was not deleted after kill")
	}
}

func TestEnsureTenantNodeStoppedNotFound(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	if err := api.ensureTenantNodeStopped(context.Background(), 99999); err != nil {
		t.Fatalf("missing node must be OK: %v", err)
	}
	if actions := fake.recordedNodeActions(); len(actions) != 0 {
		t.Fatalf("node actions = %#v, want none", actions)
	}
}

func TestDeleteTenantNodePowerOffNotRunningFallsThrough(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = 20 * time.Millisecond
	tenantPowerInterval = 5 * time.Millisecond

	fake := newFake(t)
	fake.nodePowerOffNotRunning = true
	api := fake.api(t)
	tenant := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	node := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	nodeID, _ := parseID(node.Id, "tenant node")
	machineID := intField(fake.nodes[nodeID]["machine"])
	// Machine status claims running, but poweroff returns 422 not-running.
	// After the 422, re-check sees running still true so Kill is used.
	fake.machines[machineID]["running"] = true
	fake.machines[machineID]["status"] = "running"
	fake.mu.Unlock()

	if err := api.deleteTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	nodeActions := fake.recordedNodeActions()
	if len(nodeActions) != 2 || nodeActions[0]["action"] != "poweroff" || nodeActions[1]["action"] != "kill" {
		t.Fatalf("node actions = %#v, want poweroff then kill", nodeActions)
	}
}

func TestDeleteTenantNodeKillWaitTimeout(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = 15 * time.Millisecond
	tenantPowerInterval = 5 * time.Millisecond

	fake := newFake(t)
	fake.nodePowerOffStuck = true
	fake.nodeKillStuck = true
	api := fake.api(t)
	tenant := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	node := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	nodeID, _ := parseID(node.Id, "tenant node")
	machineID := intField(fake.nodes[nodeID]["machine"])
	fake.machines[machineID]["running"] = true
	fake.machines[machineID]["status"] = "running"
	fake.mu.Unlock()

	err := api.deleteTenantNode(context.Background(), node)
	if err == nil {
		t.Fatal("expected timeout waiting for node to stop after kill")
	}
	if !strings.Contains(err.Error(), "timed out waiting for tenant node") {
		t.Fatalf("error = %v, want stop timeout", err)
	}
	if !strings.Contains(err.Error(), "still exists and can be imported") {
		t.Fatalf("error = %v, want import guidance", err)
	}
	nodeActions := fake.recordedNodeActions()
	if len(nodeActions) != 2 || nodeActions[0]["action"] != "poweroff" || nodeActions[1]["action"] != "kill" {
		t.Fatalf("node actions = %#v, want poweroff then kill", nodeActions)
	}
}

func TestDeleteStoppedTenantNodeSkipsStop(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	tenant := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	node := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	if err := api.deleteTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	if actions := fake.recordedActions(); len(actions) != 0 {
		t.Fatalf("tenant actions = %#v", actions)
	}
	if nodeActions := fake.recordedNodeActions(); len(nodeActions) != 0 {
		t.Fatalf("node actions = %#v", nodeActions)
	}
}

func TestDeleteTenantNodeRetriesLastNode405(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = time.Second
	tenantPowerInterval = time.Millisecond

	fake := newFake(t)
	fake.lastNodeDeleteFails = 2
	api := fake.api(t)
	tenant := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	node := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	if err := api.deleteTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	deletes := fake.callCount(http.MethodDelete, "/api/v4/tenant_nodes/"+node.Id.ValueString())
	if deletes < 3 {
		t.Fatalf("delete calls = %d, want at least 3 (2x 405 then success)", deletes)
	}
	if _, ok := fake.nodes[mustParseID(t, node.Id)]; ok {
		t.Fatal("node still present after successful retry delete")
	}
}

func TestDeleteTenantNodePermanentError(t *testing.T) {
	fake := newFake(t)
	fake.nodeDeleteFailStatus = http.StatusInternalServerError
	fake.nodeDeleteFailMessage = "forced node delete failure"
	api := fake.api(t)
	tenant := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	node := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	err := api.deleteTenantNode(context.Background(), node)
	if err == nil {
		t.Fatal("expected permanent delete error")
	}
	if !strings.Contains(err.Error(), "forced node delete failure") {
		t.Fatalf("error = %v, want forced failure message", err)
	}
	if fake.callCount(http.MethodDelete, "/api/v4/tenant_nodes/"+node.Id.ValueString()) != 1 {
		t.Fatal("permanent errors must not be retried")
	}
}

func TestDeleteTenantNodeLastNode405Timeout(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = 20 * time.Millisecond
	tenantPowerInterval = 5 * time.Millisecond

	fake := newFake(t)
	fake.lastNodeDeleteFails = 1000
	api := fake.api(t)
	tenant := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	node := &TenantNodeResourceModel{
		TenantID: tenant.Id,
		Name:     types.StringValue("node1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	err := api.deleteTenantNode(context.Background(), node)
	if err == nil {
		t.Fatal("expected timeout waiting for last-node delete")
	}
	if !strings.Contains(err.Error(), "timed out waiting to delete tenant node") {
		t.Fatalf("error = %v, want timeout message", err)
	}
	if !strings.Contains(err.Error(), "still exists and can be imported") {
		t.Fatalf("error = %v, want import guidance", err)
	}
}

func TestIsLastNodeDeleteError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "not found", err: &vergeos.NotFoundError{Resource: "tenant_nodes", ID: 1}, want: false},
		{
			name: "405 last node",
			err:  &vergeos.APIError{StatusCode: http.StatusMethodNotAllowed, Endpoint: "/tenant_nodes/1", Message: "Only the last node can be deleted"},
			want: true,
		},
		{
			name: "405 running",
			err:  &vergeos.APIError{StatusCode: http.StatusMethodNotAllowed, Endpoint: "/tenant_nodes/1", Message: "Tenant node cannot be deleted while running"},
			want: false,
		},
		{
			name: "500",
			err:  &vergeos.APIError{StatusCode: http.StatusInternalServerError, Endpoint: "/tenant_nodes/1", Message: "Only the last node can be deleted"},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isLastNodeDeleteError(tc.err); got != tc.want {
				t.Fatalf("isLastNodeDeleteError() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestReconcilePowerOnCreateDefersWithoutNodes(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		PowerState: types.BoolValue(true),
	}
	if err := api.createTenant(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	id := mustParseID(t, data.Id)
	deferred, err := api.reconcilePowerOnCreate(context.Background(), id, data.PowerState, data.PreferredNode)
	if err != nil {
		t.Fatal(err)
	}
	if !deferred {
		t.Fatal("expected deferred power-on when no nodes exist")
	}
	if actions := fake.recordedActions(); len(actions) != 0 {
		t.Fatalf("actions = %#v, want none (no orphan running vnet)", actions)
	}
	status, err := api.tenantStatus(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if tenantPoweredOn(status) {
		t.Fatalf("status = %#v, want offline", status)
	}
}

// TestUpdateDefersPowerOnWithoutNodes covers #219: update with powerstate=true
// and no vergeio_tenant_node must defer like create, not PowerOn + waitPower.
func TestUpdateDefersPowerOnWithoutNodes(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	state := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		Password:   types.StringValue("Tf-acc-tenant-password"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	plan := *state
	plan.PowerState = types.BoolValue(true)
	deferred, err := api.updateTenant(context.Background(), &plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if !deferred {
		t.Fatal("expected deferred power-on on update when no nodes exist")
	}
	if actions := fake.recordedActions(); len(actions) != 0 {
		t.Fatalf("actions = %#v, want none (no PowerOn, no orphan vnet)", actions)
	}
}

// TestUpdatePowersOnWhenNodePresent covers #219: update with a tenant node
// still powers on through reconcilePower.
func TestUpdatePowersOnWhenNodePresent(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	state := &TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		Password:   types.StringValue("Tf-acc-tenant-password"),
		PowerState: types.BoolValue(false),
	}
	if err := api.createTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	node := &TenantNodeResourceModel{
		TenantID: state.Id,
		Name:     types.StringValue("n1"),
		CPUCores: types.Int32Value(2),
		RAM:      types.Int32Value(2048),
		Enabled:  types.BoolValue(true),
	}
	if err := api.createTenantNode(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	plan := *state
	plan.PowerState = types.BoolValue(true)
	deferred, err := api.updateTenant(context.Background(), &plan, state)
	if err != nil {
		t.Fatal(err)
	}
	if deferred {
		t.Fatal("expected power-on when a node exists")
	}
	actions := fake.recordedActions()
	if len(actions) == 0 || actions[0]["action"] != "poweron" {
		t.Fatalf("actions = %#v, want poweron", actions)
	}
	if err := api.readTenant(context.Background(), &plan); err != nil {
		t.Fatal(err)
	}
	if !plan.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want true after power on", plan.PowerState)
	}
}

// TestUpdateResourceDefersPowerOnWithoutNodesKeepsPlannedTrue covers #219 at
// the resource Update layer: deferred update must not PowerOn and must keep
// planned powerstate=true in the apply response (same as create #207).
func TestUpdateResourceDefersPowerOnWithoutNodesKeepsPlannedTrue(t *testing.T) {
	fake := newFake(t)
	ctx := context.Background()
	createResp := createTenantResource(t, ctx, fake, TenantResourceModel{
		Name:       types.StringValue("customer-a"),
		Password:   types.StringValue("Tf-acc-tenant-password"),
		PowerState: types.BoolValue(true),
	})
	if createResp.Diagnostics.HasError() {
		t.Fatalf("create diagnostics = %s", diagnosticText(createResp.Diagnostics))
	}
	var state TenantResourceModel
	if diags := createResp.State.Get(ctx, &state); diags.HasError() {
		t.Fatalf("state get: %s", diagnosticText(diags))
	}
	// Simulate post-apply refresh: actual powerstate is offline.
	state.PowerState = types.BoolValue(false)
	plan := state
	plan.PowerState = types.BoolValue(true)
	updateResp := updateTenantResource(t, ctx, fake, plan, state)
	if updateResp.Diagnostics.HasError() {
		t.Fatalf("update diagnostics = %s", diagnosticText(updateResp.Diagnostics))
	}
	if actions := fake.recordedActions(); len(actions) != 0 {
		t.Fatalf("actions = %#v, want deferred power-on with no orphan vnet", actions)
	}
	var stored TenantResourceModel
	if diags := updateResp.State.Get(ctx, &stored); diags.HasError() {
		t.Fatalf("state get: %s", diagnosticText(diags))
	}
	if !stored.PowerState.ValueBool() {
		t.Fatalf("powerstate = %#v, want planned true kept after deferred update", stored.PowerState)
	}
}

func mustParseID(t *testing.T, v types.String) int {
	t.Helper()
	id, err := parseID(v, "id")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEnsurePoweredOffWaitsForStartingBeforePowerOff(t *testing.T) {
	origTimeout, origInterval := tenantPowerTimeout, tenantPowerInterval
	t.Cleanup(func() {
		tenantPowerTimeout = origTimeout
		tenantPowerInterval = origInterval
	})
	tenantPowerTimeout = time.Second
	tenantPowerInterval = time.Millisecond

	fake := newFake(t)
	fake.transitionPower = true
	api := fake.api(t)
	fake.mu.Lock()
	id := 3
	fake.tenants[id] = map[string]any{"$key": id, "name": "t", "ui_address": 0}
	fake.status[id] = statusObjectTransitional(id, true)
	fake.seenTransition[id] = false
	fake.mu.Unlock()
	if err := api.ensurePoweredOff(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	actions := fake.recordedActions()
	if len(actions) != 1 || actions[0]["action"] != "poweroff" {
		t.Fatalf("actions = %#v", actions)
	}
	status, err := api.tenantStatus(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !tenantPoweredOff(status) {
		t.Fatalf("status = %#v", status)
	}
}

func powerTenant(t *testing.T, api *API, data *TenantResourceModel) error {
	t.Helper()
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		return err
	}
	return api.reconcilePower(context.Background(), id, data.PowerState, data.PreferredNode)
}

func createTenantResource(t *testing.T, ctx context.Context, fake *fakeVerge, planModel TenantResourceModel) *resource.CreateResponse {
	t.Helper()
	r := &TenantResource{api: fake.api(t)}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("plan: %v", diags)
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, resp)
	return resp
}

func updateTenantResource(t *testing.T, ctx context.Context, fake *fakeVerge, planModel, stateModel TenantResourceModel) *resource.UpdateResponse {
	t.Helper()
	r := &TenantResource{api: fake.api(t)}
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &planModel); diags.HasError() {
		t.Fatalf("plan: %v", diags)
	}
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &stateModel); diags.HasError() {
		t.Fatalf("state: %v", diags)
	}
	resp := &resource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan, State: state}, resp)
	return resp
}

func tenantStateID(t *testing.T, ctx context.Context, state tfsdk.State) string {
	t.Helper()
	if state.Raw.Type() == nil || state.Raw.IsNull() {
		return ""
	}
	var got TenantResourceModel
	if diags := state.Get(ctx, &got); diags.HasError() {
		t.Fatalf("state: %v", diags)
	}
	return got.Id.ValueString()
}

func diagnosticText(diags diag.Diagnostics) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Summary())
		b.WriteString(" ")
		b.WriteString(d.Detail())
		b.WriteString("\n")
	}
	return b.String()
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
