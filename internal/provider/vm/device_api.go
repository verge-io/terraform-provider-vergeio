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
	"github.com/verge-io/govergeos"
)

// Ensure provider defined types fully satisfy framework interfaces.
type deviceResourceModel struct {
	Key     types.String `tfsdk:"key"`
	Machine types.Int32  `tfsdk:"machine"`
	// MachineType            types.String                  `tfsdk:"machine_type"`
	Type                          types.String                   `tfsdk:"type"`
	Name                          types.String                   `tfsdk:"name"`
	Description                   types.String                   `tfsdk:"description"`
	ResourceGroup                 types.String                   `tfsdk:"resource_group"`
	Enabled                       types.Bool                     `tfsdk:"enabled"`
	Status                        types.Int32                    `tfsdk:"status"`
	DeviceUSBSettingsModel        *DeviceUSBSettingsModel        `tfsdk:"usb_settings"`
	DeviceTPMSettingsModel        *DeviceTPMSettingsModel        `tfsdk:"tpm_settings"`
	DeviceNvidiaVGPUSettingsModel *DeviceNvidiaVGPUSettingsModel `tfsdk:"nvidia_vgpu_settings"`
}

type DeviceUSBSettingsModel struct {
	Key            types.Int32 `tfsdk:"key"`
	MachineDevice  types.Int32 `tfsdk:"machine_device"`
	GuestReset     types.Bool  `tfsdk:"guest_reset"`
	GuestResetsAll types.Bool  `tfsdk:"guest_resets_all"`
}

type DeviceUSBSettingsAPIModel struct {
	Key            int32 `json:"$key,omitempty"`
	MachineDevice  int32 `json:"machine_device,omitempty"`
	GuestReset     bool  `json:"guest_reset,omitempty"`
	GuestResetsAll bool  `json:"guest_resets_all,omitempty"`
}

type DeviceTPMSettingsModel struct {
	Key           types.Int32  `tfsdk:"key"`
	MachineDevice types.Int32  `tfsdk:"machine_device"`
	Model         types.String `tfsdk:"model"`
	Version       types.String `tfsdk:"version"`
}

type DeviceTPMSettingsAPIModel struct {
	Key           int32  `json:"$key,omitempty"`
	MachineDevice int32  `json:"machine_device,omitempty"`
	Model         string `json:"model,omitempty"`
	Version       string `json:"version,omitempty"`
}

type DeviceNvidiaVGPUSettingsModel struct {
	Key           types.Int32  `tfsdk:"key"`
	MachineDevice types.Int32  `tfsdk:"machine_device"`
	ProfileType   types.String `tfsdk:"profile_type"`
	// AttachDrivers    types.Bool   `tfsdk:"attach_drivers"`
	FrameRateLimiter types.Int32 `tfsdk:"frame_rate_limiter"`
	DisableVNC       types.Bool  `tfsdk:"disable_vnc"`
	EnableUVM        types.Bool  `tfsdk:"enable_uvm"`
	EnableDebugging  types.Bool  `tfsdk:"enable_debugging"`
	EnableProfiling  types.Bool  `tfsdk:"enable_profiling"`
}

type DeviceNvidiaVGPUSettingsAPIModel struct {
	Key           int32  `json:"$key,omitempty"`
	MachineDevice int32  `json:"machine_device,omitempty"`
	ProfileType   string `json:"profile_type,omitempty"`
	// AttachDrivers    bool   `json:"attach_drivers,omitempty"`
	FrameRateLimiter int32 `json:"frame_rate_limiter,omitempty"`
	DisableVNC       bool  `json:"disable_vnc,omitempty"`
	EnableUVM        bool  `json:"enable_uvm,omitempty"`
	EnableDebugging  bool  `json:"enable_debugging,omitempty"`
	EnableProfiling  bool  `json:"enable_profiling,omitempty"`
}

// API resource model.
type deviceAPIResourceModel struct {
	Key     string `json:"$key,omitempty"`
	Machine int32  `json:"machine,omitempty"`
	// MachineType   string `json:"machine_type,omitempty"`
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
		"node_usb_devices",
		"node_pci_devices",
		"node_nvidia_vgpu_devices",
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
	DeviceEndpoint                   = vergeio.APIEndpoint + "/machine_devices"
	DeviceUSBSettingsEndpoint        = vergeio.APIEndpoint + "/machine_device_settings_usb"
	DeviceTPMSettingsEndpoint        = vergeio.APIEndpoint + "/machine_device_settings_tpm"
	DeviceNvidiaVGPUSettingsEndpoint = vergeio.APIEndpoint + "/machine_device_settings_nvidia_vgpu"
)

