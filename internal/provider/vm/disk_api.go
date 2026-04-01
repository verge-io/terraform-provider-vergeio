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
	"github.com/verge-io/govergeos"
)

// Ensure provider defined types fully satisfy framework interfaces.
type diskResourceModel struct {
	Key                 types.String `tfsdk:"key"`
	Machine             types.Int32  `tfsdk:"machine"`
	Name                types.String `tfsdk:"name"`
	Description         types.String `tfsdk:"description"`
	Interface           types.String `tfsdk:"interface"`
	Media               types.String `tfsdk:"media"`
	MediaSource         types.Int32  `tfsdk:"media_source"`
	DiskSize            types.Int64  `tfsdk:"disksize"`
	PreferredTier       types.String `tfsdk:"preferred_tier"`
	Enabled             types.Bool   `tfsdk:"enabled"`
	ReadOnly            types.Bool   `tfsdk:"readonly"`
	Serial              types.String `tfsdk:"serial"`
	Asset               types.String `tfsdk:"asset"`
	OrderId             types.Int32  `tfsdk:"orderid"`
	PreserveDriveFormat types.Bool   `tfsdk:"preserve_drive_format"`
}

// API resource model.
type diskAPIResourceModel struct {
	Key                 string `json:"$key,omitempty"`
	Machine             int32  `json:"machine,omitempty"`
	Name                string `json:"name,omitempty"`
	Description         string `json:"description,omitempty"`
	Interface           string `json:"interface,omitempty"`
	Media               string `json:"media,omitempty"`
	MediaSource         int32  `json:"media_source,omitempty"`
	DiskSize            int64  `json:"disksize,omitempty"`
	PreferredTier       string `json:"preferred_tier,omitempty"`
	Enabled             bool   `json:"enabled,omitempty"`
	ReadOnly            bool   `json:"readonly,omitempty"`
	Serial              string `json:"serial,omitempty"`
	Asset               string `json:"asset,omitempty"`
	OrderId             int32  `json:"orderid,omitempty"`
	PreserveDriveFormat bool   `json:"preserve_drive_format,omitempty"`
}

// Power status of the disk.
type diskAPIPowerStatus struct {
	PowerState string `json:"powerstate,omitempty"`
}

// List of valid disk interfaces.
// getValidDiskInterfaces returns hardcoded list - DEPRECATED
// Use GetDiskInterfacesFromAPI for dynamic API-based validation
func getValidDiskInterfaces() []string {
	return []string{
		"virtio",
		"ide",
		"ahci",
		"lsi53c895a",
		"megasas",
		"megasas-gen2",
		"mptsas1068",
		"virtio-scsi",
		"virtio-scsi-dedicated",
	}
}

// GetDiskInterfacesFromAPI fetches the list of valid disk interfaces from the VergeOS API via SDK
func (da *DiskApi) GetDiskInterfacesFromAPI(ctx context.Context) ([]string, error) {
	// Use SDK schema service instead of manual endpoint construction
	interfacesMap, err := da.sdk.Schema.GetValidValues(ctx, "machine_drives", "interface")
	if err != nil {
		return nil, fmt.Errorf("unable to fetch valid disk interfaces from VergeOS API: %w", err)
	}
	
	// Convert map keys to slice of strings
	interfaces := make([]string, 0, len(interfacesMap))
	for iface := range interfacesMap {
		interfaces = append(interfaces, iface)
	}
	
	return interfaces, nil
}

// List of valid disk media.
func getValidDiskMedia() []string {
	return []string{
		"cdrom",
		"disk",
		"efidisk",
		"import",
		"clone",
		"nonpersistent",
	}
}

// API endpoint for disks.
const (
	DiskEndpoint = vergeio.APIEndpoint + "/machine_drives"
)

var _ vergeio.IClient = &DiskApi{}

func NewDiskApi(c *vergeio.Client) *DiskApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &DiskApi{
		name:   "Disk Api",
		client: c,
		sdk:    sdk,
	}
}

type DiskApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (da *DiskApi) Name() string {
	return da.name
}

