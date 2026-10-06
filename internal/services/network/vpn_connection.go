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

type ipsecConnectionModel struct {
	ID               types.String      `tfsdk:"id"`
	IPSecID          types.String      `tfsdk:"ipsec_id"`
	Name             types.String      `tfsdk:"name"`
	Description      types.String      `tfsdk:"description"`
	Enabled          types.Bool        `tfsdk:"enabled"`
	KeyExchange      types.String      `tfsdk:"keyexchange"`
	RemoteGateway    types.String      `tfsdk:"remote_gateway"`
	Auth             types.String      `tfsdk:"auth"`
	Negotiation      types.String      `tfsdk:"negotiation"`
	Identifier       types.String      `tfsdk:"identifier"`
	PeerIdentifier   types.String      `tfsdk:"peer_identifier"`
	PSK              types.String      `tfsdk:"psk"`
	IKE              types.String      `tfsdk:"ike"`
	IKELifetime      types.Int64       `tfsdk:"ikelifetime"`
	Auto             types.String      `tfsdk:"auto"`
	MOBIKE           types.Bool        `tfsdk:"mobike"`
	SplitConnections types.Bool        `tfsdk:"split_connections"`
	ForceEncaps      types.Bool        `tfsdk:"forceencaps"`
	KeyingTries      types.Int64       `tfsdk:"keyingtries"`
	Rekey            types.Bool        `tfsdk:"rekey"`
	Reauth           types.Bool        `tfsdk:"reauth"`
	MarginTime       types.Int64       `tfsdk:"margintime"`
	DPDAction        types.String      `tfsdk:"dpdaction"`
	DPDDelay         types.Int64       `tfsdk:"dpddelay"`
	DPDFailures      types.Int64       `tfsdk:"dpdfailures"`
	Phase2           *ipsecPhase2Model `tfsdk:"phase2"`
}

type ipsecPhase2Model struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Mode        types.String `tfsdk:"mode"`
	Local       types.String `tfsdk:"local"`
	Remote      types.String `tfsdk:"remote"`
	Lifetime    types.Int64  `tfsdk:"lifetime"`
	Protocol    types.String `tfsdk:"protocol"`
	Ciphers     types.String `tfsdk:"ciphers"`
}

func (a *vpnAPI) createIPSecConnection(ctx context.Context, data *ipsecConnectionModel) error {
	req, err := phase1CreateRequest(data)
	if err != nil {
		return err
	}
	phase2Req, err := phase2CreateRequest(0, data.Phase2)
	if err != nil {
		return err
	}
	phase1, err := a.sdk.VNetIPSecPhase1s.Create(ctx, req)
	if err != nil {
		return err
	}
	phase2Req.Phase1 = phase1.Key.Int()
	phase2, err := a.sdk.VNetIPSecPhase2s.Create(ctx, phase2Req)
	if err != nil {
		// Create can fail after the phase 2 row is stored. Remove phase 2
		// rows first, then the phase 1. A phase 1 delete while a phase 2
		// remains is the failure that sticks.
		if cleanupErr := deletePhase1AfterPhase2(ctx, a.sdk, phase1.Key.Int()); cleanupErr != nil {
			return fmt.Errorf("create IPsec phase 2: %w (phase 1 %d was left in place: %v)", err, phase1.Key.Int(), cleanupErr)
		}
		return fmt.Errorf("create IPsec phase 2: %w", err)
	}
	data.ID = typesStringID(phase1.Key.Int())
	if data.Phase2 == nil {
		data.Phase2 = &ipsecPhase2Model{}
	}
	data.Phase2.ID = typesStringID(phase2.Key.Int())
	tflog.Debug(ctx, fmt.Sprintf("Created IPsec phase 1 %s and phase 2 %s", data.ID.ValueString(), data.Phase2.ID.ValueString()))
	return a.readIPSecConnection(ctx, data)
}

