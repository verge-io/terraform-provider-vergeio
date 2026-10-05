// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

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
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const tenantMarkdown = "VergeOS tenant: a full VergeOS instance carved from the parent, including its power state and UI address. " +
	"powerstate powers the tenant on or off and waits up to 2 minutes for terminal online or offline status and a stopped tenant network. " +
	"ui_address is the IP of the tenant UI, read from the ui_address row. " +
	"Resources inside the tenant use a second Terraform configuration. See the tenants guide. " +
	"vergeio_tenant_external_ip assigns one parent external IP to this tenant. The first assigned IP becomes ui_address. The next plan stores that address once it exists. " +
	"vergeio_tenant_network_block assigns one routed CIDR (vnet_cidrs) from a parent network to this tenant. " +
	"Assigning an address or a block can leave need_fw_apply set on the parent external network. vergeio_tenant_external_ip and vergeio_tenant_network_block report that as parent_firewall_pending. Apply that network's firewall before treating the assignment as live."

var (
	_ resource.Resource                = &TenantResource{}
	_ resource.ResourceWithImportState = &TenantResource{}
)

func NewTenantResource() resource.Resource {
	return &TenantResource{}
}

// TenantResource is vergeio_tenant.
type TenantResource struct {
	api *API
}

// TenantResourceModel is the Terraform model for vergeio_tenant.
type TenantResourceModel struct {
	Id                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Description          types.String `tfsdk:"description"`
	Password             types.String `tfsdk:"password"`
	URL                  types.String `tfsdk:"url"`
	OIDCApplication      types.Int32  `tfsdk:"oidc_application"`
	ExposeCloudSnapshots types.Bool   `tfsdk:"expose_cloud_snapshots"`
	AllowBranding        types.Bool   `tfsdk:"allow_branding"`
	ChangePassword       types.Bool   `tfsdk:"change_password"`
	ThemeAccess          types.String `tfsdk:"theme_access"`
	HelpURL              types.String `tfsdk:"help_url"`
	Note                 types.String `tfsdk:"note"`
	PowerState           types.Bool   `tfsdk:"powerstate"`
	PreferredNode        types.Int32  `tfsdk:"preferred_node"`
	UUID                 types.String `tfsdk:"uuid"`
	VNet                 types.Int32  `tfsdk:"vnet"`
	UIAddressID          types.Int32  `tfsdk:"ui_address_id"`
	UIAddress            types.String `tfsdk:"ui_address"`
	Isolate              types.Bool   `tfsdk:"isolate"`
	IsSnapshot           types.Bool   `tfsdk:"is_snapshot"`
	Status               types.String `tfsdk:"status"`
	State                types.String `tfsdk:"state"`
	Creator              types.String `tfsdk:"creator"`
	Created              types.Int64  `tfsdk:"created"`
}

func (r *TenantResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant"
}

