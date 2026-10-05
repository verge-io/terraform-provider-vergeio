// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &TenantLayer2NetworkResource{}
	_ resource.ResourceWithImportState = &TenantLayer2NetworkResource{}
)

func NewTenantLayer2NetworkResource() resource.Resource {
	return &TenantLayer2NetworkResource{}
}

// TenantLayer2NetworkResource is vergeio_tenant_layer2_network.
type TenantLayer2NetworkResource struct {
	api *API
}

// TenantLayer2NetworkResourceModel is the Terraform model for vergeio_tenant_layer2_network.
type TenantLayer2NetworkResourceModel struct {
	Id        types.String `tfsdk:"id"`
	TenantID  types.String `tfsdk:"tenant_id"`
	NetworkID types.String `tfsdk:"network_id"`
	Enabled   types.Bool   `tfsdk:"enabled"`
}

func (r *TenantLayer2NetworkResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_layer2_network"
}

func (r *TenantLayer2NetworkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One parent layer 2 network bridged into a VergeOS tenant (tenant_layer2_vnets). Requires VergeOS 26.0 or later. Changing tenant_id or network_id replaces the assignment. enabled updates in place and defaults to true. Destroy disables the assignment, then deletes it. Networks created inside the tenant remain after that host-side delete and belong to the tenant-side configuration. Leaving those components in place can block a later recreation.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "tenant_layer2_vnets key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant. Changing it replaces the assignment.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent layer 2 network, the same value as vergeio_network.id. Changing it replaces the assignment.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the assignment is active. Defaults to true. Changing it updates the assignment in place. Destroy sets this to false before it deletes the row.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
		},
	}
}

func (r *TenantLayer2NetworkResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	api, err := NewAPI(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.api = api
}

func (r *TenantLayer2NetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantLayer2NetworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.api.createTenantLayer2Network(ctx, &data)
	if data.Id.ValueString() == "" {
		if err == nil {
			err = fmt.Errorf("VergeOS did not return an id for the tenant layer 2 network")
		}
		resp.Diagnostics.AddError("Error creating tenant layer 2 network", err.Error())
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Error creating tenant layer 2 network", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantLayer2NetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantLayer2NetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenantLayer2Network(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant layer 2 network", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantLayer2NetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TenantLayer2NetworkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateTenantLayer2Network(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error updating tenant layer 2 network", err.Error())
		return
	}
	if err := r.api.readTenantLayer2Network(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tenant layer 2 network", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TenantLayer2NetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantLayer2NetworkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteTenantLayer2Network(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tenant layer 2 network", err.Error())
	}
}

func (r *TenantLayer2NetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Tenant Layer 2 Network Import ID",
			"Import vergeio_tenant_layer2_network with the tenant_layer2_vnets key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
