// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// NIC Resource Models.
type nicResourceModel struct {
	Id          types.String `tfsdk:"id"`
	Machine     types.Int32  `tfsdk:"machine"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	Interface   types.String `tfsdk:"interface"`
	Driver      types.String `tfsdk:"driver"`
	Model       types.String `tfsdk:"model"`
	Vendor      types.String `tfsdk:"vendor"`
	Port        types.Int32  `tfsdk:"port"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	VNET        types.Int32  `tfsdk:"vnet"`
	MAC         types.String `tfsdk:"macaddress"`
	Asset       types.String `tfsdk:"asset"`
}

type nicAPIResourceModel struct {
	Id          string `json:"id,omitempty"`
	Machine     int32  `json:"machine,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Driver      string `json:"driver,omitempty"`
	Model       string `json:"model,omitempty"`
	Vendor      string `json:"vendor,omitempty"`
	Port        int32  `json:"port,omitempty"`
	Enabled     bool   `json:"enabled"`
	VNET        int32  `json:"vnet,omitempty"`
	MAC         string `json:"macaddress,omitempty"`
	Asset       string `json:"asset,omitempty"`
}

// to get the power status.
type nicAPIPowerStatus struct {
	PowerState string `json:"powerstate,omitempty"`
}

// NIC Endpoint.
const (
	NICEndpoint = vergeio.APIEndpoint + "/machine_nics"
)

var _ vergeio.IClient = &NICApi{}

func NewNICApi(c *vergeio.Client) *NICApi {
	return &NICApi{
		name:   "NIC Api",
		client: c,
	}
}

type NICApi struct {
	name   string
	client *vergeio.Client
}

func (nc *NICApi) Name() string {
	return nc.name
}

// Create the NIC in the API.
func (nc *NICApi) createNIC(ctx context.Context, data *nicResourceModel) error {

	apiData := nicAPIResourceModel{
		Machine:     data.Machine.ValueInt32(),
		Name:        data.Name.ValueString(),
		Description: data.Description.ValueString(),
		Interface:   data.Interface.ValueString(),
		Driver:      data.Driver.ValueString(),
		Model:       data.Model.ValueString(),
		Vendor:      data.Vendor.ValueString(),
		Port:        data.Port.ValueInt32(),
		Enabled:     data.Enabled.ValueBool(),
		VNET:        data.VNET.ValueInt32(),
		MAC:         data.MAC.ValueString(),
		Asset:       data.Asset.ValueString(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for NIC")
	}

	// Call the API and check the response
	apiResp, err := nc.client.Post(NICEndpoint, encodedBuffer)
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 201 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	// Decode the API response
	var nicAPIResp vergeio.VergeResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&nicAPIResp); err != nil {
		return errors.New("invalid format received for creating the NIC")
	}

	// save into the Terraform state.
	data.Id = types.StringValue(nicAPIResp.Key)
	tflog.Debug(ctx, fmt.Sprintf("Created a nic with Id %v", data.Id.ValueString()))

	// Read it back from the API to get all the fields
	if readError := nc.readNIC(ctx, data); readError != nil {
		return errors.New("Error reading the nic: " + readError.Error())
	}

	return nil
}

// Update the NIC in the API.
func (nc *NICApi) updateNIC(ctx context.Context, planData *nicResourceModel, stateData *nicResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := nicAPIResourceModel{
		Machine:     vergeio.Int32ToNil(planData.Machine, stateData.Machine, 0),
		Name:        vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Description: vergeio.StringToNil(planData.Description, stateData.Description, ""),
		Interface:   vergeio.StringToNil(planData.Interface, stateData.Interface, ""),
		Driver:      vergeio.StringToNil(planData.Driver, stateData.Driver, ""),
		Model:       vergeio.StringToNil(planData.Model, stateData.Model, ""),
		Vendor:      vergeio.StringToNil(planData.Vendor, stateData.Vendor, ""),
		Port:        vergeio.Int32ToNil(planData.Port, stateData.Port, 0),
		Enabled:     vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
		VNET:        vergeio.Int32ToNil(planData.VNET, stateData.VNET, 0),
		MAC:         vergeio.StringToNil(planData.MAC, stateData.MAC, ""),
		Asset:       vergeio.StringToNil(planData.Asset, stateData.Asset, ""),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for the NIC")
	}

	// Call the API and check the response
	apiResp, err := nc.client.Put(fmt.Sprintf("%s/%s",
		NICEndpoint,
		url.PathEscape(stateData.Id.ValueString()),
	), encodedBuffer)

	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	defer apiResp.Body.Close()

	// Read data into the model to get all the attributes
	if readDataError := nc.readNIC(ctx, stateData); readDataError != nil {
		return fmt.Errorf("error Fetching Data %v", readDataError)
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated nic %v", stateData.Id.ValueString()))

	return nil
}

// Read the NIC from the API.
func (nc *NICApi) readNIC(ctx context.Context, data *nicResourceModel) error {

	tflog.Debug(ctx, "Reading the nic data")

	// Call the Get API with the nic id and get the fields we need
	// most fields are not returned by default
	apiResp, err := nc.client.Get(fmt.Sprintf("%s/%s",
		NICEndpoint,
		url.PathEscape(data.Id.ValueString()),
	), &vergeio.Options{Fields: "machine,name,description,interface,driver,model,vendor,port,enabled,vnet,macaddress,asset"})

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

	tflog.Debug(ctx, fmt.Sprintf("Read the NIC from the API %v", apiResp.Body))

	// Decode the API response
	var nicAPIResp nicAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&nicAPIResp); err != nil {
		return errors.New("invalid format received for Item")
	}

	// save into the resource model
	data.Machine = types.Int32Value(nicAPIResp.Machine)
	data.Name = types.StringValue(nicAPIResp.Name)
	data.Description = types.StringValue(nicAPIResp.Description)
	data.Interface = types.StringValue(nicAPIResp.Interface)
	data.Driver = types.StringValue(nicAPIResp.Driver)
	data.Model = types.StringValue(nicAPIResp.Model)
	data.Vendor = types.StringValue(nicAPIResp.Vendor)
	data.Port = types.Int32Value(nicAPIResp.Port)
	data.Enabled = types.BoolValue(nicAPIResp.Enabled)
	data.VNET = types.Int32Value(nicAPIResp.VNET)
	data.MAC = types.StringValue(nicAPIResp.MAC)
	data.Asset = types.StringValue(nicAPIResp.Asset)

	tflog.Debug(ctx, "NIC Data was successfully read from the API")

	return nil
}

// Delete the NIC from the API.
func (na *NICApi) deleteNIC(ctx context.Context, data *nicResourceModel, vmId types.String) error {

	tflog.Debug(ctx, "Deleting the nic")

	// VM API for hotplugging via VM Actions
	vmApi := NewVMApi(na.client)

	var powerState string = ""

	// Call the API to check if the nic is in a power state that can be deleted
	if err := na.checkNICPowerState(ctx, data.Id.ValueString(), &powerState); err != nil {
		return fmt.Errorf("error checking NIC power state %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Current NIC power state is %v", powerState))

	// Call the API to check if the vm is in a power state that can be deleted
	for strings.ToLower(powerState) != "down" {
		Retries := 1

		// Power the vm off
		if err := vmApi.hotplugNIC(ctx, data.Id.ValueString(), vmId); err != nil {
			return fmt.Errorf("failed to hotplug NIC: %v", err)
		}

		// Wait for a short period to allow the kill operation to complete
		time.Sleep(2 * time.Second)

		// Call the API to check if the nic is in a power state that can be deleted
		if err := na.checkNICPowerState(ctx, data.Id.ValueString(), &powerState); err != nil {
			return fmt.Errorf("error checking NIC power state %v", err)
		}

		Retries += 1

		// We are only going to retry 5 times before giving up
		if Retries > 5 {
			return fmt.Errorf("failed to kill NIC before deletion after %d retries", Retries)
		}
		continue
	}

	// Call the Get API with the user id and Proceed with user deletion
	_, err := na.client.Delete(fmt.Sprintf("%s/%s",
		NICEndpoint,
		url.PathEscape(data.Id.ValueString())))

	if err != nil {
		return errors.New("Error deleting the NIC: " + err.Error())
	}

	tflog.Debug(ctx, "NIC was successfully deleted")

	return nil
}

// Update, Create, Delete the NIC in the API.
// This method is called from VM update method.
func (na *NICApi) syncNICs(ctx context.Context, planData *[]*nicResourceModel, stateData *[]*nicResourceModel, machine types.Int32, vmId types.String) error {

	tflog.Debug(ctx, "Syncing NICs: Starting deletion")

	var stateToBeDeleted []string // List of disks to be deleted

	// Delete the nics that are not in the plan
	for _, state := range *stateData {
		found := false
		for _, plan := range *planData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				found = true
				break
			}
		}
		if !found {
			if err := na.deleteNIC(ctx, state, vmId); err != nil {
				return fmt.Errorf("failed to delete nic: %v", err)
			}
			stateToBeDeleted = append(stateToBeDeleted, state.Name.ValueString())
		}
	}

	// Delete the nic that are not in the state
	for _, name := range stateToBeDeleted {
		for i, state := range *stateData {
			if state.Name.ValueString() == name {
				*stateData = append((*stateData)[:i], (*stateData)[i+1:]...)
				break
			}
		}
	}

	tflog.Debug(ctx, "Syncing NICs: Starting updation")

	// Compare the plan data with the state data and update
	for _, plan := range *planData {
		for _, state := range *stateData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				if plan.Description.ValueString() != state.Description.ValueString() ||
					plan.Interface.ValueString() != state.Interface.ValueString() ||
					plan.Driver.ValueString() != state.Driver.ValueString() ||
					plan.Model.ValueString() != state.Model.ValueString() ||
					plan.Vendor.ValueString() != state.Vendor.ValueString() ||
					plan.Port.ValueInt32() != state.Port.ValueInt32() ||
					plan.VNET.ValueInt32() != state.VNET.ValueInt32() ||
					plan.MAC.ValueString() != state.MAC.ValueString() ||
					plan.Asset.ValueString() != state.Asset.ValueString() ||
					plan.Enabled.ValueBool() != state.Enabled.ValueBool() {
					if err := na.updateNIC(ctx, plan, state); err != nil {
						return fmt.Errorf("failed to update nic: %v", err)
					}
				}
			}
		}
	}

	tflog.Debug(ctx, "Syncing NICs: Starting insertion")

	// Create the nics that are not in the state
	for i, plan := range *planData {
		found := false
		for _, state := range *stateData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				found = true
				break
			}
		}
		if !found {
			plan.Machine = machine
			if err := na.createNIC(ctx, plan); err != nil {
				return fmt.Errorf("failed to create nic: %v", err)
			}
			if err := na.readNIC(ctx, plan); err != nil {
				return fmt.Errorf("failed to read disk during the sync: %v", err)
			}
			// insert plan in the stateData in a particular order
			na.insertNICInState(i, plan, stateData) // insert nic in the stateData in a particular order na.insertDiskInState(i, plan, stateData)
		}
	}
	return nil
}

// Manually insert plan in the stateData in a particular order.
func (na *NICApi) insertNICInState(insertAt int, planData *nicResourceModel, stateData *[]*nicResourceModel) {
	// If it's at the end ten just append
	if insertAt >= len(*stateData) {
		*stateData = append(*stateData, planData)
	} else {
		// insert plan in the stateData in a particular order
		for i := range *stateData {
			if i == insertAt {
				*stateData = append((*stateData)[:i], append([]*nicResourceModel{planData}, (*stateData)[i:]...)...)
				break
			}
		}
	}
}

// to get the power status of the nic.
func (na *NICApi) checkNICPowerState(ctx context.Context, key string, powerState *string) error {

	tflog.Debug(ctx, fmt.Sprintf("Checking the power state of the NIC %v", key))

	// Call the API to get the NIC status
	apiResp, err := na.client.Get(fmt.Sprintf("%s/%s",
		NICEndpoint,
		url.PathEscape(key)),
		&vergeio.Options{
			Fields: "status#status as powerState"})

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

	tflog.Debug(ctx, fmt.Sprintf("Read the powerstatus of the NIC %v", apiResp.Body))

	// Decode the API response
	var nicAPIResp nicAPIPowerStatus
	if err := json.NewDecoder(apiResp.Body).Decode(&nicAPIResp); err != nil {
		return errors.New("invalid format received for nic power state call")
	}

	// save into the resource model
	*powerState = nicAPIResp.PowerState

	tflog.Debug(ctx, fmt.Sprintf("NIC status read from API: %v", nicAPIResp.PowerState))

	return nil
}

func (va *VMApi) hotplugNIC(ctx context.Context, nicId string, vmId types.String) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the VMActions API for the NIC %v", nicId))

	// Create the action payload according to vm_actions schema
	params := VMActionParams{
		Device: nicId,
		Unplug: true,
	}
	intVMId, err := strconv.Atoi(vmId.ValueString())
	if err != nil {
		return fmt.Errorf("invalid VM ID format: %v", err)
	}
	actionPayload := VMAction{
		VM:     int32(intVMId),
		Action: "hotplugnic",
		Params: params,
	}
	tflog.Debug(ctx, fmt.Sprintf("Calling the hotplug NIC API with the action payload %v", actionPayload))
	bytedata, err := json.Marshal(actionPayload)
	if err != nil {
		return err
	}

	// Send the kill action request to the vnet_actions endpoint
	req, err := va.client.Post(VMActionEndpoint, bytes.NewBuffer(bytedata))
	if err != nil {
		return err
	}
	if req.StatusCode != 201 {
		return fmt.Errorf("failed to kill NIC: status code %v", req.StatusCode)
	}

	return nil
}
