// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	stdpath "path"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const (
	vmImportTargetVM    = "vm"
	vmImportTargetMedia = "media"

	vmImportWaitDefault   = 10 * time.Minute
	vmImportDeleteDefault = 2 * time.Minute
)

// vmImportPollInterval is how often a wait reads the import or the catalog file.
var vmImportPollInterval = 5 * time.Second

const (
	// downloadedImportDescription is the description govergeos writes when it
	// downloads a URL into the media catalog before posting vm_imports.
	downloadedImportDescription = "Downloaded for VM import "
	downloadedMediaDescription  = "Downloaded for media import "
)

var hexKeyPattern = regexp.MustCompile(`(?i)^[0-9a-f]{40}$`)

// diskImageExt lists extensions VergeOS refuses on vm_imports with
// "Unknown Import file type". Those files belong in the media catalog.
var diskImageExt = map[string]struct{}{
	".qcow2": {},
	".qcow":  {},
	".vmdk":  {},
	".vhd":   {},
	".vhdx":  {},
	".raw":   {},
	".img":   {},
	".vdi":   {},
	".iso":   {},
}

var vmPackageExt = map[string]struct{}{
	".ova": {},
	".ovf": {},
}

// importAPI is the govergeos client for vm_imports and catalog files.
type importAPI struct {
	client *vergeio.Client
	sdk    *vergeos.Client
	vm     *VMApi

	versionMu    sync.Mutex
	versionKnown bool
	version262   bool
}

func newImportAPI(c *vergeio.Client) (*importAPI, error) {
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	vm, err := NewVMApi(c)
	if err != nil {
		return nil, err
	}
	return &importAPI{client: c, sdk: sdk, vm: vm}, nil
}

type importTimeouts struct {
	Create types.String `tfsdk:"create"`
	Delete types.String `tfsdk:"delete"`
}

type importModel struct {
	ID                     types.String    `tfsdk:"id"`
	Name                   types.String    `tfsdk:"name"`
	Target                 types.String    `tfsdk:"target"`
	File                   types.Int64     `tfsdk:"file"`
	URL                    types.String    `tfsdk:"url"`
	FileName               types.String    `tfsdk:"file_name"`
	Volume                 types.String    `tfsdk:"volume"`
	VolumePath             types.String    `tfsdk:"volume_path"`
	SharedObject           types.Int64     `tfsdk:"shared_object"`
	PreserveMACs           types.Bool      `tfsdk:"preserve_macs"`
	PreserveDriveFormat    types.Bool      `tfsdk:"preserve_drive_format"`
	PreserveBIOSSerial     types.Bool      `tfsdk:"preserve_bios_serial"`
	GuestAgentPreinstalled types.Bool      `tfsdk:"guest_agent_preinstalled"`
	ReplicateTags          types.Bool      `tfsdk:"replicate_tags"`
	PreferredTier          types.String    `tfsdk:"preferred_tier"`
	NoOpticalDrives        types.Bool      `tfsdk:"no_optical_drives"`
	OverrideDriveInterface types.String    `tfsdk:"override_drive_interface"`
	OverrideNICInterface   types.String    `tfsdk:"override_nic_interface"`
	DeleteVM               types.Bool      `tfsdk:"delete_vm"`
	DeleteFile             types.Bool      `tfsdk:"delete_file"`
	Status                 types.String    `tfsdk:"status"`
	StatusInfo             types.String    `tfsdk:"status_info"`
	VMID                   types.Int64     `tfsdk:"vm_id"`
	FileID                 types.Int64     `tfsdk:"file_id"`
	UUID                   types.String    `tfsdk:"uuid"`
	Importing              types.Bool      `tfsdk:"importing"`
	Aborted                types.Bool      `tfsdk:"aborted"`
	FailedDriveCount       types.Int64     `tfsdk:"failed_drive_count"`
	Adopted                types.Bool      `tfsdk:"adopted"`
	OwnsFile               types.Bool      `tfsdk:"owns_file"`
	Timeouts               *importTimeouts `tfsdk:"timeouts"`
}

