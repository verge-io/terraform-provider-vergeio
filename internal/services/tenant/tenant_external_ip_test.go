// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"
)

func TestTenantExternalIPSchema(t *testing.T) {
	r := NewTenantExternalIPResource()
	meta := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_external_ip" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, name := range []string{"tenant_id", "network_id", "ip"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok || !attr.IsRequired() {
			t.Fatalf("%s should be required", name)
		}
		if !stringRequiresReplace(t, attr.PlanModifiers) {
			t.Fatalf("%s should require replace", name)
		}
	}
	for _, name := range []string{"hostname", "description"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok || !attr.IsOptional() || attr.IsRequired() {
			t.Fatalf("%s should be optional", name)
		}
		if !stringRequiresReplace(t, attr.PlanModifiers) {
			t.Fatalf("%s should require replace", name)
		}
	}
	apply, ok := resp.Schema.Attributes["apply_parent_firewall"].(schema.BoolAttribute)
	if !ok || !apply.IsOptional() || !apply.IsComputed() || apply.Default == nil {
		t.Fatal("apply_parent_firewall should be optional and computed with a default")
	}
	for _, name := range []string{"parent_firewall_pending", "parent_firewall_applied"} {
		attr := resp.Schema.Attributes[name]
		if attr == nil || !attr.IsComputed() || attr.IsOptional() || attr.IsRequired() {
			t.Fatalf("%s should be computed", name)
		}
	}
	if resp.Schema.MarkdownDescription == "" || !containsAll(t, resp.Schema.MarkdownDescription, "need_fw_apply", "UI address", "WithApplyParentFirewall") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestTenantExternalIPRejectsInvalidIP(t *testing.T) {
	resp := &resource.SchemaResponse{}
	NewTenantExternalIPResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	attr := resp.Schema.Attributes["ip"].(schema.StringAttribute)
	var v validator.String
	for _, candidate := range attr.Validators {
		v = candidate
	}
	if v == nil {
		t.Fatal("ip should have a validator")
	}
	bad := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("ip"),
		ConfigValue: types.StringValue("not-an-ip"),
	}, bad)
	if !bad.Diagnostics.HasError() {
		t.Fatal("invalid ip was accepted")
	}
	good := &validator.StringResponse{}
	v.ValidateString(context.Background(), validator.StringRequest{
		Path:        path.Root("ip"),
		ConfigValue: types.StringValue("203.0.113.50"),
	}, good)
	if good.Diagnostics.HasError() {
		t.Fatal(good.Diagnostics)
	}
}

func TestTenantExternalIPConfigure(t *testing.T) {
	r := &TenantExternalIPResource{}
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

func TestCreateTenantExternalIPLeavesParentFirewallPending(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	data.Hostname = types.StringValue("edge")
	data.Description = types.StringValue("ui")
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() == "" {
		t.Fatal("missing id")
	}
	if !data.ParentFirewallPending.ValueBool() || data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
	creates := fake.bodiesFor(http.MethodPost, "/api/v4/vnet_addresses")
	if len(creates) != 1 {
		t.Fatalf("create bodies = %#v", creates)
	}
	if creates[0]["ip"] != "203.0.113.50" || creates[0]["type"] != "virtual" || creates[0]["owner"] != "tenants/7" || creates[0]["hostname"] != "edge" {
		t.Fatalf("create body = %#v", creates[0])
	}
	if _, ok := creates[0]["vnet"]; !ok {
		t.Fatal("create body omitted vnet")
	}
	for _, action := range fake.recordedVNetActions() {
		if action["action"] == "refresh" {
			t.Fatal("rules were applied without apply_parent_firewall")
		}
	}
	if err := api.readTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.IP.ValueString() != "203.0.113.50" || data.TenantID.ValueString() != "7" || data.NetworkID.ValueString() != "10" {
		t.Fatalf("read model = %#v", data)
	}
	if data.Hostname.ValueString() != "edge" || data.Description.ValueString() != "ui" {
		t.Fatalf("hostname/description = %#v %#v", data.Hostname, data.Description)
	}
	if !data.ParentFirewallPending.ValueBool() {
		t.Fatal("read cleared parent_firewall_pending")
	}
}

func TestCreateTenantExternalIPAppliesParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", true)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
	var refreshed bool
	for _, action := range fake.recordedVNetActions() {
		if action["action"] == "refresh" && intField(action["vnet"]) == 10 {
			refreshed = true
		}
	}
	if !refreshed {
		t.Fatalf("vnet actions = %#v", fake.recordedVNetActions())
	}
	if err := api.readTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatal("read did not keep the applied firewall status")
	}
}

func TestCreateTenantExternalIPBecomesUIAddress(t *testing.T) {
	fake := newFake(t)
	fake.seedTenant(7, "customer-a", "", false)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	tenant := &TenantResourceModel{Id: types.StringValue("7")}
	if err := api.readTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	if tenant.UIAddress.ValueString() != "203.0.113.50" {
		t.Fatalf("ui_address = %#v", tenant.UIAddress)
	}
	if tenant.UIAddressID.ValueInt32() == 0 {
		t.Fatal("ui_address_id was not set")
	}
	second := externalIPModel("7", "10", "203.0.113.51", false)
	if err := api.createTenantExternalIP(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if err := api.readTenant(context.Background(), tenant); err != nil {
		t.Fatal(err)
	}
	if tenant.UIAddress.ValueString() != "203.0.113.50" {
		t.Fatalf("second address replaced ui_address: %#v", tenant.UIAddress)
	}
}

func TestReadTenantExternalIPRejectsReusedKey(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	data.TenantID = types.StringValue("8")
	err := api.readTenantExternalIP(context.Background(), data)
	if !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused key err = %v", err)
	}
}

