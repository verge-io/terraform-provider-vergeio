// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// Ensure provider defined types fully satisfy framework interfaces.
type diskResourceModel struct {
	Key                 types.String  `tfsdk:"key"`
	Machine             types.Int32   `tfsdk:"machine"`
	Name                types.String  `tfsdk:"name"`
	Description         types.String  `tfsdk:"description"`
	Interface           types.String  `tfsdk:"interface"`
	Media               types.String  `tfsdk:"media"`
	MediaSource         types.Int32   `tfsdk:"media_source"`
	DiskSize            types.Float64 `tfsdk:"disksize"`
	PreferredTier       types.String  `tfsdk:"preferred_tier"`
	Enabled             types.Bool    `tfsdk:"enabled"`
	ReadOnly            types.Bool    `tfsdk:"readonly"`
	Serial              types.String  `tfsdk:"serial"`
	Asset               types.String  `tfsdk:"asset"`
	OrderId             types.Int32   `tfsdk:"orderid"`
	PreserveDriveFormat types.Bool    `tfsdk:"preserve_drive_format"`
}

// diskAPIResourceModel is the drive create/update body and the decoded GET.
// Pointers keep false, 0, and "" in the JSON. A nil pointer is omitted.
type diskAPIResourceModel struct {
	Key                 *string `json:"$key,omitempty"`
	Machine             *int32  `json:"machine,omitempty"`
	Name                *string `json:"name,omitempty"`
	Description         *string `json:"description,omitempty"`
	Interface           *string `json:"interface,omitempty"`
	Media               *string `json:"media,omitempty"`
	MediaSource         *int32  `json:"media_source,omitempty"`
	DiskSize            *int64  `json:"disksize,omitempty"`
	PreferredTier       *string `json:"preferred_tier,omitempty"`
	Enabled             *bool   `json:"enabled,omitempty"`
	ReadOnly            *bool   `json:"readonly,omitempty"`
	Serial              *string `json:"serial,omitempty"`
	Asset               *string `json:"asset,omitempty"`
	OrderId             *int32  `json:"orderid,omitempty"`
	PreserveDriveFormat *bool   `json:"preserve_drive_format,omitempty"`
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
	DiskEndpoint = vergeio.DiskEndpoint
)

var _ vergeio.IClient = &DiskApi{}

