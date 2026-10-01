// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	vergeos "github.com/verge-io/govergeos"
)

const (
	VMEndpoint       = vergeio.VMEndpoint
	VMActionEndpoint = vergeio.VMActionEndpoint
)

var _ vergeio.IClient = &VMApi{}

func NewVMApi(c *vergeio.Client) (*VMApi, error) {
	// Share the cached govergeos client. A setup error is returned so
	// Configure can report it. readVM still retries when an API value was
	// built with the VergeOS client and no SDK yet.
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	return &VMApi{
		name:   "VM Api",
		client: c,
		sdk:    sdk,
	}, nil
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
	Key        int32            `json:"$key,omitempty"`
	Name       string           `json:"name,omitempty"`
	Interface  string           `json:"interface,omitempty"`
	Vnet       *vergeos.FlexInt `json:"vnet,omitempty"`
	Status     string           `json:"status,omitempty"`
	Ipaddress  string           `json:"ipaddress,omitempty"`
	MacAddress string           `json:"macaddress,omitempty"`
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

// VMAPIResourceModel is the VM create/update body.
// Scalar fields are pointers so false, 0, and "" are encoded. A nil pointer
// is omitted. Power state is left unset here; CreateVM and UpdateVM apply
// it with the power actions so an omitted powerstate does not stop a VM.
type VMAPIResourceModel struct {
	Id                   *string            `json:"id,omitempty"`
	Machine              *int32             `json:"machine,omitempty"`
	Name                 *string            `json:"name,omitempty"`
	Cluster              *int32             `json:"cluster,omitempty"`
	Description          *string            `json:"description,omitempty"`
	Enabled              *bool              `json:"enabled,omitempty"`
	MachineType          *string            `json:"machine_type,omitempty"`
	AllowHotplug         *bool              `json:"allow_hotplug,omitempty"`
	DisablePowercycle    *bool              `json:"disable_powercycle,omitempty"`
	OnPowerLoss          *string            `json:"on_power_loss,omitempty"`
	CPUCores             *int32             `json:"cpu_cores,omitempty"`
	CPUType              *string            `json:"cpu_type,omitempty"`
	RAM                  *int32             `json:"ram,omitempty"`
	Console              *string            `json:"console,omitempty"`
	Display              *string            `json:"display,omitempty"`
	Video                *string            `json:"video,omitempty"`
	Sound                *string            `json:"sound,omitempty"`
	OSFamily             *string            `json:"os_family,omitempty"`
	OSDescription        *string            `json:"os_description,omitempty"`
	RTCBase              *string            `json:"rtc_base,omitempty"`
	BootOrder            *string            `json:"boot_order,omitempty"`
	ConsolePassEnabled   *bool              `json:"console_pass_enabled,omitempty"`
	ConsolePass          *string            `json:"console_pass,omitempty"`
	USBTablet            *bool              `json:"usb_tablet,omitempty"`
	UEFI                 *bool              `json:"uefi,omitempty"`
	SecureBoot           *bool              `json:"secure_boot,omitempty"`
	SerialPort           *bool              `json:"serial_port,omitempty"`
	BootDelay            *int32             `json:"boot_delay,omitempty"`
	PreferredNode        *int32             `json:"preferred_node,omitempty"`
	SnapshotProfile      *int32             `json:"snapshot_profile,omitempty"`
	CloudInitDataSource  *string            `json:"cloudinit_datasource,omitempty"`
	CloudInitFiles       []CloudInitFileAPI `json:"cloudinit_files,omitempty"`
	PowerState           *bool              `json:"powerstate,omitempty"`
	GuestAgent           *bool              `json:"guest_agent,omitempty"`
	HAGroup              *string            `json:"ha_group,omitempty"`
	Advanced             *string            `json:"advanced,omitempty"`
	NestedVirtualization *bool              `json:"nested_virtualization,omitempty"`
	DisableHypervisor    *bool              `json:"disable_hypervisor,omitempty"`
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

// VergeOS creates a VM with one core and 1024 MiB when cpu_cores and ram
// are omitted. Provider v2.7.8 left those fields out and the platform
// applied these values. govergeos v0.3.0 stores both as plain ints, so an
// omitted field arrives as 0 and Create rejects it. Create fills the same
// platform defaults. An explicit value, including 0 or a negative number,
// is sent unchanged. Updates still omit an unset value.
const (
	defaultVMCPUCores int32 = 1
	defaultVMRAM      int32 = 1024
)

// vmCreateModel is the provider create body. Nil pointers are omitted.
// An explicit false, 0, or "" is set. cpu_cores and ram use the VergeOS
// defaults when the plan leaves them unset.
func vmCreateModel(data *VMResourceModel) VMAPIResourceModel {
	apiData := VMAPIResourceModel{
		Name:                 vergeio.KnownString(data.Name),
		Cluster:              vergeio.KnownInt32(data.Cluster),
		Description:          vergeio.KnownString(data.Description),
		Enabled:              vergeio.KnownBool(data.Enabled),
		MachineType:          vergeio.KnownString(data.MachineType),
		AllowHotplug:         vergeio.KnownBool(data.AllowHotplug),
		DisablePowercycle:    vergeio.KnownBool(data.DisablePowercycle),
		OnPowerLoss:          vergeio.KnownString(data.OnPowerLoss),
		CPUCores:             int32OrDefault(data.CPUCores, defaultVMCPUCores),
		CPUType:              vergeio.KnownString(data.CPUType),
		RAM:                  int32OrDefault(data.RAM, defaultVMRAM),
		Console:              vergeio.KnownString(data.Console),
		Display:              vergeio.KnownString(data.Display),
		Video:                vergeio.KnownString(data.Video),
		Sound:                vergeio.KnownString(data.Sound),
		OSFamily:             vergeio.KnownString(data.OSFamily),
		OSDescription:        vergeio.KnownString(data.OSDescription),
		RTCBase:              vergeio.KnownString(data.RTCBase),
		BootOrder:            vergeio.KnownString(data.BootOrder),
		ConsolePassEnabled:   vergeio.KnownBool(data.ConsolePassEnabled),
		ConsolePass:          vergeio.KnownString(data.ConsolePass),
		USBTablet:            vergeio.KnownBool(data.USBTablet),
		UEFI:                 vergeio.KnownBool(data.UEFI),
		SecureBoot:           vergeio.KnownBool(data.SecureBoot),
		SerialPort:           vergeio.KnownBool(data.SerialPort),
		BootDelay:            vergeio.KnownInt32(data.BootDelay),
		PreferredNode:        vergeio.KnownInt32(data.PreferredNode),
		SnapshotProfile:      vergeio.KnownInt32(data.SnapshotProfile),
		CloudInitDataSource:  vergeio.KnownString(data.CloudInitDataSource),
		GuestAgent:           vergeio.KnownBool(data.GuestAgent),
		HAGroup:              vergeio.KnownString(data.HAGroup),
		Advanced:             vergeio.KnownString(data.Advanced),
		NestedVirtualization: vergeio.KnownBool(data.NestedVirtualization),
		DisableHypervisor:    vergeio.KnownBool(data.DisableHypervisor),
	}

	if data.CloudInitFiles != nil {
		for _, cloudInitFile := range data.CloudInitFiles {
			apiData.CloudInitFiles = append(apiData.CloudInitFiles, CloudInitFileAPI{
				Name:     cloudInitFile.Name.ValueString(),
				Contents: cloudInitFile.Contents.ValueString(),
			})
		}
	}
	return apiData
}

// int32OrDefault returns the planned value when it is known, including 0
// and negative numbers. Null and unknown values use fallback.
func int32OrDefault(v types.Int32, fallback int32) *int32 {
	if v.IsNull() || v.IsUnknown() {
		n := fallback
		return &n
	}
	return vergeio.KnownInt32(v)
}

// vmCreateRequest builds the SDK create body from known plan values.
// Null and unknown attributes are left unset, except cpu_cores and ram,
// which use the VergeOS defaults. An explicit false, 0, or "" is sent.
// Power state is not part of this body.
func vmCreateRequest(data *VMResourceModel) (*vergeos.VMCreateRequest, error) {
	return decodeVMRequest[vergeos.VMCreateRequest](vmCreateModel(data))
}

// vmUpdateModel is the provider update body. Only attributes that differ
// from state are set. Power state is applied separately.
func vmUpdateModel(planData *VMResourceModel, stateData *VMResourceModel) VMAPIResourceModel {
	// Machine is read only and is not sent.
	return VMAPIResourceModel{
		Name:                 vergeio.ChangedString(planData.Name, stateData.Name),
		Cluster:              vergeio.ChangedInt32(planData.Cluster, stateData.Cluster),
		Description:          vergeio.ChangedString(planData.Description, stateData.Description),
		Enabled:              vergeio.ChangedBool(planData.Enabled, stateData.Enabled),
		MachineType:          vergeio.ChangedString(planData.MachineType, stateData.MachineType),
		AllowHotplug:         vergeio.ChangedBool(planData.AllowHotplug, stateData.AllowHotplug),
		DisablePowercycle:    vergeio.ChangedBool(planData.DisablePowercycle, stateData.DisablePowercycle),
		OnPowerLoss:          vergeio.ChangedString(planData.OnPowerLoss, stateData.OnPowerLoss),
		CPUCores:             vergeio.ChangedInt32(planData.CPUCores, stateData.CPUCores),
		CPUType:              vergeio.ChangedString(planData.CPUType, stateData.CPUType),
		RAM:                  vergeio.ChangedInt32(planData.RAM, stateData.RAM),
		Console:              vergeio.ChangedString(planData.Console, stateData.Console),
		Display:              vergeio.ChangedString(planData.Display, stateData.Display),
		Video:                vergeio.ChangedString(planData.Video, stateData.Video),
		Sound:                vergeio.ChangedString(planData.Sound, stateData.Sound),
		OSFamily:             vergeio.ChangedString(planData.OSFamily, stateData.OSFamily),
		OSDescription:        vergeio.ChangedString(planData.OSDescription, stateData.OSDescription),
		RTCBase:              vergeio.ChangedString(planData.RTCBase, stateData.RTCBase),
		BootOrder:            vergeio.ChangedString(planData.BootOrder, stateData.BootOrder),
		ConsolePassEnabled:   vergeio.ChangedBool(planData.ConsolePassEnabled, stateData.ConsolePassEnabled),
		ConsolePass:          vergeio.ChangedString(planData.ConsolePass, stateData.ConsolePass),
		USBTablet:            vergeio.ChangedBool(planData.USBTablet, stateData.USBTablet),
		UEFI:                 vergeio.ChangedBool(planData.UEFI, stateData.UEFI),
		SecureBoot:           vergeio.ChangedBool(planData.SecureBoot, stateData.SecureBoot),
		SerialPort:           vergeio.ChangedBool(planData.SerialPort, stateData.SerialPort),
		BootDelay:            vergeio.ChangedInt32(planData.BootDelay, stateData.BootDelay),
		PreferredNode:        vergeio.ChangedInt32(planData.PreferredNode, stateData.PreferredNode),
		SnapshotProfile:      vergeio.ChangedInt32(planData.SnapshotProfile, stateData.SnapshotProfile),
		CloudInitDataSource:  vergeio.ChangedString(planData.CloudInitDataSource, stateData.CloudInitDataSource),
		GuestAgent:           vergeio.ChangedBool(planData.GuestAgent, stateData.GuestAgent),
		HAGroup:              vergeio.ChangedString(planData.HAGroup, stateData.HAGroup),
		Advanced:             vergeio.ChangedString(planData.Advanced, stateData.Advanced),
		NestedVirtualization: vergeio.ChangedBool(planData.NestedVirtualization, stateData.NestedVirtualization),
		DisableHypervisor:    vergeio.ChangedBool(planData.DisableHypervisor, stateData.DisableHypervisor),
	}
}

// vmUpdateRequest builds the SDK update body from attributes whose planned
// value differs from state. Power state is applied separately.
func vmUpdateRequest(planData *VMResourceModel, stateData *VMResourceModel) (*vergeos.VMUpdateRequest, int, error) {
	req, err := decodeVMRequest[vergeos.VMUpdateRequest](vmUpdateModel(planData, stateData))
	if err != nil {
		return nil, 0, err
	}

	id := planData.Id
	if id.IsNull() || id.IsUnknown() || id.ValueString() == "" {
		id = stateData.Id
	}
	vmID, err := strconv.Atoi(id.ValueString())
	if err != nil {
		return nil, 0, fmt.Errorf("invalid VM ID: %v", err)
	}
	return req, vmID, nil
}

func decodeVMRequest[T any](apiData VMAPIResourceModel) (*T, error) {
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return nil, errors.New("invalid format received for VM Item")
	}
	var req T
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return nil, fmt.Errorf("failed to convert API data: %v", err)
	}
	return &req, nil
}

