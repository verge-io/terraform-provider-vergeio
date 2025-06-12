// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"

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
	Key        int32  `json:"$key,omitempty"`
	Name       string `json:"name,omitempty"`
	Interface  string `json:"interface,omitempty"`
	Vnet       string `json:"vnet,omitempty"`
	Status     string `json:"status,omitempty"`
	Ipaddress  string `json:"ipaddress,omitempty"`
	MacAddress string `json:"macaddress,omitempty"`
	// ExternalIP string `json:"external_ip,omitempty"`
}

// CloudInitFile represents a cloud-init file with name and contents.
type CloudInitFileAPI struct {
	Name     string `json:"name"`
	Contents string `json:"contents"`
}

type VMAPIGuestAgentModel struct {
	Machine struct {
		Status struct {
			AgentGuestInfo *VMAPIAgentGuestInfoModel `json:"agent_guest_info,omitempty"`
		} `json:"status,omitempty"`
	} `json:"machine,omitempty"`
}

type VMAPIAgentGuestInfoModel struct {
	Network []*VMAPIGuestAgentNetworkModel `json:"network,omitempty"`
}
type VMAPIGuestAgentNetworkModel struct {
	Name        string                        `json:"name,omitempty"`
	IPAddresses []*VMAPIGuestAgentIPAddresses `json:"ip-addresses,omitempty"`
}

type VMAPIGuestAgentIPAddresses struct {
	IPAddressType string `json:"ip-address-type,omitempty"`
	IPAddress     string `json:"ip-address,omitempty"`
}

