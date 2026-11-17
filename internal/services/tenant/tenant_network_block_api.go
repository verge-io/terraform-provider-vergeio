// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func tenantNetworkBlockCreateRequest(data *TenantNetworkBlockResourceModel) (*vergeos.TenantNetworkBlockCreateRequest, error) {
	tenantID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return nil, err
	}
	networkID, err := parseID(data.NetworkID, "network")
	if err != nil {
		return nil, err
	}
	cidr := strings.TrimSpace(data.CIDR.ValueString())
	if err := validateNetworkCIDR(cidr); err != nil {
		return nil, err
	}
	return &vergeos.TenantNetworkBlockCreateRequest{
		Tenant:      tenantID,
		VNet:        networkID,
		CIDR:        cidr,
		Description: optionalString(data.Description),
	}, nil
}

func assignNetworkBlockFirewall(data *TenantNetworkBlockResourceModel, status *vergeos.ParentFirewallStatus) {
	if status == nil {
		return
	}
	data.ParentFirewallPending = types.BoolValue(status.Pending)
	data.ParentFirewallApplied = types.BoolValue(status.Applied)
}

func assignTenantNetworkBlock(data *TenantNetworkBlockResourceModel, block *vergeos.TenantNetworkBlock) {
	data.Id = idString(block.Key.Int())
	if tenantID := block.TenantKey(); tenantID > 0 {
		data.TenantID = idString(tenantID)
	}
	if networkID := block.VNet.Int(); networkID > 0 {
		data.NetworkID = idString(networkID)
	}
	data.CIDR = storedCIDR(data.CIDR, block.CIDR)
	data.Description = blankToNull(block.Description)
}

// storedCIDR keeps a configured CIDR when it names the same network VergeOS
// returned, so an equivalent encoding does not plan a replacement.
func storedCIDR(configured types.String, apiCIDR string) types.String {
	apiCIDR = strings.TrimSpace(apiCIDR)
	if !configured.IsNull() && !configured.IsUnknown() {
		current := strings.TrimSpace(configured.ValueString())
		if current != "" && cidrsMatch(current, apiCIDR) {
			return types.StringValue(current)
		}
	}
	if apiCIDR == "" {
		return types.StringNull()
	}
	return types.StringValue(apiCIDR)
}

func cidrsMatch(a, b string) bool {
	_, left, errA := net.ParseCIDR(strings.TrimSpace(a))
	_, right, errB := net.ParseCIDR(strings.TrimSpace(b))
	if errA != nil || errB != nil {
		return strings.TrimSpace(a) == strings.TrimSpace(b)
	}
	return left.String() == right.String()
}

// ensureTenantNetworkBlockOwned returns NotFound when the stored key now
// points at another block. Skip each check when that attribute is unset (import).
func ensureTenantNetworkBlockOwned(data *TenantNetworkBlockResourceModel, block *vergeos.TenantNetworkBlock) error {
	if tenantID, ok, err := configuredPositiveID(data.TenantID); err != nil {
		return err
	} else if ok && block.TenantKey() != tenantID {
		return &vergeos.NotFoundError{Resource: "TenantNetworkBlock", ID: block.Key.Int()}
	}
	if networkID, ok, err := configuredPositiveID(data.NetworkID); err != nil {
		return err
	} else if ok && block.VNet.Int() != networkID {
		return &vergeos.NotFoundError{Resource: "TenantNetworkBlock", ID: block.Key.Int()}
	}
	if !data.CIDR.IsNull() && !data.CIDR.IsUnknown() {
		cidr := strings.TrimSpace(data.CIDR.ValueString())
		if cidr != "" && !cidrsMatch(cidr, block.CIDR) {
			return &vergeos.NotFoundError{Resource: "TenantNetworkBlock", ID: block.Key.Int()}
		}
	}
	return nil
}