var _ vergeio.IClient = &DeviceApi{}

func NewDeviceApi(c *vergeio.Client) *DeviceApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &DeviceApi{
		name:   "Device Api",
		client: c,
		sdk:    sdk,
	}
}

type DeviceApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (da *DeviceApi) Name() string {
	return da.name
}

// Create the Device in the API.
func (da *DeviceApi) createDevice(ctx context.Context, data *deviceResourceModel) error {

	// Prepare the API data packet
	apiData := deviceAPIResourceModel{
		Machine: data.Machine.ValueInt32(),
		Type:    data.Type.ValueString(),
		// MachineType:   data.MachineType.ValueString(),
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

	// Read the device API response to get the key
	data.Key = types.StringValue(deviceAPIResp.Key)
	tflog.Debug(ctx, fmt.Sprintf("Created a device with Id %v", data.Key.ValueString()))

	// Read it back from the API to get all the fields
	if readError := da.readDevice(ctx, data); readError != nil {
		return errors.New("Error reading the device: " + readError.Error())
	}

	// If the device is a USB device then we need to create or update the USB
	switch data.Type.ValueString() {
	case "node_usb_devices":
		// If the USB settings are not set then skip updating USB settings
		if data.DeviceUSBSettingsModel == nil {
			tflog.Debug(ctx, "No USB settings found for device: "+data.Name.ValueString())
			break
		}
		// first read the USB settings to get the key
		if err := da.readUSBSettings(ctx, data, true /* read key only */); err != nil {
			return fmt.Errorf("failed to read USB settings: %v", err)
		}
		// Now update the USB settings
		if err := da.updateUSBSettings(ctx, data, data.DeviceUSBSettingsModel.Key); err != nil {
			return fmt.Errorf("failed to update USB settings: %v", err)
		}
		// read it again to get the rest of the fields
		if err := da.readUSBSettings(ctx, data, false /* read everything */); err != nil {
			return fmt.Errorf("failed to read USB settings: %v", err)
		}
	case "tpm":
		// If the USB settings are not set then skip updating USB settings
		if data.DeviceTPMSettingsModel == nil {
			tflog.Debug(ctx, "No TPM settings found for device: "+data.Name.ValueString())
			break
		}
		// first read the TPM settings to get the key
		if err := da.readTPMSettings(ctx, data, true /* read key only */); err != nil {
			return fmt.Errorf("failed to read TPM settings: %v", err)
		}
		// Now update the TPM settings
		if err := da.updateTPMSettings(ctx, data, data.DeviceTPMSettingsModel.Key); err != nil {
			return fmt.Errorf("failed to update TPM settings: %v", err)
		}
		// read it again to get the rest of the fields
		if err := da.readTPMSettings(ctx, data, false /* read everything */); err != nil {
			return fmt.Errorf("failed to read TPM settings: %v", err)
		}
	case "node_nvidia_vgpu_devices":
		// If the Nvidia VGPU settings are not set then skip updating Nvidia VGPU settings
		if data.DeviceNvidiaVGPUSettingsModel == nil {
			tflog.Debug(ctx, "No Nvidia VGPU settings found for device: "+data.Name.ValueString())
			break
		}
		// first read the Nvidia VGPU settings to get the key
		if err := da.readNvidiaVGPUSettings(ctx, data, true /* read key only */); err != nil {
			return fmt.Errorf("failed to read Nvidia VGPU settings: %v", err)
		}
		// Now update the Nvidia VGPU settings
		if err := da.updateNvidiaVGPUSettings(ctx, data, data.DeviceNvidiaVGPUSettingsModel.Key); err != nil {
			return fmt.Errorf("failed to update Nvidia VGPU settings: %v", err)
		}
		// read it again to get the rest of the fields
		if err := da.readNvidiaVGPUSettings(ctx, data, false /* read everything */); err != nil {
			return fmt.Errorf("failed to read Nvidia VGPU settings: %v", err)
		}
	}

	// save into the Terraform state.
	data.Key = types.StringValue(deviceAPIResp.Key)
	tflog.Debug(ctx, fmt.Sprintf("Created a device with Id %v", data.Key.ValueString()))

	return nil
}

// Update the Device in the API.
func (da *DeviceApi) updateDevice(ctx context.Context, planData *deviceResourceModel, stateData *deviceResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := deviceAPIResourceModel{
		Machine:     vergeio.Int32ToNil(planData.Machine, stateData.Machine, 0),
		Name:        vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Description: vergeio.StringToNil(planData.Description, stateData.Description, ""),
		Enabled:     vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
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

	switch stateData.Type.ValueString() {
	case "node_usb_devices":
		// If the USB settings are not set then skip updating USB settings
		if stateData.DeviceUSBSettingsModel == nil {
			tflog.Debug(ctx, "No USB settings found for device: "+stateData.Name.ValueString())
			break
		}
		// Now update the USB settings
		if err := da.updateUSBSettings(ctx, planData, stateData.DeviceUSBSettingsModel.Key); err != nil {
			return fmt.Errorf("failed to update USB settings: %v", err)
		}
		//  read the USB settings to get updated data
		if err := da.readUSBSettings(ctx, stateData, false /* read all data */); err != nil {
			return fmt.Errorf("failed to read USB settings: %v", err)
		}
	case "tpm":
		// Now update the TPM settings
		if err := da.updateTPMSettings(ctx, planData, stateData.DeviceTPMSettingsModel.Key); err != nil {
			return fmt.Errorf("failed to update TPM settings: %v", err)
		}
		//  read the TPM settings to get updated data
		if err := da.readTPMSettings(ctx, stateData, false /* read all data */); err != nil {
			return fmt.Errorf("failed to read TPM settings: %v", err)
		}
	case "node_nvidia_vgpu_devices":
		// Now update the Nvidia VGPU settings
		if err := da.updateNvidiaVGPUSettings(ctx, planData, stateData.DeviceNvidiaVGPUSettingsModel.Key); err != nil {
			return fmt.Errorf("failed to update Nvidia VGPU settings: %v", err)
		}
		//  read the Nvidia VGPU settings to get updated data
		if err := da.readNvidiaVGPUSettings(ctx, stateData, false /* read all data */); err != nil {
			return fmt.Errorf("failed to read Nvidia VGPU settings: %v", err)
		}
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
	// data.MachineType = types.StringValue(deviceAPIResp.MachineType)
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
					plan.Enabled.ValueBool() != state.Enabled.ValueBool() ||
					(plan.Type.ValueString() == "node_usb_devices" &&
						(plan.DeviceUSBSettingsModel.GuestReset != state.DeviceUSBSettingsModel.GuestReset ||
							plan.DeviceUSBSettingsModel.GuestResetsAll != state.DeviceUSBSettingsModel.GuestResetsAll)) ||
					(plan.Type.ValueString() == "tpm" &&
						(plan.DeviceTPMSettingsModel.Model != state.DeviceTPMSettingsModel.Model)) ||
					(plan.Type.ValueString() == "node_nvidia_vgpu_devices" &&
						(plan.DeviceNvidiaVGPUSettingsModel.ProfileType != state.DeviceNvidiaVGPUSettingsModel.ProfileType ||
							// plan.DeviceNvidiaVGPUSettingsModel.AttachDrivers != state.DeviceNvidiaVGPUSettingsModel.AttachDrivers ||
							plan.DeviceNvidiaVGPUSettingsModel.FrameRateLimiter != state.DeviceNvidiaVGPUSettingsModel.FrameRateLimiter ||
							plan.DeviceNvidiaVGPUSettingsModel.DisableVNC != state.DeviceNvidiaVGPUSettingsModel.DisableVNC ||
							plan.DeviceNvidiaVGPUSettingsModel.EnableUVM != state.DeviceNvidiaVGPUSettingsModel.EnableUVM ||
							plan.DeviceNvidiaVGPUSettingsModel.EnableDebugging != state.DeviceNvidiaVGPUSettingsModel.EnableDebugging ||
							plan.DeviceNvidiaVGPUSettingsModel.EnableProfiling != state.DeviceNvidiaVGPUSettingsModel.EnableProfiling)) {
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

// USB Settings API
// UpdateUSBSettings updates USB settings for the device.
func (da *DeviceApi) updateUSBSettings(ctx context.Context, data *deviceResourceModel, key types.Int32) error {
	tflog.Debug(ctx, "Updating USB settings for device: "+data.Name.ValueString())

	if data.DeviceUSBSettingsModel == nil {
		tflog.Debug(ctx, "No USB settings found for device: "+data.Name.ValueString())
		return nil
	}

	// Prepare the API data packet
	apiData := map[string]interface{}{
		"guest_reset":      data.DeviceUSBSettingsModel.GuestReset.ValueBool(),
		"guest_resets_all": data.DeviceUSBSettingsModel.GuestResetsAll.ValueBool(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for USB settings")
	}

	// Call the API and check the response
	apiResp, err := da.client.Put(fmt.Sprintf("%s/%s",
		DeviceUSBSettingsEndpoint,
		url.PathEscape(fmt.Sprintf("%d", key.ValueInt32())),
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

	tflog.Debug(ctx, "USB settings updated successfully for device: "+data.Name.ValueString())
	return nil
}

// readUSBSettings reads USB settings for the device.
func (da *DeviceApi) readUSBSettings(ctx context.Context, data *deviceResourceModel, readKeyOnly bool) error {
	tflog.Debug(ctx, "Reading USB settings for device: "+data.Name.ValueString())

	// Call the API and check the response
	apiResp, err := da.client.Get(DeviceUSBSettingsEndpoint, &vergeio.Options{Fields: "most", Filter: fmt.Sprintf("machine_device eq %s", data.Key.ValueString())})
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	// Decode the API response
	var usbSettingsAPIResp []DeviceUSBSettingsAPIModel
	if err := json.NewDecoder(apiResp.Body).Decode(&usbSettingsAPIResp); err != nil {
		return errors.New("invalid format received for USB : " + err.Error())
	}

	if len(usbSettingsAPIResp) > 0 {
		// read the key based on the machine device
		data.DeviceUSBSettingsModel.Key = types.Int32Value(usbSettingsAPIResp[0].Key)
		data.DeviceUSBSettingsModel.MachineDevice = types.Int32Value(usbSettingsAPIResp[0].MachineDevice)
		if !readKeyOnly {
			data.DeviceUSBSettingsModel.GuestReset = types.BoolValue(usbSettingsAPIResp[0].GuestReset)
			data.DeviceUSBSettingsModel.GuestResetsAll = types.BoolValue(usbSettingsAPIResp[0].GuestResetsAll)
		}
	}

	tflog.Debug(ctx, "USB settings read successfully for device: "+data.Name.ValueString())
	return nil
}

// UpdateTPMSettings updates TPM settings for the device.
func (da *DeviceApi) updateTPMSettings(ctx context.Context, data *deviceResourceModel, key types.Int32) error {
	tflog.Debug(ctx, "Updating TPM settings for device: "+data.Name.ValueString())

	// Prepare the API data packet
	apiData := map[string]interface{}{
		"model": data.DeviceTPMSettingsModel.Model.ValueString(),
		// "version": data.DeviceTPMSettingsModel.Version.ValueString(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for TPM settings")
	}

	// Call the API and check the response
	apiResp, err := da.client.Put(fmt.Sprintf("%s/%s",
		DeviceTPMSettingsEndpoint,
		url.PathEscape(fmt.Sprintf("%d", key.ValueInt32())),
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

	tflog.Debug(ctx, "TPM settings updated successfully for device: "+data.Name.ValueString())
	return nil
}

// readTPMSettings reads TPM settings for the device.
func (da *DeviceApi) readTPMSettings(ctx context.Context, data *deviceResourceModel, readKeyOnly bool) error {
	tflog.Debug(ctx, "Reading TPM settings for device: "+data.Name.ValueString())

	// Call the API and check the response
	apiResp, err := da.client.Get(DeviceTPMSettingsEndpoint, &vergeio.Options{Fields: "most", Filter: fmt.Sprintf("machine_device eq %s", data.Key.ValueString())})
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	// Decode the API response
	var tpmSettingsAPIResp []DeviceTPMSettingsAPIModel
	if err := json.NewDecoder(apiResp.Body).Decode(&tpmSettingsAPIResp); err != nil {
		return errors.New("invalid format received for TPM : " + err.Error())
	}

	if len(tpmSettingsAPIResp) > 0 {
		// read the key based on the machine device
		data.DeviceTPMSettingsModel.Key = types.Int32Value(tpmSettingsAPIResp[0].Key)
		data.DeviceTPMSettingsModel.MachineDevice = types.Int32Value(tpmSettingsAPIResp[0].MachineDevice)
		if !readKeyOnly {
			data.DeviceTPMSettingsModel.Model = types.StringValue(tpmSettingsAPIResp[0].Model)
			data.DeviceTPMSettingsModel.Version = types.StringValue(tpmSettingsAPIResp[0].Version)
		}
	}

	tflog.Debug(ctx, "TPM settings read successfully for device: "+data.Name.ValueString())
	return nil
}

// UpdateNvidiaVGPUSettings updates Nvidia VGPU settings for the device.
func (da *DeviceApi) updateNvidiaVGPUSettings(ctx context.Context, data *deviceResourceModel, key types.Int32) error {
	tflog.Debug(ctx, "Updating Nvidia VGPU settings for device: "+data.Name.ValueString())

	// Prepare the API data packet
	apiData := map[string]interface{}{
		"profile_type": data.DeviceNvidiaVGPUSettingsModel.ProfileType.ValueString(),
		// "attach_drivers":     data.DeviceNvidiaVGPUSettingsModel.AttachDrivers.ValueBool(),
		"frame_rate_limiter": data.DeviceNvidiaVGPUSettingsModel.FrameRateLimiter.ValueInt32(),
		"disable_vnc":        data.DeviceNvidiaVGPUSettingsModel.DisableVNC.ValueBool(),
		"enable_uvm":         data.DeviceNvidiaVGPUSettingsModel.EnableUVM.ValueBool(),
		"enable_debugging":   data.DeviceNvidiaVGPUSettingsModel.EnableDebugging.ValueBool(),
		"enable_profiling":   data.DeviceNvidiaVGPUSettingsModel.EnableProfiling.ValueBool(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for Nvidia VGPU settings")
	}

	// Call the API and check the response
	apiResp, err := da.client.Put(fmt.Sprintf("%s/%s",
		DeviceNvidiaVGPUSettingsEndpoint,
		url.PathEscape(fmt.Sprintf("%d", key.ValueInt32())),
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

	tflog.Debug(ctx, " Nvidia VGPU settings updated successfully for device: "+data.Name.ValueString())
	return nil
}

// readNvidiaVGPUSettings reads Nvidia VGPU settings for the device.
func (da *DeviceApi) readNvidiaVGPUSettings(ctx context.Context, data *deviceResourceModel, readKeyOnly bool) error {
	tflog.Debug(ctx, "Reading Nvidia VGPU settings for device: "+data.Name.ValueString())

	// Call the API and check the response
	apiResp, err := da.client.Get(DeviceNvidiaVGPUSettingsEndpoint, &vergeio.Options{Fields: "most", Filter: fmt.Sprintf("machine_device eq %s", data.Key.ValueString())})
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from the API %d", apiResp.StatusCode)
	}

	// Decode the API response
	var nvidiaVGPUSettingsAPIResp []DeviceNvidiaVGPUSettingsAPIModel
	if err := json.NewDecoder(apiResp.Body).Decode(&nvidiaVGPUSettingsAPIResp); err != nil {
		return errors.New("invalid format received for Nvidia VGPU : " + err.Error())
	}

	if len(nvidiaVGPUSettingsAPIResp) > 0 {
		// read the key based on the machine device
		data.DeviceNvidiaVGPUSettingsModel.Key = types.Int32Value(nvidiaVGPUSettingsAPIResp[0].Key)
		data.DeviceNvidiaVGPUSettingsModel.MachineDevice = types.Int32Value(nvidiaVGPUSettingsAPIResp[0].MachineDevice)
		if !readKeyOnly {
			data.DeviceNvidiaVGPUSettingsModel.ProfileType = types.StringValue(nvidiaVGPUSettingsAPIResp[0].ProfileType)
			// data.DeviceNvidiaVGPUSettingsModel.AttachDrivers = types.BoolValue(nvidiaVGPUSettingsAPIResp[0].AttachDrivers)
			data.DeviceNvidiaVGPUSettingsModel.FrameRateLimiter = types.Int32Value(nvidiaVGPUSettingsAPIResp[0].FrameRateLimiter)
			data.DeviceNvidiaVGPUSettingsModel.DisableVNC = types.BoolValue(nvidiaVGPUSettingsAPIResp[0].DisableVNC)
			data.DeviceNvidiaVGPUSettingsModel.EnableUVM = types.BoolValue(nvidiaVGPUSettingsAPIResp[0].EnableUVM)
			data.DeviceNvidiaVGPUSettingsModel.EnableDebugging = types.BoolValue(nvidiaVGPUSettingsAPIResp[0].EnableDebugging)
			data.DeviceNvidiaVGPUSettingsModel.EnableProfiling = types.BoolValue(nvidiaVGPUSettingsAPIResp[0].EnableProfiling)
		}
	}

	tflog.Debug(ctx, "Nvidia VGPU settings read successfully for device: "+data.Name.ValueString())
	return nil
}
