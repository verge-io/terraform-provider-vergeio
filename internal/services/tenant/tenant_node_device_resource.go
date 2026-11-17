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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &TenantNodeDeviceResource{}
	_ resource.ResourceWithImportState = &TenantNodeDeviceResource{}
)

func NewTenantNodeDeviceResource() resource.Resource {
	return &TenantNodeDeviceResource{}
}

// TenantNodeDeviceResource is vergeio_tenant_node_device.
type TenantNodeDeviceResource struct {
	api *API
}

// TenantNodeDeviceResourceModel is the Terraform model for vergeio_tenant_node_device.
// Count is device_count. Terraform reserves count, so the argument cannot use that name.
// The API field is still count.
type TenantNodeDeviceResourceModel struct {
	Id            types.String `tfsdk:"id"`
	TenantNodeID  types.String `tfsdk:"tenant_node_id"`
	Name          types.String `tfsdk:"name"`
	Description   types.String `tfsdk:"description"`
	Type          types.String `tfsdk:"type"`
	ResourceGroup types.String `tfsdk:"resource_group"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	Optional      types.Bool   `tfsdk:"optional"`
	Count         types.Int32  `tfsdk:"device_count"`
	DeviceIDs     types.List   `tfsdk:"device_ids"`
	Machine       types.Int32  `tfsdk:"machine"`
	MachineType   types.String `tfsdk:"machine_type"`
	OrderID       types.Int32  `tfsdk:"orderid"`
	UUID          types.String `tfsdk:"uuid"`
	Created       types.Int64  `tfsdk:"created"`
	Modified      types.Int64  `tfsdk:"modified"`
}

func (r *TenantNodeDeviceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_node_device"
}

func (r *TenantNodeDeviceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A host device passed through to a VergeOS tenant node. resource_group is the resource group UUID. The device is thick provisioned: the tenant owns it once it is attached, even when no VM inside the tenant is using it, and it cannot be assigned to another VM or tenant until this resource is destroyed. Changing tenant_node_id, type, or device_count replaces the device.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "machine_devices key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_node_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant_node. Changing it replaces the device.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Device name. VergeOS requires a name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Device description. Omit to leave the current description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Device type: node_usb_devices, tpm, node_pci_devices, node_nvidia_vgpu_devices, node_host_gpu_devices, node_sriov_nic_devices, gpu, or nvidia_vgpu. gpu and nvidia_vgpu are the older names. Changing it replaces the device.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(tenantNodeDeviceTypes()...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"resource_group": schema.StringAttribute{
				MarkdownDescription: "Resource group UUID, the same value as a vergeio_resource_groups id. This is the resource_group stored on the device. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the device is enabled. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"optional": schema.BoolAttribute{
				MarkdownDescription: "When true, the tenant node can start without this device. Defaults to false.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"device_count": schema.Int32Attribute{
				MarkdownDescription: "How many copies VergeOS creates. Sent as count on create only. Defaults to 1. The device row does not store this number, so Terraform keeps the configured value. Changing it replaces the device. Every copy is listed in device_ids, and destroy removes all of them.",
				Optional:            true,
				Computed:            true,
				Default:             int32default.StaticInt32(1),
				Validators: []validator.Int32{
					int32validator.Between(1, 16),
				},
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"device_ids": schema.ListAttribute{
				MarkdownDescription: "machine_devices keys created with this device. A copy has the same type and a name of this device or name-N. Destroy removes every key.",
				ElementType:         types.StringType,
				Computed:            true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"machine": schema.Int32Attribute{
				MarkdownDescription: "Machine key of the tenant node. VergeOS stores the device against this key, not the tenant node key.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"machine_type": schema.StringAttribute{
				MarkdownDescription: "Machine kind stored on the device. A tenant node device is tenant.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"orderid": schema.Int32Attribute{
				MarkdownDescription: "Device order, from 0 through 64.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "Device UUID assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created": schema.Int64Attribute{
				MarkdownDescription: "Creation time as a Unix timestamp.",
				Computed:            true,
			},
			"modified": schema.Int64Attribute{
				MarkdownDescription: "Last modification time as a Unix timestamp.",
				Computed:            true,
			},
		},
	}
}

func (r *TenantNodeDeviceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TenantNodeDeviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantNodeDeviceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.api.createTenantNodeDevice(ctx, &data)
	if err != nil {
		if data.Id.ValueString() != "" {
			resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
		}
		resp.Diagnostics.AddError("Error creating tenant node device", err.Error())
		return
	}
	if data.Id.ValueString() == "" {
		resp.Diagnostics.AddError("Error creating tenant node device", "VergeOS did not return an id for the tenant node device")
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantNodeDeviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantNodeDeviceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenantNodeDevice(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant node device", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantNodeDeviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TenantNodeDeviceResourceModel
	var state TenantNodeDeviceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateTenantNodeDevice(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating tenant node device", err.Error())
		return
	}
	if err := r.api.readTenantNodeDevice(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tenant node device", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TenantNodeDeviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantNodeDeviceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteTenantNodeDevice(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tenant node device", err.Error())
	}
}

func (r *TenantNodeDeviceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Tenant Node Device Import ID",
			"Import vergeio_tenant_node_device with the machine_devices key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
