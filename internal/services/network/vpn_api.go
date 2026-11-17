// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// vpnAPI is the govergeos client for IPsec and WireGuard.
type vpnAPI struct {
	sdk *vergeos.Client
}

func newVPNAPI(c *vergeio.Client) (*vpnAPI, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &vpnAPI{sdk: sdk}, nil
}

func optionalInt(v types.Int64) *int {
	n := vergeio.KnownInt64(v)
	if n == nil {
		return nil
	}
	i := int(*n)
	return &i
}

func blankString(v string) types.String {
	v = strings.TrimSpace(v)
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

func int64Value(v int) types.Int64 {
	return types.Int64Value(int64(v))
}

// secretFromAPI keeps a secret the API hides. A non-empty API value wins.
// An empty response keeps the value already stored.
func secretFromAPI(api string, prior types.String) types.String {
	if strings.TrimSpace(api) != "" {
		return types.StringValue(api)
	}
	if !prior.IsNull() && !prior.IsUnknown() && strings.TrimSpace(prior.ValueString()) != "" {
		return prior
	}
	return types.StringNull()
}

func optionalPositiveID(v types.String) (int, bool, error) {
	if v.IsNull() || v.IsUnknown() {
		return 0, false, nil
	}
	text := strings.TrimSpace(v.ValueString())
	if text == "" {
		return 0, false, nil
	}
	id, err := parsePositiveID(text)
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

func normalizeApply(v types.Bool) types.Bool {
	if v.IsNull() || v.IsUnknown() {
		return types.BoolValue(true)
	}
	return v
}

func normalizePeerFirewall(v types.String) types.String {
	if v.IsNull() || v.IsUnknown() || strings.TrimSpace(v.ValueString()) == "" {
		return types.StringValue(vergeos.WireGuardPeerFirewallSiteToSite)
	}
	return v
}

// applyStagedFirewall refreshes a running network after WireGuard stages
// rules. The platform adds "Accept WireGuard" and leaves need_fw_apply set.
// wrote is always true: the caller just changed an interface or a peer.
func (a *vpnAPI) applyStagedFirewall(ctx context.Context, networkID int, apply bool) (*firewallNotice, error) {
	if networkID <= 0 {
		return nil, fmt.Errorf("network id is required to apply WireGuard firewall rules")
	}
	network, err := a.sdk.Networks.Get(ctx, networkID)
	if err != nil {
		return nil, err
	}
	switch decideFirewallApply(true, apply, networkIsRunning(network), network.NeedFWApply) {
	case firewallApplyNow:
		tflog.Debug(ctx, fmt.Sprintf("Applying firewall rules on network %d after a WireGuard change", networkID))
		if err := a.sdk.Networks.ApplyRules(ctx, networkID); err != nil {
			return nil, fmt.Errorf("apply firewall rules on network %d after a WireGuard change: %w", networkID, err)
		}
		return nil, nil
	case firewallApplySkippedStopped:
		tflog.Warn(ctx, fmt.Sprintf("Network %d is not running; WireGuard firewall rules stay staged until it starts", networkID))
		return stoppedFirewallNotice(network, networkID), nil
	case firewallApplySkippedDisabled:
		tflog.Warn(ctx, fmt.Sprintf("Network %d firewall apply is disabled; Accept WireGuard rules stay staged", networkID))
		return &firewallNotice{
			Summary: "WireGuard firewall rules were staged",
			Detail: fmt.Sprintf(
				"Network %s has staged firewall rules from WireGuard, including Accept WireGuard. apply is false, so Terraform did not apply them. The tunnel stays half configured until a later apply.",
				networkLabel(network, networkID),
			),
		}, nil
	default:
		return nil, nil
	}
}

// noteIPSecStatus reads vnet_ipsec_connections. The rows are live security
// associations, not configuration, so they are not stored. A status error
// does not fail the tunnel read.
func (a *vpnAPI) noteIPSecStatus(ctx context.Context, phase1ID int) {
	if phase1ID <= 0 {
		return
	}
	conns, err := a.sdk.VNetIPSecConnections.ListByPhase1(ctx, phase1ID)
	if err != nil {
		tflog.Warn(ctx, fmt.Sprintf("IPsec connection status for phase 1 %d was not read: %v", phase1ID, err))
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("IPsec phase 1 %d has %d active security association(s)", phase1ID, len(conns)))
}

func stringState() []planmodifier.String {
	return []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
}

func boolState() []planmodifier.Bool {
	return []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}
}

func intState() []planmodifier.Int64 {
	return []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}
}

func (nc *NetworkApi) refuseNetworkDelete(ctx context.Context, data *NetworkResourceModel) error {
	if nc == nil || nc.sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	if data == nil {
		return fmt.Errorf("missing network")
	}
	networkID, err := strconv.Atoi(strings.TrimSpace(data.Id.ValueString()))
	if err != nil || networkID <= 0 {
		return fmt.Errorf("invalid network ID format: %v", data.Id.ValueString())
	}
	return refuseNetworkDelete(ctx, nc.sdk, networkID)
}