// Create a new VM.
func (va *VMApi) CreateVM(ctx context.Context, data *VMResourceModel) error {

	req, err := vmCreateRequest(data)
	if err != nil {
		return err
	}

	vm, err := va.sdk.VMs.Create(ctx, req)
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

// plannedPowerChange reports whether UpdateVM should change VM power.
// A null or unknown powerstate means the configuration did not set it, so
// the current power is left alone. A known value is applied only when it
// differs from the VM's current power. powerOn is meaningful only when
// change is true.
func plannedPowerChange(planned types.Bool, current bool) (change bool, powerOn bool) {
	if planned.IsNull() || planned.IsUnknown() {
		return false, false
	}
	desired := planned.ValueBool()
	if desired == current {
		return false, false
	}
	return true, desired
}

// applyPlannedPowerState powers the VM on or off only when the plan asks
// for a known powerstate that differs from the VM's current power.
func (va *VMApi) applyPlannedPowerState(ctx context.Context, planData *VMResourceModel, stateData *VMResourceModel) error {
	if planData.PowerState.IsNull() || planData.PowerState.IsUnknown() {
		tflog.Debug(ctx, "Planned powerstate is unset; leaving VM power unchanged")
		return nil
	}

	currentPowerState, err := va.isVMRunning(ctx, planData.Id.ValueString())
	if err != nil {
		return err
	}
	change, powerOn := plannedPowerChange(planData.PowerState, *currentPowerState)
	if !change {
		return nil
	}
	if powerOn {
		tflog.Debug(ctx, "Planned powerstate is on and the VM is stopped; powering on")
		if err := va.powerOnVM(ctx, planData); err != nil {
			return err
		}
		stateData.PowerState = types.BoolValue(true)
		return nil
	}

	tflog.Debug(ctx, "Planned powerstate is off and the VM is running; requesting a graceful poweroff")
	timeout, err := shutdownTimeoutFromPlan(planData)
	if err != nil {
		return err
	}
	force := false
	if !planData.ForcePowerOff.IsNull() && !planData.ForcePowerOff.IsUnknown() {
		force = planData.ForcePowerOff.ValueBool()
	}
	// Post poweroff and poll. Do not call VMService.PowerOff: it sends kill.
	if err := va.gracefulPowerOff(ctx, planData.Id.ValueString(), planData.Name.ValueString(), timeout, force); err != nil {
		return err
	}
	stateData.PowerState = types.BoolValue(false)
	return nil
}

// deviceAttachAttempts is how many times a hotplugged drive or NIC is read.
// The first read is immediate. Later reads wait deviceAttachInterval.
// A drive hotplug on VergeOS 26.1 was online within about three seconds.
const (
	deviceAttachAttempts = 6
	deviceAttachInterval = time.Second
)

// deviceDetachTimeout is how long delete waits after one unplug for a drive
// or NIC to leave the guest. On VergeOS 26.1 a guest stayed in hotplug for
// several minutes, so this is a deadline rather than a few short retries.
// The first status read is immediate. Tests shorten these.
var (
	deviceDetachTimeout  = 5 * time.Minute
	deviceDetachInterval = 2 * time.Second
)

// deviceShouldAttach reports whether a newly created drive or NIC should be
// hotplugged. A disabled device is meant to stay offline or down.
func deviceShouldAttach(enabled types.Bool) bool {
	if enabled.IsNull() || enabled.IsUnknown() {
		return true
	}
	return enabled.ValueBool()
}

// readVMPowerState reports whether the VM is running.
// Sync uses this so a device created on a stopped VM is left for the next
// boot, and a device created on a running VM is hotplugged.
func readVMPowerState(ctx context.Context, client *vergeio.Client, vmId types.String) (bool, error) {
	tflog.Debug(ctx, "Reading VM power state before adding a drive or NIC")
	if client == nil {
		return false, errors.New("missing API client")
	}
	if vmId.IsNull() || vmId.IsUnknown() || strings.TrimSpace(vmId.ValueString()) == "" {
		return false, errors.New("missing VM id")
	}

	apiResp, err := client.Get(ctx, vergeio.ObjectPath(VMEndpoint, vmId.ValueString()), &vergeio.Options{Fields: "machine#status#running as powerstate"})
	if err != nil {
		return false, err
	}
	if apiResp == nil || apiResp.Body == nil {
		return false, errors.New("missing response from the API")
	}
	defer func() { _ = apiResp.Body.Close() }()

	var status struct {
		PowerState bool `json:"powerstate"`
	}
	if err := json.NewDecoder(apiResp.Body).Decode(&status); err != nil {
		return false, fmt.Errorf("invalid VM power state response: %w", err)
	}
	return status.PowerState, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// waitForStatus polls until the device status matches want.
// A match returns the status and a nil error. A read failure returns an
// empty status and that error. Exhausting the attempts returns the last
// status and an error so the caller can say a power cycle is required.
func waitForStatus(ctx context.Context, read func() (string, error), want string) (string, error) {
	var last string
	for attempt := 0; attempt < deviceAttachAttempts; attempt++ {
		status, err := read()
		if err != nil {
			return "", err
		}
		last = strings.TrimSpace(status)
		if strings.EqualFold(last, want) {
			return last, nil
		}
		if attempt == deviceAttachAttempts-1 {
			break
		}
		if err := sleepContext(ctx, deviceAttachInterval); err != nil {
			return last, err
		}
	}
	return last, fmt.Errorf("status is still %q", last)
}

// waitForStatusWithin polls until the device status matches want or the
// deadline passes. The deadline is fixed when the wait starts. The first
// read is immediate. A match returns the status and a nil error. A read
// failure returns an empty status and that error. A timeout returns the
// last status and an error.
func waitForStatusWithin(ctx context.Context, read func() (string, error), want string, timeout, interval time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		if err := ctx.Err(); err != nil {
			return last, err
		}
		status, err := read()
		if err != nil {
			return "", err
		}
		last = strings.TrimSpace(status)
		if strings.EqualFold(last, want) {
			return last, nil
		}
		if !time.Now().Before(deadline) {
			return last, fmt.Errorf("status is still %q", last)
		}
		wait := interval
		if interval > 0 {
			if remaining := time.Until(deadline); remaining < wait {
				wait = remaining
			}
		}
		if err := sleepContext(ctx, wait); err != nil {
			return last, err
		}
	}
}

// deviceUnplugInProgress reports a status that means an unplug is already
// running. Sending hotplug again in that state returns 422.
func deviceUnplugInProgress(status string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(status)), "hotplug")
}