// VMAPIResourceModel describes the data model received from the Verge API.
type VMAPIResourceModel struct {
	Id                   string             `json:"id,omitempty"`
	Machine              int32              `json:"machine,omitempty"`
	Name                 string             `json:"name,omitempty"`
	Cluster              string             `json:"cluster,omitempty"`
	Description          string             `json:"description,omitempty"`
	Enabled              bool               `json:"enabled,omitempty"`
	MachineType          string             `json:"machine_type,omitempty"`
	AllowHotplug         bool               `json:"allow_hotplug,omitempty"`
	DisablePowercycle    bool               `json:"disable_powercycle,omitempty"`
	CPUCores             int32              `json:"cpu_cores,omitempty"`
	CPUType              string             `json:"cpu_type,omitempty"`
	RAM                  int32              `json:"ram,omitempty"`
	Console              string             `json:"console,omitempty"`
	Display              string             `json:"display,omitempty"`
	Video                string             `json:"video,omitempty"`
	Sound                string             `json:"sound,omitempty"`
	OSFamily             string             `json:"os_family,omitempty"`
	OSDescription        string             `json:"os_description,omitempty"`
	RTCBase              string             `json:"rtc_base,omitempty"`
	BootOrder            string             `json:"boot_order,omitempty"`
	ConsolePassEnabled   bool               `json:"console_pass_enabled,omitempty"`
	ConsolePass          string             `json:"console_pass,omitempty"`
	USBTablet            bool               `json:"usb_tablet,omitempty"`
	UEFI                 bool               `json:"uefi,omitempty"`
	SecureBoot           bool               `json:"secure_boot,omitempty"`
	SerialPort           bool               `json:"serial_port,omitempty"`
	BootDelay            int32              `json:"boot_delay,omitempty"`
	PreferredNode        string             `json:"preferred_node,omitempty"`
	SnapshotProfile      string             `json:"snapshot_profile,omitempty"`
	CloudInitDataSource  string             `json:"cloudinit_datasource,omitempty"`
	CloudInitFiles       []CloudInitFileAPI `json:"cloudinit_files,omitempty"`
	PowerState           bool               `json:"powerstate,omitempty"`
	GuestAgent           bool               `json:"guest_agent,omitempty"`
	HAGroup              string             `json:"ha_group,omitempty"`
	Advanced             string             `json:"advanced,omitempty"`
	NestedVirtualization bool               `json:"nested_virtualization"`
	DisableHypervisor    bool               `json:"disable_hypervisor"`
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

// VMPowerState represents the structure for virtual vm power state requests.
type VMPowerState struct {
	PowerState *bool `json:"powerstate,omitempty"`
}

// Create a new VM.
func (va *VMApi) CreateVM(ctx context.Context, data *VMResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := VMAPIResourceModel{
		Machine:              data.Machine.ValueInt32(),
		Name:                 data.Name.ValueString(),
		Cluster:              data.Cluster.ValueString(),
		Description:          data.Description.ValueString(),
		Enabled:              data.Enabled.ValueBool(),
		MachineType:          data.MachineType.ValueString(),
		AllowHotplug:         data.AllowHotplug.ValueBool(),
		DisablePowercycle:    data.DisablePowercycle.ValueBool(),
		CPUCores:             data.CPUCores.ValueInt32(),
		CPUType:              data.CPUType.ValueString(),
		RAM:                  data.RAM.ValueInt32(),
		Console:              data.Console.ValueString(),
		Display:              data.Display.ValueString(),
		Video:                data.Video.ValueString(),
		Sound:                data.Sound.ValueString(),
		OSFamily:             data.OSFamily.ValueString(),
		OSDescription:        data.OSDescription.ValueString(),
		RTCBase:              data.RTCBase.ValueString(),
		BootOrder:            data.BootOrder.ValueString(),
		ConsolePassEnabled:   data.ConsolePassEnabled.ValueBool(),
		ConsolePass:          data.ConsolePass.ValueString(),
		USBTablet:            data.USBTablet.ValueBool(),
		UEFI:                 data.UEFI.ValueBool(),
		SecureBoot:           data.SecureBoot.ValueBool(),
		SerialPort:           data.SerialPort.ValueBool(),
		BootDelay:            data.BootDelay.ValueInt32(),
		PreferredNode:        data.PreferredNode.ValueString(),
		SnapshotProfile:      data.SnapshotProfile.ValueString(),
		CloudInitDataSource:  data.CloudInitDataSource.ValueString(),
		PowerState:           data.PowerState.ValueBool(),
		GuestAgent:           data.GuestAgent.ValueBool(),
		HAGroup:              data.HAGroup.ValueString(),
		Advanced:             data.Advanced.ValueString(),
		NestedVirtualization: data.NestedVirtualization.ValueBool(),
		DisableHypervisor:    data.DisableHypervisor.ValueBool(),
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

	// If the power state is set to true, we have to check the power state and wait for it to be running.
	if data.PowerState.ValueBool() { //} || data.PowerState.ValueString() == "" {
		tflog.Debug(ctx, "Power state is set to true. Now check the VM currentpower state")

		var currentPowerState *bool
		var err error

		// Check the power state of the VM
		if currentPowerState, err = va.isVMRunning(ctx, data.Id.ValueString()); err != nil {
			return fmt.Errorf("error checking the power state of the VM: %v", err)
		}
		tflog.Debug(ctx, fmt.Sprintf("VM power state after creation %v", data.PowerState.ValueBool()))

		Retries := 1

		// If the power state is not running, we have to wait for it to be running.
		for !*currentPowerState {

			// Wait for a short period to allow the kill operation to complete
			time.Sleep(5 * time.Second)

			// Check the power state of the VM
			if currentPowerState, err = va.isVMRunning(ctx, data.Id.ValueString()); err != nil {
				return fmt.Errorf("error checking the power state of the VM: %v", err)
			}
			tflog.Debug(ctx, fmt.Sprintf("VM power state after the wait %v", data.PowerState.ValueBool()))

			Retries += 1

			// We are only going to retry 5 times before giving up
			if Retries > 5 {
				// TODO: add logic to rollback the VM creation if the power state is not running after 5 retries
				// for now we will just return
				break
			}
			continue
		}
	}

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
		Name:                 vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Cluster:              vergeio.StringToNil(planData.Cluster, stateData.Cluster, ""),
		Description:          vergeio.StringToNil(planData.Description, stateData.Description, ""),
		Enabled:              vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
		MachineType:          vergeio.StringToNil(planData.MachineType, stateData.MachineType, ""),
		AllowHotplug:         vergeio.BoolToNil(planData.AllowHotplug, stateData.AllowHotplug, false),
		DisablePowercycle:    vergeio.BoolToNil(planData.DisablePowercycle, stateData.DisablePowercycle, false),
		CPUCores:             vergeio.Int32ToNil(planData.CPUCores, stateData.CPUCores, 0),
		CPUType:              vergeio.StringToNil(planData.CPUType, stateData.CPUType, ""),
		RAM:                  vergeio.Int32ToNil(planData.RAM, stateData.RAM, 0),
		Console:              vergeio.StringToNil(planData.Console, stateData.Console, ""),
		Display:              vergeio.StringToNil(planData.Display, stateData.Display, ""),
		Video:                vergeio.StringToNil(planData.Video, stateData.Video, ""),
		Sound:                vergeio.StringToNil(planData.Sound, stateData.Sound, ""),
		OSFamily:             vergeio.StringToNil(planData.OSFamily, stateData.OSFamily, ""),
		OSDescription:        vergeio.StringToNil(planData.OSDescription, stateData.OSDescription, ""),
		RTCBase:              vergeio.StringToNil(planData.RTCBase, stateData.RTCBase, ""),
		BootOrder:            vergeio.StringToNil(planData.BootOrder, stateData.BootOrder, ""),
		ConsolePassEnabled:   vergeio.BoolToNil(planData.ConsolePassEnabled, stateData.ConsolePassEnabled, false),
		ConsolePass:          vergeio.StringToNil(planData.ConsolePass, stateData.ConsolePass, ""),
		USBTablet:            vergeio.BoolToNil(planData.USBTablet, stateData.USBTablet, false),
		UEFI:                 vergeio.BoolToNil(planData.UEFI, stateData.UEFI, false),
		SecureBoot:           vergeio.BoolToNil(planData.SecureBoot, stateData.SecureBoot, false),
		SerialPort:           vergeio.BoolToNil(planData.SerialPort, stateData.SerialPort, false),
		BootDelay:            vergeio.Int32ToNil(planData.BootDelay, stateData.BootDelay, 0),
		PreferredNode:        vergeio.StringToNil(planData.PreferredNode, stateData.PreferredNode, ""),
		SnapshotProfile:      vergeio.StringToNil(planData.SnapshotProfile, stateData.SnapshotProfile, ""),
		CloudInitDataSource:  vergeio.StringToNil(planData.CloudInitDataSource, stateData.CloudInitDataSource, ""),
		PowerState:           vergeio.BoolToNil(planData.PowerState, stateData.PowerState, false),
		GuestAgent:           vergeio.BoolToNil(planData.GuestAgent, stateData.GuestAgent, false),
		HAGroup:              vergeio.StringToNil(planData.HAGroup, stateData.HAGroup, ""),
		Advanced:             vergeio.StringToNil(planData.Advanced, stateData.Advanced, ""),
		NestedVirtualization: vergeio.BoolToNil(planData.NestedVirtualization, stateData.NestedVirtualization, false),
		DisableHypervisor:    vergeio.BoolToNil(planData.DisableHypervisor, stateData.DisableHypervisor, false),
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

	// We now have to handle the desired power state.
	// If it's set to true, we have to check the power state.
	if planData.PowerState.ValueBool() {
		tflog.Debug(ctx, "Power state is set to true, checking the current power state")

		var currentPowerState *bool
		var err error

		if currentPowerState, err = va.isVMRunning(ctx, planData.Id.ValueString()); err != nil {
			return err
		}

		// If the current power state is not running, we have to power it on.
		if !*currentPowerState {
			// It's not running, so we have to power it on.
			if err := va.powerOnVM(ctx, planData); err != nil {
				return err
			}
			stateData.PowerState = types.BoolValue(true)
		}
		// otherwise, if it's set to false then we need to make sure the VM is powered off.
	} else if !planData.PowerState.ValueBool() {
		tflog.Debug(ctx, "Power state is set to false, checking current the power state")

		var currentPowerState *bool
		var err error

		if currentPowerState, err = va.isVMRunning(ctx, planData.Id.ValueString()); err != nil {
			return err
		}
		if *currentPowerState {
			// It's running, so we have to power it off.
			if err := va.killVM(ctx, planData); err != nil {
				return err
			}
			stateData.PowerState = types.BoolValue(false)
		}
	}

	//just wait for 30 second to make sure the vm is up and running
	time.Sleep(5 * time.Second)

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

// This function checks the power state of the VM by calling the API and set the powerstate to true or false
func (va *VMApi) isVMRunning(ctx context.Context, vmId string) (*bool, error) {

	// call the Get API with the vm id and get the fields we need
	apiResp, err := va.client.Get(fmt.Sprintf("%s/%s",
		VMEndpoint,
		url.PathEscape(vmId)),
		&vergeio.Options{
			Fields: "machine#status#running as powerstate"})

	// error checking
	if err != nil {
		return nil, err
	}
	if apiResp == nil {
		return nil, errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return nil, fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the powerstate %v", apiResp.Body))

	// Decode the API response
	var vmAPIResp VMPowerState
	if err := json.NewDecoder(apiResp.Body).Decode(&vmAPIResp); err != nil {
		return nil, fmt.Errorf("invalid format received for VM Item: %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("VM powerstate read from API: %v", vmAPIResp.PowerState))

	return vmAPIResp.PowerState, nil
}

// func (va *VMApi) checkVMPowerState(ctx context.Context, data *VMResourceModel) error {

// 	// Send the kill action request to the vnet_actions endpoint
// 	apiResp, err := va.client.Get(fmt.Sprintf("%s/%s",
// 		VMEndpoint,
// 		url.PathEscape(data.Id.ValueString())),
// 		&vergeio.Options{
// 			Fields: "machine#status#display(status) as powerState"})

// 	// error checking
// 	if err != nil {
// 		return err
// 	}
// 	if apiResp == nil {
// 		return errors.New("missing response from the API")
// 	}
// 	if apiResp.StatusCode != 200 {
// 		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
// 	}

// 	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", apiResp.Body))

// 	// Decode the API response
// 	var vmAPIResp VMAPIResourceModel
// 	if err := json.NewDecoder(apiResp.Body).Decode(&vmAPIResp); err != nil {
// 		return fmt.Errorf("invalid format received for VM Item: %v", err)
// 	}

// 	// save into the resource model
// 	data.PowerState = types.StringValue(vmAPIResp.PowerState)

// 	tflog.Debug(ctx, fmt.Sprintf("VM status read from API: %v", data.PowerState.ValueString()))

// 	return nil
// }

func (va *VMApi) killVM(ctx context.Context, data *VMResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill VM API for VM %v", data.Id.ValueString()))
	return va.changeVMPowerState(ctx, data, "kill")
}

func (va *VMApi) powerOnVM(ctx context.Context, data *VMResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Calling the Power On VM API for VM %v", data.Id.ValueString()))
	return va.changeVMPowerState(ctx, data, "poweron")
}

func (va *VMApi) changeVMPowerState(ctx context.Context, data *VMResourceModel, desiredState string) error {

	tflog.Debug(ctx, fmt.Sprintf("Change the power state for VM %v to %v", data.Id.ValueString(), desiredState))

	// Convert vmID string to int
	vmIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid VM ID format: %v", err)
	}

	// Create the action payload according to vnet_actions schema
	actionPayload := VMAction{
		VM:     int32(vmIDInt),
		Action: desiredState,
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
		return fmt.Errorf("failed to change the VM power state: status code %v", req.StatusCode)
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
	), &vergeio.Options{Fields: "id,machine,name,cluster,description,enabled,machine_type,allow_hotplug,disable_powercycle,cpu_cores,cpu_type,ram,console,display,video,sound,os_family,os_description,rtc_base,boot_order,console_pass_enabled,console_pass,usb_tablet,uefi,secure_boot,serial_port,boot_delay,preferred_node,snapshot_profile,cloudinit_datasource,ha_group,guest_agent,advanced,nested_virtualization,disable_hypervisor,machine#status#running as powerstate"})
	// ), &vergeio.Options{Fields: "id,machine,name,cluster,description,enabled,machine_type,allow_hotplug,disable_powercycle,cpu_cores,cpu_type,ram,console,display,video,sound,os_family,os_description,rtc_base,boot_order,console_pass_enabled,console_pass,usb_tablet,uefi,secure_boot,serial_port,boot_delay,preferred_node,snapshot_profile,cloudinit_datasource,ha_group,machine#status#running as powerstate,guest_agent,advanced,nested_virtualization,disable_hypervisor"})

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
	data.NestedVirtualization = types.BoolValue(vmAPIResp.NestedVirtualization)
	data.DisableHypervisor = types.BoolValue(vmAPIResp.DisableHypervisor)
	data.PowerState = types.BoolValue(vmAPIResp.PowerState)

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

// Read the guest agent info to get the ip addresses
func (va *VMApi) readGuestAgentInfo(ctx context.Context, data *VMResourceModel, toIgnoreCider string) error {

	tflog.Debug(ctx, "Reading the guest agent data")

	// Now read the guest agent info
	apiResp, err := va.client.Get(fmt.Sprintf("%s/%s",
		VMEndpoint,
		url.PathEscape(data.Id.ValueString()),
	), &vergeio.Options{Fields: "dashboard"})

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
	tflog.Debug(ctx, fmt.Sprintf("Read the guest agent info %v", apiResp.Body))

	// If there is no body, return
	if apiResp.Body == nil {
		return nil
	}

	// Decode the API response
	var gaResp VMAPIGuestAgentModel
	if err := json.NewDecoder(apiResp.Body).Decode(&gaResp); err != nil {
		// return fmt.Errorf("invalid format received for VM Item: %v", err)
		// Instead of rutning the error, return nil
		data.GuestAgentIPs = types.ListNull(types.StringType)
		return nil
	}

	// If there is no guest agent info, return
	if gaResp.Machine.Status.AgentGuestInfo == nil {
		data.GuestAgentIPs = types.ListNull(types.StringType)
		return nil
	}

	// Get the IP addresses
	var Ips []*types.String
	for _, network := range gaResp.Machine.Status.AgentGuestInfo.Network {
		for _, ip := range network.IPAddresses {
			if ip.IPAddressType == "ipv4" {
				var pointer = types.StringPointerValue(&ip.IPAddress)
				if !pointer.IsNull() && !isIPInCIDR(&ip.IPAddress, toIgnoreCider) {
					Ips = append(Ips, &pointer)
				}
			}
		}
	}
	if len(Ips) > 0 {
		data.GuestAgentIPs, _ = types.ListValueFrom(ctx, types.StringType, Ips)
	} else {
		data.GuestAgentIPs = types.ListNull(types.StringType)
	}

	tflog.Debug(ctx, fmt.Sprintf("Guest Agent IPs %v", data.GuestAgentIPs))

	return nil
}

// Read VMs from the API. Used by the data source
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

	// Filter the response for snapshots
	isSnapshotFilterSet := !data.IsSnapshot.IsNull()
	isSnapshotFilter := data.IsSnapshot.ValueBool()

	for _, vmAPIRespItem := range vmAPIResp {

		if isSnapshotFilterSet {
			if isSnapshotFilter != vmAPIRespItem.IsSnapshot {
				continue
			}
		}

		vmModel := VMModel{
			Id:          types.Int32Value(vmAPIRespItem.Id),
			Name:        types.StringValue(vmAPIRespItem.Name),
			Key:         types.Int32Value(vmAPIRespItem.Key),
			IsSnapshot:  types.BoolValue(vmAPIRespItem.IsSnapshot),
			CPUType:     types.StringValue(vmAPIRespItem.CPUType),
			MachineType: types.StringValue(vmAPIRespItem.MachineType),
			OSFamily:    types.StringValue(vmAPIRespItem.OSFamily),
			UEFI:        types.BoolValue(vmAPIRespItem.UEFI),
		}

		if vmAPIRespItem.Machine.Drives != nil {
			var drives []*VMDriveModel

			for _, vmDrive := range vmAPIRespItem.Machine.Drives {
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
			vmModel.Drives = drives
		}

		if vmAPIRespItem.Machine.Nics != nil {
			var nics []*VMNicModel

			for _, vmNic := range vmAPIRespItem.Machine.Nics {
				nic := &VMNicModel{
					Key:        types.Int32Value(vmNic.Key),
					Name:       types.StringValue(vmNic.Name),
					Interface:  types.StringValue(vmNic.Interface),
					Vnet:       types.StringValue(vmNic.Vnet),
					Status:     types.StringValue(vmNic.Status),
					Ipaddress:  types.StringValue(vmNic.Ipaddress),
					MacAddress: types.StringValue(vmNic.MacAddress),
				}
				nics = append(nics, nic)
			}
			vmModel.Nics = nics
		}

		data.Vms = append(data.Vms, &vmModel)
	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

func isIPInCIDR(ipStr *string, cidrStr string) bool {
	ip := net.ParseIP(*ipStr)
	if ip == nil {
		return false
	}

	_, cidrNet, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false
	}

	return cidrNet.Contains(ip)
}
