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

type wireGuardModel struct {
	ID                types.String `tfsdk:"id"`
	NetworkID         types.String `tfsdk:"network_id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	IP                types.String `tfsdk:"ip"`
	ListenPort        types.Int64  `tfsdk:"listen_port"`
	MTU               types.Int64  `tfsdk:"mtu"`
	PrivateKey        types.String `tfsdk:"private_key"`
	EndpointIP        types.String `tfsdk:"endpoint_ip"`
	ConfigureFirewall types.Bool   `tfsdk:"configure_firewall"`
	ExternalIP        types.String `tfsdk:"external_ip"`
	Apply             types.Bool   `tfsdk:"apply"`
	PublicKey         types.String `tfsdk:"public_key"`
}

type wireGuardPeerModel struct {
	ID                types.String `tfsdk:"id"`
	WireGuardID       types.String `tfsdk:"wireguard_id"`
	Name              types.String `tfsdk:"name"`
	Description       types.String `tfsdk:"description"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	Endpoint          types.String `tfsdk:"endpoint"`
	Port              types.Int64  `tfsdk:"port"`
	PeerIP            types.String `tfsdk:"peer_ip"`
	PublicKey         types.String `tfsdk:"public_key"`
	PresharedKey      types.String `tfsdk:"preshared_key"`
	AllowedIPs        types.String `tfsdk:"allowed_ips"`
	ConfigureFirewall types.String `tfsdk:"configure_firewall"`
	Keepalive         types.Int64  `tfsdk:"keepalive"`
	AutogeneratePeer  types.Bool   `tfsdk:"autogenerate_peer"`
	Apply             types.Bool   `tfsdk:"apply"`
	PeerConfig        types.String `tfsdk:"peer_config"`
}

func (a *vpnAPI) createWireGuard(ctx context.Context, data *wireGuardModel) (*firewallNotice, error) {
	req, err := wireGuardCreateRequest(data)
	if err != nil {
		return nil, err
	}
	created, err := a.sdk.VNetWireGuards.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	id := created.Key.Int()
	data.ID = typesStringID(id)
	tflog.Debug(ctx, fmt.Sprintf("Created WireGuard interface %s on network %d", data.ID.ValueString(), req.VNet))
	if err := a.readWireGuard(ctx, data); err != nil {
		return nil, dropCreatedWireGuard(ctx, a.sdk, id, err)
	}
	notice, err := a.applyStagedFirewall(ctx, req.VNet, applyEnabled(data.Apply))
	if err != nil {
		return nil, dropCreatedWireGuard(ctx, a.sdk, id, fmt.Errorf("firewall rules were not applied: %w", err))
	}
	return notice, nil
}

// dropCreatedWireGuard deletes an interface that create wrote but did not
// store in state. Delete disables the interface and applies a running
// network first, which is what VergeOS requires. A stopped network is
// deleted without that apply.
func dropCreatedWireGuard(ctx context.Context, sdk *vergeos.Client, id int, cause error) error {
	if err := deleteWireGuardInterface(ctx, sdk, id); err != nil {
		return fmt.Errorf("WireGuard interface %d was created but not stored in state: %w (%v)", id, cause, err)
	}
	return fmt.Errorf("WireGuard interface %d was created but not stored in state, and the row was removed: %w", id, cause)
}

func (a *vpnAPI) readWireGuard(ctx context.Context, data *wireGuardModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("wireguard id: %w", err)
	}
	row, err := a.sdk.VNetWireGuards.Get(ctx, id)
	if err != nil {
		return err
	}
	if want, ok, err := optionalPositiveID(data.NetworkID); err != nil {
		return err
	} else if ok && row.VNet.Int() != want {
		return &vergeos.NotFoundError{Resource: "VNetWireGuard", ID: id}
	}
	privateKey := data.PrivateKey
	configure := data.ConfigureFirewall
	externalIP := data.ExternalIP
	apply := data.Apply
	data.ID = typesStringID(row.Key.Int())
	data.NetworkID = typesStringID(row.VNet.Int())
	data.Name = types.StringValue(row.Name)
	data.Description = blankString(row.Description)
	data.Enabled = types.BoolValue(row.Enabled)
	data.IP = types.StringValue(row.IP)
	data.ListenPort = int64Value(row.ListenPort)
	data.MTU = int64Value(row.MTU)
	data.PrivateKey = privateKey
	data.EndpointIP = blankString(row.EndpointIP)
	data.ConfigureFirewall = configure
	data.ExternalIP = externalIP
	data.Apply = normalizeApply(apply)
	data.PublicKey = blankString(row.PublicKey)
	return nil
}