// unplugAlreadyInProgress reports the API error from a second unplug.
func unplugAlreadyInProgress(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "already hotplug")
}

// sendUnplug sends the unplug action once. A device that is already
// hotplugging is left to finish; the caller polls until it is detached.
func sendUnplug(ctx context.Context, status string, send func() error) error {
	if deviceUnplugInProgress(status) {
		tflog.Debug(ctx, fmt.Sprintf("Device status %q means unplug is already in progress", strings.TrimSpace(status)))
		return nil
	}
	if err := send(); err != nil {
		if unplugAlreadyInProgress(err) {
			tflog.Debug(ctx, fmt.Sprintf("Unplug is already in progress: %v", err))
			return nil
		}
		return err
	}
	return nil
}

func detachLabel(name, id, idKind string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%q (%s %s)", name, idKind, id)
	case name != "":
		return fmt.Sprintf("%q", name)
	case id != "":
		return fmt.Sprintf("%s %s", idKind, id)
	default:
		return "unknown"
	}
}

// waitUntilDetached polls until the device reaches want. On timeout the
// error names the device and the last status.
func waitUntilDetached(ctx context.Context, read func() (string, error), want, kind, name, id, idKind string) error {
	status, err := waitForStatusWithin(ctx, read, want, deviceDetachTimeout, deviceDetachInterval)
	if err == nil {
		return nil
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return ctx.Err()
	}
	label := detachLabel(name, id, idKind)
	if status == "" {
		return fmt.Errorf("%s %s was unplugged but its status could not be read: %w", kind, label, err)
	}
	return fmt.Errorf("%s %s stayed %q after unplug and did not reach %s before the timeout", kind, label, status, want)
}

