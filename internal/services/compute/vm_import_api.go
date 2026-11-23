// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
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
	// govergeos downloads a URL and posts vm_imports in the same call. VergeOS
	// then tries to extract an OVA that is still empty. Download first, wait
	// until the catalog file has a size, and import that file id.
	// timeouts.create is one budget for the download and the import.
	timeout, err := importTimeout(data.Timeouts, "create", vmImportWaitDefault)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(timeout)
	downloadedFile, err := a.downloadImportURL(ctx, req, deadline)
	if err != nil {
		return err
	}
	if downloadedFile > 0 {
		req.File = &downloadedFile
		req.URL = ""
		req.FileName = ""
	}
	created, err := a.sdk.VMImports.Create(ctx, req)
	if err != nil {
		if downloadedFile > 0 {
			if cleanupErr := a.deleteFile(ctx, downloadedFile); cleanupErr != nil {
				return fmt.Errorf("%w (catalog file %d was left in place: %v)", err, downloadedFile, cleanupErr)
			}
		}
		return err
	}
	key := vmImportKey(created)
	if created.AlreadyExisted && key == "" {
		if downloadedFile > 0 {
			if cleanupErr := a.deleteFile(ctx, downloadedFile); cleanupErr != nil {
				return fmt.Errorf("VM %q already exists, and catalog file %d was left in place: %v", req.Name, downloadedFile, cleanupErr)
			}
		}
		return fmt.Errorf("VM %q already exists, and VergeOS did not return one vm_imports row for that name", req.Name)
	}
	if key == "" {
		if downloadedFile > 0 {
			if cleanupErr := a.deleteFile(ctx, downloadedFile); cleanupErr != nil {
				return fmt.Errorf("VergeOS did not return a vm_imports key for %q (catalog file %d was left in place: %v)", req.Name, downloadedFile, cleanupErr)
			}
		}
		return fmt.Errorf("VergeOS did not return a vm_imports key for %q", req.Name)
	}
	if created.AlreadyExisted && downloadedFile > 0 {
		if cleanupErr := a.deleteFile(ctx, downloadedFile); cleanupErr != nil {
			return fmt.Errorf("VM %q already exists, and catalog file %d was left in place: %v", req.Name, downloadedFile, cleanupErr)
		}
		downloadedFile = 0
	}
	data.ID = types.StringValue(key)
	data.Adopted = types.BoolValue(created.AlreadyExisted)
	data.OwnsFile = types.BoolValue(downloadedFile > 0)
	if downloadedFile > 0 {
		data.FileID = types.Int64Value(int64(downloadedFile))
	}
	data.Target = types.StringValue(vmImportTargetVM)
	if created.AlreadyExisted {
		tflog.Debug(ctx, fmt.Sprintf("VM %s already exists; using import %s", req.Name, key))
		return a.readVMImport(ctx, data, key)
	}
	return a.waitForCreatedImport(ctx, data, key, deadline, timeout)
}

func (a *importAPI) waitForCreatedImport(ctx context.Context, data *importModel, key string, deadline time.Time, timeout time.Duration) error {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		if cleanupErr := a.deleteOwned(ctx, data); cleanupErr != nil {
			return fmt.Errorf("vm import %s did not finish before %s (cleanup: %v)", key, timeout, cleanupErr)
		}
		data.ID = types.StringNull()
		return fmt.Errorf("vm import %s did not finish before %s", key, timeout)
	}
	finished, err := a.sdk.VMImports.Wait(ctx, key, &vergeos.VMImportWaitOptions{
		Timeout:      remaining,
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
	if importAdopted(data) {
		return nil
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

func (a *importAPI) downloadImportURL(ctx context.Context, req *vergeos.VMImportCreateRequest, deadline time.Time) (int, error) {
	if req == nil || req.URL == "" {
		return 0, nil
	}
	name := strings.TrimSpace(req.FileName)
	if name == "" {
		var err error
		name, err = fileNameFromImportURL(req.URL)
		if err != nil {
			return 0, err
		}
	}
	file, err := a.sdk.Files.Create(ctx, &vergeos.FileCreateRequest{
		Name:          name,
		Description:   downloadedImportDescription + req.Name,
		URL:           req.URL,
		PreferredTier: req.PreferredTier,
	})
	if err != nil {
		return 0, err
	}
	if file == nil || file.Key.Int() <= 0 {
		return 0, fmt.Errorf("VergeOS did not return a file id for %q", name)
	}
	ready, err := a.waitForFileUntil(ctx, file.Key.Int(), deadline)
	if err != nil {
		if cleanupErr := a.deleteFile(ctx, file.Key.Int()); cleanupErr != nil {
			return 0, fmt.Errorf("%w (catalog file %d was left in place: %v)", err, file.Key.Int(), cleanupErr)
		}
		return 0, err
	}
	return ready.Key.Int(), nil
}

func (a *importAPI) waitForFile(ctx context.Context, id int, timeout time.Duration) (*vergeos.File, error) {
	if timeout <= 0 {
		timeout = vmImportWaitDefault
	}
	return a.waitForFileUntil(ctx, id, time.Now().Add(timeout))
}

func (a *importAPI) waitForFileUntil(ctx context.Context, id int, deadline time.Time) (*vergeos.File, error) {
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
			return nil, fmt.Errorf("catalog file %d did not finish downloading before the create timeout", id)
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
