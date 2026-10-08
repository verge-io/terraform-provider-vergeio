// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	_ action.Action                   = &TenantSnapshotAction{}
	_ action.ActionWithConfigure      = &TenantSnapshotAction{}
	_ action.ActionWithValidateConfig = &TenantSnapshotAction{}
)

func NewTenantSnapshotAction() action.Action {
	return &TenantSnapshotAction{}
}

// TenantSnapshotAction takes a snapshot of a whole tenant.
// The snapshot is not Terraform state.
type TenantSnapshotAction struct {
	sdk *vergeos.Client
}

type tenantSnapshotActionModel struct {
	TenantID         types.String `tfsdk:"tenant_id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	RetentionSeconds types.Int64  `tfsdk:"retention_seconds"`
}

func (a *TenantSnapshotAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_snapshot"
}

func (a *TenantSnapshotAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Take a snapshot of a VergeOS tenant. Invoke it with `terraform apply -invoke`, or from a resource `lifecycle` `action_trigger`, before a change inside the tenant. The snapshot is a full tenant snapshot and is not stored in Terraform state. The vergeio_tenant_snapshot resource keeps a snapshot in state. This action does not. Requires Terraform 1.14 or later. OpenTofu does not implement actions, and no resource behavior depends on this action.",
		Attributes: map[string]schema.Attribute{
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Tenant id, the same value as vergeio_tenant.id.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Snapshot name. When omitted, VergeOS assigns one.",
				Optional:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Snapshot description.",
				Optional:            true,
			},
			"retention_seconds": schema.Int64Attribute{
				MarkdownDescription: "How long to keep the snapshot, in seconds, from the time the action runs. When omitted, VergeOS chooses the expiration.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
		},
	}
}

func (a *TenantSnapshotAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	sdk, diags := shared.ActionSDK(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.sdk = sdk
}

func (a *TenantSnapshotAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data tenantSnapshotActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.TenantID, "tenant_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("tenant_id"), "Invalid Tenant ID", err.Error())
	}
}

func (a *TenantSnapshotAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var data tenantSnapshotActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.sdk == nil {
		resp.Diagnostics.AddError("VergeOS Client Missing", "The provider has not been configured.")
		return
	}
	tenantID, known, err := shared.KnownPositiveID(data.TenantID, "tenant_id")
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("tenant_id is required")
		}
		resp.Diagnostics.AddAttributeError(path.Root("tenant_id"), "Invalid Tenant ID", err.Error())
		return
	}
	create := &vergeos.TenantSnapshotCreateRequest{Tenant: tenantID}
	if !data.Name.IsNull() && !data.Name.IsUnknown() {
		create.Name = strings.TrimSpace(data.Name.ValueString())
	}
	if !data.Description.IsNull() && !data.Description.IsUnknown() {
		create.Description = strings.TrimSpace(data.Description.ValueString())
	}
	if !data.RetentionSeconds.IsNull() && !data.RetentionSeconds.IsUnknown() {
		expires := time.Now().Unix() + data.RetentionSeconds.ValueInt64()
		create.Expires = &expires
	}
	shared.ReportProgress(resp, fmt.Sprintf("Taking a snapshot of tenant %d", tenantID))
	snap, err := a.sdk.TenantSnapshots.Create(ctx, create)
	if err != nil {
		resp.Diagnostics.AddError("Error Taking Tenant Snapshot", err.Error())
		return
	}
	tflog.Info(ctx, fmt.Sprintf("tenant snapshot %d (%s) created for tenant %d", snap.Key.Int(), snap.Name, tenantID))
	shared.ReportProgress(resp, fmt.Sprintf("Created tenant snapshot %s (%d) of tenant %d", snap.Name, snap.Key.Int(), tenantID))
}
