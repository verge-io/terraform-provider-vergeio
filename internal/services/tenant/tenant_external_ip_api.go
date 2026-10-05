// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// parentFirewallSettleTimeout bounds how long a successful apply waits for
// need_fw_apply to clear. VergeOS accepts the refresh before the flag drops.
// parentFirewallPollInterval is the pause between those reads. Tests shorten both.
var (
	parentFirewallSettleTimeout = 60 * time.Second
	parentFirewallPollInterval  = time.Second
)

func tenantExternalIPCreateRequest(data *TenantExternalIPResourceModel) (*vergeos.TenantExternalIPCreateRequest, error) {
	tenantID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return nil, err
	}
	networkID, err := parseID(data.NetworkID, "network")
	if err != nil {
		return nil, err
	}
	ip := strings.TrimSpace(data.IP.ValueString())
	if net.ParseIP(ip) == nil {
		return nil, fmt.Errorf("ip must be a valid IP address")
	}
	return &vergeos.TenantExternalIPCreateRequest{
		Tenant:      tenantID,
		VNet:        networkID,
		IP:          ip,
		Hostname:    optionalString(data.Hostname),
		Description: optionalString(data.Description),
	}, nil
}

func optionalString(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return strings.TrimSpace(v.ValueString())
}

func blankToNull(v string) types.String {
	v = strings.TrimSpace(v)
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

func parentFirewallOpts(apply types.Bool) []vergeos.ParentFirewallOption {
	if apply.IsNull() || apply.IsUnknown() || !apply.ValueBool() {
		return nil
	}
	return []vergeos.ParentFirewallOption{vergeos.WithApplyParentFirewall()}
}

func applyParentFirewall(apply types.Bool) bool {
	return !apply.IsNull() && !apply.IsUnknown() && apply.ValueBool()
}

// syncParentFirewall matches govergeos parentFirewallFollowUp for an update,
// which has no TenantExternalIPService method of its own. Create and delete
// pass WithApplyParentFirewall into the service instead.
func (a *API) syncParentFirewall(ctx context.Context, networkID int, apply bool) (*vergeos.ParentFirewallStatus, error) {
	status := &vergeos.ParentFirewallStatus{NetworkID: networkID}
	if networkID <= 0 {
		return status, nil
	}
	var applyErr error
	if apply {
		applyErr = a.sdk.Networks.ApplyRules(ctx, networkID)
		if applyErr == nil {
			status.Applied = true
		}
	}
	network, err := a.sdk.Networks.Get(ctx, networkID)
	if err != nil {
		if applyErr != nil {
			return status, fmt.Errorf("failed to apply rules to network %d: %w", networkID, applyErr)
		}
		return status, fmt.Errorf("failed to read firewall status for network %d: %w", networkID, err)
	}
	status.Pending = network.NeedFWApply
	if applyErr != nil {
		return status, fmt.Errorf("failed to apply rules to network %d: %w", networkID, applyErr)
	}
	if err := a.waitForParentFirewallClear(ctx, status); err != nil {
		return status, err
	}
	return status, nil
}

// waitForParentFirewallClear polls need_fw_apply after this call applied the
// parent rules. govergeos reads the flag once, and that read can still be
// true after ApplyRules succeeds. A call that did not apply returns
// immediately so a legitimately pending flag is not held until the timeout.
func (a *API) waitForParentFirewallClear(ctx context.Context, status *vergeos.ParentFirewallStatus) error {
	if status == nil || !status.Applied || !status.Pending || status.NetworkID <= 0 {
		return nil
	}
	deadline := time.Now().Add(parentFirewallSettleTimeout)
	for status.Pending {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			tflog.Warn(ctx, fmt.Sprintf("parent network %d still has need_fw_apply set after applying firewall rules", status.NetworkID))
			return nil
		}
		wait := parentFirewallPollInterval
		if wait > remaining {
			wait = remaining
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
		network, err := a.sdk.Networks.Get(ctx, status.NetworkID)
		if err != nil {
			return fmt.Errorf("failed to read firewall status for network %d: %w", status.NetworkID, err)
		}
		status.Pending = network.NeedFWApply
	}
	return nil
}

func assignFirewallStatus(data *TenantExternalIPResourceModel, status *vergeos.ParentFirewallStatus) {
	if status == nil {
		return
	}
	data.ParentFirewallPending = types.BoolValue(status.Pending)
	data.ParentFirewallApplied = types.BoolValue(status.Applied)
}

func assignTenantExternalIP(data *TenantExternalIPResourceModel, address *vergeos.TenantExternalIP) {
	data.Id = idString(address.Key.Int())
	if tenantID := address.TenantKey(); tenantID > 0 {
		data.TenantID = idString(tenantID)
	}
	if networkID := address.VNet.Int(); networkID > 0 {
		data.NetworkID = idString(networkID)
	}
	data.IP = types.StringValue(address.IP)
	data.Hostname = blankToNull(address.Hostname)
	data.Description = blankToNull(address.Description)
}

// ensureTenantExternalIPOwned returns NotFound when the stored key now points
// at another address. Skip each check when that attribute is unset (import).
func ensureTenantExternalIPOwned(data *TenantExternalIPResourceModel, address *vergeos.TenantExternalIP) error {
	if tenantID, ok, err := configuredPositiveID(data.TenantID); err != nil {
		return err
	} else if ok && address.TenantKey() != tenantID {
		return &vergeos.NotFoundError{Resource: "TenantExternalIP", ID: address.Key.Int()}
	}
	if networkID, ok, err := configuredPositiveID(data.NetworkID); err != nil {
		return err
	} else if ok && address.VNet.Int() != networkID {
		return &vergeos.NotFoundError{Resource: "TenantExternalIP", ID: address.Key.Int()}
	}
	if !data.IP.IsNull() && !data.IP.IsUnknown() {
		ip := strings.TrimSpace(data.IP.ValueString())
		if ip != "" && address.IP != ip {
			return &vergeos.NotFoundError{Resource: "TenantExternalIP", ID: address.Key.Int()}
		}
	}
	return nil
}

func configuredPositiveID(v types.String) (int, bool, error) {
	if v.IsNull() || v.IsUnknown() || strings.TrimSpace(v.ValueString()) == "" {
		return 0, false, nil
	}
	n, err := parseID(v, "id")
	if err != nil {
		return 0, false, err
	}
	return n, true, nil
}

func (a *API) createTenantExternalIP(ctx context.Context, data *TenantExternalIPResourceModel) error {
	req, err := tenantExternalIPCreateRequest(data)
	if err != nil {
		return err
	}
	created, status, err := a.sdk.TenantExternalIPs.Create(ctx, req, parentFirewallOpts(data.ApplyParentFirewall)...)
	if created != nil {
		assignTenantExternalIP(data, created)
		assignFirewallStatus(data, status)
		tflog.Debug(ctx, fmt.Sprintf("created tenant external IP %d", created.Key.Int()))
	}
	if err != nil {
		return err
	}
	if waitErr := a.waitForParentFirewallClear(ctx, status); waitErr != nil {
		assignFirewallStatus(data, status)
		return waitErr
	}
	assignFirewallStatus(data, status)
	return nil
}

func (a *API) readTenantExternalIP(ctx context.Context, data *TenantExternalIPResourceModel) error {
	id, err := parseID(data.Id, "tenant external IP")
	if err != nil {
		return err
	}
	address, err := a.sdk.TenantExternalIPs.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureTenantExternalIPOwned(data, address); err != nil {
		return err
	}
	priorApplied := data.ParentFirewallApplied
	priorApply := data.ApplyParentFirewall
	assignTenantExternalIP(data, address)
	pending, err := a.readParentFirewallPending(ctx, address.VNet.Int())
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

func (a *API) readParentFirewallPending(ctx context.Context, networkID int) (types.Bool, error) {
	if networkID <= 0 {
		return types.BoolValue(false), nil
	}
	network, err := a.sdk.Networks.Get(ctx, networkID)
	if err != nil {
		return types.BoolNull(), err
	}
	return types.BoolValue(network.NeedFWApply), nil
}

func (a *API) updateTenantExternalIP(ctx context.Context, plan *TenantExternalIPResourceModel) error {
	networkID, err := parseID(plan.NetworkID, "network")
	if err != nil {
		return err
	}
	status, err := a.syncParentFirewall(ctx, networkID, applyParentFirewall(plan.ApplyParentFirewall))
	assignFirewallStatus(plan, status)
	if err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("updated tenant external IP %s parent firewall", plan.Id.ValueString()))
	return nil
}

func (a *API) deleteTenantExternalIP(ctx context.Context, data *TenantExternalIPResourceModel) (*vergeos.ParentFirewallStatus, error) {
	id, err := parseID(data.Id, "tenant external IP")
	if err != nil {
		return nil, err
	}
	apply := applyParentFirewall(data.ApplyParentFirewall)
	status, err := a.sdk.TenantExternalIPs.Delete(ctx, id, parentFirewallOpts(data.ApplyParentFirewall)...)
	if vergeos.IsNotFoundError(err) {
		if !apply {
			tflog.Debug(ctx, fmt.Sprintf("tenant external IP %d already deleted", id))
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
		tflog.Debug(ctx, fmt.Sprintf("tenant external IP %d already deleted; applied parent firewall", id))
		return status, nil
	}
	if err != nil && status == nil {
		return nil, err
	}
	if err != nil {
		// The address is already gone. Surface the firewall error to the
		// resource so it can warn without leaving a deleted row in state.
		tflog.Warn(ctx, fmt.Sprintf("deleted tenant external IP %d; firewall follow-up: %v", id, err))
		return status, err
	}
	if waitErr := a.waitForParentFirewallClear(ctx, status); waitErr != nil {
		return status, waitErr
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant external IP %d", id))
	return status, nil
}