// Update the VM.
func (va *VMApi) UpdateVM(ctx context.Context, planData *VMResourceModel, stateData *VMResourceModel) error {

	req, vmID, err := vmUpdateRequest(planData, stateData)
	if err != nil {
		return err
	}

	_, err = va.sdk.VMs.Update(ctx, vmID, req)
	if err != nil {
		return err
	}

	// Power stays with the caller. VMResource.Update syncs drives and NICs
	// first, then applyPlannedPowerState, so a VM powered on in this apply
	// boots with those devices instead of leaving them offline.
	return nil
}

// detachCloudInit deletes the cloud-init files owned by this VM
// so it no longer depends on them on subsequent boots.
func (va *VMApi) detachCloudInit(ctx context.Context, data *VMResourceModel) error {
	vmId := data.Id.ValueString()
	tflog.Debug(ctx, fmt.Sprintf("Detaching cloud-init files from VM %s", vmId))

	files, err := va.sdk.CloudInitFiles.List(ctx, vergeos.WithFilter(fmt.Sprintf("owner eq 'vms/%s'", vmId)))
	if err != nil {
		return fmt.Errorf("error querying cloud-init files: %v", err)
	}

	for _, file := range files {
		fileKey := file.ID.Int()
		tflog.Debug(ctx, fmt.Sprintf("Deleting cloud-init file %d", fileKey))
		if err := va.sdk.CloudInitFiles.Delete(ctx, fileKey); err != nil {
			return fmt.Errorf("error deleting cloud-init file %d: %v", fileKey, err)
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

// isVMRunning reports the machine running flag.
// The VM powerstate column can stay unchanged after a power change in the
// VergeOS UI, so this does not use VM.PowerState from a plain Get.
func (va *VMApi) isVMRunning(ctx context.Context, vmId string) (*bool, error) {
	running, status, err := va.readVMPowerStatus(ctx, vmId)
	if err != nil {
		return nil, err
	}
	tflog.Debug(ctx, fmt.Sprintf("VM power read from machine status: running=%v status=%s", running, status))
	return &running, nil
}

// This function kills the VM
func (va *VMApi) killVM(ctx context.Context, data *VMResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill VM API for VM %v", data.Id.ValueString()))
	return va.changeVMPowerState(ctx, data, "kill")
}

// powerOnVM starts the VM and waits until machine status says it is running.
// The poweron action is posted directly. VMService.PowerOn returns without
// sending that action when the powerstate column is already true, so a VM
// stopped in the UI would stay stopped.
func (va *VMApi) powerOnVM(ctx context.Context, data *VMResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Calling the Power On VM API for VM %v", data.Id.ValueString()))
	if err := va.postVMAction(ctx, data.Id.ValueString(), vmActionPowerOn); err != nil {
		return err
	}
	if err := va.waitForVMPower(ctx, data.Id.ValueString(), true, vmPowerWaitTimeout, powerOnInterval); err != nil {
		return fmt.Errorf("powering on VM %s: %w", vmShutdownLabel(data.Name.ValueString(), data.Id.ValueString()), err)
	}
	return sleepContext(ctx, powerOnSettle)
}

// changeVMPowerState powers the VM on or kills it, then waits until machine
// status matches. A timeout is an error so the following read cannot store
// a powerstate the plan did not ask for.
func (va *VMApi) changeVMPowerState(ctx context.Context, data *VMResourceModel, desiredState string) error {
	tflog.Debug(ctx, fmt.Sprintf("Change the power state for VM %v to %v", data.Id.ValueString(), desiredState))

	switch desiredState {
	case vmActionPowerOn:
		return va.powerOnVM(ctx, data)
	case vmActionKill:
		// Post kill directly. VMService.PowerOff skips the action when the
		// powerstate column is already false.
		if err := va.postVMAction(ctx, data.Id.ValueString(), vmActionKill); err != nil {
			return fmt.Errorf("failed to change the VM power state: %v", err)
		}
		if err := va.waitForVMPower(ctx, data.Id.ValueString(), false, vmPowerWaitTimeout, powerOnInterval); err != nil {
			return fmt.Errorf("killing VM %s: %w", vmShutdownLabel(data.Name.ValueString(), data.Id.ValueString()), err)
		}
		return nil
	default:
		return fmt.Errorf("invalid desired state: %s", desiredState)
	}
}

// usePlannedConsolePass copies the planned console password onto the model
// that readVM will refresh. The API does not return console_pass, so the
// value saved after apply is the planned one.
func usePlannedConsolePass(state, plan *VMResourceModel) {
	state.ConsolePass = plan.ConsolePass
}

// usePlannedShutdownSettings copies provider-only power settings onto the
// model readVM refreshes. The API does not return them.
func usePlannedShutdownSettings(state, plan *VMResourceModel) {
	state.ForcePowerOff = plan.ForcePowerOff
	state.ShutdownOnDestroy = plan.ShutdownOnDestroy
	if plan.Timeouts == nil {
		state.Timeouts = nil
		return
	}
	copied := *plan.Timeouts
	state.Timeouts = &copied
}

// applyVM copies an API VM onto the resource model.
// console_pass is a hidden password and is absent from the response. A known
// value already on data is kept: the planned value during apply, or prior
// state during refresh. Copying the missing field would store "" and the next
// plan would replace the VM, deleting its disks. An unset value is unknown
// during apply; store null so the result is known. The attribute changes only
// when configuration changes that stored value.
func applyVM(data *VMResourceModel, vm *vergeos.VM) {
	if data == nil || vm == nil {
		return
	}
	data.Machine = types.Int32Value(int32(vm.Machine))
	data.Name = types.StringValue(vm.Name)
	data.Cluster = types.Int32Value(int32(vm.Cluster.Int()))
	data.Description = types.StringValue(vm.Description)
	data.Enabled = types.BoolValue(vm.Enabled)
	// Preserve the configured machine_type if semantically equivalent to API response.
	// This handles API v26 expanding "q35" to "pc-q35-10.0" and "pc" to "pc-i440fx-10.0".
	if !shared.MachineTypesAreEquivalent(data.MachineType.ValueString(), vm.MachineType) {
		data.MachineType = types.StringValue(vm.MachineType)
	}
	data.AllowHotplug = types.BoolValue(vm.AllowHotplug)
	data.DisablePowercycle = types.BoolValue(vm.DisablePowercycle)
	data.OnPowerLoss = types.StringValue(vm.OnPowerLoss)
	data.CPUCores = types.Int32Value(int32(vm.CPUCores))
	data.CPUType = types.StringValue(vm.CPUType)
	data.RAM = types.Int32Value(int32(vm.RAM))
	data.Console = types.StringValue(vm.Console)
	data.Display = types.StringValue(vm.Display)
	data.Video = types.StringValue(vm.Video)
	data.Sound = types.StringValue(vm.Sound)
	data.OSFamily = types.StringValue(vm.OSFamily)
	data.OSDescription = types.StringValue(vm.OSDescription)
	data.RTCBase = types.StringValue(vm.RTCBase)
	data.BootOrder = types.StringValue(vm.BootOrder)
	data.ConsolePassEnabled = types.BoolValue(vm.ConsolePassEnabled)
	if data.ConsolePass.IsNull() || data.ConsolePass.IsUnknown() {
		data.ConsolePass = types.StringNull()
	}
	data.USBTablet = types.BoolValue(vm.USBTablet)
	data.UEFI = types.BoolValue(vm.UEFI)
	data.SecureBoot = types.BoolValue(vm.SecureBoot)
	data.SerialPort = types.BoolValue(vm.SerialPort)
	data.BootDelay = types.Int32Value(int32(vm.BootDelay))
	data.PreferredNode = types.Int32Value(int32(vm.PreferredNode.Int()))
	data.SnapshotProfile = types.Int32Value(int32(vm.SnapshotProfile.Int()))
	data.CloudInitDataSource = types.StringValue(vm.CloudInitDataSource)
	data.GuestAgent = types.BoolValue(vm.GuestAgent)
	data.HAGroup = types.StringValue(vm.HAGroup)
	data.Advanced = types.StringValue(vm.Advanced)
	data.NestedVirtualization = types.BoolValue(vm.NestedVirtualization)
	data.DisableHypervisor = types.BoolValue(vm.DisableHypervisor)
	data.PowerState = types.BoolValue(vm.PowerState)
	// force_power_off and shutdown_on_destroy are not VergeOS fields. Keep an
	// explicit setting and fill the default when state has never stored one,
	// such as after import.
	if data.ForcePowerOff.IsNull() || data.ForcePowerOff.IsUnknown() {
		data.ForcePowerOff = types.BoolValue(false)
	}
	if data.ShutdownOnDestroy.IsNull() || data.ShutdownOnDestroy.IsUnknown() {
		data.ShutdownOnDestroy = types.StringValue(shutdownOnDestroyGracefulThenKill)
	}

	if vm.CloudInitFiles != nil {
		for _, file := range vm.CloudInitFiles {
			data.CloudInitFiles = append(data.CloudInitFiles, CloudInitFile{
				Name:     types.StringValue(file.Name),
				Contents: types.StringValue(file.Contents),
			})
		}
	}
}

// Read the VM from the API.
func (va *VMApi) readVM(ctx context.Context, data *VMResourceModel) error {
	if err := va.ensureSDK(); err != nil {
		return err
	}
	if data == nil {
		return fmt.Errorf("VM is nil")
	}

	tflog.Debug(ctx, "Reading the vm data")

	// Call the SDK API to get the VM
	if data.Id.IsNull() || data.Id.IsUnknown() || strings.TrimSpace(data.Id.ValueString()) == "" {
		return fmt.Errorf("invalid VM ID: empty")
	}
	vmID, err := strconv.Atoi(strings.TrimSpace(data.Id.ValueString()))
	if err != nil || vmID <= 0 {
		return fmt.Errorf("invalid VM ID: %v", data.Id.ValueString())
	}

	vm, err := va.sdk.VMs.Get(ctx, vmID)
	if err != nil {
		return err
	}
	if vm == nil || vm.ID.Int() <= 0 {
		return fmt.Errorf("read VM %d: response was empty", vmID)
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", vm))

	applyVM(data, vm)
	// The column copied above can disagree with the guest. Store the machine
	// running flag so refresh sees a UI power change and the value written
	// after apply matches the plan.
	if err := va.useMachinePowerState(ctx, data); err != nil {
		return err
	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// useMachinePowerState stores machine#status#running on powerstate.
// A status payload that has no power fields keeps the value applyVM set,
// which is the case for a response that only carried the running flag
// under the powerstate alias.
func (va *VMApi) useMachinePowerState(ctx context.Context, data *VMResourceModel) error {
	running, _, err := va.readVMPowerStatus(ctx, data.Id.ValueString())
	if err != nil {
		if errors.Is(err, errVMPowerStatusMissing) {
			return nil
		}
		return err
	}
	data.PowerState = types.BoolValue(running)
	return nil
}

// ensureSDK connects the govergeos client when Configure stored the VergeOS
// client but the first setup failed. A nil sdk used to panic on VMs.Get.
func (va *VMApi) ensureSDK() error {
	if va == nil || va.client == nil {
		return fmt.Errorf("VM client is not configured")
	}
	if va.sdk != nil && va.sdk.VMs != nil {
		return nil
	}
	sdk, err := va.client.SDK()
	if err != nil {
		return fmt.Errorf("VM client is not configured: %w", err)
	}
	if sdk == nil || sdk.VMs == nil {
		return fmt.Errorf("VM client is not configured")
	}
	va.sdk = sdk
	return nil
}

// Read the guest agent info to get the ip addresses
func (va *VMApi) readGuestAgentInfo(ctx context.Context, data *VMResourceModel, toIgnoreCider string) error {

	tflog.Debug(ctx, "Reading the guest agent data")

	// Now read the guest agent info
	apiResp, err := va.client.Get(ctx, vergeio.ObjectPath(VMEndpoint, data.Id.ValueString()), &vergeio.Options{Fields: "dashboard"})

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
	fn := data.FilterName.ValueString()
	if fn != "" {
		listOpts = append(listOpts, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(fn))))
	}

	tflog.Debug(ctx, "Calling SDK VMs.List")

	// Call the SDK API
	vms, err := va.sdk.VMs.List(ctx, listOpts...)
	if err != nil {
		return fmt.Errorf("failed to list VMs via SDK: %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("SDK returned %d VMs", len(vms)))

	vms = vergeio.KeepExact(vms, fn, func(vm vergeos.VM) string { return vm.Name })

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
				tier, err := preferredTierFromAPI(vmDrive.PreferredTier)
				if err != nil {
					return err
				}
				drive := &VMDriveModel{
					Key:           types.Int32Value(vmDrive.Key),
					Name:          types.StringValue(vmDrive.Name),
					Interface:     types.StringValue(vmDrive.Interface),
					Media:         types.StringValue(vmDrive.Media),
					Description:   types.StringValue(vmDrive.Description),
					PreferredTier: tier,
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
				vnet := types.Int32Null()
				if vmNic.Vnet != nil {
					vnet = types.Int32Value(int32(vmNic.Vnet.Int()))
				}
				nic := &VMNicModel{
					Key:        types.Int32Value(vmNic.Key),
					Name:       types.StringValue(vmNic.Name),
					Interface:  types.StringValue(vmNic.Interface),
					Vnet:       vnet,
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
