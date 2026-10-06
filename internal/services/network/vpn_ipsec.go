// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

type ipsecModel struct {
	ID                        types.String `tfsdk:"id"`
	NetworkID                 types.String `tfsdk:"network_id"`
	Enabled                   types.Bool   `tfsdk:"enabled"`
	Mode                      types.String `tfsdk:"mode"`
	UniqueIDs                 types.String `tfsdk:"uniqueids"`
	Compress                  types.Bool   `tfsdk:"compress"`
	ExcludeNetwork            types.Bool   `tfsdk:"exclude_network"`
	StrongswanConf            types.String `tfsdk:"strongswan_conf"`
	IPSecConf                 types.String `tfsdk:"ipsec_conf"`
	IPSecSecrets              types.String `tfsdk:"ipsec_secrets"`
	CiscoUnity                types.Bool   `tfsdk:"cisco_unity"`
	AcceptUnencryptedMainMode types.Bool   `tfsdk:"accept_unencrypted_mainmode"`
	MSSClamp                  types.Int64  `tfsdk:"mss_clamp"`
	StrictCRLPolicy           types.String `tfsdk:"strictcrlpolicy"`
	MakeBeforeBreak           types.Bool   `tfsdk:"make_before_break"`
}

func (a *vpnAPI) createIPSec(ctx context.Context, data *ipsecModel) error {
	networkID, err := parsePositiveID(stringOrEmpty(data.NetworkID))
	if err != nil {
		return fmt.Errorf("network_id: %w", err)
	}
	req := &vergeos.VNetIPSecCreateRequest{VNet: networkID}
	req.Enabled = vergeio.KnownBool(data.Enabled)
	req.Mode = vergeio.KnownString(data.Mode)
	req.StrongswanConf = vergeio.KnownString(data.StrongswanConf)
	req.IPSecConf = vergeio.KnownString(data.IPSecConf)
	req.IPSecSecrets = vergeio.KnownString(data.IPSecSecrets)
	req.UniqueIDs = vergeio.KnownString(data.UniqueIDs)
	req.Compress = vergeio.KnownBool(data.Compress)
	req.ExcludeNetwork = vergeio.KnownBool(data.ExcludeNetwork)

	created, err := a.sdk.VNetIPSecs.Create(ctx, req)
	if err != nil {
		return err
	}
	id := created.Key.Int()
	data.ID = typesStringID(id)
	tflog.Debug(ctx, fmt.Sprintf("Created IPsec configuration %s on network %d", data.ID.ValueString(), networkID))

	if ipsecCharonConfigured(data) {
		if _, err := a.sdk.VNetIPSecs.Update(ctx, id, ipsecUpdateRequest(data)); err != nil {
			return dropCreatedIPSec(ctx, a.sdk, id, fmt.Errorf("advanced settings were not saved: %w", err))
		}
	}
	if err := a.readIPSec(ctx, data); err != nil {
		return dropCreatedIPSec(ctx, a.sdk, id, err)
	}
	return nil
}

// dropCreatedIPSec deletes an IPsec row that create wrote but did not store
// in state. The next apply would create another row, and network destroy
// stays blocked while this one remains.
func dropCreatedIPSec(ctx context.Context, sdk *vergeos.Client, id int, cause error) error {
	if err := sdk.VNetIPSecs.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return fmt.Errorf("IPsec configuration %d was created but not stored in state: %w (delete failed: %v)", id, cause, err)
	}
	return fmt.Errorf("IPsec configuration %d was created but not stored in state, and the row was removed: %w", id, cause)
}