func (a *API) createTenantNetworkBlock(ctx context.Context, data *TenantNetworkBlockResourceModel) error {
	req, err := tenantNetworkBlockCreateRequest(data)
	if err != nil {
		return err
	}
	created, status, err := a.sdk.TenantNetworkBlocks.Create(ctx, req, parentFirewallOpts(data.ApplyParentFirewall)...)
	if created != nil {
		assignTenantNetworkBlock(data, created)
		assignNetworkBlockFirewall(data, status)
		tflog.Debug(ctx, fmt.Sprintf("created tenant network block %d", created.Key.Int()))
	}
	if err != nil {
		return err
	}
	if waitErr := a.waitForParentFirewallClear(ctx, status); waitErr != nil {
		assignNetworkBlockFirewall(data, status)
		return waitErr
	}
	assignNetworkBlockFirewall(data, status)
	return nil
}

func (a *API) readTenantNetworkBlock(ctx context.Context, data *TenantNetworkBlockResourceModel) error {
	id, err := parseID(data.Id, "tenant network block")
	if err != nil {
		return err
	}
	block, err := a.sdk.TenantNetworkBlocks.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureTenantNetworkBlockOwned(data, block); err != nil {
		return err
	}
	priorApplied := data.ParentFirewallApplied
	priorApply := data.ApplyParentFirewall
	assignTenantNetworkBlock(data, block)
	pending, err := a.readParentFirewallPending(ctx, block.VNet.Int())
	if err != nil {
		return err
	}
	data.ParentFirewallPending = pending
	if priorApplied.IsNull() || priorApplied.IsUnknown() {
		data.ParentFirewallApplied = types.BoolValue(false)
	} else {
		data.ParentFirewallApplied = priorApplied
	}
	if priorApply.IsNull() || priorApply.IsUnknown() {
		data.ApplyParentFirewall = types.BoolValue(false)
	} else {
		data.ApplyParentFirewall = priorApply
	}
	return nil
}

func (a *API) updateTenantNetworkBlock(ctx context.Context, plan *TenantNetworkBlockResourceModel) error {
	networkID, err := parseID(plan.NetworkID, "network")
	if err != nil {
		return err
	}
	status, err := a.syncParentFirewall(ctx, networkID, applyParentFirewall(plan.ApplyParentFirewall))
	assignNetworkBlockFirewall(plan, status)
	if err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("updated tenant network block %s parent firewall", plan.Id.ValueString()))
	return nil
}

func (a *API) deleteTenantNetworkBlock(ctx context.Context, data *TenantNetworkBlockResourceModel) (*vergeos.ParentFirewallStatus, error) {
	id, err := parseID(data.Id, "tenant network block")
	if err != nil {
		return nil, err
	}
	apply := applyParentFirewall(data.ApplyParentFirewall)
	status, err := a.sdk.TenantNetworkBlocks.Delete(ctx, id, parentFirewallOpts(data.ApplyParentFirewall)...)
	if vergeos.IsNotFoundError(err) {
		if !apply {
			tflog.Debug(ctx, fmt.Sprintf("tenant network block %d already deleted", id))
			return nil, nil
		}
		networkID, parseErr := parseID(data.NetworkID, "network")
		if parseErr != nil {
			return nil, parseErr
		}
		status, err = a.syncParentFirewall(ctx, networkID, true)
		if err != nil {
			return status, err
		}
		tflog.Debug(ctx, fmt.Sprintf("tenant network block %d already deleted; applied parent firewall", id))
		return status, nil
	}
	// Delete failed before the row was removed. A tenant network built on
	// the block is one of those platform errors. Return it unchanged.
	if err != nil && status == nil {
		return nil, err
	}
	if err != nil {
		tflog.Warn(ctx, fmt.Sprintf("deleted tenant network block %d; firewall follow-up: %v", id, err))
		return status, err
	}
	if waitErr := a.waitForParentFirewallClear(ctx, status); waitErr != nil {
		return status, waitErr
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant network block %d", id))
	return status, nil
}
