// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const (
	tenantNodeKill  = "kill"
	tenantNodeReset = "reset"
)

var (
	_ action.Action                   = &TenantNodePowerAction{}
	_ action.ActionWithConfigure      = &TenantNodePowerAction{}
	_ action.ActionWithValidateConfig = &TenantNodePowerAction{}
)

func NewTenantNodePowerAction() action.Action {
	return &TenantNodePowerAction{}
}

// TenantNodePowerAction kills or resets one tenant node.
// VergeOS also has poweroffmaintenance. The pinned govergeos client does not
// expose that action, so this provider does not offer it.
type TenantNodePowerAction struct {
	sdk *vergeos.Client
}

type tenantNodePowerActionModel struct {
	TenantNodeID types.String `tfsdk:"tenant_node_id"`
	Operation    types.String `tfsdk:"operation"`
}

func (a *TenantNodePowerAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_node_power"
}

func (a *TenantNodePowerAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Kill or reset a VergeOS tenant node. kill cuts power. reset restarts the node. Neither operation changes vergeio_tenant_node. Destroy of that resource still powers the node off, then kills it if it does not stop. VergeOS also has poweroffmaintenance on tenant nodes. The govergeos client pinned by this provider does not expose that action, so power off for maintenance is not available here. Invoke this action with `terraform apply -invoke`, or from a resource `lifecycle` `action_trigger`. Requires Terraform 1.14 or later. OpenTofu does not implement actions, and no resource behavior depends on this action.",
		Attributes: map[string]schema.Attribute{
			"tenant_node_id": schema.StringAttribute{
				MarkdownDescription: "Tenant node id, the same value as vergeio_tenant_node.id.",
				Required:            true,
			},
			"operation": schema.StringAttribute{
				MarkdownDescription: "kill or reset.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(tenantNodeKill, tenantNodeReset),
				},
			},
		},
	}
}

func (a *TenantNodePowerAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	sdk, diags := shared.ActionSDK(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.sdk = sdk
}

func (a *TenantNodePowerAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data tenantNodePowerActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.TenantNodeID, "tenant_node_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("tenant_node_id"), "Invalid Tenant Node ID", err.Error())
	}
}

func (a *TenantNodePowerAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var data tenantNodePowerActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.sdk == nil {
		resp.Diagnostics.AddError("VergeOS Client Missing", "The provider has not been configured.")
		return
	}
	nodeID, known, err := shared.KnownPositiveID(data.TenantNodeID, "tenant_node_id")
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("tenant_node_id is required")
		}
		resp.Diagnostics.AddAttributeError(path.Root("tenant_node_id"), "Invalid Tenant Node ID", err.Error())
		return
	}
	if data.Operation.IsNull() || data.Operation.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("operation"), "Invalid Tenant Node Operation", "operation is required")
		return
	}
	operation := data.Operation.ValueString()
	shared.ReportProgress(resp, fmt.Sprintf("Tenant node %d %s", nodeID, operation))
	switch operation {
	case tenantNodeKill:
		err = a.sdk.TenantNodes.Kill(ctx, nodeID)
	case tenantNodeReset:
		err = a.sdk.TenantNodes.Reset(ctx, nodeID)
	default:
		err = fmt.Errorf("operation %q is not kill or reset", operation)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Changing Tenant Node Power", err.Error())
		return
	}
	tflog.Info(ctx, fmt.Sprintf("tenant node %d %s", nodeID, operation))
}