// Create the Disk in the API.
func (da *DiskApi) createDisk(ctx context.Context, data *diskResourceModel) error {

	// Prepare the API data packet
	apiData := diskAPIResourceModel{
		Machine:             data.Machine.ValueInt32(),
		Name:                data.Name.ValueString(),
		Description:         data.Description.ValueString(),
		Interface:           data.Interface.ValueString(),
		Media:               data.Media.ValueString(),
		MediaSource:         data.MediaSource.ValueInt32(),
		DiskSize:            data.DiskSize.ValueInt64() * 1024 * 1024 * 1024,
		PreferredTier:       data.PreferredTier.ValueString(),
		Enabled:             data.Enabled.ValueBool(),
		ReadOnly:            data.ReadOnly.ValueBool(),
		Serial:              data.Serial.ValueString(),
		Asset:               data.Asset.ValueString(),
		OrderId:             data.OrderId.ValueInt32(),
		PreserveDriveFormat: data.PreserveDriveFormat.ValueBool(),
	}

	// Make a copy of the original data
	origData := *data

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for disk Item")
	}

	// Call the API and check the response
	apiResp, err := da.client.Post(DiskEndpoint, encodedBuffer)
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
	var diskAPIResp vergeio.VergeResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&diskAPIResp); err != nil {
		return fmt.Errorf("invalid format received for creating a disk %v", err)
	}

	// save into the Terraform state.
	data.Key = types.StringValue(diskAPIResp.Key)
	tflog.Debug(ctx, fmt.Sprintf("Created a disk with Id %v", data.Key.ValueString()))

	// Read it back from the API to get all the fields
	if readError := da.readDisk(ctx, data); readError != nil {
		return errors.New("Error reading the disk: " + readError.Error())
	}

	if data.Media.ValueString() == "import" {
		// Adding delay to allow the API to process the import prcess
		time.Sleep(5 * time.Second)

		importStatus := ""
		Retries := 1

		// and also check the status of the import
		da.checkDiskPowerState(ctx, data.Key.ValueString(), &importStatus)
		tflog.Debug(ctx, fmt.Sprintf("Current import status is %v", importStatus))

		for strings.ToLower(importStatus) == "importing" {

			// Wait for a short period before checking the status again
			time.Sleep(5 * time.Second)
			Retries += 1

			// We are only going to retry 10 times before giving up
			if Retries > 10 {
				return fmt.Errorf("failed to import disk after %d retries", Retries)
			}

			// Check the status again
			da.checkDiskPowerState(ctx, data.Key.ValueString(), &importStatus)
			tflog.Debug(ctx, fmt.Sprintf("Current import status is %v", importStatus))

			continue
		}

		// Import has been finished, now resize the disk if needed
		tflog.Debug(ctx, fmt.Sprintf("Resizing the imported disk from %v to %v", origData.DiskSize.ValueInt64(), data.DiskSize.ValueInt64()))
		if data.DiskSize != origData.DiskSize {

			// Call the update API to resize the disk
			if err := da.updateDisk(ctx, &origData, data); err != nil {
				return fmt.Errorf("failed to resize disk after import: %v", err)
			}
		}
	}

	return nil
}

