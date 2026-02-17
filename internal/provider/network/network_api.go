// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// Network endpoints - DEPRECATED: SDK handles endpoints internally
// Keeping temporarily for reference during transition

// IClient interface.
var _ vergeio.IClient = &NetworkApi{}

func NewNetworkApi(c *vergeio.Client) *NetworkApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &NetworkApi{
		name:   "Network Api",
		client: c,
		sdk:    sdk,
	}
}

type NetworkApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *NetworkApi) Name() string {
	return nc.name
}

// NetworkAPIResourceModel describes the data model received from the Verge API.
type NetworkAPIResourceModel struct {
	Id                   string  `json:"id,omitempty"`
	Name                 string  `json:"name,omitempty"`
	Enabled              bool    `json:"enabled,omitempty"`
	Default_Gateway      int32   `json:"vnet_default_gateway,omitempty"`
	IPaddress            string  `json:"ipaddress,omitempty"`
	Network              string  `json:"network,omitempty"`
	DHCP                 bool    `json:"dhcp_enabled,omitempty"`
	Dynamic_DHCP         bool    `json:"dhcp_dynamic,omitempty"`
	DHCP_Sequential      bool    `json:"dhcp_sequential,omitempty"`
	DynamicIP_Start      string  `json:"dhcp_start,omitempty"`
	DynamicIP_Stop       string  `json:"dhcp_stop,omitempty"`
	On_Power_Loss        string  `json:"on_power_loss,omitempty"`
	PowerState           string  `json:"powerstate,omitempty"`
	Type                 string  `json:"type,omitempty"`
	VLAN_TAG             int32   `json:"layer2_id,omitempty"`
	MTU                  int32   `json:"mtu,omitempty"`
	Interface_Vnet       int32   `json:"interface_vnet,omitempty"`
	IPaddress_Type       string  `json:"ipaddress_type,omitempty"`
	Layer2_Type          string  `json:"layer2_type,omitempty"`
	Enable_Bonding       bool    `json:"enable_bonding,omitempty"`
	Bond_Interfaces_Args []int32 `json:"bond_interfaces_args,omitempty"`
}

