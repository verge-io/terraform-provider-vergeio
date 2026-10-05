// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestTenantNetworkBlockSchema(t *testing.T) {
	r := NewTenantNetworkBlockResource()
	meta := &resource.MetadataResponse{}
	r.Metadata(context.Background(), resource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_tenant_network_block" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &resource.SchemaResponse{}
	r.Schema(context.Background(), resource.SchemaRequest{}, resp)
	for _, name := range []string{"tenant_id", "network_id", "cidr"} {
		attr, ok := resp.Schema.Attributes[name].(schema.StringAttribute)
		if !ok || !attr.IsRequired() {
			t.Fatalf("%s should be required", name)
		}
		if !stringRequiresReplace(t, attr.PlanModifiers) {
			t.Fatalf("%s should require replace", name)
		}
	}
	desc, ok := resp.Schema.Attributes["description"].(schema.StringAttribute)
	if !ok || !desc.IsOptional() || desc.IsRequired() {
		t.Fatal("description should be optional")
	}
	if !stringRequiresReplace(t, desc.PlanModifiers) {
		t.Fatal("description should require replace")
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
	if resp.Schema.MarkdownDescription == "" || !containsAll(t, resp.Schema.MarkdownDescription, "need_fw_apply", "WithApplyParentFirewall", "tenant network") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestTenantNetworkBlockRejectsInvalidCIDR(t *testing.T) {
	resp := &resource.SchemaResponse{}
	NewTenantNetworkBlockResource().Schema(context.Background(), resource.SchemaRequest{}, resp)
	attr := resp.Schema.Attributes["cidr"].(schema.StringAttribute)
	var v validator.String
	for _, candidate := range attr.Validators {
		v = candidate
	}
	if v == nil {
		t.Fatal("cidr should have a validator")
	}
	for _, badValue := range []string{"not-a-cidr", "192.168.100.1/24", "192.168.100.0", "2001:db8::1/64"} {
		bad := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("cidr"),
			ConfigValue: types.StringValue(badValue),
		}, bad)
		if !bad.Diagnostics.HasError() {
			t.Fatalf("invalid cidr %q was accepted", badValue)
		}
	}
	for _, goodValue := range []string{"192.168.100.0/24", "203.0.113.64/28", "2001:db8::/64"} {
		good := &validator.StringResponse{}
		v.ValidateString(context.Background(), validator.StringRequest{
			Path:        path.Root("cidr"),
			ConfigValue: types.StringValue(goodValue),
		}, good)
		if good.Diagnostics.HasError() {
			t.Fatalf("%s: %v", goodValue, good.Diagnostics)
		}
	}
}

func TestTenantNetworkBlockConfigure(t *testing.T) {
	r := &TenantNetworkBlockResource{}
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

func TestCreateTenantNetworkBlockLeavesParentFirewallPending(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	data.Description = types.StringValue("customer block")
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() == "" {
		t.Fatal("missing id")
	}
	if !data.ParentFirewallPending.ValueBool() || data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
	creates := fake.bodiesFor(http.MethodPost, "/api/v4/vnet_cidrs")
	if len(creates) != 1 {
		t.Fatalf("create bodies = %#v", creates)
	}
	if creates[0]["cidr"] != "192.168.100.0/24" || creates[0]["owner"] != "tenants/7" || creates[0]["description"] != "customer block" {
		t.Fatalf("create body = %#v", creates[0])
	}
	if _, ok := creates[0]["vnet"]; !ok {
		t.Fatal("create body omitted vnet")
	}
	if _, ok := creates[0]["tenant"]; ok {
		t.Fatal("tenant must be sent as owner")
	}
	for _, action := range fake.recordedVNetActions() {
		if action["action"] == "refresh" {
			t.Fatal("rules were applied without apply_parent_firewall")
		}
	}
	if err := api.readTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.CIDR.ValueString() != "192.168.100.0/24" || data.TenantID.ValueString() != "7" || data.NetworkID.ValueString() != "10" {
		t.Fatalf("read model = %#v", data)
	}
	if data.Description.ValueString() != "customer block" {
		t.Fatalf("description = %#v", data.Description)
	}
	if !data.ParentFirewallPending.ValueBool() {
		t.Fatal("read cleared parent_firewall_pending")
	}
}

func TestCreateTenantNetworkBlockAppliesParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", true)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
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
	if err := api.readTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatal("read did not keep the applied firewall status")
	}
}

