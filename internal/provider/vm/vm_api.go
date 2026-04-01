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
	vergeos "github.com/verge-io/govergeos"
)

const (
	VMEndpoint       = vergeio.APIEndpoint + "/vms"
	VMActionEndpoint = vergeio.APIEndpoint + "/vm_actions"
)

var _ vergeio.IClient = &VMApi{}

func NewVMApi(c *vergeio.Client) *VMApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &VMApi{
		name:   "VM Api",
		client: c,
		sdk:    sdk,
	}
}

type VMApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
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

// Valid machine types - DEPRECATED: Use GetMachineTypesFromAPI instead
// This function is kept for backwards compatibility but should not be used for validation
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

// TableSchemaField represents a field in the table schema
type TableSchemaField struct {
	Type string            `json:"type"`
	List map[string]string `json:"list,omitempty"` // Map of value -> description
}

// TableSchemaResponse represents the response from the $table endpoint
type TableSchemaResponse struct {
	Fields map[string]TableSchemaField `json:"fields"` // Map of field name -> field info
}

// GetMachineTypesFromAPI fetches the list of valid machine types from the VergeOS API via SDK
func (va *VMApi) GetMachineTypesFromAPI(ctx context.Context) ([]string, error) {
	// Use SDK schema service instead of manual endpoint construction
	machineTypesMap, err := va.sdk.Schema.GetVMMachineTypes(ctx)
	if err != nil {
		return nil, fmt.Errorf("vm GetMachineTypesFromAPI SDK error: %w", err)
	}

	// Convert map keys to slice of strings
	machineTypes := make([]string, 0, len(machineTypesMap))
	for machineType := range machineTypesMap {
		machineTypes = append(machineTypes, machineType)
	}

	return machineTypes, nil
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
	Cluster              int32              `json:"cluster,omitempty"`
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
	PreferredNode        int32              `json:"preferred_node,omitempty"`
	SnapshotProfile      int32              `json:"snapshot_profile,omitempty"`
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
		Machine:             data.Machine.ValueInt32(),
		Name:                data.Name.ValueString(),
		Cluster:             data.Cluster.ValueInt32(),
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
		PreferredNode:       data.PreferredNode.ValueInt32(),
		SnapshotProfile:     data.SnapshotProfile.ValueInt32(),
		CloudInitDataSource: data.CloudInitDataSource.ValueString(),
		// We are not sending the power state here, as it will be handled separately.
		// When send the power state via the create API, it doesn't start the devices like drives and nics.
		// PowerState:           false,
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

	// Time to call the SDK API - convert to SDK request
	var req vergeos.VMCreateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	vm, err := va.sdk.VMs.Create(ctx, &req)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("VM Key after creation %v", vm.ID))

	// save into the Terraform state.
	data.Id = types.StringValue(strconv.Itoa(vm.ID.Int()))

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
		Name:                 vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Cluster:              vergeio.Int32ToNil(planData.Cluster, stateData.Cluster, 0),
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
		PreferredNode:        vergeio.Int32ToNil(planData.PreferredNode, stateData.PreferredNode, 0),
		SnapshotProfile:      vergeio.Int32ToNil(planData.SnapshotProfile, stateData.SnapshotProfile, 0),
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

	// Time to call the SDK API - convert to SDK request
	var req vergeos.VMUpdateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	vmID, err := strconv.Atoi(apiData.Id)
	if err != nil {
		return fmt.Errorf("invalid VM ID: %v", err)
	}

	_, err = va.sdk.VMs.Update(ctx, vmID, &req)
	if err != nil {
		return err
	}

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

// detachCloudInit deletes the cloud-init files owned by this VM
// so it no longer depends on them on subsequent boots.
func (va *VMApi) detachCloudInit(ctx context.Context, data *VMResourceModel) error {
	vmId := data.Id.ValueString()
	tflog.Debug(ctx, fmt.Sprintf("Detaching cloud-init files from VM %s", vmId))

	cloudInitEndpoint := vergeio.APIEndpoint + "/cloudinit_files"

	// Query cloud-init files owned by this VM
	apiResp, err := va.client.Get(cloudInitEndpoint, &vergeio.Options{
		Fields: "$key",
		Filter: fmt.Sprintf("owner eq 'vms/%s'", vmId),
	})
	if err != nil {
		return fmt.Errorf("error querying cloud-init files: %v", err)
	}
	if apiResp == nil {
		return errors.New("missing response from API when querying cloud-init files")
	}

	// Decode the list of cloud-init files
	type cloudInitFileRef struct {
		Key json.Number `json:"$key"`
	}
	var files []cloudInitFileRef
	if err := json.NewDecoder(apiResp.Body).Decode(&files); err != nil {
		return fmt.Errorf("error decoding cloud-init files response: %v", err)
	}

	// Delete each cloud-init file
	for _, file := range files {
		fileKey := file.Key.String()
		tflog.Debug(ctx, fmt.Sprintf("Deleting cloud-init file %s", fileKey))
		delResp, err := va.client.Delete(fmt.Sprintf("%s/%s",
			cloudInitEndpoint,
			url.PathEscape(fileKey)))
		if err != nil {
			return fmt.Errorf("error deleting cloud-init file %s: %v", fileKey, err)
		}
		if delResp == nil {
			return fmt.Errorf("missing response from API when deleting cloud-init file %s", fileKey)
		}
	}

	tflog.Debug(ctx, fmt.Sprintf("Deleted %d cloud-init files from VM %s", len(files), vmId))

	return nil
}