func NewDiskApi(c *vergeio.Client) *DiskApi {
	sdk, _ := vergeos.NewClient(c.SDKOptions()...)
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

// diskSizeBytes is the API disksize for a configured size in GB.
// A known 0 is returned as a non-nil pointer. Null and unknown are omitted.
func diskSizeBytes(size types.Float64) *int64 {
	if size.IsNull() || size.IsUnknown() {
		return nil
	}
	sizeBytes := int64(size.ValueFloat64() * 1024 * 1024 * 1024)
	return &sizeBytes
}

// diskSizeEpsilonGB is the smallest size difference, in GB, treated as a resize.
// Reads round to 0.01 GB, so a smaller gap is float noise, not a new size.
const diskSizeEpsilonGB = 0.001

// diskSizeChanged reports a planned size the update API should apply.
// A null or unknown plan size is unchanged. The platform rejects any
// disksize on an online IDE drive, including the size the drive already has.
func diskSizeChanged(plan, state types.Float64) bool {
	if plan.IsNull() || plan.IsUnknown() {
		return false
	}
	if state.IsNull() || state.IsUnknown() {
		return true
	}
	return math.Abs(plan.ValueFloat64()-state.ValueFloat64()) > diskSizeEpsilonGB
}

// changedDiskSizeBytes returns disksize when the planned size differs.
func changedDiskSizeBytes(plan, state types.Float64) *int64 {
	if !diskSizeChanged(plan, state) {
		return nil
	}
	return diskSizeBytes(plan)
}

// diskCreatePayload is the JSON body for drive create.
func diskCreatePayload(data *diskResourceModel) diskAPIResourceModel {
	return diskAPIResourceModel{
		Machine:             vergeio.KnownInt32(data.Machine),
		Name:                vergeio.KnownString(data.Name),
		Description:         vergeio.KnownString(data.Description),
		Interface:           vergeio.KnownString(data.Interface),
		Media:               vergeio.KnownString(data.Media),
		MediaSource:         vergeio.KnownInt32(data.MediaSource),
		DiskSize:            diskSizeBytes(data.DiskSize),
		PreferredTier:       vergeio.KnownString(data.PreferredTier),
		Enabled:             vergeio.KnownBool(data.Enabled),
		ReadOnly:            vergeio.KnownBool(data.ReadOnly),
		Serial:              vergeio.KnownString(data.Serial),
		Asset:               vergeio.KnownString(data.Asset),
		OrderId:             vergeio.KnownInt32(data.OrderId),
		PreserveDriveFormat: vergeio.KnownBool(data.PreserveDriveFormat),
	}
}

// diskUpdatePayload is the JSON body for drive update.
// Only attributes that differ from state are set.
func diskUpdatePayload(planData *diskResourceModel, stateData *diskResourceModel) diskAPIResourceModel {
	return diskAPIResourceModel{
		Machine:             vergeio.ChangedInt32(planData.Machine, stateData.Machine),
		Name:                vergeio.ChangedString(planData.Name, stateData.Name),
		Description:         vergeio.ChangedString(planData.Description, stateData.Description),
		Interface:           vergeio.ChangedString(planData.Interface, stateData.Interface),
		DiskSize:            changedDiskSizeBytes(planData.DiskSize, stateData.DiskSize),
		PreferredTier:       vergeio.ChangedString(planData.PreferredTier, stateData.PreferredTier),
		Enabled:             vergeio.ChangedBool(planData.Enabled, stateData.Enabled),
		ReadOnly:            vergeio.ChangedBool(planData.ReadOnly, stateData.ReadOnly),
		Serial:              vergeio.ChangedString(planData.Serial, stateData.Serial),
		Asset:               vergeio.ChangedString(planData.Asset, stateData.Asset),
		OrderId:             vergeio.ChangedInt32(planData.OrderId, stateData.OrderId),
		PreserveDriveFormat: vergeio.ChangedBool(planData.PreserveDriveFormat, stateData.PreserveDriveFormat),
	}
}

// Create the Disk in the API.
func (da *DiskApi) createDisk(ctx context.Context, data *diskResourceModel) error {

	apiData := diskCreatePayload(data)

	// Make a copy of the original data
	origData := *data

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for disk Item")
	}

	// Call the API and check the response
	apiResp, err := da.client.Post(ctx, DiskEndpoint, encodedBuffer)
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
		tflog.Debug(ctx, fmt.Sprintf("Resizing the imported disk from %v to %v", origData.DiskSize.ValueFloat64(), data.DiskSize.ValueFloat64()))
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

	apiData := diskUpdatePayload(planData, stateData)

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Call the API and check the response
	apiResp, err := da.client.Put(ctx, vergeio.ObjectPath(DiskEndpoint, stateData.Key.ValueString()), encodedBuffer)

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
	apiResp, err := da.client.Get(ctx, vergeio.ObjectPath(DiskEndpoint, data.Key.ValueString()), &vergeio.Options{Fields: "machine,name,disksize,interface,media,description,enabled,serial,media_source,preferred_tier,readonly,preserve_drive_format,asset,orderid"})

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
	data.Machine = types.Int32Value(vergeio.Int32Or(diskAPIResp.Machine, 0))
	data.Name = types.StringValue(vergeio.StringOr(diskAPIResp.Name, ""))
	data.Description = types.StringValue(vergeio.StringOr(diskAPIResp.Description, ""))
	data.Interface = types.StringValue(vergeio.StringOr(diskAPIResp.Interface, ""))
	data.DiskSize = types.Float64Value(math.Round(float64(vergeio.Int64Or(diskAPIResp.DiskSize, 0))/(1024*1024*1024)*100) / 100)
	data.PreferredTier = types.StringValue(vergeio.StringOr(diskAPIResp.PreferredTier, ""))
	data.Enabled = types.BoolValue(vergeio.BoolOr(diskAPIResp.Enabled, false))
	data.ReadOnly = types.BoolValue(vergeio.BoolOr(diskAPIResp.ReadOnly, false))
	data.Serial = types.StringValue(vergeio.StringOr(diskAPIResp.Serial, ""))
	data.Asset = types.StringValue(vergeio.StringOr(diskAPIResp.Asset, ""))
	data.OrderId = types.Int32Value(vergeio.Int32Or(diskAPIResp.OrderId, 0))
	data.PreserveDriveFormat = types.BoolValue(vergeio.BoolOr(diskAPIResp.PreserveDriveFormat, false))

	tflog.Debug(ctx, fmt.Sprintf("Finish reading the disk %v", data.Key.ValueString()))

	return nil
}