// Update the Disk in the API.
func (da *DiskApi) updateDisk(ctx context.Context, planData *diskResourceModel, stateData *diskResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := diskAPIResourceModel{
		Machine:             vergeio.Int32ToNil(planData.Machine, stateData.Machine, 0),
		Name:                vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Description:         vergeio.StringToNil(planData.Description, stateData.Description, ""),
		Interface:           vergeio.StringToNil(planData.Interface, stateData.Interface, ""),
		DiskSize:            vergeio.Int64ToNil(planData.DiskSize, stateData.DiskSize, 0) * 1024 * 1024 * 1024,
		PreferredTier:       vergeio.StringToNil(planData.PreferredTier, stateData.PreferredTier, ""),
		Enabled:             vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
		ReadOnly:            vergeio.BoolToNil(planData.ReadOnly, stateData.ReadOnly, false),
		Serial:              vergeio.StringToNil(planData.Serial, stateData.Serial, ""),
		Asset:               vergeio.StringToNil(planData.Asset, stateData.Asset, ""),
		OrderId:             vergeio.Int32ToNil(planData.OrderId, stateData.OrderId, 0),
		PreserveDriveFormat: vergeio.BoolToNil(planData.PreserveDriveFormat, stateData.PreserveDriveFormat, false),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Call the API and check the response
	apiResp, err := da.client.Put(fmt.Sprintf("%s/%s",
		DiskEndpoint,
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
	tflog.Debug(ctx, fmt.Sprintf("Updated disk %v", apiData))

	defer apiResp.Body.Close()

	// Read data into the model to get all the attributes
	if readDataError := da.readDisk(ctx, stateData); readDataError != nil {
		return fmt.Errorf("error reading the disk %v", readDataError)
	}

	return nil
}

// Read the Disk/drive from the API.
func (da *DiskApi) readDisk(ctx context.Context, data *diskResourceModel) error {

	tflog.Debug(ctx, "Reading the disk data")

	// Call the Get API with the disk id and get the fields we need
	// most fields are not returned by default
	apiResp, err := da.client.Get(fmt.Sprintf("%s/%s",
		DiskEndpoint,
		url.PathEscape(data.Key.ValueString()),
	), &vergeio.Options{Fields: "machine,name,disksize,interface,media,description,enabled,serial,media_source,preferred_tier,readonly,preserve_drive_format,asset,orderid"})

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

	tflog.Debug(ctx, fmt.Sprintf("Finish reading the disk %v", apiResp.Body))

	// Decode the API response
	var diskAPIResp diskAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&diskAPIResp); err != nil {
		return fmt.Errorf("invalid format received for Item %v", err)
	}

	// save into the resource model
	data.Machine = types.Int32Value(diskAPIResp.Machine)
	data.Name = types.StringValue(diskAPIResp.Name)
	data.Description = types.StringValue(diskAPIResp.Description)
	data.Interface = types.StringValue(diskAPIResp.Interface)
	data.DiskSize = types.Int64Value(diskAPIResp.DiskSize / (1024 * 1024 * 1024))
	data.PreferredTier = types.StringValue(diskAPIResp.PreferredTier)
	data.Enabled = types.BoolValue(diskAPIResp.Enabled)
	data.ReadOnly = types.BoolValue(diskAPIResp.ReadOnly)
	data.Serial = types.StringValue(diskAPIResp.Serial)
	data.Asset = types.StringValue(diskAPIResp.Asset)
	data.OrderId = types.Int32Value(diskAPIResp.OrderId)
	data.PreserveDriveFormat = types.BoolValue(diskAPIResp.PreserveDriveFormat)

	tflog.Debug(ctx, fmt.Sprintf("Finish reading the disk %v", data.Key.ValueString()))

	return nil
}

// Delete the Disk from the API.
func (da *DiskApi) deleteDisk(ctx context.Context, data *diskResourceModel, vmId types.String) error {

	tflog.Debug(ctx, "Deleting the disk")

	// VM API for hotplugging via VM Actions
	vmApi := NewVMApi(da.client)

	// Call the API to check if the vm is in a power state that can be deleted
	var powerState string = ""

	if err := da.checkDiskPowerState(ctx, data.Key.ValueString(), &powerState); err != nil {
		return fmt.Errorf("error checking disk power state %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Current disk power state is %v", powerState))

	// Call the API to check if the vm is in a power state that can be deleted
	for strings.ToLower(powerState) != "offline" {
		Retries := 1

		// Power the vm off
		if err := vmApi.hotplugDrive(ctx, data.Key.ValueString(), vmId); err != nil {
			return fmt.Errorf("failed to hotplug drive: %v", err)
		}

		// Wait for a short period to allow the kill operation to complete
		time.Sleep(2 * time.Second)

		// Call the API to check if the vm is in a power state that can be deleted
		if err := da.checkDiskPowerState(ctx, data.Key.ValueString(), &powerState); err != nil {
			return fmt.Errorf("error checking disk power state %v", err)
		}

		Retries += 1

		// We are only going to retry 5 times before giving up
		if Retries > 5 {
			return fmt.Errorf("failed to kill VM before deletion after %d retries", Retries)
		}
		continue
	}

	// Call the Get API with the user id and Proceed with user deletion
	_, err := da.client.Delete(fmt.Sprintf("%s/%s",
		DiskEndpoint,
		url.PathEscape(data.Key.ValueString())))

	// error checking
	if err != nil {
		return errors.New("Error deleting the disk: " + err.Error())
	}

	tflog.Debug(ctx, "Disk was successfully deleted")

	return nil
}

// Update, Create, Delete the Disk in the API.
// This method is called from VM update method.
func (da *DiskApi) syncDisks(ctx context.Context, planData *[]*diskResourceModel, stateData *[]*diskResourceModel, machineId types.Int32, vmId types.String) error {

	tflog.Debug(ctx, "Syncing disks: Starting deletion")

	// Delete the disks that are not in the plan
	var stateToBeDeleted []string // List of disks to be deleted

	for _, state := range *stateData {
		found := false
		for _, plan := range *planData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				found = true
				break
			}
		}
		if !found {
			if err := da.deleteDisk(ctx, state, vmId); err != nil {
				return fmt.Errorf("failed to delete disk: %v", err)
			}
			stateToBeDeleted = append(stateToBeDeleted, state.Name.ValueString())
		}
	}

	// Delete the disk record that are not in the state
	for _, name := range stateToBeDeleted {
		for i, state := range *stateData {
			if state.Name.ValueString() == name {
				*stateData = append((*stateData)[:i], (*stateData)[i+1:]...)
				break
			}
		}
	}

	tflog.Debug(ctx, "Syncing disks: Starting updation")

	// Compare the plan data with the state data and update
	for _, plan := range *planData {
		for _, state := range *stateData {
			if plan.Name.ValueString() == state.Name.ValueString() {
				if plan.Description.ValueString() != state.Description.ValueString() ||
					plan.Interface.ValueString() != state.Interface.ValueString() ||
					plan.Media.ValueString() != state.Media.ValueString() ||
					plan.MediaSource.ValueInt32() != state.MediaSource.ValueInt32() ||
					plan.DiskSize.ValueInt64() != state.DiskSize.ValueInt64() ||
					plan.PreferredTier.ValueString() != state.PreferredTier.ValueString() ||
					plan.Enabled.ValueBool() != state.Enabled.ValueBool() ||
					plan.ReadOnly.ValueBool() != state.ReadOnly.ValueBool() ||
					plan.Serial.ValueString() != state.Serial.ValueString() ||
					plan.Asset.ValueString() != state.Asset.ValueString() ||
					plan.OrderId.ValueInt32() != state.OrderId.ValueInt32() ||
					plan.PreserveDriveFormat.ValueBool() != state.PreserveDriveFormat.ValueBool() {
					if err := da.updateDisk(ctx, plan, state); err != nil {
						return fmt.Errorf("failed to update disk: %v", err)
					}
				}
			}
		}
	}

	tflog.Debug(ctx, "Syncing disks: Starting insertion")

	// Create the disks that are not in the state
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
			if err := da.createDisk(ctx, plan); err != nil {
				return fmt.Errorf("failed to create disk: %v", err)
			}
			if err := da.readDisk(ctx, plan); err != nil {
				return fmt.Errorf("failed to read disk during the sync: %v", err)
			}
			// insert plan in the stateData in a particular order
			da.insertDiskInState(i, plan, stateData)
		}
	}

	return nil
}

