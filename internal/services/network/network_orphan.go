// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// adoptOrphanNetwork loads the existing network when a create hit a name
// collision and the network matches the planned configuration. The id is
// set before power-on so a failed power-on can still be stored in state.
func (r *NetworkResource) adoptOrphanNetwork(ctx context.Context, data *NetworkResourceModel) error {
	if r == nil || r.networkApi == nil || r.networkApi.sdk == nil {
		return fmt.Errorf("network API client is not configured")
	}
	if data == nil || data.Name.IsNull() || data.Name.IsUnknown() {
		return fmt.Errorf("network name is empty")
	}
	name := strings.TrimSpace(data.Name.ValueString())
	existing, err := r.networkApi.sdk.Networks.GetByName(ctx, name)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return fmt.Errorf("network %q is already in use, but no network with that name was found. Delete the conflicting network in VergeOS, or import it with `terraform import vergeio_network.<name> <id>`", name)
		}
		return fmt.Errorf("network %q is already in use, and looking it up failed: %w. Delete it in VergeOS, or import it with `terraform import vergeio_network.<name> <id>`", name, err)
	}
	if ok, reason := networkPlanMatches(data, existing); !ok {
		id := existing.Key.Int()
		return fmt.Errorf("network %q already exists with id %d and was not stored because %s. Import it with `terraform import vergeio_network.<name> %d`, or delete it in VergeOS and apply again", name, id, reason, id)
	}

	id := existing.Key.Int()
	if _, on := networkPowerRequested(data.PowerState); on && !existing.Running {
		if err := r.networkApi.sdk.Networks.PowerOn(ctx, id); err != nil {
			return fmt.Errorf("network %q already exists with id %d and was not stored because it did not power on: %v. Import it with `terraform import vergeio_network.<name> %d`, or delete it in VergeOS and apply again", name, id, err, id)
		}
	}
	data.Id = types.StringValue(strconv.Itoa(id))
	tflog.Info(ctx, fmt.Sprintf("Adopting existing network %s (id %s) after a name conflict", name, data.Id.ValueString()))
	return nil
}

func adoptedNetworkNotStored(data *NetworkResourceModel, cause error) string {
	name := ""
	id := ""
	if data != nil {
		if !data.Name.IsNull() && !data.Name.IsUnknown() {
			name = data.Name.ValueString()
		}
		if networkIDSet(data) {
			id = data.Id.ValueString()
		}
	}
	return fmt.Sprintf("network %q already exists with id %s and was not stored because reading it failed: %v. Import it with `terraform import vergeio_network.<name> %s`, or delete it in VergeOS and apply again", name, id, cause, id)
}

// networkPlanMatches reports whether an existing network is the one this
// create would have made. Only attributes the plan sets are compared.
// A stopped network still matches when the plan powers it on; create turns
// it on after the key is stored. A running network does not match a plan
// that says powerstate = false, because create does not power one off.
func networkPlanMatches(plan *NetworkResourceModel, existing *vergeos.Network) (bool, string) {
	if plan == nil || existing == nil {
		return false, "the network was not found"
	}
	var diffs []string
	diffNetworkString("name", plan.Name, existing.Name, &diffs)
	diffNetworkString("description", plan.Description, existing.Description, &diffs)
	diffNetworkBool("enabled", plan.Enabled, existing.Enabled, &diffs)
	diffNetworkString("ipaddress", plan.IPaddress, existing.IPAddress, &diffs)
	diffNetworkString("network", plan.Network, existing.Network, &diffs)
	diffNetworkInt32("vnet_default_gateway", plan.Default_Gateway, existing.DefaultGateway.Int(), &diffs)
	diffNetworkString("ipaddress_type", plan.IPaddress_Type, existing.IPAddressType, &diffs)
	diffNetworkString("dnslist", plan.DNSList, existing.DNSList, &diffs)
	diffNetworkString("domain", plan.Domain, existing.Domain, &diffs)
	diffNetworkBool("dhcp_enabled", plan.DHCP, existing.DHCPEnabled, &diffs)
	diffNetworkBool("dynamic_dhcp", plan.Dynamic_DHCP, existing.DHCPDynamic, &diffs)
	diffNetworkBool("dhcp_sequential", plan.DHCP_Sequential, existing.DHCPSequential, &diffs)
	diffNetworkString("dhcp_start", plan.DynamicIP_Start, existing.DHCPStart, &diffs)
	diffNetworkString("dhcp_stop", plan.DynamicIP_Stop, existing.DHCPStop, &diffs)
	diffNetworkString("on_power_loss", plan.On_Power_Loss, existing.OnPowerLoss, &diffs)
	diffNetworkString("type", plan.Type, existing.Type, &diffs)
	diffNetworkInt32("layer2_id", plan.VLAN_TAG, existing.VLAN, &diffs)
	diffNetworkInt32("mtu", plan.MTU, existing.MTU, &diffs)
	diffNetworkInt64("rate_limit", plan.RateLimit, existing.RateLimit, &diffs)
	diffNetworkInt32("interface_vnet", plan.Interface_Vnet, existing.InterfaceVnet.Int(), &diffs)
	diffNetworkString("layer2_type", plan.Layer2_Type, existing.Layer2Type, &diffs)
	diffNetworkBool("enable_bonding", plan.Enable_Bonding, existing.EnableBonding, &diffs)
	bonds, comparable := configuredBondIDs(plan.Bond_Interfaces_Args)
	if !comparable {
		return false, "bond_interfaces_args cannot be compared"
	}
	if bonds != nil && !bondIDsEqual(bonds, existing.BondInterfacesArgs) {
		diffs = append(diffs, fmt.Sprintf("bond_interfaces_args is %v, configuration is %v", existing.BondInterfacesArgs, bonds))
	}
	if !plan.PowerState.IsNull() && !plan.PowerState.IsUnknown() && !plan.PowerState.ValueBool() && existing.Running {
		diffs = append(diffs, "powerstate is true, configuration is false")
	}
	if len(diffs) == 0 {
		return true, ""
	}
	return false, strings.Join(diffs, "; ")
}