func (a *vpnAPI) readIPSecConnection(ctx context.Context, data *ipsecConnectionModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("ipsec connection id: %w", err)
	}
	phase1, err := a.sdk.VNetIPSecPhase1s.Get(ctx, id)
	if err != nil {
		return err
	}
	if want, ok, err := optionalPositiveID(data.IPSecID); err != nil {
		return err
	} else if ok && phase1.IPSec.Int() != want {
		return &vergeos.NotFoundError{Resource: "VNetIPSecPhase1", ID: id}
	}
	phase2s, err := a.sdk.VNetIPSecPhase2s.ListByPhase1(ctx, id)
	if err != nil {
		return fmt.Errorf("list IPsec phase 2 for phase 1 %d: %w", id, err)
	}
	phase2, err := selectPhase2(data.Phase2, phase2s)
	if err != nil {
		return err
	}
	psk := data.PSK
	assignPhase1(data, phase1)
	data.PSK = psk
	assignPhase2(data, phase2)
	a.noteIPSecStatus(ctx, id)
	return nil
}

func (a *vpnAPI) updateIPSecConnection(ctx context.Context, data *ipsecConnectionModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("ipsec connection id: %w", err)
	}
	if _, err := a.sdk.VNetIPSecPhase1s.Update(ctx, id, phase1UpdateRequest(data)); err != nil {
		return err
	}
	if data.Phase2 == nil {
		return fmt.Errorf("phase2 is required")
	}
	phase2ID, _, err := optionalPositiveID(data.Phase2.ID)
	if err != nil {
		return fmt.Errorf("phase2.id: %w", err)
	}
	if phase2ID == 0 {
		req, err := phase2CreateRequest(id, data.Phase2)
		if err != nil {
			return err
		}
		created, err := a.sdk.VNetIPSecPhase2s.Create(ctx, req)
		if err != nil {
			return err
		}
		data.Phase2.ID = typesStringID(created.Key.Int())
	} else if _, err := a.sdk.VNetIPSecPhase2s.Update(ctx, phase2ID, phase2UpdateRequest(data.Phase2)); err != nil {
		return err
	}
	return a.readIPSecConnection(ctx, data)
}

// deleteIPSecConnection deletes phase 2, then phase 1. Extra phase 2 rows
// under the same phase 1 are deleted too. Leaving one would make the phase 1
// delete fail from then on.
func (a *vpnAPI) deleteIPSecConnection(ctx context.Context, data *ipsecConnectionModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("ipsec connection id: %w", err)
	}
	return deletePhase1AfterPhase2(ctx, a.sdk, id)
}

func selectPhase2(want *ipsecPhase2Model, rows []vergeos.VNetIPSecPhase2) (*vergeos.VNetIPSecPhase2, error) {
	if len(rows) == 0 {
		return nil, fmt.Errorf("IPsec phase 1 has no phase 2")
	}
	if want != nil {
		if id, ok, err := optionalPositiveID(want.ID); err != nil {
			return nil, err
		} else if ok {
			for i := range rows {
				if rows[i].Key.Int() == id {
					return &rows[i], nil
				}
			}
			return nil, fmt.Errorf("IPsec phase 2 %d was not found under this phase 1", id)
		}
		name := ""
		if !want.Name.IsNull() && !want.Name.IsUnknown() {
			name = strings.TrimSpace(want.Name.ValueString())
		}
		if name != "" {
			var match *vergeos.VNetIPSecPhase2
			for i := range rows {
				if rows[i].Name == name {
					if match != nil {
						return nil, fmt.Errorf("IPsec phase 1 has more than one phase 2 named %q", name)
					}
					match = &rows[i]
				}
			}
			if match != nil {
				return match, nil
			}
		}
	}
	if len(rows) == 1 {
		return &rows[0], nil
	}
	return nil, fmt.Errorf("IPsec phase 1 has %d phase 2 rows. This resource manages one phase 2", len(rows))
}