// Delete the VM from the API.
func (va *VMApi) deleteVM(ctx context.Context, data *VMResourceModel) error {

	tflog.Debug(ctx, "Deleting the vm")

	// Call the SDK API to delete the VM
	vmID, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid VM ID: %v", err)
	}

	err = va.sdk.VMs.Delete(ctx, vmID)
	if err != nil {
		return errors.New("Error deleting the vm: " + err.Error())
	}

	tflog.Debug(ctx, "VM was successfully deleted")

	return nil
}

// This function checks the power state of the VM by calling the API and set the powerstate to true or false
func (va *VMApi) isVMRunning(ctx context.Context, vmId string) (*bool, error) {

	// call the SDK API to get the VM and check power state
	vmID, err := strconv.Atoi(vmId)
	if err != nil {
		return nil, fmt.Errorf("invalid VM ID: %v", err)
	}

	vm, err := va.sdk.VMs.Get(ctx, vmID)
	if err != nil {
		return nil, err
	}

	tflog.Debug(ctx, fmt.Sprintf("VM powerstate read from API: %v", vm.PowerState))

	return &vm.PowerState, nil
}

// This function kills the VM
func (va *VMApi) killVM(ctx context.Context, data *VMResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill VM API for VM %v", data.Id.ValueString()))
	return va.changeVMPowerState(ctx, data, "kill")
}

// This function powers on the VM
func (va *VMApi) powerOnVM(ctx context.Context, data *VMResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Calling the Power On VM API for VM %v", data.Id.ValueString()))
	err := va.changeVMPowerState(ctx, data, "poweron")
	if err != nil {
		return err
	}

	// wait for the vm to come up
	time.Sleep(10 * time.Second)

	return nil
}

// This function changes the power state of the VM
// It "kill" or "poweron" the VM based on the desired state
// It also waits for the VM to be in the desired state
func (va *VMApi) changeVMPowerState(ctx context.Context, data *VMResourceModel, desiredState string) error {

	tflog.Debug(ctx, fmt.Sprintf("Change the power state for VM %v to %v", data.Id.ValueString(), desiredState))

	// Convert vmID string to int
	vmIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid VM ID format: %v", err)
	}

	// Send the power action using SDK
	switch desiredState {
	case "poweron":
		err = va.sdk.VMs.PowerOn(ctx, vmIDInt)
	case "kill":
		err = va.sdk.VMs.PowerOff(ctx, vmIDInt) // Kill maps to PowerOff in SDK
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState)
	}
	if err != nil {
		return fmt.Errorf("failed to change the VM power state: %v", err)
	}

	// Now wait for the VM to be in the desired state (even though SDK waits internally, we verify)
	tflog.Debug(ctx, fmt.Sprintf("Waiting for the VM %v to be in the desired state %v", data.Id.ValueString(), desiredState))

	var currentPowerState *bool = nil
	var desiredBoolState bool
	switch desiredState {
	case "poweron":
		desiredBoolState = true
	case "kill":
		desiredBoolState = false
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState)
	}
	boolDesiredState := &desiredBoolState

	// Check the power state of the VM
	if currentPowerState, err = va.isVMRunning(ctx, data.Id.ValueString()); err != nil {
		return fmt.Errorf("error checking the power state of the VM: %v", err)
	}
	Retries := 1

	// If the power state is not as desired, wait a bit more (SDK might still be completing)
	for *currentPowerState != *boolDesiredState {

		// Wait for a short period to allow the operation to complete
		time.Sleep(5 * time.Second)

		// Check the power state of the VM
		if currentPowerState, err = va.isVMRunning(ctx, data.Id.ValueString()); err != nil {
			return fmt.Errorf("error checking the power state of the VM: %v", err)
		}
		tflog.Debug(ctx, fmt.Sprintf("VM power state after the wait %v", *currentPowerState))

		Retries += 1

		// We are only going to retry 5 times before giving up
		if Retries > 5 {
			// TODO: add logic to rollback the VM creation if the power state is not running after 5 retries
			// for now we will just return
			break
		}
		continue
	}

	return nil
}

