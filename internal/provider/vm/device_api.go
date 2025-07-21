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

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
type deviceResourceModel struct {
	Key           types.String `tfsdk:"key"`
	Machine       types.Int32  `tfsdk:"machine"`
	MachineType   types.String `tfsdk:"machine_type"`
	Type          types.String `tfsdk:"type"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	ResourceGroup types.String `tfsdk:"resource_group"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	Status        types.Int32  `tfsdk:"status"`
}

// API resource model.
type deviceAPIResourceModel struct {
	Key           string `json:"$key,omitempty"`
	Machine       int32  `json:"machine,omitempty"`
	MachineType   string `json:"machine_type,omitempty"`
	Type          string `json:"type,omitempty"`
	Name          string `json:"name,omitempty"`
	Description   string `json:"description,omitempty"`
	ResourceGroup string `json:"resource_group,omitempty"`
	Enabled       bool   `json:"enabled,omitempty"`
	Status        int32  `json:"status,omitempty"`
}

// List of valid device interfaces.
func getValidDeviceTypes() []string {
	return []string{
		"tpm",
		"usb",
		"pci",
		"vgpu",
	}
}

// VergeResponse structure.
type DeviceResponse struct {
	Key      string             `json:"$key,omitempty"`
	Response DeviceUUIDResponse `json:"response,omitempty"`
	Error    string             `json:"err,omitempty"`
}

// Device UUID response
type DeviceUUIDResponse struct {
	UUID string `json:"uuid,omitempty"`
}

// API endpoint for devices.
const (
	DeviceEndpoint = vergeio.APIEndpoint + "/machine_devices"
)

var _ vergeio.IClient = &DeviceApi{}

func NewDeviceApi(c *vergeio.Client) *DeviceApi {
	return &DeviceApi{
		name:   "Device Api",
		client: c,
	}
}

type DeviceApi struct {
	name   string
	client *vergeio.Client
}

func (da *DeviceApi) Name() string {
	return da.name
}

// Create the Device in the API.
func (da *DeviceApi) createDevice(ctx context.Context, data *deviceResourceModel) error {

	// Prepare the API data packet
	apiData := deviceAPIResourceModel{
		Machine:       data.Machine.ValueInt32(),
		Type:          data.Type.ValueString(),
		MachineType:   data.MachineType.ValueString(),
		Name:          data.Name.ValueString(),
		Description:   data.Description.ValueString(),
		Enabled:       data.Enabled.ValueBool(),
		ResourceGroup: data.ResourceGroup.ValueString(),
		Status:        data.Status.ValueInt32(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for device Item")
	}

	// Call the API and check the response
	apiResp, err := da.client.Post(DeviceEndpoint, encodedBuffer)
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
	var deviceAPIResp DeviceResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&deviceAPIResp); err != nil {
		return fmt.Errorf("invalid format received for creating a device %v", err)
	}

	// save into the Terraform state.
	data.Key = types.StringValue(deviceAPIResp.Key)
	tflog.Debug(ctx, fmt.Sprintf("Created a device with Id %v", data.Key.ValueString()))

	// Read it back from the API to get all the fields
	if readError := da.readDevice(ctx, data); readError != nil {
		return errors.New("Error reading the device: " + readError.Error())
	}

	return nil
}