type importDataModel struct {
	ID                     types.String `tfsdk:"id"`
	Name                   types.String `tfsdk:"name"`
	Wait                   types.Bool   `tfsdk:"wait"`
	Timeout                types.String `tfsdk:"timeout"`
	Status                 types.String `tfsdk:"status"`
	StatusInfo             types.String `tfsdk:"status_info"`
	VMID                   types.Int64  `tfsdk:"vm_id"`
	FileID                 types.Int64  `tfsdk:"file_id"`
	Volume                 types.String `tfsdk:"volume"`
	VolumePath             types.String `tfsdk:"volume_path"`
	SharedObject           types.Int64  `tfsdk:"shared_object"`
	UUID                   types.String `tfsdk:"uuid"`
	PreferredTier          types.String `tfsdk:"preferred_tier"`
	Importing              types.Bool   `tfsdk:"importing"`
	Aborted                types.Bool   `tfsdk:"aborted"`
	FailedDriveCount       types.Int64  `tfsdk:"failed_drive_count"`
	PreserveMACs           types.Bool   `tfsdk:"preserve_macs"`
	PreserveDriveFormat    types.Bool   `tfsdk:"preserve_drive_format"`
	PreserveBIOSSerial     types.Bool   `tfsdk:"preserve_bios_serial"`
	GuestAgentPreinstalled types.Bool   `tfsdk:"guest_agent_preinstalled"`
	ReplicateTags          types.Bool   `tfsdk:"replicate_tags"`
	NoOpticalDrives        types.Bool   `tfsdk:"no_optical_drives"`
	OverrideDriveInterface types.String `tfsdk:"override_drive_interface"`
	OverrideNICInterface   types.String `tfsdk:"override_nic_interface"`
}

func validateImportModel(data *importModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if data == nil {
		diags.AddError("Missing VM import", "The import configuration was empty.")
		return diags
	}
	target := vmImportTargetVM
	if !data.Target.IsNull() && !data.Target.IsUnknown() {
		target = data.Target.ValueString()
	}
	sources := 0
	fileSet := intSet(data.File)
	urlSet := stringSet(data.URL)
	volumeSet := stringSet(data.Volume)
	sharedSet := intSet(data.SharedObject)
	if fileSet {
		sources++
	}
	if urlSet {
		sources++
	}
	if volumeSet {
		sources++
	}
	if sharedSet {
		sources++
	}
	known := !data.File.IsUnknown() && !data.URL.IsUnknown() && !data.Volume.IsUnknown() && !data.SharedObject.IsUnknown()
	if known && target == vmImportTargetMedia {
		if fileSet || volumeSet || sharedSet || stringSet(data.VolumePath) {
			diags.AddAttributeError(path.Root("url"), "Media import takes a URL", "A disk image is downloaded into the media catalog. Set url, and leave file, volume, volume_path, and shared_object empty.")
		}
		if !urlSet {
			diags.AddAttributeError(path.Root("url"), "Missing disk image URL", "Set url to an http or https address of the disk image.")
		}
	}
	if known && target == vmImportTargetVM && sources != 1 {
		diags.AddAttributeError(path.Root("name"), "VM import needs one source", "Set one of file, url, volume, or shared_object.")
	}
	if known && stringSet(data.VolumePath) && !volumeSet {
		diags.AddAttributeError(path.Root("volume_path"), "volume_path needs volume", "Set volume to the NAS volume key that holds the OVA or OVF.")
	}
	if known && stringSet(data.FileName) && !urlSet {
		diags.AddAttributeError(path.Root("file_name"), "file_name needs url", "file_name names the catalog file downloaded from url.")
	}
	if urlSet {
		if err := validateImportURL(data.URL.ValueString()); err != nil {
			diags.AddAttributeError(path.Root("url"), "Invalid import URL", err.Error())
		} else if err := validateImportExtension(target, data.URL.ValueString(), stringValue(data.FileName)); err != nil {
			diags.AddAttributeError(path.Root("target"), "Import target does not match the file", err.Error())
		}
	}
	if stringSet(data.PreferredTier) {
		switch data.PreferredTier.ValueString() {
		case "1", "2", "3", "4", "5":
		default:
			diags.AddAttributeError(path.Root("preferred_tier"), "Invalid storage tier", "preferred_tier must be a digit from 1 through 5.")
		}
	}
	if stringSet(data.Volume) && !hexKeyPattern.MatchString(data.Volume.ValueString()) {
		diags.AddAttributeError(path.Root("volume"), "Invalid volume key", "volume is the NAS volume key, a 40 character hex string.")
	}
	return diags
}

func (a *importAPI) create(ctx context.Context, data *importModel) error {
	if data == nil {
		return fmt.Errorf("vm import is missing")
	}
	target := data.Target.ValueString()
	if target == "" {
		target = vmImportTargetVM
		data.Target = types.StringValue(target)
	}
	if target == vmImportTargetMedia {
		return a.createMedia(ctx, data)
	}
	return a.createVMImport(ctx, data)
}

