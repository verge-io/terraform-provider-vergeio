// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const incomingDescription = "An incoming site sync on a vergeio_site. site_id is vergeio_site.id. Changing site_id replaces the sync. min_snapshots is the number of snapshots kept even when retention would delete them. registration_code is the code the sending system uses. VergeOS may fill it in after create."

var (
	_ resource.Resource                = &incomingResource{}
	_ resource.ResourceWithConfigure   = &incomingResource{}
	_ resource.ResourceWithImportState = &incomingResource{}
	_ resource.ResourceWithIdentity    = &incomingResource{}

	syncTiers = []string{"unspecified", "1", "2", "3", "4", "5"}
)

func NewSyncIncomingResource() resource.Resource {
	return &incomingResource{}
}

type incomingResource struct {
	api *API
}

type incomingModel struct {
	ID               types.String `tfsdk:"id"`
	SiteID           types.String `tfsdk:"site_id"`
	Name             types.String `tfsdk:"name"`
	Description      types.String `tfsdk:"description"`
	PublicIP         types.String `tfsdk:"public_ip"`
	ForceTier        types.String `tfsdk:"force_tier"`
	VSANHost         types.String `tfsdk:"vsan_host"`
	VSANPort         types.Int64  `tfsdk:"vsan_port"`
	RequestURL       types.String `tfsdk:"request_url"`
	MinSnapshots     types.Int64  `tfsdk:"min_snapshots"`
	Enabled          types.Bool   `tfsdk:"enabled"`
	SyncID           types.String `tfsdk:"sync_id"`
	RegistrationCode types.String `tfsdk:"registration_code"`
	Status           types.String `tfsdk:"status"`
	StatusInfo       types.String `tfsdk:"status_info"`
	State            types.String `tfsdk:"state"`
	LastSync         types.Int64  `tfsdk:"last_sync"`
	SystemCreated    types.Bool   `tfsdk:"system_created"`
}

func (r *incomingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_sync_incoming"
}

func (r *incomingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: incomingDescription,
		Attributes: map[string]schema.Attribute{
			"id":            idAttr("Incoming sync key assigned by VergeOS."),
			"site_id":       replaceString("Site key, the same value as vergeio_site.id. Changing it replaces the sync. Terraform reads this value from the sync.", stringvalidator.RegexMatches(regexpPositive, "Use the site key, a positive integer.")),
			"name":          schema.StringAttribute{MarkdownDescription: "Sync name. Must be unique on the site.", Required: true},
			"description":   optString("What this sync receives. Omit to leave the current value unchanged."),
			"public_ip":     optString("Address the sending system connects from. Omit to leave the current value unchanged."),
			"force_tier":    optString("Storage tier forced for received data: unspecified, or 1 through 5. Omit to leave the current value unchanged.", stringvalidator.OneOf(syncTiers...)),
			"vsan_host":     optString("vSAN host for this sync. Omit to leave the current value unchanged."),
			"vsan_port":     optInt("vSAN port, from 1 through 65535. Omit to leave the current value unchanged.", int64validator.Between(1, 65535)),
			"request_url":   optString("URL the sending system uses. Omit to leave the current value unchanged."),
			"min_snapshots": optInt("Minimum snapshots to keep, including when retention would delete them. Omit to leave the current value unchanged.", int64validator.AtLeast(0)),
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the sync accepts data. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"sync_id":           stableString("SHA1 id of this sync."),
			"registration_code": schema.StringAttribute{MarkdownDescription: "Registration code the sending system uses. VergeOS may fill it in after create.", Computed: true, Sensitive: true},
			"status":            volatileString("Current sync status."),
			"status_info":       volatileString("Detail for status."),
			"state":             volatileString("Connection state."),
			"last_sync":         timestampAttr("Time of the last sync, as seconds since the epoch. 0 means the sync has not run."),
			"system_created":    volatileBool("Whether VergeOS created this sync."),
		},
	}
}

func (r *incomingResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Incoming sync key, a positive integer.")
}

func (r *incomingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureAPI(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *incomingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan incomingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createIncoming(ctx, &plan); err != nil {
		if incomingIDSet(&plan) {
			r.rememberIncoming(ctx, resp, &plan)
		}
		resp.Diagnostics.AddError("Error creating incoming site sync", err.Error())
		return
	}
	r.rememberIncoming(ctx, resp, &plan)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, incomingForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *incomingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data incomingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readIncoming(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading incoming site sync", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, incomingForState(&data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *incomingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state incomingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateIncoming(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating incoming site sync", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, incomingForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *incomingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data incomingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseID(data.ID, "incoming sync")
	if err != nil {
		resp.Diagnostics.AddError("Error deleting incoming site sync", err.Error())
		return
	}
	if err := r.api.deleteIncoming(ctx, id); err != nil {
		resp.Diagnostics.AddError("Error deleting incoming site sync", err.Error())
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted incoming site sync %d", id))
}

func (r *incomingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importPositive(ctx, req, resp, "Invalid incoming site sync import id", "Import vergeio_site_sync_incoming with the sync key, a positive integer.")
}

func (r *incomingResource) rememberIncoming(ctx context.Context, resp *resource.CreateResponse, data *incomingModel) {
	if !incomingIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, incomingForState(data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func incomingIDSet(data *incomingModel) bool {
	return data != nil && !data.ID.IsNull() && !data.ID.IsUnknown() && data.ID.ValueString() != ""
}