func TestCreateTenantNetworkBlockWaitsForParentFirewall(t *testing.T) {
	fake := newFake(t)
	fake.mu.Lock()
	fake.fwSettleReads = 2
	fake.mu.Unlock()
	setParentFirewallWait(t, time.Millisecond, time.Second)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", true)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
	if got := fake.callCount(http.MethodGet, "/api/v4/vnets/10"); got < 3 {
		t.Fatalf("vnet gets = %d, want at least 3", got)
	}
}

func TestReadTenantNetworkBlockRejectsReusedKey(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	original := *data
	data.TenantID = types.StringValue("8")
	if err := api.readTenantNetworkBlock(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused tenant err = %v", err)
	}
	*data = original
	data.NetworkID = types.StringValue("11")
	if err := api.readTenantNetworkBlock(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused network err = %v", err)
	}
	*data = original
	data.CIDR = types.StringValue("10.1.0.0/24")
	if err := api.readTenantNetworkBlock(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("reused cidr err = %v", err)
	}
}

func TestReadTenantNetworkBlockNotFound(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	data.Id = types.StringValue("99")
	err := api.readTenantNetworkBlock(context.Background(), data)
	if !vergeos.IsNotFoundError(err) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateTenantNetworkBlockAppliesParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	data.ApplyParentFirewall = types.BoolValue(true)
	if err := api.updateTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
}

func TestUpdateTenantNetworkBlockWaitsForParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.fwSettleReads = 2
	fake.mu.Unlock()
	setParentFirewallWait(t, time.Millisecond, time.Second)
	before := fake.callCount(http.MethodGet, "/api/v4/vnets/10")
	data.ApplyParentFirewall = types.BoolValue(true)
	if err := api.updateTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
	if got := fake.callCount(http.MethodGet, "/api/v4/vnets/10") - before; got < 3 {
		t.Fatalf("vnet gets after apply = %d, want at least 3", got)
	}
	if err := api.readTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatal("read after the flag settled still reported parent_firewall_pending")
	}
}

func TestUpdateTenantNetworkBlockKeepsPendingWhenFlagStaysSet(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.fwSettleReads = 1000
	fake.mu.Unlock()
	setParentFirewallWait(t, time.Millisecond, 15*time.Millisecond)
	data.ApplyParentFirewall = types.BoolValue(true)
	start := time.Now()
	if err := api.updateTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("apply waited %s", time.Since(start))
	}
	if !data.ParentFirewallPending.ValueBool() || !data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
}

func TestUpdateTenantNetworkBlockDoesNotWaitWhenApplyIsFalse(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := api.updateTenantNetworkBlock(ctx, data); err != nil {
		t.Fatal(err)
	}
	if !data.ParentFirewallPending.ValueBool() || data.ParentFirewallApplied.ValueBool() {
		t.Fatalf("firewall status pending=%v applied=%v", data.ParentFirewallPending, data.ParentFirewallApplied)
	}
}

