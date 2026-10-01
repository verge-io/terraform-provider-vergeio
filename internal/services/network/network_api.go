// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// Network endpoints - DEPRECATED: SDK handles endpoints internally
// Keeping temporarily for reference during transition

// IClient interface.
var _ vergeio.IClient = &NetworkApi{}

func NewNetworkApi(c *vergeio.Client) (*NetworkApi, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &NetworkApi{
		name:   "Network Api",
		client: c,
		sdk:    sdk,
	}, nil
}

type NetworkApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *NetworkApi) Name() string {
	return nc.name
}

// NetworkAPIResourceModel is the network create/update body.
// Pointer fields keep false, 0, and "" in the JSON. A nil pointer is omitted.
type NetworkAPIResourceModel struct {
	Id                   *string `json:"id,omitempty"`
	Name                 *string `json:"name,omitempty"`
	Description          *string `json:"description,omitempty"`
	Enabled              *bool   `json:"enabled,omitempty"`
	Default_Gateway      *int32  `json:"vnet_default_gateway,omitempty"`
	IPaddress            *string `json:"ipaddress,omitempty"`
	Network              *string `json:"network,omitempty"`
	DHCP                 *bool   `json:"dhcp_enabled,omitempty"`
	Dynamic_DHCP         *bool   `json:"dhcp_dynamic,omitempty"`
	DHCP_Sequential      *bool   `json:"dhcp_sequential,omitempty"`
	DynamicIP_Start      *string `json:"dhcp_start,omitempty"`
	DynamicIP_Stop       *string `json:"dhcp_stop,omitempty"`
	DNSList              *string `json:"dnslist,omitempty"`
	Domain               *string `json:"domain,omitempty"`
	On_Power_Loss        *string `json:"on_power_loss,omitempty"`
	Type                 *string `json:"type,omitempty"`
	VLAN_TAG             *int32  `json:"layer2_id,omitempty"`
	MTU                  *int32  `json:"mtu,omitempty"`
	RateLimit            *int64  `json:"rate_limit,omitempty"`
	Interface_Vnet       *int32  `json:"interface_vnet,omitempty"`
	IPaddress_Type       *string `json:"ipaddress_type,omitempty"`
	Layer2_Type          *string `json:"layer2_type,omitempty"`
	Enable_Bonding       *bool   `json:"enable_bonding,omitempty"`
	Bond_Interfaces_Args []int32 `json:"bond_interfaces_args,omitempty"`
}

