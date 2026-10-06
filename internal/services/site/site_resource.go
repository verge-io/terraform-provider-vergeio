// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"fmt"
	"regexp"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/float64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const siteDescription = "A remote VergeOS system this one pairs with. name and url are required. auth_password_wo is sent when the site is created and when auth_password_wo_version changes. Terraform stores the version, not the password. On create those values are the pairing user and password. On a later change they are sent as the remote user and remote password. Create does not ask VergeOS to build syncs. Declare vergeio_site_sync_incoming and vergeio_site_sync_outgoing for the syncs you want. Cloud snapshots use vergeio_snapshot_profile. VergeOS uses that same profile for VMs, volumes, and cloud snapshots."

var (
	_ resource.Resource                = &siteResource{}
	_ resource.ResourceWithConfigure   = &siteResource{}
	_ resource.ResourceWithImportState = &siteResource{}
	_ resource.ResourceWithIdentity    = &siteResource{}

	configDirections = []string{"disabled", "send", "receive", "both"}
	managementModes  = []string{"disabled", "manage", "managed", "both"}
	countryPattern   = regexp.MustCompile(`^([A-Za-z]{2})?$`)
)

func NewSiteResource() resource.Resource {
	return &siteResource{}
}

type siteResource struct {
	api *API
}

type siteModel struct {
	ID                        types.String  `tfsdk:"id"`
	Name                      types.String  `tfsdk:"name"`
	URL                       types.String  `tfsdk:"url"`
	Description               types.String  `tfsdk:"description"`
	Domain                    types.String  `tfsdk:"domain"`
	City                      types.String  `tfsdk:"city"`
	Country                   types.String  `tfsdk:"country"`
	Timezone                  types.String  `tfsdk:"timezone"`
	AllowInsecure             types.Bool    `tfsdk:"allow_insecure"`
	ConfigCloudSnapshots      types.String  `tfsdk:"config_cloud_snapshots"`
	ConfigStatistics          types.String  `tfsdk:"config_statistics"`
	ConfigManagement          types.String  `tfsdk:"config_management"`
	ConfigRepairServer        types.String  `tfsdk:"config_repair_server"`
	StatisticsInterval        types.Int64   `tfsdk:"statistics_interval"`
	StatisticsRetention       types.Int64   `tfsdk:"statistics_retention"`
	RequestURL                types.String  `tfsdk:"request_url"`
	Latitude                  types.Float64 `tfsdk:"latitude"`
	Longitude                 types.Float64 `tfsdk:"longitude"`
	Enabled                   types.Bool    `tfsdk:"enabled"`
	AuthUser                  types.String  `tfsdk:"auth_user"`
	AuthPasswordWO            types.String  `tfsdk:"auth_password_wo"`
	AuthPasswordWOVersion     types.Int64   `tfsdk:"auth_password_wo_version"`
	SiteID                    types.String  `tfsdk:"site_id"`
	Status                    types.String  `tfsdk:"status"`
	StatusInfo                types.String  `tfsdk:"status_info"`
	AuthenticationStatus      types.String  `tfsdk:"authentication_status"`
	VSANHost                  types.String  `tfsdk:"vsan_host"`
	VSANPort                  types.Int64   `tfsdk:"vsan_port"`
	IsTenant                  types.Bool    `tfsdk:"is_tenant"`
	IncomingSyncsEnabled      types.Bool    `tfsdk:"incoming_syncs_enabled"`
	OutgoingSyncsEnabled      types.Bool    `tfsdk:"outgoing_syncs_enabled"`
	RepairsOutgoingEnabled    types.Bool    `tfsdk:"repairs_outgoing_enabled"`
	IncomingStatsEnabled      types.Bool    `tfsdk:"incoming_stats_enabled"`
	OutgoingStatsEnabled      types.Bool    `tfsdk:"outgoing_stats_enabled"`
	OutgoingManagementEnabled types.Bool    `tfsdk:"outgoing_management_enabled"`
	IncomingManagementEnabled types.Bool    `tfsdk:"incoming_management_enabled"`
	RemoteUser                types.String  `tfsdk:"remote_user"`
	LastStatUpdate            types.Int64   `tfsdk:"last_stat_update"`
	Created                   types.Int64   `tfsdk:"created"`
	Modified                  types.Int64   `tfsdk:"modified"`
	Creator                   types.String  `tfsdk:"creator"`
}

func (r *siteResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site"
}

