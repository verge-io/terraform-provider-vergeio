// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &TenantNodeResource{}
	_ resource.ResourceWithImportState = &TenantNodeResource{}
)

func NewTenantNodeResource() resource.Resource {
	return &TenantNodeResource{}
}

// TenantNodeResource is vergeio_tenant_node.
type TenantNodeResource struct {
	api *API
}

// TenantNodeResourceModel is the Terraform model for vergeio_tenant_node.
type TenantNodeResourceModel struct {
	Id              types.String `tfsdk:"id"`
	TenantID        types.String `tfsdk:"tenant_id"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	CPUCores        types.Int32  `tfsdk:"cpu_cores"`
	RAM             types.Int32  `tfsdk:"ram"`
	Cluster         types.Int32  `tfsdk:"cluster"`
	ClusterFailover types.Int32  `tfsdk:"cluster_failover"`
	PreferredNode   types.Int32  `tfsdk:"preferred_node"`
	HAGroup         types.String `tfsdk:"ha_group"`
	OnPowerLoss     types.String `tfsdk:"on_power_loss"`
	NodeID          types.Int32  `tfsdk:"nodeid"`
	Machine         types.Int32  `tfsdk:"machine"`
	IsSnapshot      types.Bool   `tfsdk:"is_snapshot"`
	Creator         types.String `tfsdk:"creator"`
	Created         types.Int64  `tfsdk:"created"`
	Modified        types.Int64  `tfsdk:"modified"`
}

func (r *TenantNodeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_node"
}

func (r *TenantNodeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Compute handed to a VergeOS tenant. cpu_cores and ram are the capacity of one tenant node. Changing tenant_id replaces the node. Destroy gracefully powers off this node when it is running, then kills it if it does not stop in time, then deletes it so sibling nodes stay up. VergeOS rejects deleting a node while that node is running.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant node key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant. Changing it replaces the node.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Node name, unique within the tenant. Omit to let VergeOS assign one.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Node description. Omit to leave an existing description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the tenant node is enabled.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"cpu_cores": schema.Int32Attribute{
				MarkdownDescription: "CPU cores allocated to the node.",
				Required:            true,
				Validators: []validator.Int32{
					int32validator.AtLeast(1),
				},
			},
			"ram": schema.Int32Attribute{
				MarkdownDescription: "RAM allocated to the node, in megabytes. Minimum 2048.",
				Required:            true,
				Validators: []validator.Int32{
					int32validator.AtLeast(2048),
				},
			},
			"cluster": schema.Int32Attribute{
				MarkdownDescription: "Target cluster key. Omit to leave the current cluster unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"cluster_failover": schema.Int32Attribute{
				MarkdownDescription: "Failover cluster key. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"preferred_node": schema.Int32Attribute{
				MarkdownDescription: "Preferred host node key. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"ha_group": schema.StringAttribute{
				MarkdownDescription: "High-availability group name.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"on_power_loss": schema.StringAttribute{
				MarkdownDescription: "Behavior when the host loses power: power_on, last_state, or leave_off.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("power_on", "last_state", "leave_off"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"nodeid": schema.Int32Attribute{
				MarkdownDescription: "Node identifier within the tenant.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"machine": schema.Int32Attribute{
				MarkdownDescription: "Underlying machine row key.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"is_snapshot": schema.BoolAttribute{
				MarkdownDescription: "Whether this node record is a snapshot.",
				Computed:            true,
			},
			"creator": schema.StringAttribute{
				MarkdownDescription: "Username that created the node.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created": schema.Int64Attribute{
				MarkdownDescription: "Creation time as a Unix timestamp.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"modified": schema.Int64Attribute{
				MarkdownDescription: "Last modification time as a Unix timestamp.",
				Computed:            true,
			},
		},
	}
}

func (r *TenantNodeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TenantNodeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantNodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createTenantNode(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error creating tenant node", err.Error())
		return
	}
	if err := r.api.readTenantNode(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tenant node", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantNodeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantNodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenantNode(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant node", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantNodeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TenantNodeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateTenantNode(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating tenant node", err.Error())
		return
	}
	if err := r.api.readTenantNode(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tenant node", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TenantNodeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantNodeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteTenantNode(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tenant node", err.Error())
		return
	}
}

func (r *TenantNodeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Tenant Node Import ID",
			"Import vergeio_tenant_node with the tenant node key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