type NetworkAPIDataSourceModel struct {
	Id          int32  `json:"$key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// VNetAction represents the structure for virtual network action requests.
type VNetAction struct {
	VNet   int             `json:"vnet"`
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

// createNetwork creates a new network.
func (nc *NetworkApi) createNetwork(ctx context.Context, data *NetworkResourceModel) error {

	apiData := NetworkAPIResourceModel{
		Name:            data.Name.ValueString(),
		Enabled:         data.Enabled.ValueBool(),
		Default_Gateway: data.Default_Gateway.ValueInt32(),
		IPaddress:       data.IPaddress.ValueString(),
		Network:         data.Network.ValueString(),
		DHCP:            data.DHCP.ValueBool(),
		Dynamic_DHCP:    data.Dynamic_DHCP.ValueBool(),
		DHCP_Sequential: data.DHCP_Sequential.ValueBool(),
		DynamicIP_Start: data.DynamicIP_Start.ValueString(),
		DynamicIP_Stop:  data.DynamicIP_Stop.ValueString(),
		On_Power_Loss:   data.On_Power_Loss.ValueString(),
		PowerState:      data.PowerState.ValueString(),
		Type:            data.Type.ValueString(),
		VLAN_TAG:        data.VLAN_TAG.ValueInt32(),
		MTU:             data.MTU.ValueInt32(),
		Interface_Vnet:  data.Interface_Vnet.ValueInt32(),
		IPaddress_Type:  data.IPaddress_Type.ValueString(),
		Layer2_Type:     data.Layer2_Type.ValueString(),
		Enable_Bonding:  data.Enable_Bonding.ValueBool(),
	}

	// if data.Bond_Interfaces_Args != nil {
	for _, arg := range data.Bond_Interfaces_Args.Elements() {
		apiData.Bond_Interfaces_Args = append(apiData.Bond_Interfaces_Args, arg.(types.Int32).ValueInt32())
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for network Item")
	}

	// Convert to SDK request
	var req vergeos.NetworkCreateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Call SDK API
	network, err := nc.sdk.Networks.Create(ctx, &req)
	if err != nil {
		return err
	}

	// save into the Terraform state.
	data.Id = types.StringValue(fmt.Sprintf("%d", network.ID.Int()))
	tflog.Debug(ctx, fmt.Sprintf("Created a network with Id %v", data.Id.ValueString()))

	return nil
}

// updateNetwork updates an existing network.
func (nc *NetworkApi) updateNetwork(ctx context.Context, planData *NetworkResourceModel, stateData *NetworkResourceModel) error {

	// Prepare the API data packet from the plan
	var defaultGateway int32
	if !planData.Default_Gateway.IsNull() {
		defaultGateway = planData.Default_Gateway.ValueInt32()
	}
	apiData := NetworkAPIResourceModel{

		Id:              vergeio.StringToNil(planData.Id, stateData.Id, ""),
		Name:            vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Enabled:         vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
		Default_Gateway: defaultGateway,
		//		Default_Gateway: vergeio.Int32ToNil(planData.Default_Gateway, stateData.Default_Gateway, 0),
		IPaddress:       vergeio.StringToNil(planData.IPaddress, stateData.IPaddress, ""),
		Network:         vergeio.StringToNil(planData.Network, stateData.Network, ""),
		DHCP:            vergeio.BoolToNil(planData.DHCP, stateData.DHCP, false),
		Dynamic_DHCP:    vergeio.BoolToNil(planData.Dynamic_DHCP, stateData.Dynamic_DHCP, false),
		DHCP_Sequential: vergeio.BoolToNil(planData.DHCP_Sequential, stateData.DHCP_Sequential, false),
		DynamicIP_Start: vergeio.StringToNil(planData.DynamicIP_Start, stateData.DynamicIP_Start, ""),
		DynamicIP_Stop:  vergeio.StringToNil(planData.DynamicIP_Stop, stateData.DynamicIP_Stop, ""),
		On_Power_Loss:   vergeio.StringToNil(planData.On_Power_Loss, stateData.On_Power_Loss, ""),
		PowerState:      vergeio.StringToNil(planData.PowerState, stateData.PowerState, ""),
		// Type is readonly and cannot be updated
		// Type:            vergeio.StringToNil(planData.Type, stateData.Type, ""),
		VLAN_TAG:       vergeio.Int32ToNil(planData.VLAN_TAG, stateData.VLAN_TAG, 0),
		MTU:            vergeio.Int32ToNil(planData.MTU, stateData.MTU, 0),
		Interface_Vnet: vergeio.Int32ToNil(planData.Interface_Vnet, stateData.Interface_Vnet, 0),
		IPaddress_Type: vergeio.StringToNil(planData.IPaddress_Type, stateData.IPaddress_Type, ""),
		Layer2_Type:    vergeio.StringToNil(planData.Layer2_Type, stateData.Layer2_Type, ""),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Convert to SDK request
	var req vergeos.NetworkUpdateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Parse network ID
	networkIDInt, err := strconv.Atoi(apiData.Id)
	if err != nil {
		return fmt.Errorf("invalid network ID format: %v", err)
	}

	// Call SDK API
	_, err = nc.sdk.Networks.Update(ctx, networkIDInt, &req)
	if err != nil {
		return err
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", apiData))

	return nil
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

	// Convert SDK response to API model for field mapping consistency
	var powerState string
	if network.PowerState {
		powerState = "running"
	} else {
		powerState = "stopped"
	}
	
	networkAPIResp := NetworkAPIResourceModel{
		PowerState: powerState,
	}

	// save into the resource model
	data.PowerState = types.StringValue(networkAPIResp.PowerState)

	tflog.Debug(ctx, "Network status read from API is: "+data.PowerState.ValueString())

	return nil
}

// killNetwork Powers off a network.
func (nc *NetworkApi) killNetwork(ctx context.Context, data *NetworkResourceModel) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill Network API for Network %v", data.Id.ValueString()))

	// Convert networkID string to int
	networkIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid Network ID format: %v", err)
	}

	// Create the action payload according to vnet_actions schema
	actionPayload := VNetAction{
		VNet:   networkIDInt,
		Action: "kill",
		Params: json.RawMessage("{}"), // Empty params for kill action
	}
	
	// Convert to JSON payload
	bytedata, err := json.Marshal(actionPayload)
	if err != nil {
		return err
	}
	
	// Note: Using legacy HTTP client for actions until SDK adds network actions support
	req, err := nc.client.Post("api/v4/vnet_actions", bytes.NewBuffer(bytedata))
	if err != nil {
		return err
	}
	if req.StatusCode != 201 {
		return fmt.Errorf("failed to kill Network: status code %v", req.StatusCode)
	}

	return nil
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

	// Convert SDK response to API model for field mapping consistency
	// Note: Some fields may not be available in SDK yet, using defaults where needed
	networkAPIResp := NetworkAPIResourceModel{
		Name:        network.Name,
		Enabled:     network.Enabled,
		IPaddress:   network.IPAddress,
		Network:     network.Network,
		DHCP:        network.DHCPEnabled,
		Dynamic_DHCP: network.DHCPDynamic,
		DHCP_Sequential: network.DHCPSequential,
		DynamicIP_Start: network.DHCPStart,
		DynamicIP_Stop:  network.DHCPStop,
		On_Power_Loss:   network.OnPowerLoss,
		Type:            network.Type,
		// Fields not yet available in SDK - using defaults
		VLAN_TAG:       0,  // TODO: Update when SDK exposes Layer2ID/VLANID
		MTU:            1500, // Default MTU
		Interface_Vnet: 0,
		IPaddress_Type: "static", // Default
		Layer2_Type:    "vlan",   // Default
		Enable_Bonding: false,    // Default
	}

	// Bond interfaces not yet available in SDK
	// TODO: Update when SDK exposes BondInterfacesArgs

	// save into the resource model
	data.Name = types.StringValue(networkAPIResp.Name)
	data.Enabled = types.BoolValue(networkAPIResp.Enabled)
	data.IPaddress = types.StringValue(networkAPIResp.IPaddress)
	data.Network = types.StringValue(networkAPIResp.Network)
	data.DHCP = types.BoolValue(networkAPIResp.DHCP)
	data.Dynamic_DHCP = types.BoolValue(networkAPIResp.Dynamic_DHCP)
	data.DHCP_Sequential = types.BoolValue(networkAPIResp.DHCP_Sequential)
	data.DynamicIP_Start = types.StringValue(networkAPIResp.DynamicIP_Start)
	data.DynamicIP_Stop = types.StringValue(networkAPIResp.DynamicIP_Stop)
	data.On_Power_Loss = types.StringValue(networkAPIResp.On_Power_Loss)
	data.Type = types.StringValue(networkAPIResp.Type)
	data.VLAN_TAG = types.Int32Value(networkAPIResp.VLAN_TAG)
	data.MTU = types.Int32Value(networkAPIResp.MTU)
	data.Interface_Vnet = types.Int32Value(networkAPIResp.Interface_Vnet)
	data.IPaddress_Type = types.StringValue(networkAPIResp.IPaddress_Type)
	data.Layer2_Type = types.StringValue(networkAPIResp.Layer2_Type)
	data.Enable_Bonding = types.BoolValue(networkAPIResp.Enable_Bonding)

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// Read the Networks from the API for data source.
func (va *NetworkApi) readNetworks(ctx context.Context, data *NetworkDataSourceModel) error {

	tflog.Debug(ctx, "Reading the network data")

	// What fields do we want
	opts := vergeio.Options{Fields: "description,name,$key"}

	//  Build name filter
	if fn := data.FilterName.ValueString(); fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn)
	}

	// Build type filter
	if ft := data.FilterType.ValueString(); ft != "" {
		if opts.Filter != "" {
			opts.Filter = fmt.Sprintf("%s and type eq '%s'", opts.Filter, ft)
		} else {
			opts.Filter = fmt.Sprintf("type eq '%s'", ft)
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

	// Convert SDK networks to API model for existing field mapping logic
	var networkAPIResp []NetworkAPIDataSourceModel
	for _, network := range networks {
		networkAPIResp = append(networkAPIResp, NetworkAPIDataSourceModel{
			Id:          int32(network.ID.Int()),
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

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