// Update the Device in the API.
func (da *DeviceApi) updateDevice(ctx context.Context, planData *deviceResourceModel, stateData *deviceResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := deviceAPIResourceModel{
		Machine:       vergeio.Int32ToNil(planData.Machine, stateData.Machine, 0),
		Type:          vergeio.StringToNil(planData.Type, stateData.Type, ""),
		MachineType:   vergeio.StringToNil(planData.MachineType, stateData.MachineType, ""),
		Name:          vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Description:   vergeio.StringToNil(planData.Description, stateData.Description, ""),
		Enabled:       vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
		ResourceGroup: vergeio.StringToNil(planData.ResourceGroup, stateData.ResourceGroup, ""),
		Status:        vergeio.Int32ToNil(planData.Status, stateData.Status, 0),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Call the API and check the response
	apiResp, err := da.client.Put(fmt.Sprintf("%s/%s",
		DeviceEndpoint,
		url.PathEscape(stateData.Key.ValueString()),
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

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Updated device %v", apiData))

	defer apiResp.Body.Close()

	// Read data into the model to get all the attributes
	if readDataError := da.readDevice(ctx, stateData); readDataError != nil {
		return fmt.Errorf("error reading the device %v", readDataError)
	}

	return nil
}

// Read the Device/device from the API.
func (da *DeviceApi) readDevice(ctx context.Context, data *deviceResourceModel) error {

	tflog.Debug(ctx, "Reading the device data with key: "+data.Key.ValueString())

	// Call the Get API with the device id and get the fields we need
	// most fields are not returned by default
	apiResp, err := da.client.Get(fmt.Sprintf("%s/%s",
		DeviceEndpoint,
		url.PathEscape(data.Key.ValueString()),
	), &vergeio.Options{Fields: "machine,type,machine_type,name,description,enabled,resource_group,status"})

	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("Finish reading the device %v", apiResp.Body))

	// Decode the API response
	var deviceAPIResp deviceAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&deviceAPIResp); err != nil {
		return fmt.Errorf("invalid format received for Item %v", err)
	}

	// save into the resource model
	data.Machine = types.Int32Value(deviceAPIResp.Machine)
	data.Type = types.StringValue(deviceAPIResp.Type)
	data.MachineType = types.StringValue(deviceAPIResp.MachineType)
	data.Name = types.StringValue(deviceAPIResp.Name)
	data.Description = types.StringValue(deviceAPIResp.Description)
	data.Enabled = types.BoolValue(deviceAPIResp.Enabled)
	data.ResourceGroup = types.StringValue(deviceAPIResp.ResourceGroup)
	data.Status = types.Int32Value(deviceAPIResp.Status)

	tflog.Debug(ctx, fmt.Sprintf("Finish reading the device %v", data.Key.ValueString()))

	return nil
}

// Delete the Device from the API.
func (da *DeviceApi) deleteDevice(ctx context.Context, data *deviceResourceModel) error {

	tflog.Debug(ctx, "Deleting the device")

	// Call the Get API with the user id and Proceed with user deletion
	_, err := da.client.Delete(fmt.Sprintf("%s/%s",
		DeviceEndpoint,
		url.PathEscape(data.Key.ValueString())))

	// error checking
	if err != nil {
		return errors.New("Error deleting the device: " + err.Error())
	}

	tflog.Debug(ctx, "Device was successfully deleted")

	return nil
}

// Update, Create, Delete the Device in the API.
// This method is called from VM update method.
func (da *DeviceApi) syncDevices(ctx context.Context, planData *[]*deviceResourceModel, stateData *[]*deviceResourceModel, machineId types.Int32) error {

	tflog.Debug(ctx, "Syncing devices: Starting deletion")

	// Delete the devices that are not in the plan
	var stateToBeDeleted []string // List of devices to be deleted

	for _, state := range *stateData {
		found := false
		for _, plan := range *planData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				found = true
				break
			}
		}
		if !found {
			if err := da.deleteDevice(ctx, state); err != nil {
				return fmt.Errorf("failed to delete device: %v", err)
			}
			stateToBeDeleted = append(stateToBeDeleted, state.Name.ValueString())
		}
	}

	// Delete the device record that are not in the state
	for _, name := range stateToBeDeleted {
		for i, state := range *stateData {
			if state.Name.ValueString() == name {
				*stateData = append((*stateData)[:i], (*stateData)[i+1:]...)
				break
			}
		}
	}

	tflog.Debug(ctx, "Syncing devices: Starting updation")

	// Compare the plan data with the state data and update
	for _, plan := range *planData {
		for _, state := range *stateData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				if plan.Description.ValueString() != state.Description.ValueString() ||
					plan.Type.ValueString() != state.Type.ValueString() ||
					plan.MachineType.ValueString() != state.MachineType.ValueString() ||
					plan.Enabled.ValueBool() != state.Enabled.ValueBool() ||
					plan.Status.ValueInt32() != state.Status.ValueInt32() ||
					plan.ResourceGroup.ValueString() != state.ResourceGroup.String() {
					if err := da.updateDevice(ctx, plan, state); err != nil {
						return fmt.Errorf("failed to update device: %v", err)
					}
				}
			}
		}
	}

	tflog.Debug(ctx, "Syncing devices: Starting insertion")

	// Create the devices that are not in the state
	for i, plan := range *planData {
		found := false
		for _, state := range *stateData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				found = true
				break
			}
		}
		if !found {
			plan.Machine = machineId
			if err := da.createDevice(ctx, plan); err != nil {
				return fmt.Errorf("failed to create device: %v", err)
			}
			if err := da.readDevice(ctx, plan); err != nil {
				return fmt.Errorf("failed to read device during the sync: %v", err)
			}
			// insert plan in the stateData in a particular order
			da.insertDeviceInState(i, plan, stateData)
		}
	}

	return nil
}

// Manually insert plan in the stateData in a particular order.
func (da *DeviceApi) insertDeviceInState(insertAt int, planData *deviceResourceModel, stateData *[]*deviceResourceModel) {
	// If it's at the end ten just append
	if insertAt >= len(*stateData) {
		*stateData = append(*stateData, planData)
	} else {
		// insert plan in the stateData in a particular order
		for i := range *stateData {
			if i == insertAt {
				*stateData = append((*stateData)[:i], append([]*deviceResourceModel{planData}, (*stateData)[i:]...)...)
				break
			}
		}
	}
}
