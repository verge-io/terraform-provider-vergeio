// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &NetworkResource{}
var _ resource.ResourceWithImportState = &NetworkResource{}
var _ resource.ResourceWithUpgradeState = &NetworkResource{}

func NewNetworkResource() resource.Resource {
	return &NetworkResource{}
}

// NetworkResource defines the resource implementation.
type NetworkResource struct {
	networkApi *NetworkApi
}

// NetworkResourceModel describes the resource data model.
type NetworkResourceModel struct {
	Id                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Enabled              types.Bool   `tfsdk:"enabled"`
	Default_Gateway      types.Int32  `tfsdk:"vnet_default_gateway"`
	IPaddress            types.String `tfsdk:"ipaddress"`
	Network              types.String `tfsdk:"network"`
	DHCP                 types.Bool   `tfsdk:"dhcp_enabled"`
	Dynamic_DHCP         types.Bool   `tfsdk:"dynamic_dhcp"`
	DHCP_Sequential      types.Bool   `tfsdk:"dhcp_sequential"`
	DynamicIP_Start      types.String `tfsdk:"dhcp_start"`
	DynamicIP_Stop       types.String `tfsdk:"dhcp_stop"`
	On_Power_Loss        types.String `tfsdk:"on_power_loss"`
	PowerState           types.Bool   `tfsdk:"powerstate"`
	RestartOnChange      types.Bool   `tfsdk:"restart_on_change"`
	NeedRestart          types.Bool   `tfsdk:"need_restart"`
	Type                 types.String `tfsdk:"type"`
	VLAN_TAG             types.Int32  `tfsdk:"layer2_id"`
	MTU                  types.Int32  `tfsdk:"mtu"`
	Interface_Vnet       types.Int32  `tfsdk:"interface_vnet"`
	IPaddress_Type       types.String `tfsdk:"ipaddress_type"`
	Layer2_Type          types.String `tfsdk:"layer2_type"`
	Enable_Bonding       types.Bool   `tfsdk:"enable_bonding"`
	Bond_Interfaces_Args types.List   `tfsdk:"bond_interfaces_args"`
}

// Metadata returns the resource type name.
func (r *NetworkResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *NetworkResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Network or Vnet resource in VergeIO",
		// Version 1 stores powerstate as a bool. Version 0 stored the
		// strings "true" and "false".
		Version: 1,

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
			"powerstate": schema.BoolAttribute{
				MarkdownDescription: "Whether the network is powered on. Read from the API powerstate boolean after create and import.",
				Optional:            true,
				Computed:            true,
			},
			"restart_on_change": schema.BoolAttribute{
				MarkdownDescription: "Restart a running network after an update that VergeOS stages with need_restart, such as a DHCP range or address change. Defaults to true. Set to false to keep the network up until a maintenance window.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"need_restart": schema.BoolAttribute{
				MarkdownDescription: "Whether VergeOS has staged a change that is not live until the network restarts. True after an update when the network was not restarted.",
				Computed:            true,
				// Stays unknown on purpose. An update can set this, and the plan
				// should not claim the previous value will remain.
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Type of Network",
				Optional:            true,
				Computed:            true,
				// Readonly after create. Updates do not send it.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"layer2_id": schema.Int32Attribute{
				MarkdownDescription: "VLAN ID",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"mtu": schema.Int32Attribute{
				MarkdownDescription: "Network MTU",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"interface_vnet": schema.Int32Attribute{
				MarkdownDescription: "Key/ID of the physical network",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"ipaddress_type": schema.StringAttribute{
				MarkdownDescription: "IP address type of the vnet",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"layer2_type": schema.StringAttribute{
				MarkdownDescription: "Layer2 type of the vnet",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"enable_bonding": schema.BoolAttribute{
				MarkdownDescription: "Enable bonding",
				Optional:            true,
				Computed:            true,
			},
			"bond_interfaces_args": schema.ListAttribute{
				ElementType:         types.Int32Type,
				MarkdownDescription: "Bonding interfaces arguments",
				Optional:            true,
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
		// if the resource was not found, likely deleted outside of terraform
		// remove the resource from the state
		// and return
		if strings.Contains(readDataError.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}

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
		return
	}

	// DHCP and address changes are staged. Restart a running network so the
	// apply is not reported as live while need_restart is still set.
	if err := r.restartAfterUpdate(ctx, &planData, &resp.Diagnostics); err != nil {
		resp.Diagnostics.AddError(
			"Error Restarting Network",
			err.Error(),
		)
		return
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

// restartAfterUpdate restarts a running network when VergeOS staged the
// update. When the restart is skipped, it warns so the apply output shows
// that the change is not live yet.
func (r *NetworkResource) restartAfterUpdate(ctx context.Context, data *NetworkResourceModel, diags *diag.Diagnostics) error {
	notice, err := r.networkApi.reconcileStagedRestart(ctx, data)
	if err != nil {
		return err
	}
	if notice.Detail != "" {
		diags.AddWarning(notice.Summary, notice.Detail)
	}
	return nil
}

func (r *NetworkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data NetworkResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Kill a running network once, then poll until it is stopped.
	if err := r.networkApi.stopNetworkBeforeDelete(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Failed to stop Network before deletion:",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Network state before deletion %v", data.PowerState.ValueBool()))

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

// UpgradeState converts version 0 network state, which stored powerstate as
// the strings "true" and "false", to the bool stored by version 1.
// "running" and "stopped" are accepted because an earlier delete path wrote
// those words into the same attribute. A blank string becomes null.
func (r *NetworkResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
			tflog.Info(ctx, "Upgrading network state to v1: powerstate bool")

			normalized, err := normalizeNetworkStateJSON(req.RawState.JSON)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error upgrading network state",
					fmt.Sprintf("Failed to normalize state: %s", err),
				)
				return
			}

			newSchemaType := resp.State.Schema.Type().TerraformType(ctx)
			newStateValue, err := tftypes.ValueFromJSON(normalized, newSchemaType)
			if err != nil {
				resp.Diagnostics.AddError(
					"Error upgrading network state",
					fmt.Sprintf("Failed to parse state: %s", err),
				)
				return
			}

			resp.State.Raw = newStateValue
		}},
	}
}

func normalizeNetworkStateJSON(raw []byte) ([]byte, error) {
	var state map[string]json.RawMessage
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}
	rawVal, ok := state["powerstate"]
	if !ok {
		return json.Marshal(state)
	}
	converted, err := networkPowerStateOrNull(rawVal)
	if err != nil {
		return nil, fmt.Errorf("powerstate: %w", err)
	}
	state["powerstate"] = converted
	return json.Marshal(state)
}

func networkPowerStateOrNull(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return json.RawMessage("null"), nil
	}
	switch string(trimmed) {
	case "true", "false":
		return json.RawMessage(trimmed), nil
	}
	if trimmed[0] != '"' {
		return nil, fmt.Errorf("cannot convert %s to bool", trimmed)
	}

	var s string
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return nil, err
	}
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "running":
		return json.RawMessage("true"), nil
	case "false", "stopped":
		return json.RawMessage("false"), nil
	case "":
		return json.RawMessage("null"), nil
	default:
		return nil, fmt.Errorf("cannot convert %q to bool", s)
	}
}