func assignPhase1(data *ipsecConnectionModel, row *vergeos.VNetIPSecPhase1) {
	data.ID = typesStringID(row.Key.Int())
	data.IPSecID = typesStringID(row.IPSec.Int())
	data.Name = types.StringValue(row.Name)
	data.Description = blankString(row.Description)
	data.Enabled = types.BoolValue(row.Enabled)
	data.KeyExchange = blankString(row.KeyExchange)
	data.RemoteGateway = types.StringValue(row.RemoteGateway)
	data.Auth = blankString(row.Auth)
	data.Negotiation = blankString(row.Negotiation)
	data.Identifier = blankString(row.Identifier)
	data.PeerIdentifier = blankString(row.PeerIdentifier)
	data.IKE = blankString(row.IKE)
	data.IKELifetime = int64Value(row.IKELifetime)
	data.Auto = blankString(row.Auto)
	data.MOBIKE = types.BoolValue(row.MOBIKE)
	data.SplitConnections = types.BoolValue(row.SplitConnections)
	data.ForceEncaps = types.BoolValue(row.ForceEncaps)
	data.KeyingTries = int64Value(row.KeyingTries)
	data.Rekey = types.BoolValue(row.Rekey)
	data.Reauth = types.BoolValue(row.Reauth)
	data.MarginTime = int64Value(row.MarginTime)
	data.DPDAction = blankString(row.DPDAction)
	data.DPDDelay = int64Value(row.DPDDelay)
	data.DPDFailures = int64Value(row.DPDFailures)
}

func assignPhase2(data *ipsecConnectionModel, row *vergeos.VNetIPSecPhase2) {
	if data.Phase2 == nil {
		data.Phase2 = &ipsecPhase2Model{}
	}
	data.Phase2.ID = typesStringID(row.Key.Int())
	data.Phase2.Name = types.StringValue(row.Name)
	data.Phase2.Description = blankString(row.Description)
	data.Phase2.Enabled = types.BoolValue(row.Enabled)
	data.Phase2.Mode = blankString(row.Mode)
	data.Phase2.Local = types.StringValue(row.Local)
	data.Phase2.Remote = blankString(row.Remote)
	data.Phase2.Lifetime = int64Value(row.Lifetime)
	data.Phase2.Protocol = blankString(row.Protocol)
	data.Phase2.Ciphers = blankString(row.Ciphers)
}