func TestDeleteTenantNetworkBlock(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	status, err := api.deleteTenantNetworkBlock(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if status == nil || !status.Pending || status.Applied {
		t.Fatalf("delete status = %#v", status)
	}
	if err := api.readTenantNetworkBlock(context.Background(), data); !vergeos.IsNotFoundError(err) {
		t.Fatalf("block still readable: %v", err)
	}
	status, err = api.deleteTenantNetworkBlock(context.Background(), data)
	if err != nil || status != nil {
		t.Fatalf("second delete status=%#v err=%v", status, err)
	}
}

func TestDeleteTenantNetworkBlockAppliesParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", true)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.vnetActions = nil
	fake.mu.Unlock()
	status, err := api.deleteTenantNetworkBlock(context.Background(), data)
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

func TestDeleteTenantNetworkBlockWaitsForParentFirewall(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	fake.fwSettleReads = 2
	fake.vnetActions = nil
	fake.mu.Unlock()
	setParentFirewallWait(t, time.Millisecond, time.Second)
	data.ApplyParentFirewall = types.BoolValue(true)
	before := fake.callCount(http.MethodGet, "/api/v4/vnets/10")
	status, err := api.deleteTenantNetworkBlock(context.Background(), data)
	if err != nil {
		t.Fatal(err)
	}
	if status == nil || status.Pending || !status.Applied {
		t.Fatalf("delete status = %#v", status)
	}
	if got := fake.callCount(http.MethodGet, "/api/v4/vnets/10") - before; got < 3 {
		t.Fatalf("vnet gets after delete apply = %d, want at least 3", got)
	}
}

func TestDeleteTenantNetworkBlockInUse(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.0/24", false)
	if err := api.createTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	const platform = "Cannot delete network block while a network is still using this block"
	fake.mu.Lock()
	fake.cidrDeleteStatus = http.StatusConflict
	fake.cidrDeleteMessage = platform
	fake.mu.Unlock()
	status, err := api.deleteTenantNetworkBlock(context.Background(), data)
	if err == nil || status != nil {
		t.Fatalf("in-use delete status=%#v err=%v", status, err)
	}
	if vergeos.IsNotFoundError(err) {
		t.Fatalf("in-use delete was treated as not found: %v", err)
	}
	if !strings.Contains(err.Error(), platform) {
		t.Fatalf("platform error was rewritten: %v", err)
	}
	if err := api.readTenantNetworkBlock(context.Background(), data); err != nil {
		t.Fatalf("block was removed despite the platform error: %v", err)
	}
}

func TestNetworkBlockFirewallPendingWarning(t *testing.T) {
	pending := networkBlockModel("7", "10", "192.168.100.0/24", false)
	pending.ParentFirewallPending = types.BoolValue(true)
	summary, detail := networkBlockFirewallPendingWarning(false, *pending)
	if summary == "" || !containsAll(t, detail, "need_fw_apply", "10") {
		t.Fatalf("warning = %q %q", summary, detail)
	}
	if summary, detail = networkBlockFirewallPendingWarning(true, *pending); summary == "" || !containsAll(t, detail, "still set") {
		t.Fatalf("applied warning = %q %q", summary, detail)
	}
	clear := *pending
	clear.ParentFirewallPending = types.BoolValue(false)
	if summary, detail = networkBlockFirewallPendingWarning(false, clear); summary != "" || detail != "" {
		t.Fatalf("unexpected warning %q %q", summary, detail)
	}
}

func TestCreateTenantNetworkBlockRejectsHostBits(t *testing.T) {
	fake := newFake(t)
	api := fake.api(t)
	data := networkBlockModel("7", "10", "192.168.100.1/24", false)
	err := api.createTenantNetworkBlock(context.Background(), data)
	if err == nil {
		t.Fatal("host bits were accepted")
	}
	if len(fake.bodiesFor(http.MethodPost, "/api/v4/vnet_cidrs")) != 0 {
		t.Fatal("invalid cidr was sent")
	}
}

func networkBlockModel(tenantID, networkID, cidr string, apply bool) *TenantNetworkBlockResourceModel {
	return &TenantNetworkBlockResourceModel{
		TenantID:            types.StringValue(tenantID),
		NetworkID:           types.StringValue(networkID),
		CIDR:                types.StringValue(cidr),
		ApplyParentFirewall: types.BoolValue(apply),
	}
}
