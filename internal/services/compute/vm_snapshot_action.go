// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ action.Action                   = &VMSnapshotAction{}
	_ action.ActionWithConfigure      = &VMSnapshotAction{}
	_ action.ActionWithValidateConfig = &VMSnapshotAction{}
)

func NewVMSnapshotAction() action.Action {
	return &VMSnapshotAction{}
}

// VMSnapshotAction takes an instant VM snapshot.
// The snapshot is not Terraform state. vergeio_vm does not depend on it.
type VMSnapshotAction struct {
	sdk *vergeos.Client
}

type vmSnapshotActionModel struct {
	VMID             types.String `tfsdk:"vm_id"`
	Name             types.String `tfsdk:"name"`
	RetentionSeconds types.Int64  `tfsdk:"retention_seconds"`
	Quiesce          types.Bool   `tfsdk:"quiesce"`
}

func (a *VMSnapshotAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_snapshot"
}

func (a *VMSnapshotAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Take an instant snapshot of a VergeOS VM. Invoke it with `terraform apply -invoke`, or from a resource `lifecycle` `action_trigger`, for example before a VM update. The snapshot is not stored in Terraform state. Requires Terraform 1.14 or later. OpenTofu does not implement actions, and no resource behavior depends on this action.",
		Attributes: map[string]schema.Attribute{
			"vm_id": schema.StringAttribute{
				MarkdownDescription: "VM id, the same value as vergeio_vm.id.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Snapshot name. When omitted, the name is snapshot-YYYYMMDD-HHMMSS in UTC.",
				Optional:            true,
			},
			"retention_seconds": schema.Int64Attribute{
				MarkdownDescription: "How long to keep the snapshot, in seconds. When omitted, the snapshot is kept for 24 hours.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"quiesce": schema.BoolAttribute{
				MarkdownDescription: "Ask the guest agent to freeze filesystems while the snapshot is taken. Requires a running guest agent. When false or omitted, the snapshot is still created and no quiesce request is sent. If the snapshot is created and the quiesce request then fails, the action returns that error and the snapshot remains.",
				Optional:            true,
			},
		},
	}
}

func (a *VMSnapshotAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	sdk, diags := shared.ActionSDK(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.sdk = sdk
}

func (a *VMSnapshotAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data vmSnapshotActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.VMID, "vm_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("vm_id"), "Invalid VM ID", err.Error())
	}
}

func (a *VMSnapshotAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var data vmSnapshotActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.sdk == nil {
		resp.Diagnostics.AddError("VergeOS Client Missing", "The provider has not been configured.")
		return
	}
	vmID, known, err := shared.KnownPositiveID(data.VMID, "vm_id")
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("vm_id is required")
		}
		resp.Diagnostics.AddAttributeError(path.Root("vm_id"), "Invalid VM ID", err.Error())
		return
	}
	opts := &vergeos.VMSnapshotOptions{}
	if !data.Name.IsNull() && !data.Name.IsUnknown() {
		opts.Name = strings.TrimSpace(data.Name.ValueString())
	}
	if !data.RetentionSeconds.IsNull() && !data.RetentionSeconds.IsUnknown() {
		opts.Retention = int(data.RetentionSeconds.ValueInt64())
	}
	if !data.Quiesce.IsNull() && !data.Quiesce.IsUnknown() && data.Quiesce.ValueBool() {
		opts.Quiesce = true
	}
	shared.ReportProgress(resp, fmt.Sprintf("Taking a snapshot of VM %d", vmID))
	snap, err := a.sdk.VMs.Snapshot(ctx, vmID, opts)
	if snap != nil {
		tflog.Info(ctx, fmt.Sprintf("VM snapshot %d (%s) created for VM %d", snap.Key.Int(), snap.Name, vmID))
		shared.ReportProgress(resp, fmt.Sprintf("Created snapshot %s (%d) of VM %d", snap.Name, snap.Key.Int(), vmID))
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Taking VM Snapshot", err.Error())
	}
}