func (r *siteResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: siteDescription,
		Attributes: map[string]schema.Attribute{
			"id":                     idAttr("Site key assigned by VergeOS."),
			"name":                   schema.StringAttribute{MarkdownDescription: "Site name. Must be unique.", Required: true},
			"url":                    schema.StringAttribute{MarkdownDescription: "URL of the remote VergeOS system.", Required: true},
			"description":            optString("What this site is for. Omit to leave the current value unchanged."),
			"domain":                 optString("DNS domain of the remote system. Omit to leave the current value unchanged."),
			"city":                   optString("City shown for this site. Omit to leave the current value unchanged."),
			"country":                optString("Two letter country code, or empty. Omit to leave the current value unchanged.", stringvalidator.RegexMatches(countryPattern, "Use a two letter country code, or leave it empty.")),
			"timezone":               optString("Timezone name, such as America/Chicago. Omit to leave the current value unchanged."),
			"allow_insecure":         optBool("Allow a TLS certificate VergeOS would otherwise reject. Omit to leave the current value unchanged."),
			"config_cloud_snapshots": optString("Cloud snapshot direction: disabled, send, receive, or both. Omit to leave the current value unchanged.", stringvalidator.OneOf(configDirections...)),
			"config_statistics":      optString("Statistics direction: disabled, send, receive, or both. Omit to leave the current value unchanged.", stringvalidator.OneOf(configDirections...)),
			"config_management":      optString("Management direction: disabled, manage, managed, or both. Omit to leave the current value unchanged.", stringvalidator.OneOf(managementModes...)),
			"config_repair_server":   optString("Repair server direction: disabled, send, receive, or both. Omit to leave the current value unchanged.", stringvalidator.OneOf(configDirections...)),
			"statistics_interval":    optInt("Seconds between statistics collections. Minimum 300. Omit to leave the current value unchanged.", int64validator.AtLeast(300)),
			"statistics_retention":   optInt("Seconds to keep collected statistics. Minimum 1. Omit to leave the current value unchanged.", int64validator.AtLeast(1)),
			"request_url":            optString("URL the remote system uses to connect back. Omit to leave the current value unchanged."),
			"latitude":               optFloat("Latitude from -90 through 90. Omit to leave the current value unchanged.", float64validator.Between(-90, 90)),
			"longitude":              optFloat("Longitude from -180 through 180. Omit to leave the current value unchanged.", float64validator.Between(-180, 180)),
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the site connection is enabled. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"auth_user": optString("User sent when the site is created, and sent as the remote user when auth_password_wo_version changes. VergeOS does not return it. Omit to leave the stored value unchanged."),
			"auth_password_wo": schema.StringAttribute{
				MarkdownDescription: "Password sent when the site is created and when auth_password_wo_version changes. Terraform does not store it. Requires Terraform 1.11 or OpenTofu 1.11.",
				Optional:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("auth_password_wo_version")),
				},
			},
			"auth_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of auth_password_wo. Increment it to send a new password. Terraform stores this number, not the password.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("auth_password_wo")),
				},
			},
			"site_id":                     stableString("SHA1 id of the remote system."),
			"status":                      volatileString("Current site status."),
			"status_info":                 volatileString("Detail for status."),
			"authentication_status":       volatileString("Whether the site is authenticated."),
			"vsan_host":                   volatileString("vSAN host reported for this site."),
			"vsan_port":                   volatileInt("vSAN port reported for this site."),
			"is_tenant":                   volatileBool("Whether the remote system is a tenant."),
			"incoming_syncs_enabled":      volatileBool("Whether incoming syncs are enabled on this site."),
			"outgoing_syncs_enabled":      volatileBool("Whether outgoing syncs are enabled on this site."),
			"repairs_outgoing_enabled":    volatileBool("Whether outgoing repairs are enabled."),
			"incoming_stats_enabled":      volatileBool("Whether incoming statistics are enabled."),
			"outgoing_stats_enabled":      volatileBool("Whether outgoing statistics are enabled."),
			"outgoing_management_enabled": volatileBool("Whether this system can manage the remote system."),
			"incoming_management_enabled": volatileBool("Whether the remote system can manage this system."),
			"remote_user":                 volatileString("Remote user stored by VergeOS."),
			"last_stat_update":            timestampAttr("Time statistics were last updated, as seconds since the epoch."),
			"created":                     timestampAttr("Time the site was created, as seconds since the epoch."),
			"modified":                    timestampAttr("Time the site was last modified, as seconds since the epoch."),
			"creator":                     stableString("User that created the site."),
		},
	}
}

func (r *siteResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Site key, a positive integer.")
}

func (r *siteResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureAPI(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *siteResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config siteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createSite(ctx, &plan, secret(config.AuthPasswordWO)); err != nil {
		if siteIDSet(&plan) {
			r.rememberSite(ctx, resp, &plan)
		}
		resp.Diagnostics.AddError("Error creating site", err.Error())
		return
	}
	r.rememberSite(ctx, resp, &plan)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, siteForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *siteResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data siteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readSite(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading site", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, siteForState(&data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *siteResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config siteModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateSite(ctx, &plan, &state, secret(config.AuthPasswordWO)); err != nil {
		resp.Diagnostics.AddError("Error updating site", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, siteForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *siteResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data siteModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseID(data.ID, "site")
	if err != nil {
		resp.Diagnostics.AddError("Error deleting site", err.Error())
		return
	}
	if err := r.api.deleteSite(ctx, id); err != nil {
		resp.Diagnostics.AddError("Error deleting site", err.Error())
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted site %d", id))
}

func (r *siteResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importPositive(ctx, req, resp, "Invalid site import id", "Import vergeio_site with the site key, a positive integer.")
}

func (r *siteResource) rememberSite(ctx context.Context, resp *resource.CreateResponse, data *siteModel) {
	if !siteIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, siteForState(data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func siteIDSet(data *siteModel) bool {
	return data != nil && !data.ID.IsNull() && !data.ID.IsUnknown() && data.ID.ValueString() != ""
}