func (a *importAPI) createMedia(ctx context.Context, data *importModel) error {
	name := strings.TrimSpace(data.Name.ValueString())
	rawURL := strings.TrimSpace(data.URL.ValueString())
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if err := validateImportURL(rawURL); err != nil {
		return err
	}
	if err := validateImportExtension(vmImportTargetMedia, rawURL, stringValue(data.FileName)); err != nil {
		return err
	}
	catalogName := strings.TrimSpace(stringValue(data.FileName))
	if catalogName == "" {
		catalogName = name
	}
	req := &vergeos.FileCreateRequest{
		Name:          catalogName,
		Description:   downloadedMediaDescription + name,
		URL:           rawURL,
		PreferredTier: strings.TrimSpace(stringValue(data.PreferredTier)),
	}
	file, err := a.sdk.Files.Create(ctx, req)
	if err != nil {
		return err
	}
	if file == nil || file.Key.Int() <= 0 {
		return fmt.Errorf("VergeOS did not return a file id for %q", catalogName)
	}
	data.ID = types.StringValue(strconv.Itoa(file.Key.Int()))
	data.OwnsFile = types.BoolValue(true)
	data.Adopted = types.BoolValue(false)
	data.Target = types.StringValue(vmImportTargetMedia)
	timeout, err := importTimeout(data.Timeouts, "create", vmImportWaitDefault)
	if err != nil {
		return err
	}
	ready, err := a.waitForFile(ctx, file.Key.Int(), timeout)
	if err != nil {
		if cleanupErr := a.deleteFile(ctx, file.Key.Int()); cleanupErr != nil {
			return fmt.Errorf("%w (the catalog file %d was left in place: %v)", err, file.Key.Int(), cleanupErr)
		}
		data.ID = types.StringNull()
		return err
	}
	return applyMediaFile(data, ready)
}

func (a *importAPI) createVMImport(ctx context.Context, data *importModel) error {
	req, err := vmImportRequest(data)
	if err != nil {
		return err
	}
	if err := validateImportExtension(vmImportTargetVM, req.URL, req.FileName); err != nil {
		return err
	}
	created, err := a.sdk.VMImports.Create(ctx, req)
	if err != nil {
		if req.URL != "" {
			if cleanupErr := a.deleteOrphanDownload(ctx, req); cleanupErr != nil {
				return fmt.Errorf("%w (catalog cleanup: %v)", err, cleanupErr)
			}
		}
		return err
	}
	key := vmImportKey(created)
	if created.AlreadyExisted && key == "" {
		return fmt.Errorf("VM %q already exists, and VergeOS did not return one vm_imports row for that name", req.Name)
	}
	if key == "" {
		return fmt.Errorf("VergeOS did not return a vm_imports key for %q", req.Name)
	}
	data.ID = types.StringValue(key)
	data.Adopted = types.BoolValue(created.AlreadyExisted)
	data.OwnsFile = types.BoolValue(req.URL != "" && !created.AlreadyExisted)
	data.Target = types.StringValue(vmImportTargetVM)
	if created.AlreadyExisted {
		tflog.Debug(ctx, fmt.Sprintf("VM %s already exists; using import %s", req.Name, key))
		return a.readVMImport(ctx, data, key)
	}
	timeout, err := importTimeout(data.Timeouts, "create", vmImportWaitDefault)
	if err != nil {
		return err
	}
	finished, err := a.sdk.VMImports.Wait(ctx, key, &vergeos.VMImportWaitOptions{
		Timeout:      timeout,
		PollInterval: vmImportPollInterval,
	})
	if err != nil {
		if cleanupErr := a.deleteOwned(ctx, data); cleanupErr != nil {
			return fmt.Errorf("%w (cleanup: %v)", err, cleanupErr)
		}
		data.ID = types.StringNull()
		return err
	}
	return a.applyVMImport(ctx, data, finished)
}

func (a *importAPI) read(ctx context.Context, data *importModel) error {
	if data == nil {
		return fmt.Errorf("vm import is missing")
	}
	id := strings.TrimSpace(data.ID.ValueString())
	if id == "" {
		return fmt.Errorf("vm import id is empty")
	}
	target := ""
	if !data.Target.IsNull() && !data.Target.IsUnknown() {
		target = data.Target.ValueString()
	}
	if hexKeyPattern.MatchString(id) {
		if target == vmImportTargetMedia {
			return fmt.Errorf("vm import %s is a vm_imports key and target is media", id)
		}
		return a.readVMImport(ctx, data, id)
	}
	fileID, err := strconv.Atoi(id)
	if err != nil || fileID <= 0 {
		return fmt.Errorf("vm import id %q is not a vm_imports key or a media file id", id)
	}
	if target == vmImportTargetVM {
		return fmt.Errorf("vm import %s is a media file id and target is vm", id)
	}
	return a.readMedia(ctx, data, fileID)
}

func (a *importAPI) readVMImport(ctx context.Context, data *importModel, id string) error {
	row, err := a.sdk.VMImports.Get(ctx, id)
	if err != nil {
		return err
	}
	if row == nil || vmImportKey(row) == "" {
		return &vergeos.NotFoundError{Resource: "VMImport", ID: id}
	}
	return a.applyVMImport(ctx, data, row)
}

func (a *importAPI) readMedia(ctx context.Context, data *importModel, id int) error {
	file, err := a.sdk.Files.Get(ctx, id)
	if err != nil {
		return err
	}
	if file == nil || file.Key.Int() <= 0 {
		return &vergeos.NotFoundError{Resource: "File", ID: id}
	}
	if !mediaOwned(data, file) {
		return &vergeos.NotFoundError{Resource: "File", ID: id}
	}
	return applyMediaFile(data, file)
}