// Manually insert plan in the stateData in a particular order.
func (da *DiskApi) insertDiskInState(insertAt int, planData *diskResourceModel, stateData *[]*diskResourceModel) {
	// If it's at the end ten just append
	if insertAt >= len(*stateData) {
		*stateData = append(*stateData, planData)
	} else {
		// insert plan in the stateData in a particular order
		for i := range *stateData {
			if i == insertAt {
				*stateData = append((*stateData)[:i], append([]*diskResourceModel{planData}, (*stateData)[i:]...)...)
				break
			}
		}
	}
}

// Check the power state of the disk
func (da *DiskApi) checkDiskPowerState(ctx context.Context, key string, powerState *string) error {

	tflog.Debug(ctx, fmt.Sprintf("Checking the power state of the disk %v", key))

	// Send the kill action request to the vnet_actions endpoint
	apiResp, err := da.client.Get(fmt.Sprintf("%s/%s",
		DiskEndpoint,
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

	// Decode the API response
	var diskAPIResp diskAPIPowerStatus
	if err := json.NewDecoder(apiResp.Body).Decode(&diskAPIResp); err != nil {
		return errors.New("invalid format received for disk power state call")
	}

	// save into the resource model
	*powerState = diskAPIResp.PowerState

	tflog.Debug(ctx, fmt.Sprintf("Disk status read from API: %v", diskAPIResp.PowerState))

	return nil
}

// Hotplug the disk so that it can be deleted
func (va *VMApi) hotplugDrive(ctx context.Context, driveId string, vmId types.String) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the hotplug drive API for the drive %v", driveId))

	// Create the action payload according to vnet_actions schema
	params := VMActionParams{
		Device: driveId,
		Unplug: true,
	}
	intVMId, err := strconv.Atoi(vmId.ValueString())
	if err != nil {
		return fmt.Errorf("invalid VM ID format: %v", err)
	}
	actionPayload := VMAction{
		VM:     int32(intVMId),
		Action: "hotplugdrive",
		Params: params,
	}
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
		return fmt.Errorf("failed to hotplug drive: status code %v", req.StatusCode)
	}

	return nil
}
