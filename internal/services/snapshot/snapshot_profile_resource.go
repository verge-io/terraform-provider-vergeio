// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &SnapshotProfileResource{}
	_ resource.ResourceWithImportState = &SnapshotProfileResource{}
)

func NewSnapshotProfileResource() resource.Resource {
	return &SnapshotProfileResource{}
}

// SnapshotProfileResource is vergeio_snapshot_profile.
type SnapshotProfileResource struct {
	api *SnapshotProfileApi
}

// SnapshotProfileResourceModel is the Terraform model for vergeio_snapshot_profile.
type SnapshotProfileResourceModel struct {
	Id             types.String  `tfsdk:"id"`
	Name           types.String  `tfsdk:"name"`
	Description    types.String  `tfsdk:"description"`
	IgnoreWarnings types.Bool    `tfsdk:"ignore_warnings"`
	Period         []periodModel `tfsdk:"period"`
}

// periodModel is one schedule inside a snapshot profile.
type periodModel struct {
	Key          types.String `tfsdk:"key"`
	Name         types.String `tfsdk:"name"`
	Frequency    types.String `tfsdk:"frequency"`
	Hour         types.Int32  `tfsdk:"hour"`
	Minute       types.Int32  `tfsdk:"minute"`
	DayOfWeek    types.String `tfsdk:"day_of_week"`
	DayOfMonth   types.Int32  `tfsdk:"day_of_month"`
	Month        types.Int32  `tfsdk:"month"`
	Retention    types.Int64  `tfsdk:"retention"`
	Quiesce      types.Bool   `tfsdk:"quiesce"`
	SkipMissed   types.Bool   `tfsdk:"skip_missed"`
	MaxTier      types.String `tfsdk:"max_tier"`
	MinSnapshots types.Int32  `tfsdk:"min_snapshots"`
	Immutable    types.Bool   `tfsdk:"immutable"`
}

func (r *SnapshotProfileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snapshot_profile"
}

func (r *SnapshotProfileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "VergeOS snapshot profile. Each period block sets the frequency, the time of day, how long the snapshot is kept, and whether the guest is quiesced. Retention is required on every period and has no default. vergeio_vm.snapshot_profile is tonumber of this resource's id. A period left out of the configuration is deleted. Changing a period name replaces that period.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Snapshot profile key assigned by VergeOS. vergeio_vm.snapshot_profile uses this key as a number: tonumber(vergeio_snapshot_profile.example.id).",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Profile name. Must be unique.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Profile description. Omit to leave an existing description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ignore_warnings": schema.BoolAttribute{
				MarkdownDescription: "Suppress warnings about the estimated snapshot count. Omit to leave the current value unchanged. An omitted value is not sent as false.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
		},
		Blocks: map[string]schema.Block{
			"period": schema.ListNestedBlock{
				MarkdownDescription: "When the profile takes a snapshot. name is unique in the profile. hour and minute are the time of day. retention is required, is a number of seconds, and has no default. A period left out of the configuration is deleted. Changing name replaces that period.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							MarkdownDescription: "Period key assigned by VergeOS.",
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								periodRefreshModifier{},
							},
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Period name. Must be unique within the profile. Changing it replaces the period.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
							},
						},
						"frequency": schema.StringAttribute{
							MarkdownDescription: "How often the snapshot runs: custom, hourly, daily, weekly, monthly, or yearly.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.OneOf("custom", "hourly", "daily", "weekly", "monthly", "yearly"),
							},
						},
						"hour":         optionalInt32("Hour of day, 0 through 23. With minute, this is the time of day. Omit to leave the current value unchanged. An omitted value is not sent.", int32validator.Between(0, 23)),
						"minute":       optionalInt32("Minute of the hour, 0 through 59. Omit to leave the current value unchanged. An omitted value is not sent. Set 0 explicitly for the top of the hour.", int32validator.Between(0, 59)),
						"day_of_week":  optionalString("Day of the week for a weekly period: sun, mon, tue, wed, thu, fri, sat, or any. Omit to leave the current value unchanged.", stringvalidator.OneOf("sun", "mon", "tue", "wed", "thu", "fri", "sat", "any")),
						"day_of_month": optionalInt32("Day of the month, 1 through 31. 0 means any day. Omit to leave the current value unchanged.", int32validator.Between(0, 31)),
						"month":        optionalInt32("Month of the year, 1 through 12. 0 means any month. Omit to leave the current value unchanged.", int32validator.Between(0, 12)),
						"retention": schema.Int64Attribute{
							MarkdownDescription: "How long to keep each snapshot, in seconds. Required. There is no default. VergeOS expires the snapshot after this many seconds. A missing retention is not keep-forever; clients that omit it have stored a 24 hour expiry. Zero is rejected.",
							Required:            true,
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
							},
						},
						"quiesce":       optionalBool("Quiesce the guest while the snapshot is taken. Requires a guest agent. Applies to VMs and volumes. Omit to leave the current value unchanged. An omitted value is not sent as false."),
						"skip_missed":   optionalBool("Skip the snapshot when the scheduled time was missed. Omit to leave the current value unchanged. An omitted value is not sent."),
						"max_tier":      optionalString("Highest storage tier that may hold the snapshot, from 1 (no restriction) through 5. Omit to leave the current value unchanged.", stringvalidator.OneOf("1", "2", "3", "4", "5")),
						"min_snapshots": optionalInt32("Minimum number of snapshots to retain, including during an outage longer than retention. Omit to leave the current value unchanged. An omitted value is not sent.", int32validator.AtLeast(0)),
						"immutable":     optionalBool("Lock the snapshot until it is unlocked. Applies to system snapshots. Omit to leave the current value unchanged. An omitted value is not sent."),
					},
				},
			},
		},
	}
}