func (a *vpnAPI) updateWireGuard(ctx context.Context, data *wireGuardModel) (*firewallNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return nil, fmt.Errorf("wireguard id: %w", err)
	}
	if _, err := a.sdk.VNetWireGuards.Update(ctx, id, wireGuardUpdateRequest(data)); err != nil {
		return nil, err
	}
	if err := a.readWireGuard(ctx, data); err != nil {
		return nil, err
	}
	networkID, err := parsePositiveID(stringOrEmpty(data.NetworkID))
	if err != nil {
		return nil, err
	}
	return a.applyStagedFirewall(ctx, networkID, applyEnabled(data.Apply))
}

// deleteWireGuard refuses while a peer remains. VergeOS rejects a delete
// while the interface is enabled or the disable has not landed on the
// running router, so the interface is disabled and that same network is
// applied first. The delete is retried until the apply has landed. A
// stopped network is deleted without that apply. A later apply clears a
// removed Accept WireGuard rule that the delete leaves staged on a
// running network. A stopped network skips that apply too.
func (a *vpnAPI) deleteWireGuard(ctx context.Context, data *wireGuardModel) (*firewallNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return nil, fmt.Errorf("wireguard id: %w", err)
	}
	peers, err := a.sdk.VNetWireGuardPeers.ListByWireGuard(ctx, id)
	if err != nil && !vergeos.IsNotFoundError(err) {
		if _, getErr := a.sdk.VNetWireGuards.Get(ctx, id); getErr != nil && vergeos.IsNotFoundError(getErr) {
			return a.applyWireGuardNetwork(ctx, data)
		}
		return nil, fmt.Errorf("list WireGuard peers for interface %d: %w", id, err)
	}
	if len(peers) > 0 {
		names := make([]string, 0, len(peers))
		for _, peer := range peers {
			names = append(names, fmt.Sprintf("%d (%s)", peer.Key.Int(), peer.Name))
		}
		return nil, fmt.Errorf("WireGuard interface %d still has peer %s. Delete vergeio_network_wireguard_peer first. %s", id, strings.Join(names, ", "), vpnDeleteOrder)
	}
	row, err := a.sdk.VNetWireGuards.Get(ctx, id)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return a.applyWireGuardNetwork(ctx, data)
		}
		return nil, err
	}
	if err := deleteWireGuardInterface(ctx, a.sdk, id); err != nil {
		return nil, err
	}
	networkID := row.VNet.Int()
	if networkID <= 0 {
		parsed, parseErr := parsePositiveID(stringOrEmpty(data.NetworkID))
		if parseErr != nil {
			return nil, fmt.Errorf("network_id: %w", parseErr)
		}
		networkID = parsed
	}
	return a.applyStagedFirewall(ctx, networkID, applyEnabled(data.Apply))
}

func (a *vpnAPI) applyWireGuardNetwork(ctx context.Context, data *wireGuardModel) (*firewallNotice, error) {
	networkID, err := parsePositiveID(stringOrEmpty(data.NetworkID))
	if err != nil {
		return nil, fmt.Errorf("network_id: %w", err)
	}
	return a.applyStagedFirewall(ctx, networkID, applyEnabled(data.Apply))
}