func TestReadTenantExternalIPNotFound(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	data.Id = types.StringValue("99")
	err := api.readTenantExternalIP(context.Background(), data)
	if !vergeos.IsNotFoundError(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateTenantExternalIPAppliesParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	data.ApplyParentFirewall = types.BoolValue(true)
	if err := api.updateTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
}

func TestUpdateTenantExternalIPWaitsForParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.fwSettleReads = 2
	fake.mu.Unlock()
	setParentFirewallWait(t, time.Millisecond, time.Second)
	before := fake.callCount(http.MethodGet, "/api/v4/vnets/10")
	data.ApplyParentFirewall = types.BoolValue(true)
	if err := api.updateTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
	if got := fake.callCount(http.MethodGet, "/api/v4/vnets/10") - before; got < 3 {
		t.Fatalf("vnet gets after apply = %d, want at least 3", got)
	}
	if err := api.readTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatal("read after the flag settled still reported parent_firewall_pending")
	}
}

func TestCreateTenantExternalIPWaitsForParentFirewall(t *testing.T) {
	fake := newFake(t)
	fake.mu.Lock()
	fake.fwSettleReads = 2
	fake.mu.Unlock()
	setParentFirewallWait(t, time.Millisecond, time.Second)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", true)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
	if got := fake.callCount(http.MethodGet, "/api/v4/vnets/10"); got < 3 {
		t.Fatalf("vnet gets = %d, want at least 3", got)
	}
}

func TestUpdateTenantExternalIPKeepsPendingWhenFlagStaysSet(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.fwSettleReads = 1000
	fake.mu.Unlock()
	setParentFirewallWait(t, time.Millisecond, 15*time.Millisecond)
	data.ApplyParentFirewall = types.BoolValue(true)
	start := time.Now()
	if err := api.updateTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("apply waited %s", time.Since(start))
	}
	if !data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
}

func TestUpdateTenantExternalIPDoesNotWaitWhenApplyIsFalse(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := api.updateTenantExternalIP(ctx, data); err != nil {
		t.Fatal(err)
	}
	if !data.ParentFirewallPending.ValueBool() || data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
}

func setParentFirewallWait(t *testing.T, interval, timeout time.Duration) {
	t.Helper()
	prevInterval := parentFirewallPollInterval
	prevTimeout := parentFirewallSettleTimeout
	parentFirewallPollInterval = interval
	parentFirewallSettleTimeout = timeout
	t.Cleanup(func() {
		parentFirewallPollInterval = prevInterval
		parentFirewallSettleTimeout = prevTimeout
	})
}

func TestDeleteTenantExternalIP(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", false)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	status, err := api.deleteTenantExternalIP(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if status == nil || !status.Pending || status.Applied {
		t.Fatalf("delete status = %#v", status)
	}
	if err := api.readTenantExternalIP(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("address still readable: %v", err)
	}
	status, err = api.deleteTenantExternalIP(context.Background(), data)
	if err != nil || status != nil {
		t.Fatalf("second delete status=%#v err=%v", status, err)
	}
}

func TestDeleteTenantExternalIPAppliesParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := externalIPModel("7", "10", "203.0.113.50", true)
	if err := api.createTenantExternalIP(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.vnetActions = nil
	fake.mu.Unlock()
	status, err := api.deleteTenantExternalIP(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if status == nil || status.Pending || !status.Applied {
		t.Fatalf("delete status = %#v", status)
	}
	var refreshed bool
	for _, action := range fake.recordedVNetActions() {
		if action["action"] == "refresh" {
			refreshed = true
		}
	}
	if !refreshed {
		t.Fatal("delete did not apply parent firewall rules")
	}
}

func TestParentFirewallPendingWarning(t *testing.T) {
	pending := externalIPModel("7", "10", "203.0.113.50", false)
	pending.ParentFirewallPending = types.BoolValue(true)
	summary, detail := parentFirewallPendingWarning(false, *pending)
	if summary == "" || !containsAll(t, detail, "need_fw_apply", "10") {
		t.Fatalf("warning = %q %q", summary, detail)
	}
	if summary, detail = parentFirewallPendingWarning(true, *pending); summary == "" || !containsAll(t, detail, "still set") {
		t.Fatalf("applied warning = %q %q", summary, detail)
	}
	clear := *pending
	clear.ParentFirewallPending = types.BoolValue(false)
	if summary, detail = parentFirewallPendingWarning(false, clear); summary != "" || detail != "" {
		t.Fatalf("unexpected warning %q %q", summary, detail)
	}
}

func externalIPModel(tenantID, networkID, ip string, apply bool) *TenantExternalIPResourceModel {
	return &TenantExternalIPResourceModel{
		TenantID:            types.StringValue(tenantID),
		NetworkID:           types.StringValue(networkID),
		IP:                  types.StringValue(ip),
		ApplyParentFirewall: types.BoolValue(apply),
	}
}

func stringRequiresReplace(t *testing.T, mods []planmodifier.String) bool {
	t.Helper()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: types.StringValue("b")}
		mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
			StateValue:  types.StringValue("a"),
			PlanValue:   types.StringValue("b"),
			ConfigValue: types.StringValue("b"),
			State:       tfsdk.State{Raw: raw},
			Plan:        tfsdk.Plan{Raw: raw},
		}, resp)
		if resp.RequiresReplace {
			return true
		}
	}
	return false
}