// readDisksByMachine lists machine_drives for a VM machine and reads each row.
// Import only has the VM id, so drives have to be discovered by machine rather
// than by keys already stored in state. Order is orderid, then name, then key
// so two reads return the same block order.
func (da *DiskApi) readDisksByMachine(ctx context.Context, machineID int32) ([]*diskResourceModel, error) {
	drives, err := da.sdk.VMDrives.List(ctx, int(machineID))
	if err != nil {
		return nil, fmt.Errorf("listing drives for machine %d: %w", machineID, err)
	}
	sortDrivesForState(drives)
	if len(drives) == 0 {
		return nil, nil
	}

	disks := make([]*diskResourceModel, 0, len(drives))
	for _, drive := range drives {
		disk := &diskResourceModel{Key: types.StringValue(strconv.Itoa(drive.ID.Int()))}
		if err := da.readDisk(ctx, disk); err != nil {
			return nil, fmt.Errorf("reading drive %d: %w", drive.ID.Int(), err)
		}
		disks = append(disks, disk)
	}
	return disks, nil
}

// sortDrivesForState orders drives the way nested blocks are stored.
func sortDrivesForState(drives []vergeos.VMDrive) {
	sort.SliceStable(drives, func(i, j int) bool {
		if drives[i].OrderID != drives[j].OrderID {
			return drives[i].OrderID < drives[j].OrderID
		}
		if drives[i].Name != drives[j].Name {
			return drives[i].Name < drives[j].Name
		}
		return drives[i].ID.Int() < drives[j].ID.Int()
	})
}

// preserveDiskConfigFields copies attributes readDisk does not refresh.
// media and media_source stay as configured; a refresh must not drop them.
func preserveDiskConfigFields(prior, current []*diskResourceModel) {
	byKey := make(map[string]*diskResourceModel, len(prior))
	for _, disk := range prior {
		if disk == nil || disk.Key.IsNull() || disk.Key.ValueString() == "" {
			continue
		}
		byKey[disk.Key.ValueString()] = disk
	}
	for _, disk := range current {
		if disk == nil || disk.Key.IsNull() {
			continue
		}
		old, ok := byKey[disk.Key.ValueString()]
		if !ok {
			continue
		}
		disk.Media = old.Media
		disk.MediaSource = old.MediaSource
	}
}

