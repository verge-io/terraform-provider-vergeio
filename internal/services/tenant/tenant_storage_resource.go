// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &TenantStorageResource{}
	_ resource.ResourceWithImportState = &TenantStorageResource{}
)

func NewTenantStorageResource() resource.Resource {
	return &TenantStorageResource{}
}

// TenantStorageResource is vergeio_tenant_storage.
type TenantStorageResource struct {
	api *API
}

// TenantStorageResourceModel is the Terraform model for vergeio_tenant_storage.
type TenantStorageResourceModel struct {
	Id          types.String `tfsdk:"id"`
	TenantID    types.String `tfsdk:"tenant_id"`
	Tier        types.Int32  `tfsdk:"tier"`
	Provisioned types.Int64  `tfsdk:"provisioned"`
	Used        types.Int64  `tfsdk:"used"`
	Allocated   types.Int64  `tfsdk:"allocated"`
	UsedPct     types.Int32  `tfsdk:"used_pct"`
	LastUpdate  types.Int64  `tfsdk:"last_update"`
}

func (r *TenantStorageResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_storage"
}

func (r *TenantStorageResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Storage tier allocation for a VergeOS tenant. provisioned is the quota in bytes. Changing tenant_id or tier replaces the allocation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant storage key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant. Changing it replaces the allocation.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"tier": schema.Int32Attribute{
				MarkdownDescription: "Storage tier key. VergeOS does not allow the tier to change after creation, so changing it replaces the allocation.",
				Required:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"provisioned": schema.Int64Attribute{
				MarkdownDescription: "Provisioned storage in bytes.",
				Required:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"used": schema.Int64Attribute{
				MarkdownDescription: "Used storage in bytes.",
				Computed:            true,
			},
			"allocated": schema.Int64Attribute{
				MarkdownDescription: "Allocated storage in bytes.",
				Computed:            true,
			},
			"used_pct": schema.Int32Attribute{
				MarkdownDescription: "Percentage of provisioned storage that is used.",
				Computed:            true,
			},
			"last_update": schema.Int64Attribute{
				MarkdownDescription: "Last usage update as a Unix timestamp.",
				Computed:            true,
			},
		},
	}
}

func (r *TenantStorageResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.api = NewAPI(client)
}

func (r *TenantStorageResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantStorageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createTenantStorage(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error creating tenant storage", err.Error())
		return
	}
	if err := r.api.readTenantStorage(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tenant storage", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantStorageResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantStorageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenantStorage(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant storage", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantStorageResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TenantStorageResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateTenantStorage(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating tenant storage", err.Error())
		return
	}
	if err := r.api.readTenantStorage(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tenant storage", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TenantStorageResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantStorageResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteTenantStorage(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tenant storage", err.Error())
		return
	}
}

func (r *TenantStorageResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