func (a *vpnAPI) readIPSec(ctx context.Context, data *ipsecModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("ipsec id: %w", err)
	}
	row, err := a.sdk.VNetIPSecs.Get(ctx, id)
	if err != nil {
		return err
	}
	if want, ok, err := optionalPositiveID(data.NetworkID); err != nil {
		return err
	} else if ok && row.VNet.Int() != want {
		return &vergeos.NotFoundError{Resource: "VNetIPSec", ID: id}
	}
	// Get does not return the advanced files or several charon settings.
	// Keep the configured values so refresh does not clear them.
	secrets := data.IPSecSecrets
	conf := data.IPSecConf
	strong := data.StrongswanConf
	cisco := data.CiscoUnity
	accept := data.AcceptUnencryptedMainMode
	mss := data.MSSClamp
	makeBeforeBreak := data.MakeBeforeBreak

	data.ID = typesStringID(row.Key.Int())
	data.NetworkID = typesStringID(row.VNet.Int())
	data.Enabled = types.BoolValue(row.Enabled)
	data.Mode = blankString(row.Mode)
	data.UniqueIDs = blankString(row.UniqueIDs)
	data.Compress = types.BoolValue(row.Compress)
	data.ExcludeNetwork = types.BoolValue(row.ExcludeNetwork)
	data.StrictCRLPolicy = blankString(row.StrictCRLPolicy)
	data.IPSecSecrets = secrets
	data.IPSecConf = conf
	data.StrongswanConf = strong
	data.CiscoUnity = cisco
	data.AcceptUnencryptedMainMode = accept
	data.MSSClamp = mss
	data.MakeBeforeBreak = makeBeforeBreak
	return nil
}

func (a *vpnAPI) updateIPSec(ctx context.Context, data *ipsecModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("ipsec id: %w", err)
	}
	if _, err := a.sdk.VNetIPSecs.Update(ctx, id, ipsecUpdateRequest(data)); err != nil {
		return err
	}
	return a.readIPSec(ctx, data)
}

// deleteIPSec refuses while a phase 1 row remains. The connection resource
// deletes phase 2 before phase 1. Doing that here could hit the stuck trigger.
func (a *vpnAPI) deleteIPSec(ctx context.Context, data *ipsecModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("ipsec id: %w", err)
	}
	phase1s, err := a.sdk.VNetIPSecPhase1s.ListByIPSec(ctx, id)
	if err != nil {
		return fmt.Errorf("list IPsec phase 1 for configuration %d: %w", id, err)
	}
	if len(phase1s) > 0 {
		names := make([]string, 0, len(phase1s))
		for _, phase1 := range phase1s {
			names = append(names, fmt.Sprintf("%d (%s)", phase1.Key.Int(), phase1.Name))
		}
		return fmt.Errorf("IPsec configuration %d still has phase 1 %s. Delete vergeio_network_ipsec_connection first so phase 2 is removed before phase 1. %s", id, strings.Join(names, ", "), vpnDeleteOrder)
	}
	if err := a.sdk.VNetIPSecs.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	return nil
}

func ipsecUpdateRequest(data *ipsecModel) *vergeos.VNetIPSecUpdateRequest {
	return &vergeos.VNetIPSecUpdateRequest{
		Enabled:                   vergeio.KnownBool(data.Enabled),
		Mode:                      vergeio.KnownString(data.Mode),
		StrongswanConf:            vergeio.KnownString(data.StrongswanConf),
		IPSecConf:                 vergeio.KnownString(data.IPSecConf),
		IPSecSecrets:              vergeio.KnownString(data.IPSecSecrets),
		UniqueIDs:                 vergeio.KnownString(data.UniqueIDs),
		Compress:                  vergeio.KnownBool(data.Compress),
		ExcludeNetwork:            vergeio.KnownBool(data.ExcludeNetwork),
		CiscoUnity:                vergeio.KnownBool(data.CiscoUnity),
		AcceptUnencryptedMainMode: vergeio.KnownBool(data.AcceptUnencryptedMainMode),
		MSSClamp:                  optionalInt(data.MSSClamp),
		StrictCRLPolicy:           vergeio.KnownString(data.StrictCRLPolicy),
		MakeBeforeBreak:           vergeio.KnownBool(data.MakeBeforeBreak),
	}
}

func ipsecCharonConfigured(data *ipsecModel) bool {
	if data == nil {
		return false
	}
	return vergeio.KnownBool(data.CiscoUnity) != nil ||
		vergeio.KnownBool(data.AcceptUnencryptedMainMode) != nil ||
		optionalInt(data.MSSClamp) != nil ||
		vergeio.KnownString(data.StrictCRLPolicy) != nil ||
		vergeio.KnownBool(data.MakeBeforeBreak) != nil
}
