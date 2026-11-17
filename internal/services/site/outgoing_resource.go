// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const outgoingDescription = "An outgoing site sync to a vergeio_site. site_id is vergeio_site.id. Changing site_id replaces the sync. registration_code_wo is required. It is the registration code from the incoming sync on the remote system, and VergeOS rejects a create that omits it. The code is sent only when this sync is created. A later version change is stored and does not register the sync again. Replace the sync to register again. Each period selects a vergeio_snapshot_profile period (period.key) and sets how long the remote copy is kept. A period left out of the configuration is deleted. Changing profile_period replaces that period."

var (
	_ resource.Resource                = &outgoingResource{}
	_ resource.ResourceWithConfigure   = &outgoingResource{}
	_ resource.ResourceWithImportState = &outgoingResource{}
	_ resource.ResourceWithIdentity    = &outgoingResource{}
)

func NewSyncOutgoingResource() resource.Resource {
	return &outgoingResource{}
}

type outgoingResource struct {
	api *API
}

type outgoingModel struct {
	ID                           types.String      `tfsdk:"id"`
	SiteID                       types.String      `tfsdk:"site_id"`
	Name                         types.String      `tfsdk:"name"`
	Description                  types.String      `tfsdk:"description"`
	URL                          types.String      `tfsdk:"url"`
	DestinationTier              types.String      `tfsdk:"destination_tier"`
	Threads                      types.Int64       `tfsdk:"threads"`
	FileThreads                  types.Int64       `tfsdk:"file_threads"`
	Encryption                   types.Bool        `tfsdk:"encryption"`
	Compression                  types.Bool        `tfsdk:"compression"`
	NetInteg                     types.Bool        `tfsdk:"netinteg"`
	SendThrottle                 types.Int64       `tfsdk:"send_throttle"`
	QueueRetryCount              types.Int64       `tfsdk:"queue_retry_count"`
	QueueRetryIntervalSeconds    types.Int64       `tfsdk:"queue_retry_interval_seconds"`
	QueueRetryIntervalMultiplier types.Bool        `tfsdk:"queue_retry_interval_multiplier"`
	Note                         types.String      `tfsdk:"note"`
	Enabled                      types.Bool        `tfsdk:"enabled"`
	RegistrationCodeWO           types.String      `tfsdk:"registration_code_wo"`
	RegistrationCodeWOVersion    types.Int64       `tfsdk:"registration_code_wo_version"`
	Period                       []syncPeriodModel `tfsdk:"period"`
	Status                       types.String      `tfsdk:"status"`
	StatusInfo                   types.String      `tfsdk:"status_info"`
	State                        types.String      `tfsdk:"state"`
	User                         types.String      `tfsdk:"user"`
	RemoteSiteID                 types.String      `tfsdk:"remote_site_id"`
	RemoteVSANHost               types.String      `tfsdk:"remote_vsan_host"`
	RemoteVSANPort               types.Int64       `tfsdk:"remote_vsan_port"`
	RemoteSyncID                 types.String      `tfsdk:"remote_sync_id"`
	RemoteMinSnapshots           types.Int64       `tfsdk:"remote_min_snapshots"`
	RemoteSnapsStatus            types.String      `tfsdk:"remote_snaps_status"`
	RemoteSnapsStatusInfo        types.String      `tfsdk:"remote_snaps_status_info"`
	RemoteSnapsLastRefresh       types.Int64       `tfsdk:"remote_snaps_last_refresh"`
	LastRun                      types.Int64       `tfsdk:"last_run"`
}

type syncPeriodModel struct {
	ProfilePeriod     types.String `tfsdk:"profile_period"`
	Retention         types.Int64  `tfsdk:"retention"`
	Priority          types.Int64  `tfsdk:"priority"`
	DoNotExpire       types.Bool   `tfsdk:"do_not_expire"`
	DestinationPrefix types.String `tfsdk:"destination_prefix"`
	Key               types.String `tfsdk:"key"`
	ScheduleTask      types.Int64  `tfsdk:"schedule_task"`
	Task              types.Int64  `tfsdk:"task"`
}