func phase1CreateRequest(data *ipsecConnectionModel) (*vergeos.VNetIPSecPhase1CreateRequest, error) {
	if data == nil {
		return nil, fmt.Errorf("ipsec connection is required")
	}
	ipsecID, err := parsePositiveID(stringOrEmpty(data.IPSecID))
	if err != nil {
		return nil, fmt.Errorf("ipsec_id: %w", err)
	}
	name := strings.TrimSpace(stringOrEmpty(data.Name))
	gateway := strings.TrimSpace(stringOrEmpty(data.RemoteGateway))
	if name == "" || gateway == "" {
		return nil, fmt.Errorf("name and remote_gateway are required")
	}
	req := &vergeos.VNetIPSecPhase1CreateRequest{
		IPSec:         ipsecID,
		Name:          name,
		RemoteGateway: gateway,
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	req.Enabled = vergeio.KnownBool(data.Enabled)
	req.KeyExchange = vergeio.KnownString(data.KeyExchange)
	req.Auth = vergeio.KnownString(data.Auth)
	req.Negotiation = vergeio.KnownString(data.Negotiation)
	req.Identifier = vergeio.KnownString(data.Identifier)
	req.PeerIdentifier = vergeio.KnownString(data.PeerIdentifier)
	req.PSK = vergeio.KnownString(data.PSK)
	req.IKE = vergeio.KnownString(data.IKE)
	req.IKELifetime = optionalInt(data.IKELifetime)
	req.Auto = vergeio.KnownString(data.Auto)
	req.MOBIKE = vergeio.KnownBool(data.MOBIKE)
	req.SplitConnections = vergeio.KnownBool(data.SplitConnections)
	req.ForceEncaps = vergeio.KnownBool(data.ForceEncaps)
	req.KeyingTries = optionalInt(data.KeyingTries)
	req.Rekey = vergeio.KnownBool(data.Rekey)
	req.Reauth = vergeio.KnownBool(data.Reauth)
	req.MarginTime = optionalInt(data.MarginTime)
	req.DPDAction = vergeio.KnownString(data.DPDAction)
	req.DPDDelay = optionalInt(data.DPDDelay)
	req.DPDFailures = optionalInt(data.DPDFailures)
	return req, nil
}

func phase1UpdateRequest(data *ipsecConnectionModel) *vergeos.VNetIPSecPhase1UpdateRequest {
	req := &vergeos.VNetIPSecPhase1UpdateRequest{
		Name:             vergeio.KnownString(data.Name),
		Description:      vergeio.KnownString(data.Description),
		Enabled:          vergeio.KnownBool(data.Enabled),
		KeyExchange:      vergeio.KnownString(data.KeyExchange),
		RemoteGateway:    vergeio.KnownString(data.RemoteGateway),
		Auth:             vergeio.KnownString(data.Auth),
		Negotiation:      vergeio.KnownString(data.Negotiation),
		Identifier:       vergeio.KnownString(data.Identifier),
		PeerIdentifier:   vergeio.KnownString(data.PeerIdentifier),
		PSK:              vergeio.KnownString(data.PSK),
		IKE:              vergeio.KnownString(data.IKE),
		IKELifetime:      optionalInt(data.IKELifetime),
		Auto:             vergeio.KnownString(data.Auto),
		MOBIKE:           vergeio.KnownBool(data.MOBIKE),
		SplitConnections: vergeio.KnownBool(data.SplitConnections),
		ForceEncaps:      vergeio.KnownBool(data.ForceEncaps),
		KeyingTries:      optionalInt(data.KeyingTries),
		Rekey:            vergeio.KnownBool(data.Rekey),
		Reauth:           vergeio.KnownBool(data.Reauth),
		MarginTime:       optionalInt(data.MarginTime),
		DPDAction:        vergeio.KnownString(data.DPDAction),
		DPDDelay:         optionalInt(data.DPDDelay),
		DPDFailures:      optionalInt(data.DPDFailures),
	}
	return req
}

func phase2CreateRequest(phase1ID int, data *ipsecPhase2Model) (*vergeos.VNetIPSecPhase2CreateRequest, error) {
	if data == nil {
		return nil, fmt.Errorf("phase2 is required")
	}
	name := strings.TrimSpace(stringOrEmpty(data.Name))
	local := strings.TrimSpace(stringOrEmpty(data.Local))
	if name == "" || local == "" {
		return nil, fmt.Errorf("phase2 name and local are required")
	}
	req := &vergeos.VNetIPSecPhase2CreateRequest{
		Phase1: phase1ID,
		Name:   name,
		Local:  local,
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	if remote := vergeio.KnownString(data.Remote); remote != nil {
		req.Remote = *remote
	}
	req.Enabled = vergeio.KnownBool(data.Enabled)
	req.Mode = vergeio.KnownString(data.Mode)
	req.Lifetime = optionalInt(data.Lifetime)
	req.Protocol = vergeio.KnownString(data.Protocol)
	req.Ciphers = vergeio.KnownString(data.Ciphers)
	return req, nil
}

func phase2UpdateRequest(data *ipsecPhase2Model) *vergeos.VNetIPSecPhase2UpdateRequest {
	return &vergeos.VNetIPSecPhase2UpdateRequest{
		Name:        vergeio.KnownString(data.Name),
		Description: vergeio.KnownString(data.Description),
		Enabled:     vergeio.KnownBool(data.Enabled),
		Mode:        vergeio.KnownString(data.Mode),
		Local:       vergeio.KnownString(data.Local),
		Remote:      vergeio.KnownString(data.Remote),
		Lifetime:    optionalInt(data.Lifetime),
		Protocol:    vergeio.KnownString(data.Protocol),
		Ciphers:     vergeio.KnownString(data.Ciphers),
	}
}
