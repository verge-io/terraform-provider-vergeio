// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	stdpath "path"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

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
	timeouts := data.Timeouts
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
	if fileID := flexIntOrNull(row.File); !fileID.IsNull() {
		data.FileID = fileID
	} else if inputs.FileID.IsNull() || inputs.FileID.IsUnknown() {
		data.FileID = types.Int64Null()
	} else {
		data.FileID = inputs.FileID
	}
	data.Timeouts = timeouts
	if adopted.IsNull() || adopted.IsUnknown() {
		data.Adopted = types.BoolValue(false)
	} else {
		data.Adopted = adopted
	}
	if !stringSet(inputs.Name) {
		return a.applyImportedVMImport(ctx, data, row, inputs, newer)
	}
	applyConfiguredVMImport(data, row, inputs, newer)
	return nil
}

func (a *importAPI) applyImportedVMImport(ctx context.Context, data *importModel, row *vergeos.VMImport, inputs importModel, newer bool) error {
	if err := a.fillImportedSources(ctx, data, row); err != nil {
		return err
	}
	data.PreserveMACs = types.BoolValue(row.PreserveMACs)
	data.PreserveDriveFormat = types.BoolValue(row.PreserveDriveFormat)
	data.NoOpticalDrives = types.BoolValue(row.NoOpticalDrives)
	if newer {
		data.PreserveBIOSSerial = types.BoolValue(row.PreserveBIOSSerial)
		data.GuestAgentPreinstalled = types.BoolValue(row.GuestAgentPreinstalled)
		data.ReplicateTags = types.BoolValue(row.ReplicateTags)
	} else {
		data.PreserveBIOSSerial = types.BoolNull()
		data.GuestAgentPreinstalled = types.BoolNull()
		data.ReplicateTags = types.BoolNull()
	}
	data.PreferredTier = stringOrNull(row.PreferredTier)
	data.OverrideDriveInterface = stringOrNull(row.OverrideDriveInterface)
	data.OverrideNICInterface = stringOrNull(row.OverrideNICInterface)
	data.DeleteVM = boolOr(inputs.DeleteVM, true)
	data.DeleteFile = boolOr(inputs.DeleteFile, true)
	if !inputs.OwnsFile.IsNull() && !inputs.OwnsFile.IsUnknown() {
		data.OwnsFile = inputs.OwnsFile
	}
	return nil
}

func applyConfiguredVMImport(data *importModel, row *vergeos.VMImport, inputs importModel, newer bool) {
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
	if data.SharedObject.IsUnknown() {
		data.SharedObject = types.Int64Null()
	}
	data.DeleteVM = inputs.DeleteVM
	data.DeleteFile = inputs.DeleteFile
	if inputs.OwnsFile.IsNull() || inputs.OwnsFile.IsUnknown() {
		data.OwnsFile = types.BoolValue(false)
	} else {
		data.OwnsFile = inputs.OwnsFile
	}
}

func (a *importAPI) fillImportedSources(ctx context.Context, data *importModel, row *vergeos.VMImport) error {
	data.File = types.Int64Null()
	data.URL = types.StringNull()
	data.FileName = types.StringNull()
	data.Volume = stringOrNull(row.Volume)
	data.VolumePath = stringOrNull(row.VolumePath)
	data.SharedObject = flexIntOrNull(row.SharedObject)
	data.OwnsFile = types.BoolValue(false)
	if stringSet(data.Volume) {
		data.SharedObject = types.Int64Null()
		return nil
	}
	if intSet(data.SharedObject) {
		data.Volume = types.StringNull()
		data.VolumePath = types.StringNull()
		return nil
	}
	data.Volume = types.StringNull()
	data.VolumePath = types.StringNull()
	data.SharedObject = types.Int64Null()
	fileID := flexIntOrNull(row.File)
	if fileID.IsNull() {
		return nil
	}
	file, err := a.sdk.Files.Get(ctx, int(fileID.ValueInt64()))
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			data.File = fileID
			return nil
		}
		return err
	}
	if rawURL, ok := sourceURLFromFile(file, downloadedImportDescription, row.Name); ok {
		data.URL = types.StringValue(rawURL)
		data.OwnsFile = types.BoolValue(true)
		return nil
	}
	data.File = fileID
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
	rec := parseDownloadDescription(file.Description, downloadedMediaDescription)
	if stringSet(name) {
		data.Name = name
	} else if rec.urlInDescription {
		data.Name = types.StringValue(rec.name)
	} else if file.Name != "" {
		data.Name = types.StringValue(file.Name)
	}
	switch {
	case stringSet(urlValue):
		data.URL = urlValue
	case !stringSet(name) && rec.urlInDescription:
		data.URL = types.StringValue(rec.rawURL)
	case !stringSet(name) && rec.marker && validateImportURL(strings.TrimSpace(file.URL)) == nil:
		data.URL = types.StringValue(strings.TrimSpace(file.URL))
	default:
		data.URL = urlValue
	}
	data.FileName = fileName
	if !stringSet(name) && strings.TrimSpace(file.PreferredTier) != "" && (tier.IsNull() || tier.IsUnknown()) {
		data.PreferredTier = types.StringValue(strings.TrimSpace(file.PreferredTier))
	} else {
		data.PreferredTier = mergeString(tier, file.PreferredTier)
	}
	data.Status = types.StringValue("complete")
	data.StatusInfo = types.StringNull()
	data.VMID = types.Int64Null()
	data.UUID = types.StringNull()
	data.Importing = types.BoolValue(false)
	data.Aborted = types.BoolValue(false)
	data.FailedDriveCount = types.Int64Value(0)
	data.Adopted = types.BoolValue(false)
	data.DeleteVM = boolOr(deleteVMFlag, true)
	data.DeleteFile = boolOr(deleteFileFlag, true)
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
	return downloadOwned(file.Description, downloadedMediaDescription, name)
}

func fileReady(file *vergeos.File) bool {
	if file == nil {
		return false
	}
	// AllocatedBytes is reserved space and UsedBytes can move before the
	// object has content. Filesize is the logical size VergeOS reports
	// once the download has produced a file.
	return file.Filesize > 0
}

func importAdopted(data *importModel) bool {
	return data != nil && !data.Adopted.IsNull() && !data.Adopted.IsUnknown() && data.Adopted.ValueBool()
}

func deleteVM(data *importModel) bool {
	if data == nil || importAdopted(data) {
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
	// An omitted optional attribute stays null. Copying a platform default
	// into state fails the apply consistency check, because the plan value
	// was null and the attribute is not computed.
	if config.IsNull() || config.IsUnknown() {
		return types.StringNull()
	}
	if strings.TrimSpace(api) != "" {
		return types.StringValue(api)
	}
	if strings.TrimSpace(config.ValueString()) == "" {
		return types.StringNull()
	}
	return config
}

func boolOr(v types.Bool, fallback bool) types.Bool {
	if v.IsNull() || v.IsUnknown() {
		return types.BoolValue(fallback)
	}
	return v
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
