// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ValidateConfig requires a cloud-init body. contents and contents_wo are
// both optional so a write-only body can omit the stored one. Schema
// validators reject setting both, and contents_wo without a version.
func (r *VMResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data VMResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	for i, file := range data.CloudInitFiles {
		if file.Contents.IsUnknown() || file.ContentsWO.IsUnknown() {
			continue
		}
		hasContents := !file.Contents.IsNull()
		hasWriteOnly := !file.ContentsWO.IsNull()
		if hasContents || hasWriteOnly {
			continue
		}
		resp.Diagnostics.AddAttributeError(
			path.Root("cloudinit_files").AtListIndex(i).AtName("contents"),
			"Missing cloud-init file contents",
			"Set contents or contents_wo. contents is stored in state. contents_wo is not. contents_wo requires contents_wo_version, and incrementing that version writes the body again.",
		)
	}
}

// applyVMWriteOnly copies write-only secrets onto plan for the API call.
// hasState false is create. Update copies a secret only when its version
// changed, so an unrelated update does not send it again. plan.CloudInitFiles
// must be a clone; this writes into those elements.
func applyVMWriteOnly(plan, state *VMResourceModel, config *VMResourceModel) {
	if plan == nil || config == nil {
		return
	}
	var stateVersion types.Int64
	hasState := state != nil
	if hasState {
		stateVersion = state.ConsolePassWOVersion
	}
	shared.ApplyWriteOnlyString(&plan.ConsolePass, plan.ConsolePassWOVersion, stateVersion, config.ConsolePassWO, hasState)

	n := len(plan.CloudInitFiles)
	if len(config.CloudInitFiles) < n {
		n = len(config.CloudInitFiles)
	}
	for i := 0; i < n; i++ {
		var fileStateVersion types.Int64
		fileHasState := false
		if state != nil {
			if existing, ok := cloudInitStateFile(state.CloudInitFiles, plan.CloudInitFiles[i].Name.ValueString()); ok {
				fileStateVersion = existing.ContentsWOVersion
				fileHasState = true
			}
		}
		shared.ApplyWriteOnlyString(
			&plan.CloudInitFiles[i].Contents,
			plan.CloudInitFiles[i].ContentsWOVersion,
			fileStateVersion,
			config.CloudInitFiles[i].ContentsWO,
			fileHasState,
		)
	}
}

func cloudInitStateFile(files []CloudInitFile, name string) (CloudInitFile, bool) {
	key := cloudInitFileNameKey(name)
	for _, file := range files {
		if cloudInitFileNameKey(file.Name.ValueString()) == key {
			return file, true
		}
	}
	return CloudInitFile{}, false
}

// scrubVMWriteOnly clears secrets that must not be stored. A set version
// means the write-only attribute supplied the value.
func scrubVMWriteOnly(data *VMResourceModel) {
	if data == nil {
		return
	}
	data.ConsolePassWO = types.StringNull()
	if shared.WriteOnlyVersionSet(data.ConsolePassWOVersion) {
		data.ConsolePass = types.StringNull()
	}
	data.CloudInitFiles = scrubCloudInitFiles(data.CloudInitFiles)
}

func scrubCloudInitFiles(files []CloudInitFile) []CloudInitFile {
	if files == nil {
		return nil
	}
	out := cloneCloudInitFiles(files)
	for i := range out {
		out[i].ContentsWO = types.StringNull()
		if shared.WriteOnlyVersionSet(out[i].ContentsWOVersion) {
			out[i].Contents = types.StringNull()
		}
	}
	return out
}

// consolePassForAPI is the console password to send on create.
func consolePassForAPI(data *VMResourceModel) *string {
	if data == nil {
		return nil
	}
	if password := vergeio.KnownString(data.ConsolePass); password != nil {
		return password
	}
	return vergeio.KnownString(data.ConsolePassWO)
}

// cloudInitEffectiveContents is the body to send. contents wins. contents_wo
// is the fallback used when the plan still holds only the write-only value.
func cloudInitEffectiveContents(file CloudInitFile) (string, bool) {
	if body := vergeio.KnownString(file.Contents); body != nil {
		return *body, true
	}
	if body := vergeio.KnownString(file.ContentsWO); body != nil {
		return *body, true
	}
	return "", false
}

func cloudInitWriteOnly(file CloudInitFile) bool {
	return shared.WriteOnlyVersionSet(file.ContentsWOVersion)
}
