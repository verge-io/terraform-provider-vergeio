// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Network endpoints.
const (
	NetworkEndpoint       = vergeio.APIEndpoint + "/vnets"
	NetworkActionEndpoint = vergeio.APIEndpoint + "/vnet_actions"
)

// IClient interface.
var _ vergeio.IClient = &NetworkApi{}

func NewNetworkApi(c *vergeio.Client) *NetworkApi {
	return &NetworkApi{
		name:   "Network Api",
		client: c,
	}
}

type NetworkApi struct {
	name   string
	client *vergeio.Client
}

func (nc *NetworkApi) Name() string {
	return nc.name
}

// NetworkAPIResourceModel describes the data model received from the Verge API.
type NetworkAPIResourceModel struct {
	Id              string `json:"id,omitempty"`
	Name            string `json:"name,omitempty"`
	Enabled         bool   `json:"enabled,omitempty"`
	Default_Gateway int32  `json:"vnet_default_gateway,omitempty"`
	IPaddress       string `json:"ipaddress,omitempty"`
	Network         string `json:"network,omitempty"`
	DHCP            bool   `json:"dhcp_enabled,omitempty"`
	Dynamic_DHCP    bool   `json:"dhcp_dynamic,omitempty"`
	DHCP_Sequential bool   `json:"dhcp_sequential,omitempty"`
	DynamicIP_Start string `json:"dhcp_start,omitempty"`
	DynamicIP_Stop  string `json:"dhcp_stop,omitempty"`
	On_Power_Loss   string `json:"on_power_loss,omitempty"`
	PowerState      string `json:"powerstate,omitempty"`
	Type            string `json:"type,omitempty"`
	VLAN_TAG        int32  `json:"layer2_id,omitempty"`
	MTU             int32  `json:"mtu,omitempty"`
	Interface_Vnet  int32  `json:"interface_vnet,omitempty"`
	IPaddress_Type  string `json:"ipaddress_type,omitempty"`
	Layer2_Type     string `json:"layer2_type,omitempty"`
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
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for network Item")
	}

	// Time to call the API
	apiResp, err := nc.client.Post(NetworkEndpoint, encodedBuffer)
	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 201 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	// Decode the API response
	var networkAPIResp vergeio.VergeResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&networkAPIResp); err != nil {
		return errors.New("invalid format received for Item")
	}

	// save into the Terraform state.
	data.Id = types.StringValue(networkAPIResp.Key)
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

	// Time to call the API
	apiResp, err := nc.client.Put(fmt.Sprintf("%s/%s",
		NetworkEndpoint,
		url.PathEscape(apiData.Id),
	), encodedBuffer)
	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", apiData))

	defer apiResp.Body.Close()

	return nil
}

// deleteNetwork deletes a network.
func (nc *NetworkApi) deleteNetwork(ctx context.Context, data *NetworkResourceModel) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill Network API for Network %v", data.Id.ValueString()))

	// call the API
	apiResp, err := nc.client.Delete(fmt.Sprintf("%s/%s",
		NetworkEndpoint,
		url.PathEscape(data.Id.ValueString()),
	))
	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Deleted the network with the ID %v", data.Id))

	defer apiResp.Body.Close()

	return nil
}

// Checks the power state of the network.
func (nc *NetworkApi) checkNetworkPowerState(ctx context.Context, data *NetworkResourceModel) error {

	// Send the kill action request to the vnet_actions endpoint
	apiResp, err := nc.client.Get(fmt.Sprintf("%s/%s",
		NetworkEndpoint,
		url.PathEscape(data.Id.ValueString())),
		&vergeio.Options{
			Fields: "machine#status#display(status) as powerState"})

	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", apiResp.Body))

	// Decode the API response
	var networkAPIResp NetworkAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&networkAPIResp); err != nil {
		return errors.New("invalid format received for Item")
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
	bytedata, err := json.Marshal(actionPayload)
	if err != nil {
		return err
	}
	// Send the kill action request to the vnet_actions endpoint
	req, err := nc.client.Post(NetworkActionEndpoint, bytes.NewBuffer(bytedata))
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

	// Call the Get API with the network id and get the fields we need
	// most fields are not returned by default
	apiResp, err := nc.client.Get(fmt.Sprintf("%s/%s",
		NetworkEndpoint,
		url.PathEscape(data.Id.ValueString()),
	), &vergeio.Options{Fields: "name,enabled,ipaddress,network,dhcp_enabled,dhcp_dynamic,dhcp_sequential,dhcp_start,dhcp_stop,on_power_loss,type,layer2_id,mtu,interface_vnet,ipaddress_type,layer2_type"})

	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("read the resource %v", apiResp.Body))

	// Decode the API response
	var networkAPIResp NetworkAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&networkAPIResp); err != nil {
		return errors.New("invalid format received for Item")
	}
	// if networkAPIResp.Default_Gateway != 0 {
	//     data.Default_Gateway = types.Int32Value(networkAPIResp.Default_Gateway)
	// } else {
	//     data.Default_Gateway = types.Int32Null()
	// }
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

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// Read the Networks from the API for data source.
func (va *NetworkApi) readNetworks(ctx context.Context, data *NetworkDataSourceModel) error {

	tflog.Debug(ctx, "Reading the network data")

	// What fields do we want
	opts := vergeio.Options{Fields: "description,name,$key"}

	// Build filter
	if fn := data.FilterName.ValueString(); fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn)
	}

	// Call the API
	apiResp, err := va.client.Get(NetworkEndpoint,
		&opts)

	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", apiResp.Body))

	// Decode the API response
	var networkAPIResp []NetworkAPIDataSourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&networkAPIResp); err != nil {
		return errors.New("invalid format received for VM Item")
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