func wireGuardCreateRequest(data *wireGuardModel) (*vergeos.VNetWireGuardCreateRequest, error) {
	networkID, err := parsePositiveID(stringOrEmpty(data.NetworkID))
	if err != nil {
		return nil, fmt.Errorf("network_id: %w", err)
	}
	name := strings.TrimSpace(stringOrEmpty(data.Name))
	ip := strings.TrimSpace(stringOrEmpty(data.IP))
	if name == "" || ip == "" {
		return nil, fmt.Errorf("name and ip are required")
	}
	req := &vergeos.VNetWireGuardCreateRequest{
		VNet: networkID,
		Name: name,
		IP:   ip,
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	req.Enabled = vergeio.KnownBool(data.Enabled)
	req.ListenPort = optionalInt(data.ListenPort)
	req.MTU = optionalInt(data.MTU)
	if key := vergeio.KnownString(data.PrivateKey); key != nil {
		req.PrivateKey = *key
	}
	if endpoint := vergeio.KnownString(data.EndpointIP); endpoint != nil {
		req.EndpointIP = *endpoint
	}
	req.ConfigureFirewall = vergeio.KnownBool(data.ConfigureFirewall)
	if externalID, ok, err := optionalPositiveID(data.ExternalIP); err != nil {
		return nil, fmt.Errorf("external_ip: %w", err)
	} else if ok {
		req.ExternalIP = &externalID
	}
	apply := applyEnabled(data.Apply)
	req.AutoApplyFirewall = &apply
	return req, nil
}

func wireGuardUpdateRequest(data *wireGuardModel) *vergeos.VNetWireGuardUpdateRequest {
	// configure_firewall and external_ip exist on the create request only.
	// VNetWireGuardUpdateRequest has no fields for them. A change replaces
	// the interface instead of sending an update the API would ignore.
	req := &vergeos.VNetWireGuardUpdateRequest{
		Name:        vergeio.KnownString(data.Name),
		Description: vergeio.KnownString(data.Description),
		Enabled:     vergeio.KnownBool(data.Enabled),
		IP:          vergeio.KnownString(data.IP),
		ListenPort:  optionalInt(data.ListenPort),
		MTU:         optionalInt(data.MTU),
		PrivateKey:  vergeio.KnownString(data.PrivateKey),
		EndpointIP:  vergeio.KnownString(data.EndpointIP),
	}
	return req
}

func (a *vpnAPI) createWireGuardPeer(ctx context.Context, data *wireGuardPeerModel) (*firewallNotice, error) {
	req, err := wireGuardPeerCreateRequest(data)
	if err != nil {
		return nil, err
	}
	created, err := a.sdk.VNetWireGuardPeers.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	id := created.Key.Int()
	data.ID = typesStringID(id)
	tflog.Debug(ctx, fmt.Sprintf("Created WireGuard peer %s on interface %d", data.ID.ValueString(), req.WireGuard))
	if err := a.readWireGuardPeer(ctx, data); err != nil {
		return nil, dropCreatedWireGuardPeer(ctx, a.sdk, id, err)
	}
	notice, err := a.applyPeerFirewall(ctx, req.WireGuard, applyEnabled(data.Apply))
	if err != nil {
		return nil, dropCreatedWireGuardPeer(ctx, a.sdk, id, fmt.Errorf("firewall rules were not applied: %w", err))
	}
	return notice, nil
}

func dropCreatedWireGuardPeer(ctx context.Context, sdk *vergeos.Client, id int, cause error) error {
	if err := sdk.VNetWireGuardPeers.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return fmt.Errorf("WireGuard peer %d was created but not stored in state: %w (delete failed: %v)", id, cause, err)
	}
	return fmt.Errorf("WireGuard peer %d was created but not stored in state, and the row was removed: %w", id, cause)
}

func (a *vpnAPI) readWireGuardPeer(ctx context.Context, data *wireGuardPeerModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("wireguard peer id: %w", err)
	}
	row, err := a.sdk.VNetWireGuardPeers.Get(ctx, id)
	if err != nil {
		return err
	}
	if want, ok, err := optionalPositiveID(data.WireGuardID); err != nil {
		return err
	} else if ok && row.WireGuard.Int() != want {
		return &vergeos.NotFoundError{Resource: "VNetWireGuardPeer", ID: id}
	}
	apply := data.Apply
	priorSecret := data.PresharedKey
	data.ID = typesStringID(row.Key.Int())
	data.WireGuardID = typesStringID(row.WireGuard.Int())
	data.Name = types.StringValue(row.Name)
	data.Description = blankString(row.Description)
	data.Enabled = types.BoolValue(row.Enabled)
	data.Endpoint = blankString(row.Endpoint)
	data.Port = int64Value(row.Port)
	data.PeerIP = types.StringValue(row.PeerIP)
	data.PublicKey = types.StringValue(row.PublicKey)
	data.PresharedKey = secretFromAPI(row.PresharedKey, priorSecret)
	data.AllowedIPs = types.StringValue(row.AllowedIPs)
	data.ConfigureFirewall = blankString(row.ConfigureFirewall)
	data.ConfigureFirewall = normalizePeerFirewall(data.ConfigureFirewall)
	data.Keepalive = int64Value(row.Keepalive)
	data.AutogeneratePeer = types.BoolValue(row.AutogeneratePeer)
	data.Apply = normalizeApply(apply)
	data.PeerConfig = types.StringNull()
	if row.AutogeneratePeer {
		config, err := a.sdk.VNetWireGuardPeers.GetConfig(ctx, id)
		if err != nil && !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("read WireGuard peer config %d: %w", id, err)
		}
		data.PeerConfig = blankString(config)
	}
	return nil
}

