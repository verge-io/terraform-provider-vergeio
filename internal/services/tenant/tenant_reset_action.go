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
	_ action.Action                   = &TenantResetAction{}
	_ action.ActionWithConfigure      = &TenantResetAction{}
	_ action.ActionWithValidateConfig = &TenantResetAction{}
)

func NewTenantResetAction() action.Action {
	return &TenantResetAction{}
}

// TenantResetAction restarts a tenant without editing vergeio_tenant.
type TenantResetAction struct {
	sdk *vergeos.Client
}

type tenantResetActionModel struct {
	TenantID types.String `tfsdk:"tenant_id"`
}

func (a *TenantResetAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_reset"
}

func (a *TenantResetAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Restart a VergeOS tenant. This posts the tenant reset action and does not change vergeio_tenant. A later plan of that resource restores a declared powerstate when the running state differs. Invoke it with `terraform apply -invoke`, or from a resource `lifecycle` `action_trigger`. Requires Terraform 1.14 or later. OpenTofu does not implement actions, and no resource behavior depends on this action.",
		Attributes: map[string]schema.Attribute{
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Tenant id, the same value as vergeio_tenant.id.",
				Required:            true,
			},
		},
	}
}

func (a *TenantResetAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	sdk, diags := shared.ActionSDK(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.sdk = sdk
}

func (a *TenantResetAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data tenantResetActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.TenantID, "tenant_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("tenant_id"), "Invalid Tenant ID", err.Error())
	}
}

func (a *TenantResetAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var data tenantResetActionModel
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
	shared.ReportProgress(resp, fmt.Sprintf("Resetting tenant %d", tenantID))
	if err := a.sdk.Tenants.Reset(ctx, tenantID); err != nil {
		resp.Diagnostics.AddError("Error Resetting Tenant", err.Error())
		return
	}
	tflog.Info(ctx, fmt.Sprintf("reset tenant %d", tenantID))
	shared.ReportProgress(resp, fmt.Sprintf("Reset tenant %d", tenantID))
}