// configuredBondIDs reads a configured bond member list.
// A null or unknown list is not configured. comparable is false when a
// member is missing or not an int32, and that network must not be adopted.
func configuredBondIDs(list types.List) ([]int, bool) {
	if list.IsNull() || list.IsUnknown() {
		return nil, true
	}
	elems := list.Elements()
	out := make([]int, len(elems))
	for i, elem := range elems {
		id, ok := elem.(types.Int32)
		if !ok || id.IsNull() || id.IsUnknown() {
			return nil, false
		}
		out[i] = int(id.ValueInt32())
	}
	return out, true
}

func bondIDsEqual(planned []int, got []int) bool {
	if len(planned) != len(got) {
		return false
	}
	for i := range planned {
		if planned[i] != got[i] {
			return false
		}
	}
	return true
}

func diffNetworkString(attr string, planned types.String, got string, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if planned.ValueString() == got {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("%s is %q, configuration is %q", attr, got, planned.ValueString()))
}

func diffNetworkBool(attr string, planned types.Bool, got bool, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if planned.ValueBool() == got {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("%s is %t, configuration is %t", attr, got, planned.ValueBool()))
}

func diffNetworkInt32(attr string, planned types.Int32, got int, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if int(planned.ValueInt32()) == got {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("%s is %d, configuration is %d", attr, got, planned.ValueInt32()))
}

func diffNetworkInt64(attr string, planned types.Int64, got int64, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if planned.ValueInt64() == got {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("%s is %d, configuration is %d", attr, got, planned.ValueInt64()))
}

func networkIDSet(data *NetworkResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && strings.TrimSpace(data.Id.ValueString()) != ""
}

// keepNetwork writes the network into the create response as soon as its
// key is known. Terraform keeps that state when a later step returns an
// error, so the next apply updates this network instead of creating another
// one with the same name.
func (r *NetworkResource) keepNetwork(ctx context.Context, resp *resource.CreateResponse, data *NetworkResourceModel) {
	if resp == nil || !networkIDSet(data) {
		return
	}
	stored := networkPartial(ctx, data)
	ctx = shared.WritableContext(ctx)
	resp.Diagnostics.Append(resp.State.Set(ctx, &stored)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
}

// networkPartial drops unknown plan values. State cannot store them.
// The original model is left intact so the follow-up read can still fill it.
func networkPartial(ctx context.Context, data *NetworkResourceModel) NetworkResourceModel {
	stored := *data
	stored.Id = concreteString(data.Id)
	stored.Name = concreteString(data.Name)
	stored.Description = concreteString(data.Description)
	stored.Enabled = concreteBool(data.Enabled)
	stored.Default_Gateway = concreteInt32(data.Default_Gateway)
	stored.IPaddress = concreteString(data.IPaddress)
	stored.Network = concreteString(data.Network)
	stored.DHCP = concreteBool(data.DHCP)
	stored.Dynamic_DHCP = concreteBool(data.Dynamic_DHCP)
	stored.DHCP_Sequential = concreteBool(data.DHCP_Sequential)
	stored.DynamicIP_Start = concreteString(data.DynamicIP_Start)
	stored.DynamicIP_Stop = concreteString(data.DynamicIP_Stop)
	stored.DNSList = concreteString(data.DNSList)
	stored.Domain = concreteString(data.Domain)
	stored.On_Power_Loss = concreteString(data.On_Power_Loss)
	stored.PowerState = concreteBool(data.PowerState)
	stored.RestartOnChange = concreteBool(data.RestartOnChange)
	stored.NeedRestart = concreteBool(data.NeedRestart)
	stored.Type = concreteString(data.Type)
	stored.VLAN_TAG = concreteInt32(data.VLAN_TAG)
	stored.MTU = concreteInt32(data.MTU)
	stored.RateLimit = concreteInt64(data.RateLimit)
	stored.Interface_Vnet = concreteInt32(data.Interface_Vnet)
	stored.IPaddress_Type = concreteString(data.IPaddress_Type)
	stored.Layer2_Type = concreteString(data.Layer2_Type)
	stored.Enable_Bonding = concreteBool(data.Enable_Bonding)
	if stored.Bond_Interfaces_Args.IsNull() || stored.Bond_Interfaces_Args.IsUnknown() || stored.Bond_Interfaces_Args.ElementType(ctx) == nil {
		stored.Bond_Interfaces_Args = types.ListNull(types.Int32Type)
	}
	return stored
}

func concreteString(v types.String) types.String {
	if v.IsNull() || v.IsUnknown() {
		return types.StringNull()
	}
	return v
}

func concreteBool(v types.Bool) types.Bool {
	if v.IsNull() || v.IsUnknown() {
		return types.BoolNull()
	}
	return v
}

func concreteInt32(v types.Int32) types.Int32 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int32Null()
	}
	return v
}

func concreteInt64(v types.Int64) types.Int64 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int64Null()
	}
	return v
}
