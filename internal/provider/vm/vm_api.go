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

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	VMEndpoint       = vergeio.APIEndpoint + "/vms"
	VMActionEndpoint = vergeio.APIEndpoint + "/vm_actions"
)

var _ vergeio.IClient = &VMApi{}

func NewVMApi(c *vergeio.Client) *VMApi {
	return &VMApi{
		name:   "VM Api",
		client: c,
	}
}

type VMApi struct {
	name   string
	client *vergeio.Client
}

func (va *VMApi) Name() string {
	return va.name
}

// Valid OS families.
func getValidOSFamilies() []string {
	return []string{
		"linux",
		"windows",
		"freebsd",
		"other",
	}
}

// Valid machine types.
func getValidMachineTypes() []string {
	return []string{"pc",
		"pc-i440fx-2.7",
		"pc-i440fx-2.8",
		"pc-i440fx-2.9",
		"pc-i440fx-2.10",
		"pc-i440fx-2.11",
		"pc-i440fx-2.12",
		"pc-i440fx-3.0",
		"pc-i440fx-3.1",
		"pc-i440fx-4.0",
		"pc-i440fx-4.1",
		"pc-i440fx-4.2",
		"pc-i440fx-5.0",
		"pc-i440fx-5.1",
		"pc-i440fx-5.2",
		"pc-i440fx-6.0",
		"pc-i440fx-6.1",
		"pc-i440fx-6.2",
		"pc-i440fx-7.0",
		"pc-i440fx-7.1",
		"pc-i440fx-7.2",
		"pc-i440fx-8.0",
		"pc-i440fx-8.1",
		"pc-i440fx-8.2",
		"pc-i440fx-9.0",
		"q35",
		"pc-q35-2.7",
		"pc-q35-2.8",
		"pc-q35-2.9",
		"pc-q35-2.10",
		"pc-q35-2.11",
		"pc-q35-2.12",
		"pc-q35-3.0",
		"pc-q35-3.1",
		"pc-q35-4.0",
		"pc-q35-4.1",
		"pc-q35-4.2",
		"pc-q35-5.0",
		"pc-q35-5.1",
		"pc-q35-5.2",
		"pc-q35-6.0",
		"pc-q35-6.1",
		"pc-q35-6.2",
		"pc-q35-7.0",
		"pc-q35-7.1",
		"pc-q35-7.2",
		"pc-q35-8.0",
		"pc-q35-8.1",
		"pc-q35-8.2",
		"pc-q35-9.0",
		"yottabyte",
	}
}