// deleteDisk unplugs a drive that is still attached, waits until it is
// offline, and then deletes it. The unplug action is sent once. Sending it
// again while the guest is still releasing the drive returns 422.
func (da *DiskApi) deleteDisk(ctx context.Context, data *diskResourceModel, vmId types.String) error {

	tflog.Debug(ctx, "Deleting the disk")

	key := data.Key.ValueString()
	var powerState string
	if err := da.checkDiskPowerState(ctx, key, &powerState); err != nil {
		return fmt.Errorf("error checking disk power state %v", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Current disk power state is %v", powerState))

	if !strings.EqualFold(strings.TrimSpace(powerState), "offline") {
		if err := sendUnplug(ctx, powerState, func() error {
			return NewVMApi(da.client).hotplugDrive(ctx, key, vmId, true)
		}); err != nil {
			return fmt.Errorf("failed to unplug drive: %v", err)
		}

		name := ""
		if !data.Name.IsNull() && !data.Name.IsUnknown() {
			name = data.Name.ValueString()
		}
		if err := waitUntilDetached(ctx, func() (string, error) {
			var current string
			readErr := da.checkDiskPowerState(ctx, key, &current)
			return current, readErr
		}, "offline", "drive", name, key, "key"); err != nil {
			return err
		}
	}

	// Call the Get API with the user id and Proceed with user deletion
	_, err := da.client.Delete(ctx, vergeio.ObjectPath(DiskEndpoint, key))

	// error checking
	if err != nil {
		return errors.New("Error deleting the disk: " + err.Error())
	}

	tflog.Debug(ctx, "Disk was successfully deleted")

	return nil
}

// diskBlockID is the drive key stored in state. An empty id means the block
// has no key yet and pairBlocks may match it by name.
func diskBlockID(disk *diskResourceModel) string {
	if disk == nil || disk.Key.IsNull() || disk.Key.IsUnknown() {
		return ""
	}
	return disk.Key.ValueString()
}

func diskBlockName(disk *diskResourceModel) string {
	if disk == nil {
		return ""
	}
	return disk.Name.ValueString()
}

// diskNeedsRecreate reports a change the drive update API cannot apply.
// media and media_source are create-time fields. The schema marks them
// RequiresReplace so the plan shows a VM replacement. syncDisks still
// refuses the change so an update cannot delete the drive and create an
// empty one.
func diskNeedsRecreate(plan, state *diskResourceModel) bool {
	if plan == nil || state == nil {
		return false
	}
	if !plan.Media.IsUnknown() && !plan.Media.Equal(state.Media) {
		return true
	}
	if !plan.MediaSource.IsUnknown() && !plan.MediaSource.Equal(state.MediaSource) {
		return true
	}
	return false
}

// diskNeedsUpdate reports a difference the drive update API can PUT.
// Name is included so a rename is an update of the existing key.
// A null or unknown plan value is not a change. Optional+Computed attributes
// are unknown on an update that does not set them, and ValueString, ValueBool,
// and ValueInt32 read those as "", false, or 0. Comparing the zero values
// would PUT every drive on any VM change.
func diskNeedsUpdate(plan, state *diskResourceModel) bool {
	return vergeio.ChangedString(plan.Name, state.Name) != nil ||
		vergeio.ChangedString(plan.Description, state.Description) != nil ||
		vergeio.ChangedString(plan.Interface, state.Interface) != nil ||
		diskSizeChanged(plan.DiskSize, state.DiskSize) ||
		vergeio.ChangedString(plan.PreferredTier, state.PreferredTier) != nil ||
		vergeio.ChangedBool(plan.Enabled, state.Enabled) != nil ||
		vergeio.ChangedBool(plan.ReadOnly, state.ReadOnly) != nil ||
		vergeio.ChangedString(plan.Serial, state.Serial) != nil ||
		vergeio.ChangedString(plan.Asset, state.Asset) != nil ||
		vergeio.ChangedInt32(plan.OrderId, state.OrderId) != nil ||
		vergeio.ChangedBool(plan.PreserveDriveFormat, state.PreserveDriveFormat) != nil
}

// Update, Create, Delete the Disk in the API.
// This method is called from VM update method.
func (da *DiskApi) syncDisks(ctx context.Context, planData *[]*diskResourceModel, stateData *[]*diskResourceModel, machineId types.Int32, vmId types.String) error {
	plan := *planData
	state := *stateData
	updates, creates, deletes := pairBlocks(plan, state, diskBlockID, diskBlockName)

	for _, pair := range updates {
		if diskNeedsRecreate(pair.plan, pair.state) {
			return fmt.Errorf("drive %q: changing media or media_source replaces the VM; refusing to delete and recreate the drive", pair.plan.Name.ValueString())
		}
	}

	tflog.Debug(ctx, "Syncing disks: Starting deletion")
	for _, old := range deletes {
		if err := da.deleteDisk(ctx, old, vmId); err != nil {
			return fmt.Errorf("failed to delete disk: %v", err)
		}
	}

	tflog.Debug(ctx, "Syncing disks: Starting updation")
	for _, pair := range updates {
		if !diskNeedsUpdate(pair.plan, pair.state) {
			continue
		}
		if err := da.updateDisk(ctx, pair.plan, pair.state); err != nil {
			return fmt.Errorf("failed to update disk: %v", err)
		}
	}

	// A drive created while the VM is stopped is present at the next boot.
	// Update powers the VM on only after this sync. A drive created while
	// the VM is already running stays offline until hotplugdrive.
	vmRunning := false
	if len(creates) > 0 {
		running, err := readVMPowerState(ctx, da.client, vmId)
		if err != nil {
			return fmt.Errorf("failed to read VM power state before adding drives: %w", err)
		}
		vmRunning = running
	}

	tflog.Debug(ctx, "Syncing disks: Starting insertion")
	for _, created := range creates {
		created.Machine = machineId
		if err := da.createDisk(ctx, created); err != nil {
			return fmt.Errorf("failed to create disk: %v", err)
		}
		if err := da.readDisk(ctx, created); err != nil {
			return fmt.Errorf("failed to read disk during the sync: %v", err)
		}
		if vmRunning {
			if err := da.attachCreatedDrive(ctx, created, vmId); err != nil {
				return err
			}
		}
	}

	*stateData = syncedOrder(plan, updates)
	return nil
}

// Check the power state of the disk
func (da *DiskApi) checkDiskPowerState(ctx context.Context, key string, powerState *string) error {

	tflog.Debug(ctx, fmt.Sprintf("Checking the power state of the disk %v", key))

	// Send the kill action request to the vnet_actions endpoint
	apiResp, err := da.client.Get(ctx, vergeio.ObjectPath(DiskEndpoint, key),
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

// attachCreatedDrive hotplugs a drive that was just created on a running VM
// and waits until it is online. A disabled drive is left offline.
// When VergeOS refuses the action, or the drive never comes online, the
// error tells the caller to power cycle the VM. IDE is the known interface
// the platform will not hotplug.
func (da *DiskApi) attachCreatedDrive(ctx context.Context, disk *diskResourceModel, vmId types.String) error {
	if disk == nil || !deviceShouldAttach(disk.Enabled) {
		tflog.Debug(ctx, "Skipping hotplug for a disabled drive")
		return nil
	}

	name := disk.Name.ValueString()
	iface := disk.Interface.ValueString()
	vmAPI := NewVMApi(da.client)
	if err := vmAPI.hotplugDrive(ctx, disk.Key.ValueString(), vmId, false); err != nil {
		return fmt.Errorf("drive %q (interface %q) was created on running VM %s, but VergeOS refused to hotplug it: %w. Some interfaces, such as IDE, cannot be hotplugged and stay offline until the VM is power cycled", name, iface, vmId.ValueString(), err)
	}

	status, err := waitForStatus(ctx, func() (string, error) {
		var current string
		readErr := da.checkDiskPowerState(ctx, disk.Key.ValueString(), &current)
		return current, readErr
	}, "online")
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if status == "" {
			return fmt.Errorf("drive %q was hotplugged but its status could not be read: %w", name, err)
		}
		return fmt.Errorf("drive %q (interface %q) stayed %q after hotplug on running VM %s. Power cycle the VM so the guest can see the drive", name, iface, status, vmId.ValueString())
	}
	return nil
}

// hotplugDrive sends hotplugdrive. unplug detaches the drive; otherwise the
// action attaches a drive that is already assigned to the VM.
func (va *VMApi) hotplugDrive(ctx context.Context, driveId string, vmId types.String, unplug bool) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the hotplug drive API for the drive %v unplug %v", driveId, unplug))

	// Create the action payload according to vnet_actions schema.
	// Unplug is omitted when attaching. The API treats a missing unplug flag
	// as hotplug, which brings an offline drive online.
	params := VMActionParams{
		Device: driveId,
	}
	if unplug {
		params.Unplug = true
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
	req, err := va.client.Post(ctx, VMActionEndpoint, bytes.NewBuffer(bytedata))
	if err != nil {
		return err
	}
	if req.StatusCode != 201 {
		return fmt.Errorf("failed to hotplug drive: status code %v", req.StatusCode)
	}

	return nil
}
