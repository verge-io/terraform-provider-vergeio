// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ action.Action                   = &TenantNodeMigrateAction{}
	_ action.ActionWithConfigure      = &TenantNodeMigrateAction{}
	_ action.ActionWithValidateConfig = &TenantNodeMigrateAction{}
)

func NewTenantNodeMigrateAction() action.Action {
	return &TenantNodeMigrateAction{}
}

// TenantNodeMigrateAction moves a tenant node onto another host.
// vergeio_tenant_node is left unchanged.
type TenantNodeMigrateAction struct {
	sdk *vergeos.Client
}

type tenantNodeMigrateActionModel struct {
	TenantNodeID types.String `tfsdk:"tenant_node_id"`
	TargetNode   types.String `tfsdk:"target_node"`
}

func (a *TenantNodeMigrateAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_node_migrate"
}

func (a *TenantNodeMigrateAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Move a VergeOS tenant node onto another host. target_node is the host node key. The vergeio_tenant_node resource is not updated. Invoke this action with `terraform apply -invoke`, or from a resource `lifecycle` `action_trigger`. Requires Terraform 1.14 or later. OpenTofu does not implement actions, and no resource behavior depends on this action.",
		Attributes: map[string]schema.Attribute{
			"tenant_node_id": schema.StringAttribute{
				MarkdownDescription: "Tenant node id, the same value as vergeio_tenant_node.id.",
				Required:            true,
			},
			"target_node": schema.StringAttribute{
				MarkdownDescription: "Host node key to move the tenant node onto.",
				Required:            true,
			},
		},
	}
}

func (a *TenantNodeMigrateAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	sdk, diags := shared.ActionSDK(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.sdk = sdk
}

func (a *TenantNodeMigrateAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data tenantNodeMigrateActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.TenantNodeID, "tenant_node_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("tenant_node_id"), "Invalid Tenant Node ID", err.Error())
	}
	if _, _, err := shared.KnownPositiveID(data.TargetNode, "target_node"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("target_node"), "Invalid Target Node", err.Error())
	}
}

func (a *TenantNodeMigrateAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var data tenantNodeMigrateActionModel
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
	target, known, err := shared.KnownPositiveID(data.TargetNode, "target_node")
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("target_node is required")
		}
		resp.Diagnostics.AddAttributeError(path.Root("target_node"), "Invalid Target Node", err.Error())
		return
	}
	shared.ReportProgress(resp, fmt.Sprintf("Migrating tenant node %d to host %d", nodeID, target))
	if err := a.sdk.TenantNodes.Migrate(ctx, nodeID, target); err != nil {
		resp.Diagnostics.AddError("Error Migrating Tenant Node", err.Error())
		return
	}
	tflog.Info(ctx, fmt.Sprintf("migrated tenant node %d to host %d", nodeID, target))
	shared.ReportProgress(resp, fmt.Sprintf("Migrated tenant node %d to host %d", nodeID, target))
}