type VMAPIDataSourceModel struct {
	Id          int32  `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	Key         int32  `json:"$key,omitempty"`
	IsSnapshot  bool   `json:"is_snapshot,omitempty"`
	CPUType     string `json:"cpu_type,omitempty"`
	MachineType string `json:"machine_type,omitempty"`
	OSFamily    string `json:"os_family,omitempty"`
	UEFI        bool   `json:"uefi,omitempty"`
	Machine     struct {
		Drives []*VMDriveAPIDataSourceModel `json:"drives,omitempty"`
		Nics   []*VMNICAPIDataSourceModel   `json:"nics,omitempty"`
	} `json:"machine,omitempty"`
}

type VMDriveAPIDataSourceModel struct {
	Key           int32                              `json:"$key,omitempty"`
	Name          string                             `json:"name,omitempty"`
	Interface     string                             `json:"interface,omitempty"`
	Media         string                             `json:"media,omitempty"`
	Description   string                             `json:"description,omitempty"`
	PreferredTier string                             `json:"preferred_tier,omitempty"`
	MediaSource   *VMDriveMediaSourceDataSourceModel `json:"media_source,omitempty"`
}

type VMDriveMediaSourceDataSourceModel struct {
	Key            int32 `json:"$key,omitempty"`
	UsedBytes      int64 `json:"used_bytes,omitempty"`
	AllocatedBytes int64 `json:"allocated_bytes,omitempty"`
	Filesize       int64 `json:"filesize,omitempty"`
}

type VMNICAPIDataSourceModel struct {
	Key       int32  `json:"$key,omitempty"`
	Name      string `json:"name,omitempty"`
	Interface string `json:"interface,omitempty"`
	Vnet      string `json:"vnet,omitempty"`
	Status    string `json:"status,omitempty"`
	Ipaddress string `json:"ipaddress,omitempty"`
}

// CloudInitFile represents a cloud-init file with name and contents.
type CloudInitFileAPI struct {
	Name     string `json:"name"`
	Contents string `json:"contents"`
}

// VMAPIResourceModel describes the data model received from the Verge API.
type VMAPIResourceModel struct {
	Id                  string             `json:"id,omitempty"`
	Machine             int32              `json:"machine,omitempty"`
	Name                string             `json:"name,omitempty"`
	Cluster             string             `json:"cluster,omitempty"`
	Description         string             `json:"description,omitempty"`
	Enabled             bool               `json:"enabled,omitempty"`
	MachineType         string             `json:"machine_type,omitempty"`
	AllowHotplug        bool               `json:"allow_hotplug,omitempty"`
	DisablePowercycle   bool               `json:"disable_powercycle,omitempty"`
	CPUCores            int32              `json:"cpu_cores,omitempty"`
	CPUType             string             `json:"cpu_type,omitempty"`
	RAM                 int32              `json:"ram,omitempty"`
	Console             string             `json:"console,omitempty"`
	Display             string             `json:"display,omitempty"`
	Video               string             `json:"video,omitempty"`
	Sound               string             `json:"sound,omitempty"`
	OSFamily            string             `json:"os_family,omitempty"`
	OSDescription       string             `json:"os_description,omitempty"`
	RTCBase             string             `json:"rtc_base,omitempty"`
	BootOrder           string             `json:"boot_order,omitempty"`
	ConsolePassEnabled  bool               `json:"console_pass_enabled,omitempty"`
	ConsolePass         string             `json:"console_pass,omitempty"`
	USBTablet           bool               `json:"usb_tablet,omitempty"`
	UEFI                bool               `json:"uefi,omitempty"`
	SecureBoot          bool               `json:"secure_boot,omitempty"`
	SerialPort          bool               `json:"serial_port,omitempty"`
	BootDelay           int32              `json:"boot_delay,omitempty"`
	PreferredNode       string             `json:"preferred_node,omitempty"`
	SnapshotProfile     string             `json:"snapshot_profile,omitempty"`
	CloudInitDataSource string             `json:"cloudinit_datasource,omitempty"`
	CloudInitFiles      []CloudInitFileAPI `json:"cloudinit_files,omitempty"`
	PowerState          string             `json:"powerstate,omitempty"`
	GuestAgent          bool               `json:"guest_agent,omitempty"`
	HAGroup             string             `json:"ha_group,omitempty"`
	Advanced            string             `json:"advanced,omitempty"`
}

// VNetAction represents the structure for virtual vm action requests.
type VMAction struct {
	VM     int32          `json:"vm,omitempty"`
	Action string         `json:"action,omitempty"`
	Params VMActionParams `json:"params,omitempty"`
}

type VMActionParams struct {
	Device string `json:"device,omitempty"`
	Unplug bool   `json:"unplug,omitempty"`
}

type NewResponse struct {
	Key      string             `json:"$key,omitempty"`
	Response NewResponseMachine `json:"response,omitempty"`
}

type NewResponseMachine struct {
	Machine string `json:"machine,omitempty"`
}

// Create a new VM.
func (va *VMApi) CreateVM(ctx context.Context, data *VMResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := VMAPIResourceModel{
		Machine:             data.Machine.ValueInt32(),
		Name:                data.Name.ValueString(),
		Cluster:             data.Cluster.ValueString(),
		Description:         data.Description.ValueString(),
		Enabled:             data.Enabled.ValueBool(),
		MachineType:         data.MachineType.ValueString(),
		AllowHotplug:        data.AllowHotplug.ValueBool(),
		DisablePowercycle:   data.DisablePowercycle.ValueBool(),
		CPUCores:            data.CPUCores.ValueInt32(),
		CPUType:             data.CPUType.ValueString(),
		RAM:                 data.RAM.ValueInt32(),
		Console:             data.Console.ValueString(),
		Display:             data.Display.ValueString(),
		Video:               data.Video.ValueString(),
		Sound:               data.Sound.ValueString(),
		OSFamily:            data.OSFamily.ValueString(),
		OSDescription:       data.OSDescription.ValueString(),
		RTCBase:             data.RTCBase.ValueString(),
		BootOrder:           data.BootOrder.ValueString(),
		ConsolePassEnabled:  data.ConsolePassEnabled.ValueBool(),
		ConsolePass:         data.ConsolePass.ValueString(),
		USBTablet:           data.USBTablet.ValueBool(),
		UEFI:                data.UEFI.ValueBool(),
		SecureBoot:          data.SecureBoot.ValueBool(),
		SerialPort:          data.SerialPort.ValueBool(),
		BootDelay:           data.BootDelay.ValueInt32(),
		PreferredNode:       data.PreferredNode.ValueString(),
		SnapshotProfile:     data.SnapshotProfile.ValueString(),
		CloudInitDataSource: data.CloudInitDataSource.ValueString(),
		PowerState:          data.PowerState.ValueString(),
		GuestAgent:          data.GuestAgent.ValueBool(),
		HAGroup:             data.HAGroup.ValueString(),
		Advanced:            data.Advanced.ValueString(),
	}

	// Add the cloud init files
	if data.CloudInitFiles != nil {
		for _, cloudInitFile := range data.CloudInitFiles {
			apiData.CloudInitFiles = append(apiData.CloudInitFiles, CloudInitFileAPI{
				Name:     cloudInitFile.Name.ValueString(),
				Contents: cloudInitFile.Contents.ValueString(),
			})
		}
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Time to call the API
	apiResp, err := va.client.Post(VMEndpoint, encodedBuffer)
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
	var vmAPIResp NewResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&vmAPIResp); err != nil {
		return fmt.Errorf("invalid format received for VM Item: %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("VM Key after creation %v", vmAPIResp.Response))

	// save into the Terraform state.
	data.Id = types.StringValue(vmAPIResp.Key)

	tflog.Debug(ctx, fmt.Sprintf("VM Id after creation %v", data.Id))

	// Read the VM from the API to get all the data.
	if readError := va.readVM(ctx, data); readError != nil {
		return errors.New("Error reading the VM: " + readError.Error())
	}

	return nil
}

// Update the VM.
func (va *VMApi) UpdateVM(ctx context.Context, planData *VMResourceModel, stateData *VMResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := VMAPIResourceModel{
		Id: vergeio.StringToNil(planData.Id, stateData.Id, ""),
		// Machine is read only
		Name:                vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Cluster:             vergeio.StringToNil(planData.Cluster, stateData.Cluster, ""),
		Description:         vergeio.StringToNil(planData.Description, stateData.Description, ""),
		Enabled:             vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
		MachineType:         vergeio.StringToNil(planData.MachineType, stateData.MachineType, ""),
		AllowHotplug:        vergeio.BoolToNil(planData.AllowHotplug, stateData.AllowHotplug, false),
		DisablePowercycle:   vergeio.BoolToNil(planData.DisablePowercycle, stateData.DisablePowercycle, false),
		CPUCores:            vergeio.Int32ToNil(planData.CPUCores, stateData.CPUCores, 0),
		CPUType:             vergeio.StringToNil(planData.CPUType, stateData.CPUType, ""),
		RAM:                 vergeio.Int32ToNil(planData.RAM, stateData.RAM, 0),
		Console:             vergeio.StringToNil(planData.Console, stateData.Console, ""),
		Display:             vergeio.StringToNil(planData.Display, stateData.Display, ""),
		Video:               vergeio.StringToNil(planData.Video, stateData.Video, ""),
		Sound:               vergeio.StringToNil(planData.Sound, stateData.Sound, ""),
		OSFamily:            vergeio.StringToNil(planData.OSFamily, stateData.OSFamily, ""),
		OSDescription:       vergeio.StringToNil(planData.OSDescription, stateData.OSDescription, ""),
		RTCBase:             vergeio.StringToNil(planData.RTCBase, stateData.RTCBase, ""),
		BootOrder:           vergeio.StringToNil(planData.BootOrder, stateData.BootOrder, ""),
		ConsolePassEnabled:  vergeio.BoolToNil(planData.ConsolePassEnabled, stateData.ConsolePassEnabled, false),
		ConsolePass:         vergeio.StringToNil(planData.ConsolePass, stateData.ConsolePass, ""),
		USBTablet:           vergeio.BoolToNil(planData.USBTablet, stateData.USBTablet, false),
		UEFI:                vergeio.BoolToNil(planData.UEFI, stateData.UEFI, false),
		SecureBoot:          vergeio.BoolToNil(planData.SecureBoot, stateData.SecureBoot, false),
		SerialPort:          vergeio.BoolToNil(planData.SerialPort, stateData.SerialPort, false),
		BootDelay:           vergeio.Int32ToNil(planData.BootDelay, stateData.BootDelay, 0),
		PreferredNode:       vergeio.StringToNil(planData.PreferredNode, stateData.PreferredNode, ""),
		SnapshotProfile:     vergeio.StringToNil(planData.SnapshotProfile, stateData.SnapshotProfile, ""),
		CloudInitDataSource: vergeio.StringToNil(planData.CloudInitDataSource, stateData.CloudInitDataSource, ""),
		PowerState:          vergeio.StringToNil(planData.PowerState, stateData.PowerState, ""),
		GuestAgent:          vergeio.BoolToNil(planData.GuestAgent, stateData.GuestAgent, false),
		HAGroup:             vergeio.StringToNil(planData.HAGroup, stateData.HAGroup, ""),
		Advanced:            vergeio.StringToNil(planData.Advanced, stateData.Advanced, ""),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return fmt.Errorf("invalid format received for VM Item: %v", err)
	}

	// Time to call the API
	apiResp, err := va.client.Put(fmt.Sprintf("%s/%s",
		VMEndpoint,
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

	defer apiResp.Body.Close()

	return nil
}

// Delete the VM from the API.
func (va *VMApi) deleteVM(ctx context.Context, data *VMResourceModel) error {

	tflog.Debug(ctx, "Deleting the vm")

	// Call the Get API with the VM id and Proceed with user deletion
	_, err := va.client.Delete(fmt.Sprintf("%s/%s",
		VMEndpoint,
		url.PathEscape(data.Id.ValueString())))

	// error checking
	if err != nil {
		return errors.New("Error deleting the vm: " + err.Error())
	}

	tflog.Debug(ctx, "VM was successfully deleted")

	return nil
}

func (va *VMApi) checkVMPowerState(ctx context.Context, data *VMResourceModel) error {

	// Send the kill action request to the vnet_actions endpoint
	apiResp, err := va.client.Get(fmt.Sprintf("%s/%s",
		VMEndpoint,
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
	var vmAPIResp VMAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&vmAPIResp); err != nil {
		return fmt.Errorf("invalid format received for VM Item: %v", err)
	}

	// save into the resource model
	data.PowerState = types.StringValue(vmAPIResp.PowerState)

	tflog.Debug(ctx, fmt.Sprintf("VM status read from API: %v", data.PowerState.ValueString()))

	return nil
}

func (va *VMApi) killVM(ctx context.Context, data *VMResourceModel) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill VM API for VM %v", data.Id.ValueString()))

	// Convert vmID string to int
	vmIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid VM ID format: %v", err)
	}

	// Create the action payload according to vnet_actions schema
	actionPayload := VMAction{
		VM:     int32(vmIDInt),
		Action: "kill",
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
		return fmt.Errorf("failed to kill VM: status code %v", req.StatusCode)
	}

	return nil
}

// Read the VM from the API.
func (va *VMApi) readVM(ctx context.Context, data *VMResourceModel) error {

	tflog.Debug(ctx, "Reading the vm data")

	// Call the Get API with the vm id and get the fields we need
	// most fields are not returned by default
	apiResp, err := va.client.Get(fmt.Sprintf("%s/%s",
		VMEndpoint,
		url.PathEscape(data.Id.ValueString()),
	), &vergeio.Options{Fields: "id,machine,name,cluster,description,enabled,machine_type,allow_hotplug,disable_powercycle,cpu_cores,cpu_type,ram,console,display,video,sound,os_family,os_description,rtc_base,boot_order,console_pass_enabled,console_pass,usb_tablet,uefi,secure_boot,serial_port,boot_delay,preferred_node,snapshot_profile,cloudinit_datasource,ha_group,powerstate,guest_agent,advanced"})

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
	var vmAPIResp VMAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&vmAPIResp); err != nil {
		return fmt.Errorf("invalid format received for VM Item: %v", err)
	}

	// save into the resource model
	data.Machine = types.Int32Value(vmAPIResp.Machine)
	data.Name = types.StringValue(vmAPIResp.Name)
	data.Cluster = types.StringValue(vmAPIResp.Cluster)
	data.Description = types.StringValue(vmAPIResp.Description)
	data.Enabled = types.BoolValue(vmAPIResp.Enabled)
	data.MachineType = types.StringValue(vmAPIResp.MachineType)
	data.AllowHotplug = types.BoolValue(vmAPIResp.AllowHotplug)
	data.DisablePowercycle = types.BoolValue(vmAPIResp.DisablePowercycle)
	data.CPUCores = types.Int32Value(vmAPIResp.CPUCores)
	data.CPUType = types.StringValue(vmAPIResp.CPUType)
	data.RAM = types.Int32Value(vmAPIResp.RAM)
	data.Console = types.StringValue(vmAPIResp.Console)
	data.Display = types.StringValue(vmAPIResp.Display)
	data.Video = types.StringValue(vmAPIResp.Video)
	data.Sound = types.StringValue(vmAPIResp.Sound)
	data.OSFamily = types.StringValue(vmAPIResp.OSFamily)
	data.OSDescription = types.StringValue(vmAPIResp.OSDescription)
	data.RTCBase = types.StringValue(vmAPIResp.RTCBase)
	data.BootOrder = types.StringValue(vmAPIResp.BootOrder)
	data.ConsolePassEnabled = types.BoolValue(vmAPIResp.ConsolePassEnabled)
	data.ConsolePass = types.StringValue(vmAPIResp.ConsolePass)
	data.USBTablet = types.BoolValue(vmAPIResp.USBTablet)
	data.UEFI = types.BoolValue(vmAPIResp.UEFI)
	data.SecureBoot = types.BoolValue(vmAPIResp.SecureBoot)
	data.SerialPort = types.BoolValue(vmAPIResp.SerialPort)
	data.BootDelay = types.Int32Value(vmAPIResp.BootDelay)
	data.PreferredNode = types.StringValue(vmAPIResp.PreferredNode)
	data.SnapshotProfile = types.StringValue(vmAPIResp.SnapshotProfile)
	data.CloudInitDataSource = types.StringValue(vmAPIResp.CloudInitDataSource)
	data.GuestAgent = types.BoolValue(vmAPIResp.GuestAgent)
	data.HAGroup = types.StringValue(vmAPIResp.HAGroup)
	data.Advanced = types.StringValue(vmAPIResp.Advanced)

	if vmAPIResp.CloudInitFiles != nil {
		for _, cloudInitFileAPI := range vmAPIResp.CloudInitFiles {
			data.CloudInitFiles = append(data.CloudInitFiles, CloudInitFile{
				Name:     types.StringValue(cloudInitFileAPI.Name),
				Contents: types.StringValue(cloudInitFileAPI.Contents),
			})
		}
	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// Read the VM from the API.
func (va *VMApi) readVMs(ctx context.Context, data *VMDataSourceModel) error {

	tflog.Debug(ctx, "Reading the vm data")

	// Define the fields. Not all the fields are returned by default
	opts := vergeio.Options{Fields: "machine#$key as id, dashboard"} //"machine,name,$key,is_snapshot,cpu_type,machine_type,os_family,uefi"}

	// Build filter
	if fn := data.FilterName.ValueString(); fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn)
	}

	// Call the Get API with filter and fields options
	apiResp, err := va.client.Get(VMEndpoint,
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

	tflog.Debug(ctx, fmt.Sprintf("===Read the VMs %#v", apiResp.Body))

	// Decode the API response
	var vmAPIResp []VMAPIDataSourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&vmAPIResp); err != nil {
		return fmt.Errorf("invalid format received for VM Item: %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("===vmAPIResp after marshaling %v", vmAPIResp[0].Machine.Drives[0].MediaSource))

	// Filter the response for snapshots
	isSnapshotFilterSet := !data.IsSnapshot.IsNull()
	isSnapshotFilter := data.IsSnapshot.ValueBool()

	for i, vmAPIResp := range vmAPIResp {

		if isSnapshotFilterSet {
			if isSnapshotFilter != vmAPIResp.IsSnapshot {
				continue
			}
		}
		data.Vms = append(data.Vms, &VMModel{
			Id:          types.Int32Value(vmAPIResp.Id),
			Name:        types.StringValue(vmAPIResp.Name),
			Key:         types.Int32Value(vmAPIResp.Key),
			IsSnapshot:  types.BoolValue(vmAPIResp.IsSnapshot),
			CPUType:     types.StringValue(vmAPIResp.CPUType),
			MachineType: types.StringValue(vmAPIResp.MachineType),
			OSFamily:    types.StringValue(vmAPIResp.OSFamily),
			UEFI:        types.BoolValue(vmAPIResp.UEFI),
		})

		if vmAPIResp.Machine.Drives != nil {
			var drives []*VMDriveModel

			for _, vmDrive := range vmAPIResp.Machine.Drives {
				drive := &VMDriveModel{
					Key:           types.Int32Value(vmDrive.Key),
					Name:          types.StringValue(vmDrive.Name),
					Interface:     types.StringValue(vmDrive.Interface),
					Media:         types.StringValue(vmDrive.Media),
					Description:   types.StringValue(vmDrive.Description),
					PreferredTier: types.StringValue(vmDrive.PreferredTier),
				}
				if vmDrive.MediaSource != nil {
					msBlock := vmDrive.MediaSource
					mediaSource := VMDriveMediasourceModel{
						Key:            types.Int32Value(msBlock.Key),
						UsedBytes:      types.Int64Value(vmDrive.MediaSource.UsedBytes),
						AllocatedBytes: types.Int64Value(vmDrive.MediaSource.AllocatedBytes),
						Filesize:       types.Int64Value(vmDrive.MediaSource.Filesize),
					}
					drive.MediaSource = &mediaSource
				}

				drives = append(drives, drive)
			}
			data.Vms[i].Drives = drives
		}

		if vmAPIResp.Machine.Nics != nil {
			var nics []*VMNicModel

			for _, vmNic := range vmAPIResp.Machine.Nics {
				nic := &VMNicModel{
					Key:       types.Int32Value(vmNic.Key),
					Name:      types.StringValue(vmNic.Name),
					Interface: types.StringValue(vmNic.Interface),
					Vnet:      types.StringValue(vmNic.Vnet),
					Status:    types.StringValue(vmNic.Status),
					Ipaddress: types.StringValue(vmNic.Ipaddress),
				}
				nics = append(nics, nic)
			}
			data.Vms[i].Nics = nics
		}
	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