// Read the VM from the API.
func (va *VMApi) readVM(ctx context.Context, data *VMResourceModel) error {

	tflog.Debug(ctx, "Reading the vm data")

	// Call the SDK API to get the VM
	vmID, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid VM ID: %v", err)
	}

	vm, err := va.sdk.VMs.Get(ctx, vmID)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", vm))

	// Convert SDK VM to API model for existing field mapping logic
	vmAPIResp := VMAPIResourceModel{
		Machine:              int32(vm.Machine),
		Name:                 vm.Name,
		Cluster:              int32(vm.Cluster.Int()),
		Description:          vm.Description,
		Enabled:              vm.Enabled,
		MachineType:          vm.MachineType,
		AllowHotplug:         vm.AllowHotplug,
		DisablePowercycle:    vm.DisablePowercycle,
		CPUCores:             int32(vm.CPUCores),
		CPUType:              vm.CPUType,
		RAM:                  int32(vm.RAM),
		Console:              vm.Console,
		Display:              vm.Display,
		Video:                vm.Video,
		Sound:                vm.Sound,
		OSFamily:             vm.OSFamily,
		OSDescription:        vm.OSDescription,
		RTCBase:              vm.RTCBase,
		BootOrder:            vm.BootOrder,
		ConsolePassEnabled:   vm.ConsolePassEnabled,
		ConsolePass:          vm.ConsolePass,
		USBTablet:            vm.USBTablet,
		UEFI:                 vm.UEFI,
		SecureBoot:           vm.SecureBoot,
		SerialPort:           vm.SerialPort,
		BootDelay:            int32(vm.BootDelay),
		PreferredNode:        int32(vm.PreferredNode.Int()),
		SnapshotProfile:      int32(vm.SnapshotProfile.Int()),
		CloudInitDataSource:  vm.CloudInitDataSource,
		GuestAgent:           vm.GuestAgent,
		HAGroup:              vm.HAGroup,
		Advanced:             vm.Advanced,
		NestedVirtualization: vm.NestedVirtualization,
		DisableHypervisor:    vm.DisableHypervisor,
		PowerState:           vm.PowerState,
	}

	// Convert cloud init files
	if vm.CloudInitFiles != nil {
		for _, file := range vm.CloudInitFiles {
			vmAPIResp.CloudInitFiles = append(vmAPIResp.CloudInitFiles, CloudInitFileAPI{
				Name:     file.Name,
				Contents: file.Contents,
			})
		}
	}

	// save into the resource model
	data.Machine = types.Int32Value(vmAPIResp.Machine)
	data.Name = types.StringValue(vmAPIResp.Name)
	data.Cluster = types.Int32Value(vmAPIResp.Cluster)
	data.Description = types.StringValue(vmAPIResp.Description)
	data.Enabled = types.BoolValue(vmAPIResp.Enabled)
	// Preserve the configured machine_type if semantically equivalent to API response.
	// This handles API v26 expanding "q35" to "pc-q35-10.0" and "pc" to "pc-i440fx-10.0".
	if !machineTypesAreEquivalent(data.MachineType.ValueString(), vmAPIResp.MachineType) {
		data.MachineType = types.StringValue(vmAPIResp.MachineType)
	}
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
	data.PreferredNode = types.Int32Value(vmAPIResp.PreferredNode)
	data.SnapshotProfile = types.Int32Value(vmAPIResp.SnapshotProfile)
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

	// Build filter conditions for SDK
	var listOpts []vergeos.ListOption

	// Add name filter if specified
	if fn := data.FilterName.ValueString(); fn != "" {
		listOpts = append(listOpts, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", fn)))
	}

	tflog.Debug(ctx, "Calling SDK VMs.List")

	// Call the SDK API
	vms, err := va.sdk.VMs.List(ctx, listOpts...)
	if err != nil {
		return fmt.Errorf("failed to list VMs via SDK: %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("SDK returned %d VMs", len(vms)))

	// Convert SDK VMs to API model for existing field mapping logic
	var vmAPIResp []VMAPIDataSourceModel
	for _, vm := range vms {
		vmAPIResp = append(vmAPIResp, VMAPIDataSourceModel{
			Id:          int32(vm.Machine), // Machine reference ID
			Name:        vm.Name,
			Key:         int32(vm.ID.Int()), // VM Key (was $key in API)
			IsSnapshot:  vm.IsSnapshot,
			CPUType:     vm.CPUType,
			MachineType: vm.MachineType,
			OSFamily:    vm.OSFamily,
			UEFI:        vm.UEFI,
			// Note: Machine.Drives and Machine.Nics are not available in basic VM list
			// These would need separate API calls if needed
		})
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