func (a *importAPI) update(ctx context.Context, plan, state *importModel) error {
	if plan == nil || state == nil {
		return fmt.Errorf("vm import is missing")
	}
	plan.ID = state.ID
	plan.Adopted = state.Adopted
	plan.OwnsFile = state.OwnsFile
	plan.Target = state.Target
	return a.read(ctx, plan)
}

func (a *importAPI) delete(ctx context.Context, data *importModel) error {
	if data == nil {
		return fmt.Errorf("vm import is missing")
	}
	return a.deleteOwned(ctx, data)
}

func (a *importAPI) deleteOwned(ctx context.Context, data *importModel) error {
	id := strings.TrimSpace(data.ID.ValueString())
	if id == "" {
		return nil
	}
	if !hexKeyPattern.MatchString(id) {
		fileID, err := strconv.Atoi(id)
		if err != nil || fileID <= 0 {
			return fmt.Errorf("vm import id %q is not a media file id", id)
		}
		return a.deleteFile(ctx, fileID)
	}
	row, err := a.sdk.VMImports.Get(ctx, id)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	if row != nil && importStillRunning(row) {
		if err := a.sdk.VMImports.Abort(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
			return err
		}
	}
	if deleteVM(data) {
		vmID := 0
		if row != nil && row.VM != nil {
			vmID = row.VM.Int()
		}
		if vmID <= 0 {
			vmID = int(int64Value(data.VMID))
		}
		if vmID > 0 {
			if err := a.deleteVM(ctx, data, vmID); err != nil {
				return err
			}
		}
	}
	if err := a.sdk.VMImports.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	if deleteDownloadedFile(data) {
		fileID := 0
		if row != nil && row.File != nil {
			fileID = row.File.Int()
		}
		if fileID <= 0 {
			fileID = int(int64Value(data.FileID))
		}
		if fileID > 0 {
			if err := a.deleteFile(ctx, fileID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *importAPI) deleteVM(ctx context.Context, data *importModel, vmID int) error {
	timeout, err := importTimeout(data.Timeouts, "delete", vmImportDeleteDefault)
	if err != nil {
		return err
	}
	name := stringValue(data.Name)
	if err := a.vm.gracefulPowerOff(ctx, strconv.Itoa(vmID), name, timeout, true); err != nil && !vmAlreadyGone(err) {
		return err
	}
	if err := a.sdk.VMs.Delete(ctx, vmID); err != nil && !vmAlreadyGone(err) {
		return err
	}
	return nil
}

func (a *importAPI) deleteFile(ctx context.Context, id int) error {
	if id <= 0 {
		return nil
	}
	if err := a.sdk.Files.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	return nil
}

func (a *importAPI) deleteOrphanDownload(ctx context.Context, req *vergeos.VMImportCreateRequest) error {
	name := req.FileName
	if name == "" {
		var err error
		name, err = fileNameFromImportURL(req.URL)
		if err != nil {
			return nil
		}
	}
	file, err := a.sdk.Files.GetByName(ctx, name)
	if vergeos.IsNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if file.URL != req.URL || file.Description != downloadedImportDescription+req.Name {
		return nil
	}
	return a.deleteFile(ctx, file.Key.Int())
}

func (a *importAPI) waitForFile(ctx context.Context, id int, timeout time.Duration) (*vergeos.File, error) {
	if timeout <= 0 {
		timeout = vmImportWaitDefault
	}
	deadline := time.Now().Add(timeout)
	for {
		file, err := a.sdk.Files.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if fileReady(file) {
			return file, nil
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return nil, fmt.Errorf("catalog file %d did not finish downloading before %s", id, timeout)
		}
		sleep := vmImportPollInterval
		if sleep > remaining {
			sleep = remaining
		}
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (a *importAPI) readData(ctx context.Context, data *importDataModel) error {
	if data == nil {
		return fmt.Errorf("vm import is missing")
	}
	row, err := a.lookupImport(ctx, data)
	if err != nil {
		return err
	}
	key := vmImportKey(row)
	if key == "" {
		return fmt.Errorf("VergeOS did not return a vm_imports key")
	}
	wait := true
	if !data.Wait.IsNull() && !data.Wait.IsUnknown() {
		wait = data.Wait.ValueBool()
	}
	if wait && importNeedsWait(row) {
		timeout := vmImportWaitDefault
		if stringSet(data.Timeout) {
			timeout, err = time.ParseDuration(strings.TrimSpace(data.Timeout.ValueString()))
			if err != nil {
				return fmt.Errorf("timeout %q is not a duration", data.Timeout.ValueString())
			}
		}
		row, err = a.sdk.VMImports.Wait(ctx, key, &vergeos.VMImportWaitOptions{
			Timeout:      timeout,
			PollInterval: vmImportPollInterval,
		})
		if err != nil {
			return err
		}
	}
	return a.applyImportData(ctx, data, row)
}

func (a *importAPI) lookupImport(ctx context.Context, data *importDataModel) (*vergeos.VMImport, error) {
	if stringSet(data.ID) {
		id := strings.TrimSpace(data.ID.ValueString())
		if !hexKeyPattern.MatchString(id) {
			return nil, fmt.Errorf("id %q is not a 40 character hex vm_imports key", id)
		}
		return a.sdk.VMImports.Get(ctx, id)
	}
	name := strings.TrimSpace(stringValue(data.Name))
	if name == "" {
		return nil, fmt.Errorf("set id or name")
	}
	return a.sdk.VMImports.GetByName(ctx, name)
}

func (a *importAPI) applyVMImport(ctx context.Context, data *importModel, row *vergeos.VMImport) error {
	if row == nil {
		return fmt.Errorf("vm import read returned no row")
	}
	key := vmImportKey(row)
	if key == "" {
		return fmt.Errorf("vm import read returned no key")
	}
	newer, err := a.atLeast262(ctx)
	if err != nil {
		return err
	}
	adopted := data.Adopted
	owns := data.OwnsFile
	timeouts := data.Timeouts
	deleteVMFlag := data.DeleteVM
	deleteFileFlag := data.DeleteFile
	target := data.Target
	inputs := *data

	data.ID = types.StringValue(key)
	data.Target = types.StringValue(vmImportTargetVM)
	if !target.IsNull() && !target.IsUnknown() && target.ValueString() != "" {
		data.Target = target
	}
	data.Name = types.StringValue(row.Name)
	data.Status = types.StringValue(row.Status)
	data.StatusInfo = stringOrNull(row.StatusInfo)
	data.UUID = stringOrNull(row.UUID)
	data.Importing = types.BoolValue(row.Importing)
	data.Aborted = types.BoolValue(row.Aborted)
	data.FailedDriveCount = types.Int64Value(int64(row.FailedDriveCount))
	data.VMID = flexIntOrNull(row.VM)
	data.FileID = flexIntOrNull(row.File)
	data.PreserveMACs = mergeBool(inputs.PreserveMACs, row.PreserveMACs, true)
	data.PreserveDriveFormat = mergeBool(inputs.PreserveDriveFormat, row.PreserveDriveFormat, true)
	data.PreserveBIOSSerial = mergeBool(inputs.PreserveBIOSSerial, row.PreserveBIOSSerial, newer)
	data.GuestAgentPreinstalled = mergeBool(inputs.GuestAgentPreinstalled, row.GuestAgentPreinstalled, newer)
	data.ReplicateTags = mergeBool(inputs.ReplicateTags, row.ReplicateTags, newer)
	data.NoOpticalDrives = mergeBool(inputs.NoOpticalDrives, row.NoOpticalDrives, true)
	data.PreferredTier = mergeString(inputs.PreferredTier, row.PreferredTier)
	data.OverrideDriveInterface = mergeString(inputs.OverrideDriveInterface, row.OverrideDriveInterface)
	data.OverrideNICInterface = mergeString(inputs.OverrideNICInterface, row.OverrideNICInterface)
	data.Volume = mergeString(inputs.Volume, row.Volume)
	data.VolumePath = mergeString(inputs.VolumePath, row.VolumePath)
	data.URL = inputs.URL
	data.FileName = inputs.FileName
	data.File = inputs.File
	data.SharedObject = inputs.SharedObject
	if data.SharedObject.IsNull() || data.SharedObject.IsUnknown() {
		data.SharedObject = flexIntOrNull(row.SharedObject)
	}
	data.DeleteVM = deleteVMFlag
	data.DeleteFile = deleteFileFlag
	data.Timeouts = timeouts
	if adopted.IsNull() || adopted.IsUnknown() {
		data.Adopted = types.BoolValue(false)
	} else {
		data.Adopted = adopted
	}
	if owns.IsNull() || owns.IsUnknown() {
		data.OwnsFile = types.BoolValue(false)
	} else {
		data.OwnsFile = owns
	}
	return nil
}

func (a *importAPI) applyImportData(ctx context.Context, data *importDataModel, row *vergeos.VMImport) error {
	if row == nil {
		return fmt.Errorf("vm import read returned no row")
	}
	newer, err := a.atLeast262(ctx)
	if err != nil {
		return err
	}
	wait := data.Wait
	timeout := data.Timeout
	data.ID = types.StringValue(vmImportKey(row))
	data.Name = types.StringValue(row.Name)
	data.Wait = wait
	data.Timeout = timeout
	data.Status = types.StringValue(row.Status)
	data.StatusInfo = stringOrNull(row.StatusInfo)
	data.UUID = stringOrNull(row.UUID)
	data.VMID = flexIntOrNull(row.VM)
	data.FileID = flexIntOrNull(row.File)
	data.Volume = stringOrNull(row.Volume)
	data.VolumePath = stringOrNull(row.VolumePath)
	data.SharedObject = flexIntOrNull(row.SharedObject)
	data.PreferredTier = stringOrNull(row.PreferredTier)
	data.Importing = types.BoolValue(row.Importing)
	data.Aborted = types.BoolValue(row.Aborted)
	data.FailedDriveCount = types.Int64Value(int64(row.FailedDriveCount))
	data.PreserveMACs = types.BoolValue(row.PreserveMACs)
	data.PreserveDriveFormat = types.BoolValue(row.PreserveDriveFormat)
	data.NoOpticalDrives = types.BoolValue(row.NoOpticalDrives)
	data.OverrideDriveInterface = stringOrNull(row.OverrideDriveInterface)
	data.OverrideNICInterface = stringOrNull(row.OverrideNICInterface)
	if newer {
		data.PreserveBIOSSerial = types.BoolValue(row.PreserveBIOSSerial)
		data.GuestAgentPreinstalled = types.BoolValue(row.GuestAgentPreinstalled)
		data.ReplicateTags = types.BoolValue(row.ReplicateTags)
	} else {
		data.PreserveBIOSSerial = types.BoolNull()
		data.GuestAgentPreinstalled = types.BoolNull()
		data.ReplicateTags = types.BoolNull()
	}
	return nil
}

func applyMediaFile(data *importModel, file *vergeos.File) error {
	if data == nil || file == nil || file.Key.Int() <= 0 {
		return fmt.Errorf("media file read returned no file")
	}
	name := data.Name
	urlValue := data.URL
	fileName := data.FileName
	tier := data.PreferredTier
	deleteVMFlag := data.DeleteVM
	deleteFileFlag := data.DeleteFile
	timeouts := data.Timeouts
	owns := data.OwnsFile
	data.ID = types.StringValue(strconv.Itoa(file.Key.Int()))
	data.Target = types.StringValue(vmImportTargetMedia)
	data.FileID = types.Int64Value(int64(file.Key.Int()))
	if stringSet(name) {
		data.Name = name
	} else if file.Name != "" {
		data.Name = types.StringValue(file.Name)
	}
	data.URL = urlValue
	data.FileName = fileName
	data.PreferredTier = mergeString(tier, file.PreferredTier)
	data.Status = types.StringValue("complete")
	data.StatusInfo = types.StringNull()
	data.VMID = types.Int64Null()
	data.UUID = types.StringNull()
	data.Importing = types.BoolValue(false)
	data.Aborted = types.BoolValue(false)
	data.FailedDriveCount = types.Int64Value(0)
	data.Adopted = types.BoolValue(false)
	data.DeleteVM = deleteVMFlag
	data.DeleteFile = deleteFileFlag
	data.Timeouts = timeouts
	if owns.IsNull() || owns.IsUnknown() {
		data.OwnsFile = types.BoolValue(true)
	} else {
		data.OwnsFile = owns
	}
	data.File = types.Int64Null()
	data.Volume = types.StringNull()
	data.VolumePath = types.StringNull()
	data.SharedObject = types.Int64Null()
	return nil
}

func (a *importAPI) atLeast262(ctx context.Context) (bool, error) {
	a.versionMu.Lock()
	defer a.versionMu.Unlock()
	if a.versionKnown {
		return a.version262, nil
	}
	info, err := a.sdk.System.GetInfo(ctx)
	if err != nil {
		return false, fmt.Errorf("reading VergeOS version: %w", err)
	}
	if info == nil {
		return false, fmt.Errorf("reading VergeOS version: empty response")
	}
	a.version262 = versionAtLeast(info.Version, 26, 2)
	a.versionKnown = true
	return a.version262, nil
}

func vmImportRequest(data *importModel) (*vergeos.VMImportCreateRequest, error) {
	name := strings.TrimSpace(data.Name.ValueString())
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	fileID, err := positiveIntPtr(data.File, "file")
	if err != nil {
		return nil, err
	}
	sharedID, err := positiveIntPtr(data.SharedObject, "shared_object")
	if err != nil {
		return nil, err
	}
	req := &vergeos.VMImportCreateRequest{
		Name:                   name,
		File:                   fileID,
		URL:                    strings.TrimSpace(stringValue(data.URL)),
		FileName:               strings.TrimSpace(stringValue(data.FileName)),
		Volume:                 strings.TrimSpace(stringValue(data.Volume)),
		VolumePath:             strings.TrimSpace(stringValue(data.VolumePath)),
		SharedObject:           sharedID,
		PreserveMACs:           vergeio.KnownBool(data.PreserveMACs),
		PreserveDriveFormat:    vergeio.KnownBool(data.PreserveDriveFormat),
		PreserveBIOSSerial:     vergeio.KnownBool(data.PreserveBIOSSerial),
		GuestAgentPreinstalled: vergeio.KnownBool(data.GuestAgentPreinstalled),
		ReplicateTags:          vergeio.KnownBool(data.ReplicateTags),
		PreferredTier:          strings.TrimSpace(stringValue(data.PreferredTier)),
		NoOpticalDrives:        vergeio.KnownBool(data.NoOpticalDrives),
		OverrideDriveInterface: overrideOrEmpty(data.OverrideDriveInterface),
		OverrideNICInterface:   overrideOrEmpty(data.OverrideNICInterface),
	}
	return req, nil
}

func validateImportExtension(target, rawURL, fileName string) error {
	if rawURL == "" {
		return nil
	}
	name := strings.TrimSpace(fileName)
	if name == "" {
		var err error
		name, err = fileNameFromImportURL(rawURL)
		if err != nil {
			return err
		}
	}
	ext := strings.ToLower(stdpath.Ext(name))
	switch target {
	case vmImportTargetMedia:
		if _, ok := vmPackageExt[ext]; ok {
			return fmt.Errorf("%s is an OVA or OVF. Leave target as vm so VergeOS imports it as a virtual machine", name)
		}
	default:
		if _, ok := diskImageExt[ext]; ok {
			return fmt.Errorf("%s is a disk image. VergeOS rejects it on vm_imports. Set target = \"media\" and attach the catalog file with vergeio_vm_drive media = \"import\"", name)
		}
	}
	return nil
}

func validateImportURL(raw string) error {
	raw = strings.TrimSpace(raw)
	rest, ok := strings.CutPrefix(raw, "https://")
	if !ok {
		rest, ok = strings.CutPrefix(raw, "http://")
	}
	if !ok || rest == "" || strings.HasPrefix(rest, "/") {
		return fmt.Errorf("url must be an absolute http or https URL")
	}
	host := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host = rest[:i]
	}
	if host == "" || strings.Contains(host, " ") {
		return fmt.Errorf("url must be an absolute http or https URL")
	}
	return nil
}

func fileNameFromImportURL(raw string) (string, error) {
	if err := validateImportURL(raw); err != nil {
		return "", err
	}
	rest := strings.TrimSpace(raw)
	rest = strings.TrimPrefix(strings.TrimPrefix(rest, "https://"), "http://")
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	slash := strings.LastIndex(rest, "/")
	if slash < 0 || slash == len(rest)-1 {
		return "", fmt.Errorf("url path must name a file, or set file_name")
	}
	name, err := pathUnescape(rest[slash+1:])
	if err != nil || name == "" || name == "." || name == "/" || strings.Contains(name, "/") {
		return "", fmt.Errorf("url path must name a file, or set file_name")
	}
	return name, nil
}

func pathUnescape(value string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			b.WriteByte(value[i])
			continue
		}
		if i+2 >= len(value) {
			return "", fmt.Errorf("bad escape")
		}
		n, err := strconv.ParseUint(value[i+1:i+3], 16, 8)
		if err != nil {
			return "", err
		}
		b.WriteByte(byte(n))
		i += 2
	}
	return b.String(), nil
}

func versionAtLeast(version string, major, minor int) bool {
	version = strings.TrimSpace(version)
	// A prerelease is not that release. 26.2.0-rc1 does not have the 26.2 fields.
	if version == "" || strings.ContainsAny(version, "-+") {
		return false
	}
	parts := strings.Split(version, ".")
	nums := make([]int, 3)
	for i := 0; i < len(parts) && i < 3; i++ {
		n, err := strconv.Atoi(parts[i])
		if err != nil {
			return false
		}
		nums[i] = n
	}
	if nums[0] != major {
		return nums[0] > major
	}
	return nums[1] >= minor
}

func importTimeout(block *importTimeouts, field string, fallback time.Duration) (time.Duration, error) {
	if block == nil {
		return fallback, nil
	}
	var raw types.String
	switch field {
	case "create":
		raw = block.Create
	case "delete":
		raw = block.Delete
	default:
		return 0, fmt.Errorf("unknown timeout %q", field)
	}
	if raw.IsNull() || raw.IsUnknown() || strings.TrimSpace(raw.ValueString()) == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(strings.TrimSpace(raw.ValueString()))
	if err != nil || d < 0 {
		return 0, fmt.Errorf("timeouts.%s %q is not a duration", field, raw.ValueString())
	}
	return d, nil
}

func importNeedsWait(row *vergeos.VMImport) bool {
	if row == nil {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(row.Status)) {
	case vergeos.VMImportStatusComplete, vergeos.VMImportStatusWarning:
		return false
	default:
		return true
	}
}

func importStillRunning(row *vergeos.VMImport) bool {
	if row == nil || row.Aborted {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(row.Status)) {
	case vergeos.VMImportStatusComplete, vergeos.VMImportStatusWarning, vergeos.VMImportStatusError, vergeos.VMImportStatusAborted:
		return false
	case vergeos.VMImportStatusImporting, vergeos.VMImportStatusInitializing:
		return true
	}
	return row.Importing
}

func mediaOwned(data *importModel, file *vergeos.File) bool {
	if data == nil || file == nil {
		return false
	}
	if !stringSet(data.Name) {
		return true
	}
	name := data.Name.ValueString()
	if file.Name == name {
		return true
	}
	if stringSet(data.FileName) && file.Name == strings.TrimSpace(data.FileName.ValueString()) {
		return true
	}
	return file.Description == downloadedMediaDescription+name
}

func fileReady(file *vergeos.File) bool {
	if file == nil {
		return false
	}
	return file.Filesize > 0 || file.UsedBytes > 0 || file.AllocatedBytes > 0
}

func deleteVM(data *importModel) bool {
	if data == nil {
		return false
	}
	if !data.Adopted.IsNull() && !data.Adopted.IsUnknown() && data.Adopted.ValueBool() {
		return false
	}
	if data.DeleteVM.IsNull() || data.DeleteVM.IsUnknown() {
		return true
	}
	return data.DeleteVM.ValueBool()
}

func deleteDownloadedFile(data *importModel) bool {
	if data == nil || data.OwnsFile.IsNull() || data.OwnsFile.IsUnknown() || !data.OwnsFile.ValueBool() {
		return false
	}
	if data.DeleteFile.IsNull() || data.DeleteFile.IsUnknown() {
		return true
	}
	return data.DeleteFile.ValueBool()
}

func vmImportKey(row *vergeos.VMImport) string {
	if row == nil {
		return ""
	}
	if row.Key != "" {
		return row.Key
	}
	return row.ID
}

func importIDSet(data *importModel) bool {
	return data != nil && !data.ID.IsNull() && !data.ID.IsUnknown() && strings.TrimSpace(data.ID.ValueString()) != ""
}

func stringSet(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && strings.TrimSpace(v.ValueString()) != ""
}

func intSet(v types.Int64) bool {
	return !v.IsNull() && !v.IsUnknown()
}

func stringValue(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

func int64Value(v types.Int64) int64 {
	n := vergeio.KnownInt64(v)
	if n == nil {
		return 0
	}
	return *n
}

func positiveIntPtr(v types.Int64, field string) (*int, error) {
	if !intSet(v) {
		return nil, nil
	}
	if v.ValueInt64() <= 0 {
		return nil, fmt.Errorf("%s must be a positive id", field)
	}
	n := int(v.ValueInt64())
	return &n, nil
}

func overrideOrEmpty(v types.String) string {
	text := strings.TrimSpace(stringValue(v))
	if strings.EqualFold(text, "default") {
		return ""
	}
	return text
}

func mergeBool(config types.Bool, api, trustAPI bool) types.Bool {
	if config.IsNull() || config.IsUnknown() {
		return types.BoolNull()
	}
	if !trustAPI {
		return config
	}
	return types.BoolValue(api)
}

func mergeString(config types.String, api string) types.String {
	if strings.TrimSpace(api) != "" {
		return types.StringValue(api)
	}
	if config.IsNull() || config.IsUnknown() || strings.TrimSpace(config.ValueString()) == "" {
		return types.StringNull()
	}
	return config
}

func stringOrNull(v string) types.String {
	if strings.TrimSpace(v) == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

func flexIntOrNull(v *vergeos.FlexInt) types.Int64 {
	if v == nil || v.Int() <= 0 {
		return types.Int64Null()
	}
	return types.Int64Value(int64(v.Int()))
}

func validateImportData(data *importDataModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if data == nil {
		diags.AddError("Missing VM import", "The data source configuration was empty.")
		return diags
	}
	idKnown := !data.ID.IsUnknown()
	nameKnown := !data.Name.IsUnknown()
	if !idKnown || !nameKnown {
		return diags
	}
	idSet := stringSet(data.ID)
	nameSet := stringSet(data.Name)
	if idSet == nameSet {
		diags.AddAttributeError(path.Root("id"), "VM import lookup needs one key", "Set id to the vm_imports key, or name to the import name. VergeOS allows more than one import to share a name, and a name that matches two rows is an error.")
	}
	if idSet && !hexKeyPattern.MatchString(strings.TrimSpace(data.ID.ValueString())) {
		diags.AddAttributeError(path.Root("id"), "Invalid vm_imports key", "id is a 40 character hex string.")
	}
	if stringSet(data.Timeout) {
		if _, err := time.ParseDuration(strings.TrimSpace(data.Timeout.ValueString())); err != nil {
			diags.AddAttributeError(path.Root("timeout"), "Invalid timeout", "timeout is a duration such as \"10m\" or \"30s\".")
		}
	}
	return diags
}