func (r *outgoingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_sync_outgoing"
}

func (r *outgoingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: outgoingDescription,
		Attributes: map[string]schema.Attribute{
			"id":                              idAttr("Outgoing sync key assigned by VergeOS."),
			"site_id":                         replaceString("Site key, the same value as vergeio_site.id. Changing it replaces the sync. Terraform reads this value from the sync.", stringvalidator.RegexMatches(regexpPositive, "Use the site key, a positive integer.")),
			"name":                            schema.StringAttribute{MarkdownDescription: "Sync name. Must be unique on the site.", Required: true},
			"description":                     optString("What this sync sends. Omit to leave the current value unchanged."),
			"url":                             optString("Remote URL for this sync. Omit to leave the current value unchanged."),
			"destination_tier":                optString("Storage tier on the destination: unspecified, or 1 through 5. Omit to leave the current value unchanged.", stringvalidator.OneOf(syncTiers...)),
			"threads":                         optInt("Data threads, from 1 through 32. Omit to leave the current value unchanged.", int64validator.Between(1, 32)),
			"file_threads":                    optInt("File scanning threads, from 1 through 64. Omit to leave the current value unchanged.", int64validator.Between(1, 64)),
			"encryption":                      optBool("Encrypt data in flight. Omit to leave the current value unchanged."),
			"compression":                     optBool("Compress data in flight. Omit to leave the current value unchanged."),
			"netinteg":                        optBool("Checksum network traffic. Omit to leave the current value unchanged."),
			"send_throttle":                   optInt("Send throttle. 0 disables it. Omit to leave the current value unchanged.", int64validator.AtLeast(0)),
			"queue_retry_count":               optInt("How many times to retry a queued transfer. Omit to leave the current value unchanged.", int64validator.AtLeast(0)),
			"queue_retry_interval_seconds":    optInt("Seconds between queue retries. Omit to leave the current value unchanged.", int64validator.AtLeast(0)),
			"queue_retry_interval_multiplier": optBool("Grow the retry interval after each attempt. Omit to leave the current value unchanged."),
			"note":                            optString("Note stored with the sync. Omit to leave the current value unchanged."),
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the sync sends data. Defaults to true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"registration_code_wo": schema.StringAttribute{
				MarkdownDescription: "Registration code from the incoming sync on the remote system. Required. VergeOS rejects a create that omits it. Sent only when this sync is created. A later change to registration_code_wo_version is stored and does not register the sync again. Replace the sync to register again. Terraform does not store the code. Requires Terraform 1.11 or OpenTofu 1.11.",
				Required:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("registration_code_wo_version")),
				},
			},
			"registration_code_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of registration_code_wo. Required. Terraform stores this number. Changing it does not send the code again.",
				Required:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("registration_code_wo")),
				},
			},
			"status":                    volatileString("Current sync status."),
			"status_info":               volatileString("Detail for status."),
			"state":                     volatileString("Connection state."),
			"user":                      volatileString("Site user reported for this sync."),
			"remote_site_id":            volatileString("SHA1 id of the remote site."),
			"remote_vsan_host":          volatileString("Remote vSAN host."),
			"remote_vsan_port":          volatileInt("Remote vSAN port."),
			"remote_sync_id":            volatileString("SHA1 id of the remote sync."),
			"remote_min_snapshots":      volatileInt("Minimum snapshots reported by the remote sync."),
			"remote_snaps_status":       volatileString("Status of the remote snapshot list."),
			"remote_snaps_status_info":  volatileString("Detail for remote_snaps_status."),
			"remote_snaps_last_refresh": timestampAttr("Time the remote snapshot list was refreshed, as seconds since the epoch."),
			"last_run":                  timestampAttr("Time of the last run, as seconds since the epoch. 0 means the sync has not run."),
		},
		Blocks: map[string]schema.Block{
			"period": schema.ListNestedBlock{
				MarkdownDescription: "One snapshot profile period to send, and how long the remote copy is kept. profile_period is vergeio_snapshot_profile.period.key. retention is seconds on the remote system and does not change the local profile. A period left out of the configuration is deleted. Changing profile_period replaces that period.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"profile_period": schema.StringAttribute{
							MarkdownDescription: "Snapshot profile period key. Changing it replaces this period.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.RegexMatches(regexpPositive, "Use the snapshot profile period key, a positive integer."),
							},
						},
						"retention": schema.Int64Attribute{
							MarkdownDescription: "How long to keep the remote copy, in seconds. Required. This does not change retention on the local snapshot profile.",
							Required:            true,
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
						},
						"priority": schema.Int64Attribute{
							MarkdownDescription: "Send order from 0 through 9. Lower numbers go first. Omit to leave the current value unchanged.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								syncPeriodModifier{},
							},
							Validators: []validator.Int64{
								int64validator.Between(0, 9),
							},
						},
						"do_not_expire": schema.BoolAttribute{
							MarkdownDescription: "Keep the source snapshot until it has been sent. Omit to leave the current value unchanged.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.Bool{
								syncPeriodModifier{},
							},
						},
						"destination_prefix": schema.StringAttribute{
							MarkdownDescription: "Prefix added to the snapshot name on the destination. Omit to leave the current value unchanged.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								syncPeriodModifier{},
							},
						},
						"key": schema.StringAttribute{
							MarkdownDescription: "Site sync period key assigned by VergeOS.",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								syncPeriodModifier{},
							},
						},
						"schedule_task": schema.Int64Attribute{
							MarkdownDescription: "Schedule task id VergeOS assigned for this period.",
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								syncPeriodModifier{},
							},
						},
						"task": schema.Int64Attribute{
							MarkdownDescription: "Task id VergeOS assigned for this period.",
							Computed:            true,
							PlanModifiers: []planmodifier.Int64{
								syncPeriodModifier{},
							},
						},
					},
				},
			},
		},
	}
}