func optionalInt32(description string, validators ...validator.Int32) schema.Int32Attribute {
	return schema.Int32Attribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Int32{
			periodRefreshModifier{},
		},
		Validators: validators,
	}
}

func optionalString(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			periodRefreshModifier{},
		},
		Validators: validators,
	}
}

func optionalBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			periodRefreshModifier{},
		},
	}
}

func (r *SnapshotProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.api = NewSnapshotProfileApi(client)
}

func (r *SnapshotProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data SnapshotProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createProfile(ctx, &data); err != nil {
		if profileIDSet(&data) {
			r.rememberProfile(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating snapshot profile", err.Error())
		return
	}
	// The profile row exists. Keep its id before the follow-up read so a
	// later error still leaves the profile in state.
	r.rememberProfile(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readProfile(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading snapshot profile", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, profileForState(&data))...)
}

func (r *SnapshotProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data SnapshotProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readProfile(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading snapshot profile", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, profileForState(&data))...)
}

func (r *SnapshotProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SnapshotProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateProfile(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating snapshot profile", err.Error())
		return
	}
	plan.Id = state.Id
	if err := r.api.readProfile(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading snapshot profile", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, profileForState(&plan))...)
}

func (r *SnapshotProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data SnapshotProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteProfile(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting snapshot profile", err.Error())
		return
	}
	tflog.Debug(ctx, "snapshot profile deleted")
}

func (r *SnapshotProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := parseIDText(req.ID, "snapshot profile"); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Snapshot Profile Import ID",
			"Import vergeio_snapshot_profile with the profile key, a positive integer.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// rememberProfile writes the profile id into the create response before the
// follow-up read. Terraform keeps that state when a later step returns an
// error, so the next apply updates the profile instead of creating another
// one with the same name.
func (r *SnapshotProfileResource) rememberProfile(ctx context.Context, resp *resource.CreateResponse, data *SnapshotProfileResourceModel) {
	if !profileIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, profileForState(data))...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored snapshot profile %s in state before read", data.Id.ValueString()))
}

func profileIDSet(data *SnapshotProfileResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && data.Id.ValueString() != ""
}

func profileForState(data *SnapshotProfileResourceModel) SnapshotProfileResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.Name = knownString(data.Name)
	stored.Description = knownString(data.Description)
	stored.IgnoreWarnings = knownBool(data.IgnoreWarnings)
	if data.Period == nil {
		stored.Period = nil
		return stored
	}
	periods := make([]periodModel, len(data.Period))
	for i, period := range data.Period {
		periods[i] = periodForState(period)
	}
	stored.Period = periods
	return stored
}

func periodForState(period periodModel) periodModel {
	return periodModel{
		Key:          knownString(period.Key),
		Name:         knownString(period.Name),
		Frequency:    knownString(period.Frequency),
		Hour:         knownInt32(period.Hour),
		Minute:       knownInt32(period.Minute),
		DayOfWeek:    knownString(period.DayOfWeek),
		DayOfMonth:   knownInt32(period.DayOfMonth),
		Month:        knownInt32(period.Month),
		Retention:    knownInt64(period.Retention),
		Quiesce:      knownBool(period.Quiesce),
		SkipMissed:   knownBool(period.SkipMissed),
		MaxTier:      knownString(period.MaxTier),
		MinSnapshots: knownInt32(period.MinSnapshots),
		Immutable:    knownBool(period.Immutable),
	}
}