func (a *vpnAPI) updateWireGuardPeer(ctx context.Context, data *wireGuardPeerModel) (*firewallNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return nil, fmt.Errorf("wireguard peer id: %w", err)
	}
	if _, err := a.sdk.VNetWireGuardPeers.Update(ctx, id, wireGuardPeerUpdateRequest(data)); err != nil {
		return nil, err
	}
	if err := a.readWireGuardPeer(ctx, data); err != nil {
		return nil, err
	}
	wireguardID, err := parsePositiveID(stringOrEmpty(data.WireGuardID))
	if err != nil {
		return nil, err
	}
	return a.applyPeerFirewall(ctx, wireguardID, applyEnabled(data.Apply))
}

func (a *vpnAPI) deleteWireGuardPeer(ctx context.Context, data *wireGuardPeerModel) (*firewallNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return nil, fmt.Errorf("wireguard peer id: %w", err)
	}
	wireguardID, _, err := optionalPositiveID(data.WireGuardID)
	if err != nil {
		return nil, fmt.Errorf("wireguard_id: %w", err)
	}
	row, err := a.sdk.VNetWireGuardPeers.Get(ctx, id)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return a.applyPeerFirewall(ctx, wireguardID, applyEnabled(data.Apply))
		}
		return nil, err
	}
	if row.WireGuard.Int() > 0 {
		wireguardID = row.WireGuard.Int()
	}
	if err := a.sdk.VNetWireGuardPeers.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return nil, err
	}
	return a.applyPeerFirewall(ctx, wireguardID, applyEnabled(data.Apply))
}

func (a *vpnAPI) applyPeerFirewall(ctx context.Context, wireguardID int, apply bool) (*firewallNotice, error) {
	if wireguardID <= 0 {
		return nil, nil
	}
	iface, err := a.sdk.VNetWireGuards.Get(ctx, wireguardID)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return a.applyStagedFirewall(ctx, iface.VNet.Int(), apply)
}

func wireGuardPeerCreateRequest(data *wireGuardPeerModel) (*vergeos.VNetWireGuardPeerCreateRequest, error) {
	wireguardID, err := parsePositiveID(stringOrEmpty(data.WireGuardID))
	if err != nil {
		return nil, fmt.Errorf("wireguard_id: %w", err)
	}
	name := strings.TrimSpace(stringOrEmpty(data.Name))
	peerIP := strings.TrimSpace(stringOrEmpty(data.PeerIP))
	publicKey := strings.TrimSpace(stringOrEmpty(data.PublicKey))
	allowed := strings.TrimSpace(stringOrEmpty(data.AllowedIPs))
	if name == "" || peerIP == "" || publicKey == "" || allowed == "" {
		return nil, fmt.Errorf("name, peer_ip, public_key, and allowed_ips are required")
	}
	req := &vergeos.VNetWireGuardPeerCreateRequest{
		WireGuard:  wireguardID,
		Name:       name,
		PeerIP:     peerIP,
		PublicKey:  publicKey,
		AllowedIPs: allowed,
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	if endpoint := vergeio.KnownString(data.Endpoint); endpoint != nil {
		req.Endpoint = *endpoint
	}
	if secret := vergeio.KnownString(data.PresharedKey); secret != nil {
		req.PresharedKey = *secret
	}
	req.Enabled = vergeio.KnownBool(data.Enabled)
	req.Port = optionalInt(data.Port)
	req.ConfigureFirewall = vergeio.KnownString(data.ConfigureFirewall)
	req.Keepalive = optionalInt(data.Keepalive)
	req.AutogeneratePeer = vergeio.KnownBool(data.AutogeneratePeer)
	return req, nil
}

func wireGuardPeerUpdateRequest(data *wireGuardPeerModel) *vergeos.VNetWireGuardPeerUpdateRequest {
	return &vergeos.VNetWireGuardPeerUpdateRequest{
		Name:              vergeio.KnownString(data.Name),
		Description:       vergeio.KnownString(data.Description),
		Enabled:           vergeio.KnownBool(data.Enabled),
		Endpoint:          vergeio.KnownString(data.Endpoint),
		Port:              optionalInt(data.Port),
		PeerIP:            vergeio.KnownString(data.PeerIP),
		PublicKey:         vergeio.KnownString(data.PublicKey),
		PresharedKey:      vergeio.KnownString(data.PresharedKey),
		AllowedIPs:        vergeio.KnownString(data.AllowedIPs),
		ConfigureFirewall: vergeio.KnownString(data.ConfigureFirewall),
		Keepalive:         optionalInt(data.Keepalive),
		AutogeneratePeer:  vergeio.KnownBool(data.AutogeneratePeer),
	}
}
