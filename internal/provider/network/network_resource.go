// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &NetworkResource{}
var _ resource.ResourceWithImportState = &NetworkResource{}

func NewNetworkResource() resource.Resource {
	return &NetworkResource{}
}

// NetworkResource defines the resource implementation.
type NetworkResource struct {
	networkApi *NetworkApi
}

// NetworkResourceModel describes the resource data model.
type NetworkResourceModel struct {
	Id              types.String `tfsdk:"id"`
	Name            types.String `tfsdk:"name"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Default_Gateway types.Int32  `tfsdk:"vnet_default_gateway"`
	IPaddress       types.String `tfsdk:"ipaddress"`
	Network         types.String `tfsdk:"network"`
	DHCP            types.Bool   `tfsdk:"dhcp_enabled"`
	Dynamic_DHCP    types.Bool   `tfsdk:"dynamic_dhcp"`
	DHCP_Sequential types.Bool   `tfsdk:"dhcp_sequential"`
	DynamicIP_Start types.String `tfsdk:"dhcp_start"`
	DynamicIP_Stop  types.String `tfsdk:"dhcp_stop"`
	On_Power_Loss   types.String `tfsdk:"on_power_loss"`
	PowerState      types.String `tfsdk:"powerstate"`
	Type            types.String `tfsdk:"type"`
	VLAN_TAG        types.Int32  `tfsdk:"layer2_id"`
	MTU             types.Int32  `tfsdk:"mtu"`
	Interface_Vnet  types.Int32  `tfsdk:"interface_vnet"`
	IPaddress_Type  types.String `tfsdk:"ipaddress_type"`
	Layer2_Type     types.String `tfsdk:"layer2_type"`
}

// Metadata returns the resource type name.
func (r *NetworkResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *NetworkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Network or Vnet resource in VergeIO",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Network id (returned as the key) in VergeIO",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Unique network name",
				Required:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Network state",
				Optional:            true,
				Computed:            true,
			},
			"vnet_default_gateway": schema.Int32Attribute{
				MarkdownDescription: "Vnet default gateway",
				Optional:            true,
				// Computed:            true,
			},
			"ipaddress": schema.StringAttribute{
				MarkdownDescription: "IP address assigned to network",
				Optional:            true,
				Computed:            true,
			},
			"network": schema.StringAttribute{
				MarkdownDescription: "Network address",
				Optional:            true,
				Computed:            true,
			},
			"dhcp_enabled": schema.BoolAttribute{
				MarkdownDescription: "Is DHCP enabled",
				Optional:            true,
				Computed:            true,
			},
			"dynamic_dhcp": schema.BoolAttribute{
				MarkdownDescription: "Is DHCP dynamic",
				Optional:            true,
				Computed:            true,
			},
			"dhcp_sequential": schema.BoolAttribute{
				MarkdownDescription: "Is DHCP sequential",
				Optional:            true,
				Computed:            true,
			},
			"dhcp_start": schema.StringAttribute{
				MarkdownDescription: "DHCP start address",
				Optional:            true,
				Computed:            true,
			},
			"dhcp_stop": schema.StringAttribute{
				MarkdownDescription: "DHCP stop address",
				Optional:            true,
				Computed:            true,
			},
			"on_power_loss": schema.StringAttribute{
				MarkdownDescription: "What to do on power loss",
				Validators: []validator.String{
					// Validate string value must be "power_on", "leave_off", or "last_state"
					stringvalidator.OneOf([]string{"power_on", "leave_off", "last_state"}...),
				},
				Optional: true,
				Computed: true,
			},
			"powerstate": schema.StringAttribute{
				MarkdownDescription: "Power state of the network",
				Optional:            true,
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Type of Network",
				Optional:            true,
				Computed:            true,
			},
			"layer2_id": schema.Int32Attribute{
				MarkdownDescription: "VLAN ID",
				Optional:            true,
				Computed:            true,
			},
			"mtu": schema.Int32Attribute{
				MarkdownDescription: "Network MTU",
				Optional:            true,
				Computed:            true,
			},
			"interface_vnet": schema.Int32Attribute{
				MarkdownDescription: "Key/ID of the physical network",
				Optional:            true,
				Computed:            true,
			},
			"ipaddress_type": schema.StringAttribute{
				MarkdownDescription: "IP address type of the vnet",
				Optional:            true,
				Computed:            true,
			},
			"layer2_type": schema.StringAttribute{
				MarkdownDescription: "Layer2 type of the vnet",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *NetworkResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
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

	r.networkApi = NewNetworkApi(client)
}

func (r *NetworkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data NetworkResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Call the API to create the network
	if err := r.networkApi.createNetwork(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Creating Network",
			err.Error(),
		)
		return
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("created a resource %v", data))

	// Read data into the model to get all the attributes
	readDataError := r.networkApi.readNetwork(ctx, &data)

	if readDataError != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data NetworkResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	readDataError := r.networkApi.readNetwork(ctx, &data)

	if readDataError != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *NetworkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planData, stateData NetworkResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &stateData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Call the API to update the networkr
	if err := r.networkApi.updateNetwork(ctx, &planData, &stateData); err != nil {
		resp.Diagnostics.AddError(
			"Error Updating Network",
			err.Error(),
		)
	}

	// Read data into the model to get all the attributes
	readDataError := r.networkApi.readNetwork(ctx, &planData)
	if readDataError != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}
	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &planData)...)
}

func (r *NetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NetworkResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Call the API to check the current power state
	if err := r.networkApi.checkNetworkPowerState(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Failed to check Power State before deletion:",
			err.Error(),
		)
		return
	}

	// Call the API to check if the network is in a power state that can be deleted
	for strings.ToLower(data.PowerState.ValueString()) != "stopped" {
		Retries := 1

		tflog.Debug(ctx, fmt.Sprintf("Current network power state is %v", data))

		// Power the network off
		if err := r.networkApi.killNetwork(ctx, &data); err != nil {
			resp.Diagnostics.AddError(
				"Failed to kill Network before deletion:",
				err.Error(),
			)
			return
		}

		// Wait for a short period to allow the kill operation to complete
		time.Sleep(1 * time.Second)

		// Call the API to check if the network is in a power state that can be deleted
		if err := r.networkApi.checkNetworkPowerState(ctx, &data); err != nil {
			resp.Diagnostics.AddError(
				"Failed to check Power State before deletion:",
				err.Error(),
			)
			return
		}

		Retries += 1

		if Retries > 5 {
			resp.Diagnostics.AddError(
				"Failed to kill Network before deletion:",
				fmt.Sprintf("Failed to kill Network before deletion after %d retries", Retries),
			)
			return
		}
		continue
	}

	tflog.Debug(ctx, fmt.Sprintf("Network state before deletion %v", data.PowerState.ValueString()))

	// Proceed with network deletion
	if err := r.networkApi.deleteNetwork(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Network",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "Network was successfully deleted")
}

func (r *NetworkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
