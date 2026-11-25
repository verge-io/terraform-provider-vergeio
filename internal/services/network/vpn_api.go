// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"errors"
	"fmt"
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

// parentMissingError means the IPsec row is still readable and its parent is
// gone. It unwraps to NotFoundError so refresh removes the resource.
type parentMissingError struct {
	summary string
	detail  string
	missing error
}

func (e *parentMissingError) Error() string {
	if e == nil {
		return "parent is gone"
	}
	return e.detail
}

func (e *parentMissingError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.missing
}

func parentMissingDetail(err error) (string, string, bool) {
	var missing *parentMissingError
	if !errors.As(err, &missing) || missing == nil || missing.detail == "" {
		return "", "", false
	}
	return missing.summary, missing.detail, true
}

func networkGoneError(networkID int, row string) error {
	return &parentMissingError{
		summary: "Parent network was deleted outside Terraform",
		detail: fmt.Sprintf(
			"Network %d is gone and %s is still on the system. VergeOS leaves these rows behind when the network is deleted outside Terraform, and it refuses to delete them until a network with key %d exists. Refresh removes this resource from state. The next apply creates it on the new network. Creating that network deletes IPsec rows that already point at the new key, so the recreated network does not keep the old configuration. terraform destroy -refresh=false cannot delete these rows until a network with that key exists.",
			networkID, row, networkID,
		),
		missing: &vergeos.NotFoundError{Resource: "Network", ID: networkID},
	}
}

func ipsecGoneError(ipsecID int, row string) error {
	return &parentMissingError{
		summary: "IPsec configuration was deleted outside Terraform",
		detail: fmt.Sprintf(
			"IPsec configuration %d is gone and %s is still on the system. VergeOS can leave the phase 1 and phase 2 rows behind. Refresh removes this resource from state. The next apply creates the connection again. Creating the IPsec configuration deletes phase 1 rows that already point at the new key, and creating the connection deletes phase 2 rows that already point at the new phase 1.",
			ipsecID, row,
		),
		missing: &vergeos.NotFoundError{Resource: "VNetIPSec", ID: ipsecID},
	}
}

// absentNetwork reports that the parent network is gone. Any other lookup
// result is ignored so a failed read of the network does not fail the read
// of a row that is still there.
func (a *vpnAPI) absentNetwork(ctx context.Context, networkID int, row string) error {
	if a == nil || a.sdk == nil || networkID <= 0 || strings.TrimSpace(row) == "" {
		return nil
	}
	_, err := a.sdk.Networks.Get(ctx, networkID)
	if !vergeos.IsNotFoundError(err) {
		return nil
	}
	return networkGoneError(networkID, row)
}

// absentIPSecParent reports that the IPsec configuration or its network is
// gone. A lookup that fails for any other reason is ignored.
func (a *vpnAPI) absentIPSecParent(ctx context.Context, ipsecID int, row string) error {
	if a == nil || a.sdk == nil || ipsecID <= 0 || strings.TrimSpace(row) == "" {
		return nil
	}
	ipsec, err := a.sdk.VNetIPSecs.Get(ctx, ipsecID)
	if vergeos.IsNotFoundError(err) {
		return ipsecGoneError(ipsecID, row)
	}
	if err != nil || ipsec == nil {
		return nil
	}
	return a.absentNetwork(ctx, ipsec.VNet.Int(), row)
}

// requireNetwork is the update check. A missing network is the same error
// refresh uses. Any other lookup error is returned.
func (a *vpnAPI) requireNetwork(ctx context.Context, networkID int, row string) error {
	if a == nil || a.sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	if networkID <= 0 {
		return fmt.Errorf("network id is required")
	}
	_, err := a.sdk.Networks.Get(ctx, networkID)
	if err == nil {
		return nil
	}
	if vergeos.IsNotFoundError(err) {
		return networkGoneError(networkID, row)
	}
	return err
}

// requireIPSecParent is the connection update check.
func (a *vpnAPI) requireIPSecParent(ctx context.Context, ipsecID int, row string) error {
	if a == nil || a.sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	if ipsecID <= 0 {
		return fmt.Errorf("ipsec id is required")
	}
	ipsec, err := a.sdk.VNetIPSecs.Get(ctx, ipsecID)
	if vergeos.IsNotFoundError(err) {
		return ipsecGoneError(ipsecID, row)
	}
	if err != nil {
		return err
	}
	if ipsec == nil {
		return fmt.Errorf("IPsec configuration %d read returned no row", ipsecID)
	}
	return a.requireNetwork(ctx, ipsec.VNet.Int(), row)
}

// explainIPSecDelete adds the recovery step when VergeOS refused a delete
// and the row is still there. Other errors are unchanged.
func explainIPSecDelete(err error) error {
	if err == nil || !strings.Contains(err.Error(), "VergeOS refused the delete") {
		return err
	}
	return fmt.Errorf("%w. Refresh removes this resource from state when the parent network or IPsec configuration is gone. The next apply creates it again. Creating the network deletes IPsec rows that already point at the new key, so a recreated network does not keep the old configuration. terraform destroy -refresh=false cannot delete these rows until a network with that key exists", err)
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
	networkID, err := parseID(data.Id, "network")
	if err != nil {
		return err
	}
	return refuseNetworkDelete(ctx, nc.sdk, networkID)
}