func (r *outgoingResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Outgoing sync key, a positive integer.")
}

func (r *outgoingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureAPI(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *outgoingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config outgoingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createOutgoing(ctx, &plan, secret(config.RegistrationCodeWO)); err != nil {
		if outgoingIDSet(&plan) {
			r.rememberOutgoing(ctx, resp, &plan)
		}
		resp.Diagnostics.AddError("Error creating outgoing site sync", err.Error())
		return
	}
	r.rememberOutgoing(ctx, resp, &plan)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, outgoingForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *outgoingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data outgoingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readOutgoing(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading outgoing site sync", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, outgoingForState(&data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *outgoingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state outgoingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateOutgoing(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating outgoing site sync", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, outgoingForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *outgoingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data outgoingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseID(data.ID, "outgoing sync")
	if err != nil {
		resp.Diagnostics.AddError("Error deleting outgoing site sync", err.Error())
		return
	}
	if err := r.api.deleteOutgoing(ctx, id); err != nil {
		resp.Diagnostics.AddError("Error deleting outgoing site sync", err.Error())
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted outgoing site sync %d", id))
}

func (r *outgoingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importPositive(ctx, req, resp, "Invalid outgoing site sync import id", "Import vergeio_site_sync_outgoing with the sync key, a positive integer.")
}

func (r *outgoingResource) rememberOutgoing(ctx context.Context, resp *resource.CreateResponse, data *outgoingModel) {
	if !outgoingIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, outgoingForState(data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func outgoingIDSet(data *outgoingModel) bool {
	return data != nil && !data.ID.IsNull() && !data.ID.IsUnknown() && data.ID.ValueString() != ""
}