func (r *TenantResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: tenantMarkdown,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Tenant name. Must be unique on the parent system.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Tenant description. Omit to leave an existing description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Password for the tenant admin user. Stored in state. VergeOS does not return it. Omit to leave the current password unchanged.",
				Optional:            true,
				Sensitive:           true,
			},
			"url": schema.StringAttribute{
				MarkdownDescription: "Optional URL recorded on the tenant.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"oidc_application": schema.Int32Attribute{
				MarkdownDescription: "OIDC application key for SSO. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"expose_cloud_snapshots": schema.BoolAttribute{
				MarkdownDescription: "Allow the tenant to request system cloud snapshots.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"allow_branding": schema.BoolAttribute{
				MarkdownDescription: "Allow the tenant to customize colors and logo.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"change_password": schema.BoolAttribute{
				MarkdownDescription: "Require a password change on first login. Sent only when the tenant is created. VergeOS does not return it, so Terraform keeps the configured value. Changing a configured value replaces the tenant.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplaceIfConfigured(),
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"theme_access": schema.StringAttribute{
				MarkdownDescription: "Theme visibility: specified, host_only, local_only, or both.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf("specified", "host_only", "local_only", "both"),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"help_url": schema.StringAttribute{
				MarkdownDescription: "Custom help URL. Use default for the product default. A blank value disables the help link.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"note": schema.StringAttribute{
				MarkdownDescription: "Free-form note stored on the tenant.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"powerstate": schema.BoolAttribute{
				MarkdownDescription: "Whether the tenant is powered on. true calls power on and waits until status is terminal online (not merely starting). false calls power off and waits until status is terminal offline (not merely stopping) and the tenant network is stopped. Omit to leave the current power unchanged. Destroy powers the tenant off and waits for its network to stop before delete. vergeio_tenant_node destroy gracefully powers off only that node when it is running (then kills if needed), leaving sibling nodes alone. The wait is 2 minutes. powerstate set to true defers power on when the tenant has no node yet, including the first apply of a configuration that also declares vergeio_tenant_node. Refresh stores the offline power state. The next apply powers the tenant on.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"preferred_node": schema.Int32Attribute{
				MarkdownDescription: "Host node used when powerstate changes to true. Changing it while the tenant is already on does not move the tenant. VergeOS does not return this value; state keeps the configured node.",
				Optional:            true,
			},
			"uuid": schema.StringAttribute{
				MarkdownDescription: "Tenant UUID assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vnet": schema.Int32Attribute{
				MarkdownDescription: "Key of the network VergeOS creates for the tenant.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"ui_address_id": schema.Int32Attribute{
				MarkdownDescription: "Key of the vnet address row that holds the tenant UI address. Empty until VergeOS assigns one.",
				Computed:            true,
			},
			"ui_address": schema.StringAttribute{
				MarkdownDescription: "IP address of the tenant UI, resolved from ui_address_id. The first IP from vergeio_tenant_external_ip becomes this address. The next plan stores it once VergeOS has assigned it. Use it as the host of the provider configuration that manages the inside of the tenant.",
				Computed:            true,
			},
			"isolate": schema.BoolAttribute{
				MarkdownDescription: "Whether network isolation is enabled. Read from VergeOS. This resource does not change isolation.",
				Computed:            true,
			},
			"is_snapshot": schema.BoolAttribute{
				MarkdownDescription: "Whether this tenant record is a snapshot.",
				Computed:            true,
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Tenant status string from tenant_status, such as online, starting, stopping, or offline.",
				Computed:            true,
			},
			"state": schema.StringAttribute{
				MarkdownDescription: "Simplified tenant state from tenant_status: online, offline, warning, or error.",
				Computed:            true,
			},
			"creator": schema.StringAttribute{
				MarkdownDescription: "Username that created the tenant.",
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
		},
	}
}

func (r *TenantResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TenantResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createTenant(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error creating tenant", err.Error())
		return
	}
	// The tenant row exists. Keep its id in the response before power and
	// read, so a later error still leaves the tenant in state.
	r.rememberTenant(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		resp.Diagnostics.AddError("Error creating tenant", err.Error())
		return
	}
	desiredPower := data.PowerState
	deferred, err := r.api.reconcilePowerOnCreate(ctx, id, data.PowerState, data.PreferredNode)
	if err != nil {
		resp.Diagnostics.AddError("Error creating tenant", fmt.Errorf("tenant %d was created: %w", id, err).Error())
		return
	}
	if err := r.api.readTenant(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tenant", err.Error())
		return
	}
	// powerstate=true with no nodes yet defers power-on (#207). Keep the
	// planned true so Create state matches plan; a later apply powers on
	// once vergeio_tenant_node exists. Post-apply refresh stores offline.
	if deferred && !desiredPower.IsNull() && !desiredPower.IsUnknown() && desiredPower.ValueBool() {
		data.PowerState = desiredPower
	}
	stored := tenantForState(&data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &stored)...)
}