type NetworkAPIDataSourceModel struct {
	Id          int32  `json:"$key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// createNetwork creates a new network.
// powerstate is not a field on the create body. A true value powers the
// network on after the row exists, and waits until the machine is running.
func (nc *NetworkApi) createNetwork(ctx context.Context, data *NetworkResourceModel) error {
	req, err := networkCreateRequest(data)
	if err != nil {
		return err
	}

	network, err := nc.sdk.Networks.Create(ctx, req)
	if err != nil {
		return err
	}

	data.Id = types.StringValue(fmt.Sprintf("%d", network.Key.Int()))
	tflog.Debug(ctx, fmt.Sprintf("Created a network with Id %v", data.Id.ValueString()))

	if _, on := networkPowerRequested(data.PowerState); on {
		tflog.Debug(ctx, fmt.Sprintf("Powering on network %s after create", data.Id.ValueString()))
		if err := nc.sdk.Networks.PowerOn(ctx, network.Key.Int()); err != nil {
			return fmt.Errorf("network %s was created but did not power on: %w", data.Id.ValueString(), err)
		}
	}

	return nil
}

// updateNetwork updates an existing network.
func (nc *NetworkApi) updateNetwork(ctx context.Context, planData *NetworkResourceModel, stateData *NetworkResourceModel) error {
	req, networkIDInt, err := networkUpdateRequest(planData, stateData)
	if err != nil {
		return err
	}

	// Networks.Update marshals NetworkUpdateRequest, which has no
	// ipaddress_type. A changed value is sent on its own PUT.
	if _, err := nc.sdk.Networks.Update(ctx, networkIDInt, &req.NetworkUpdateRequest); err != nil {
		return err
	}
	if req.IPAddressType != nil {
		if err := nc.putIPAddressType(ctx, networkIDInt, *req.IPAddressType); err != nil {
			return err
		}
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", req))

	// A powerstate change is not in the PUT. VergeOS ignores that field.
	return nc.applyPlannedPowerState(ctx, planData)
}

// networkCreateRequest builds the SDK create body.
// A value that is null or unknown is left unset. An explicit false, 0, or ""
// is sent.
func networkCreateRequest(data *NetworkResourceModel) (*vergeos.NetworkCreateRequest, error) {
	apiData := NetworkAPIResourceModel{
		Name:            vergeio.KnownString(data.Name),
		Description:     vergeio.KnownString(data.Description),
		Enabled:         vergeio.KnownBool(data.Enabled),
		Default_Gateway: vergeio.KnownInt32(data.Default_Gateway),
		IPaddress:       vergeio.KnownString(data.IPaddress),
		Network:         vergeio.KnownString(data.Network),
		DHCP:            vergeio.KnownBool(data.DHCP),
		Dynamic_DHCP:    vergeio.KnownBool(data.Dynamic_DHCP),
		DHCP_Sequential: vergeio.KnownBool(data.DHCP_Sequential),
		DynamicIP_Start: vergeio.KnownString(data.DynamicIP_Start),
		DynamicIP_Stop:  vergeio.KnownString(data.DynamicIP_Stop),
		DNSList:         vergeio.KnownString(data.DNSList),
		Domain:          vergeio.KnownString(data.Domain),
		On_Power_Loss:   vergeio.KnownString(data.On_Power_Loss),
		Type:            vergeio.KnownString(data.Type),
		VLAN_TAG:        vergeio.KnownInt32(data.VLAN_TAG),
		MTU:             vergeio.KnownInt32(data.MTU),
		RateLimit:       vergeio.KnownInt64(data.RateLimit),
		Interface_Vnet:  vergeio.KnownInt32(data.Interface_Vnet),
		IPaddress_Type:  vergeio.KnownString(data.IPaddress_Type),
		Layer2_Type:     vergeio.KnownString(data.Layer2_Type),
		Enable_Bonding:  vergeio.KnownBool(data.Enable_Bonding),
	}

	if !data.Bond_Interfaces_Args.IsNull() && !data.Bond_Interfaces_Args.IsUnknown() {
		for _, arg := range data.Bond_Interfaces_Args.Elements() {
			apiData.Bond_Interfaces_Args = append(apiData.Bond_Interfaces_Args, arg.(types.Int32).ValueInt32())
		}
	}

	return decodeNetworkRequest[vergeos.NetworkCreateRequest](apiData)
}

// networkUpdateBody is the change for an existing network.
// govergeos NetworkUpdateRequest has no ipaddress_type. Decoding the
// provider model into that type drops the field, so it is kept here.
// Networks.Update sends the embedded request. A set IPAddressType is
// sent on its own PUT.
type networkUpdateBody struct {
	vergeos.NetworkUpdateRequest
	IPAddressType *string `json:"ipaddress_type,omitempty"`
}

// networkUpdateRequest builds the update body from attributes that differ
// from state. Type is readonly and is not sent.
func networkUpdateRequest(planData *NetworkResourceModel, stateData *NetworkResourceModel) (*networkUpdateBody, int, error) {
	apiData := NetworkAPIResourceModel{
		Name:            vergeio.ChangedString(planData.Name, stateData.Name),
		Description:     vergeio.ChangedString(planData.Description, stateData.Description),
		Enabled:         vergeio.ChangedBool(planData.Enabled, stateData.Enabled),
		Default_Gateway: vergeio.ChangedInt32(planData.Default_Gateway, stateData.Default_Gateway),
		IPaddress:       vergeio.ChangedString(planData.IPaddress, stateData.IPaddress),
		Network:         vergeio.ChangedString(planData.Network, stateData.Network),
		DHCP:            vergeio.ChangedBool(planData.DHCP, stateData.DHCP),
		Dynamic_DHCP:    vergeio.ChangedBool(planData.Dynamic_DHCP, stateData.Dynamic_DHCP),
		DHCP_Sequential: vergeio.ChangedBool(planData.DHCP_Sequential, stateData.DHCP_Sequential),
		DynamicIP_Start: vergeio.ChangedString(planData.DynamicIP_Start, stateData.DynamicIP_Start),
		DynamicIP_Stop:  vergeio.ChangedString(planData.DynamicIP_Stop, stateData.DynamicIP_Stop),
		DNSList:         vergeio.ChangedString(planData.DNSList, stateData.DNSList),
		Domain:          vergeio.ChangedString(planData.Domain, stateData.Domain),
		On_Power_Loss:   vergeio.ChangedString(planData.On_Power_Loss, stateData.On_Power_Loss),
		VLAN_TAG:        vergeio.ChangedInt32(planData.VLAN_TAG, stateData.VLAN_TAG),
		MTU:             vergeio.ChangedInt32(planData.MTU, stateData.MTU),
		RateLimit:       vergeio.ChangedInt64(planData.RateLimit, stateData.RateLimit),
		Interface_Vnet:  vergeio.ChangedInt32(planData.Interface_Vnet, stateData.Interface_Vnet),
		IPaddress_Type:  vergeio.ChangedString(planData.IPaddress_Type, stateData.IPaddress_Type),
		Layer2_Type:     vergeio.ChangedString(planData.Layer2_Type, stateData.Layer2_Type),
		Enable_Bonding:  vergeio.ChangedBool(planData.Enable_Bonding, stateData.Enable_Bonding),
	}

	req, err := decodeNetworkRequest[vergeos.NetworkUpdateRequest](apiData)
	if err != nil {
		return nil, 0, err
	}

	id := planData.Id
	if id.IsNull() || id.IsUnknown() || id.ValueString() == "" {
		id = stateData.Id
	}
	networkIDInt, err := strconv.Atoi(id.ValueString())
	if err != nil {
		return nil, 0, fmt.Errorf("invalid network ID format: %v", err)
	}
	return &networkUpdateBody{
		NetworkUpdateRequest: *req,
		IPAddressType:        apiData.IPaddress_Type,
	}, networkIDInt, nil
}

// putIPAddressType sends ipaddress_type. NetworkUpdateRequest cannot.
func (nc *NetworkApi) putIPAddressType(ctx context.Context, id int, ipType string) error {
	if nc == nil || nc.client == nil {
		return errors.New("network API client is not configured")
	}
	raw, err := json.Marshal(struct {
		IPAddressType string `json:"ipaddress_type"`
	}{IPAddressType: ipType})
	if err != nil {
		return fmt.Errorf("failed to encode network ipaddress_type: %w", err)
	}
	resp, err := nc.client.Put(ctx, fmt.Sprintf("%s/vnets/%d", vergeio.APIEndpoint, id), bytes.NewBuffer(raw))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

func decodeNetworkRequest[T any](apiData NetworkAPIResourceModel) (*T, error) {
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return nil, errors.New("invalid format received for network Item")
	}
	var req T
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return nil, fmt.Errorf("failed to convert API data: %v", err)
	}
	return &req, nil
}

// deleteNetwork deletes a network.
func (nc *NetworkApi) deleteNetwork(ctx context.Context, data *NetworkResourceModel) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the Delete Network API for Network %v", data.Id.ValueString()))

	// Parse network ID
	networkIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid network ID format: %v", err)
	}

	// Call SDK API
	err = nc.sdk.Networks.Delete(ctx, networkIDInt)
	if err != nil {
		return err
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Deleted the network with the ID %v", data.Id))

	return nil
}

// describeNetworkPower is the word used in delete errors. The Terraform
// attribute is the API bool; these words match the earlier status strings.
func describeNetworkPower(on bool) string {
	if on {
		return "running"
	}
	return "stopped"
}

// Checks the power state of the network.
func (nc *NetworkApi) checkNetworkPowerState(ctx context.Context, data *NetworkResourceModel) error {

	// Parse network ID
	networkIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid network ID format: %v", err)
	}

	// Call SDK API with specific fields
	networks, err := nc.sdk.Networks.List(ctx, vergeos.WithFilter(fmt.Sprintf("$key eq %d", networkIDInt)))
	if err != nil {
		return err
	}

	if len(networks) == 0 {
		return errors.New("network not found")
	}

	network := networks[0]
	tflog.Debug(ctx, fmt.Sprintf("Read the network %v", network))

	data.PowerState = types.BoolValue(network.Running)

	tflog.Debug(ctx, fmt.Sprintf("Network status read from API is: %t", network.Running))

	return nil
}

// networkStopTimeout is how long Delete waits after one kill for the
// network to report stopped. The kill is not repeated. Tests shorten these.
var (
	networkStopTimeout  = 2 * time.Minute
	networkStopInterval = time.Second
)

// stopNetworkBeforeDelete kills a running network once, then polls until it
// is stopped or networkStopTimeout elapses. A network that is already
// stopped is left alone.
func (nc *NetworkApi) stopNetworkBeforeDelete(ctx context.Context, data *NetworkResourceModel) error {
	if data == nil {
		return errors.New("missing network")
	}
	if err := nc.checkNetworkPowerState(ctx, data); err != nil {
		return fmt.Errorf("failed to check power state before deletion: %w", err)
	}
	if !data.PowerState.ValueBool() {
		return nil
	}

	tflog.Debug(ctx, fmt.Sprintf("Current network power state is %s", describeNetworkPower(true)))
	if err := nc.killNetwork(ctx, data); err != nil {
		return fmt.Errorf("failed to kill network before deletion: %w", err)
	}

	deadline := time.Now().Add(networkStopTimeout)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := nc.checkNetworkPowerState(ctx, data); err != nil {
			return fmt.Errorf("failed to check power state before deletion: %w", err)
		}
		if !data.PowerState.ValueBool() {
			return nil
		}
		state := describeNetworkPower(true)
		if !time.Now().Before(deadline) {
			return fmt.Errorf("network %s stayed %q after stop and did not reach stopped before the timeout", networkStopLabel(data), state)
		}
		tflog.Debug(ctx, fmt.Sprintf("Network %s is still %s after stop", networkStopLabel(data), state))
		wait := networkStopInterval
		if networkStopInterval > 0 {
			if remaining := time.Until(deadline); remaining < wait {
				wait = remaining
			}
		}
		if err := sleepContext(ctx, wait); err != nil {
			return err
		}
	}
}

func networkStopLabel(data *NetworkResourceModel) string {
	name := ""
	id := ""
	if data != nil {
		if !data.Name.IsNull() && !data.Name.IsUnknown() {
			name = strings.TrimSpace(data.Name.ValueString())
		}
		if !data.Id.IsNull() && !data.Id.IsUnknown() {
			id = strings.TrimSpace(data.Id.ValueString())
		}
	}
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%q (id %s)", name, id)
	case name != "":
		return fmt.Sprintf("%q", name)
	case id != "":
		return fmt.Sprintf("id %s", id)
	default:
		return "unknown"
	}
}

// killNetwork powers a network off immediately.
// The vnets powerstate column is ignored by VergeOS, so this posts kill.
func (nc *NetworkApi) killNetwork(ctx context.Context, data *NetworkResourceModel) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill Network API for Network %v", data.Id.ValueString()))

	networkIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid Network ID format: %v", err)
	}

	return nc.sdk.Networks.Kill(ctx, networkIDInt)
}

// Read the Network (Vnet) from the API.
func (nc *NetworkApi) readNetwork(ctx context.Context, data *NetworkResourceModel) error {

	tflog.Debug(ctx, "Reading the network data")

	// Parse network ID
	networkIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid network ID format: %v", err)
	}

	// Call SDK API to get specific network
	network, err := nc.sdk.Networks.Get(ctx, networkIDInt)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the network %v", network))

	// Some fields are still stored as the historical defaults. Mapping them
	// from the SDK response is separate from sending explicit zero values.
	data.Name = types.StringValue(network.Name)
	data.Description = types.StringValue(network.Description)
	data.Enabled = types.BoolValue(network.Enabled)
	data.IPaddress = types.StringValue(network.IPAddress)
	data.Network = types.StringValue(network.Network)
	data.DHCP = types.BoolValue(network.DHCPEnabled)
	data.Dynamic_DHCP = types.BoolValue(network.DHCPDynamic)
	data.DHCP_Sequential = types.BoolValue(network.DHCPSequential)
	data.DynamicIP_Start = types.StringValue(network.DHCPStart)
	data.DynamicIP_Stop = types.StringValue(network.DHCPStop)
	data.DNSList = types.StringValue(network.DNSList)
	data.Domain = types.StringValue(network.Domain)
	data.On_Power_Loss = types.StringValue(network.OnPowerLoss)
	data.Type = types.StringValue(network.Type)
	data.VLAN_TAG = types.Int32Value(0)
	data.MTU = types.Int32Value(1500)
	data.RateLimit = types.Int64Value(network.RateLimit)
	data.Interface_Vnet = types.Int32Value(0)
	// An empty response keeps the historical default. A reported value, such
	// as none, is stored so apply does not write static over the plan.
	ipType := strings.TrimSpace(network.IPAddressType)
	if ipType == "" {
		ipType = "static"
	}
	data.IPaddress_Type = types.StringValue(ipType)
	data.Layer2_Type = types.StringValue("vlan")
	data.Enable_Bonding = types.BoolValue(false)
	data.PowerState = types.BoolValue(network.Running)
	data.NeedRestart = types.BoolValue(network.NeedRestart)
	// restart_on_change is not a VergeOS field. Keep an explicit setting and
	// fill the default when state has never stored one, such as after import.
	if data.RestartOnChange.IsNull() || data.RestartOnChange.IsUnknown() {
		data.RestartOnChange = types.BoolValue(true)
	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// Read the Networks from the API for data source.
func (va *NetworkApi) readNetworks(ctx context.Context, data *NetworkDataSourceModel) error {

	tflog.Debug(ctx, "Reading the network data")

	// What fields do we want
	opts := vergeio.Options{Fields: "description,name,$key"}

	fn := data.FilterName.ValueString()
	ft := data.FilterType.ValueString()

	//  Build name filter
	if fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(fn))
	}

	// Build type filter
	if ft != "" {
		typeClause := fmt.Sprintf("type eq '%s'", vergeio.EscapeFilterValue(ft))
		if opts.Filter != "" {
			opts.Filter = fmt.Sprintf("%s and %s", opts.Filter, typeClause)
		} else {
			opts.Filter = typeClause
		}
	}

	// Call the SDK API
	var listOpts []vergeos.ListOption
	if opts.Filter != "" {
		listOpts = append(listOpts, vergeos.WithFilter(opts.Filter))
	}

	networks, err := va.sdk.Networks.List(ctx, listOpts...)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", networks))

	networks = vergeio.KeepExact(networks, fn, func(network vergeos.Network) string { return network.Name })
	networks = vergeio.KeepExact(networks, ft, func(network vergeos.Network) string { return network.Type })

	// Convert SDK networks to API model for existing field mapping logic
	var networkAPIResp []NetworkAPIDataSourceModel
	for _, network := range networks {
		networkAPIResp = append(networkAPIResp, NetworkAPIDataSourceModel{
			Id:          int32(network.Key.Int()),
			Name:        network.Name,
			Description: network.Description,
		})
	}

	// save into the resource model
	for _, nwAPIResp := range networkAPIResp {
		data.Networks = append(data.Networks, &NetworkModel{
			Id:          types.Int32Value(nwAPIResp.Id),
			Name:        types.StringValue(nwAPIResp.Name),
			Description: types.StringValue(nwAPIResp.Description),
		})

	}
	// A nil slice is stored as null. Zero rows are an empty list so length() and for expressions can run.
	if data.Networks == nil {
		data.Networks = []*NetworkModel{}
	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