func (r *TenantResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenant(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TenantResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	desiredPower := plan.PowerState
	deferred, err := r.api.updateTenant(ctx, &plan, &state)
	if err != nil {
		resp.Diagnostics.AddError("Error updating tenant", err.Error())
		return
	}
	// readTenant leaves password, preferred_node, and change_password alone.
	// VergeOS does not return the first two, and refreshing change_password
	// would replace the tenant after the first-login flag clears.
	if err := r.api.readTenant(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tenant", err.Error())
		return
	}
	// powerstate=true with no nodes yet defers power-on (#219), same as
	// create (#207). Keep the planned true so Update state matches plan; a
	// later apply powers on once vergeio_tenant_node exists. Post-apply
	// refresh stores the actual offline powerstate.
	if deferred && !desiredPower.IsNull() && !desiredPower.IsUnknown() && desiredPower.ValueBool() {
		plan.PowerState = desiredPower
	}
	stored := tenantForState(&plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &stored)...)
}

func (r *TenantResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteTenant(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tenant", err.Error())
		return
	}
	tflog.Debug(ctx, "tenant deleted")
}

func (r *TenantResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Tenant Import ID",
			"Import vergeio_tenant with the tenant key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// rememberTenant writes the tenant id into the create response before power
// and the follow-up read. Terraform keeps that state when a later step
// returns an error, so the next apply updates the tenant instead of
// creating a second one with the same name.
func (r *TenantResource) rememberTenant(ctx context.Context, resp *resource.CreateResponse, data *TenantResourceModel) {
	if !tenantIDSet(data) {
		return
	}
	stored := tenantForState(data)
	resp.Diagnostics.Append(resp.State.Set(ctx, &stored)...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored tenant %s in state before power and read", data.Id.ValueString()))
}

func tenantIDSet(data *TenantResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && strings.TrimSpace(data.Id.ValueString()) != ""
}

// tenantForState copies the model and drops unknown values. State cannot
// store them. Config-only attributes such as change_password stay as
// configured when they are known, and become null when the configuration
// omitted them.
func tenantForState(data *TenantResourceModel) TenantResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.Name = knownString(data.Name)
	stored.Description = knownString(data.Description)
	stored.Password = knownString(data.Password)
	stored.URL = knownString(data.URL)
	stored.OIDCApplication = knownInt32(data.OIDCApplication)
	stored.ExposeCloudSnapshots = knownBool(data.ExposeCloudSnapshots)
	stored.AllowBranding = knownBool(data.AllowBranding)
	stored.ChangePassword = knownBool(data.ChangePassword)
	stored.ThemeAccess = knownString(data.ThemeAccess)
	stored.HelpURL = knownString(data.HelpURL)
	stored.Note = knownString(data.Note)
	stored.PowerState = knownBool(data.PowerState)
	stored.PreferredNode = knownInt32(data.PreferredNode)
	stored.UUID = knownString(data.UUID)
	stored.VNet = knownInt32(data.VNet)
	stored.UIAddressID = knownInt32(data.UIAddressID)
	stored.UIAddress = knownString(data.UIAddress)
	stored.Isolate = knownBool(data.Isolate)
	stored.IsSnapshot = knownBool(data.IsSnapshot)
	stored.Status = knownString(data.Status)
	stored.State = knownString(data.State)
	stored.Creator = knownString(data.Creator)
	stored.Created = knownInt64(data.Created)
	return stored
}

func knownString(v types.String) types.String {
	if v.IsNull() || v.IsUnknown() {
		return types.StringNull()
	}
	return v
}

func knownBool(v types.Bool) types.Bool {
	if v.IsNull() || v.IsUnknown() {
		return types.BoolNull()
	}
	return v
}

func knownInt32(v types.Int32) types.Int32 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int32Null()
	}
	return v
}

func knownInt64(v types.Int64) types.Int64 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int64Null()
	}
	return v
}
