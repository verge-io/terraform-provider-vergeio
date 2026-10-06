// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"regexp"
	"strings"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const volumeDescription = "A volume on a vergeio_nas_service. Set service_id to that service id so Terraform removes this volume before the service. Destroy disables the volume and waits until VergeOS allows the delete. An enabled volume cannot be deleted. If shares still exist and the delete is refused, Terraform removes those shares and tries again. Snapshots of the volume are not managed here. A snapshot that remains can block the delete. remote_target and encrypt are chosen when the volume is created. Changing either one replaces the volume."

var (
	_ resource.Resource                = &volumeResource{}
	_ resource.ResourceWithConfigure   = &volumeResource{}
	_ resource.ResourceWithImportState = &volumeResource{}
	_ resource.ResourceWithIdentity    = &volumeResource{}

	volumeNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	tierValues        = []string{"", "1", "2", "3", "4", "5"}
	optimizeValues    = []string{"", "general", "large"}
	cifsProtocols     = []string{"", "1.0", "2.0", "2.1", "3.0"}
	nfsProtocols      = []string{"", "2", "3", "4"}
)

func NewVolumeResource() resource.Resource {
	return &volumeResource{}
}

// volumeResource is vergeio_nas_volume.
type volumeResource struct {
	api *API
}

type volumeModel struct {
	ID                     types.String `tfsdk:"id"`
	ServiceID              types.String `tfsdk:"service_id"`
	Name                   types.String `tfsdk:"name"`
	Description            types.String `tfsdk:"description"`
	Enabled                types.Bool   `tfsdk:"enabled"`
	MaxSize                types.Int64  `tfsdk:"max_size"`
	PreferredTier          types.String `tfsdk:"preferred_tier"`
	SnapshotProfile        types.Int64  `tfsdk:"snapshot_profile"`
	Discard                types.Bool   `tfsdk:"discard"`
	ReadOnly               types.Bool   `tfsdk:"read_only"`
	Optimize               types.String `tfsdk:"optimize"`
	OwnerUser              types.String `tfsdk:"owner_user"`
	OwnerGroup             types.String `tfsdk:"owner_group"`
	AutomountSnapshots     types.Bool   `tfsdk:"automount_snapshots"`
	Note                   types.String `tfsdk:"note"`
	Encrypt                types.Bool   `tfsdk:"encrypt"`
	EncryptionKeyWO        types.String `tfsdk:"encryption_key_wo"`
	EncryptionKeyWOVersion types.Int64  `tfsdk:"encryption_key_wo_version"`
	RemoteTarget           types.String `tfsdk:"remote_target"`
	CIFSUser               types.String `tfsdk:"cifs_user"`
	CIFSPasswordWO         types.String `tfsdk:"cifs_password_wo"`
	CIFSPasswordWOVersion  types.Int64  `tfsdk:"cifs_password_wo_version"`
	CIFSProtocol           types.String `tfsdk:"cifs_protocol"`
	NFSProtocol            types.String `tfsdk:"nfs_protocol"`
	MountOptions           types.String `tfsdk:"mount_options"`
	ReadAheadKB            types.String `tfsdk:"read_ahead_kb"`
	Created                types.Int64  `tfsdk:"created"`
	Modified               types.Int64  `tfsdk:"modified"`
	Creator                types.String `tfsdk:"creator"`
	Drive                  types.Int64  `tfsdk:"drive"`
	IsSnapshot             types.Bool   `tfsdk:"is_snapshot"`
	FSType                 types.String `tfsdk:"fs_type"`
}

func (r *volumeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nas_volume"
}

func (r *volumeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: volumeDescription,
		Attributes: map[string]schema.Attribute{
			"id": idAttr("Volume id."),
			"service_id": schema.StringAttribute{
				MarkdownDescription: "NAS service id, the same value as vergeio_nas_service.id. Changing it replaces the volume.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Volume name. Letters, digits, underscore, and hyphen, with no spaces.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(volumeNamePattern, "Use letters, digits, underscore, and hyphen, with no spaces."),
				},
			},
			"description": optString("What this volume is for. Omit to leave the current value unchanged."),
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the volume is enabled. Defaults to true. Destroy disables the volume before delete even when this is true.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"max_size":            optInt("Maximum size in bytes. The smallest volume is 1048576 bytes. Omit on create and Terraform sends 1048576. Omit on update to leave the current size unchanged.", int64validator.AtLeast(volumeMinBytes)),
			"preferred_tier":      optString("Preferred storage tier, from `1` through `5`. Omit to leave the current value unchanged.", stringvalidator.OneOf(tierValues...)),
			"snapshot_profile":    optInt("Snapshot profile id. 0 means none. Omit to leave the current value unchanged.", int64validator.AtLeast(0)),
			"discard":             optBool("Enable TRIM on the volume. Omit to leave the current value unchanged."),
			"read_only":           optBool("Keep the volume read only. Omit to leave the current value unchanged."),
			"optimize":            optString("Optimization mode, `general` or `large`. Omit to leave the current value unchanged.", stringvalidator.OneOf(optimizeValues...)),
			"owner_user":          optString("Unix user that owns the volume directory. Omit to leave the current value unchanged."),
			"owner_group":         optString("Unix group that owns the volume directory. Omit to leave the current value unchanged."),
			"automount_snapshots": optBool("Mount snapshots automatically. Omit to leave the current value unchanged."),
			"note":                optString("Free form note. Omit to leave the current value unchanged."),
			"encrypt": schema.BoolAttribute{
				MarkdownDescription: "Encrypt the volume. Chosen only when the volume is created. Changing it replaces the volume.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
			"encryption_key_wo": schema.StringAttribute{
				MarkdownDescription: "Encryption passphrase sent when the volume is created. Terraform does not store it. Changing encryption_key_wo_version replaces the volume. Requires Terraform 1.11 or OpenTofu 1.11.",
				Optional:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.AlsoRequires(path.MatchRoot("encryption_key_wo_version")),
				},
			},
			"encryption_key_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of encryption_key_wo. Increment it to replace the volume with a new passphrase. Terraform stores this number, not the passphrase.",
				Optional:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(path.MatchRoot("encryption_key_wo")),
				},
			},
			"remote_target": schema.StringAttribute{
				MarkdownDescription: "Remote server and path for a CIFS or NFS mount, for example `//server/share`. Chosen only when the volume is created. Changing it replaces the volume.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cifs_user": optString("User name for a remote CIFS volume. Omit to leave the current value unchanged."),
			"cifs_password_wo": schema.StringAttribute{
				MarkdownDescription: "Password for a remote CIFS volume. Sent on create and when cifs_password_wo_version changes. Terraform does not store it. Requires Terraform 1.11 or OpenTofu 1.11.",
				Optional:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.AlsoRequires(path.MatchRoot("cifs_password_wo_version")),
				},
			},
			"cifs_password_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of cifs_password_wo. Increment it to send a new password. Terraform stores this number, not the password.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(path.MatchRoot("cifs_password_wo")),
				},
			},
			"cifs_protocol": optString("SMB protocol version: `1.0`, `2.0`, `2.1`, or `3.0`. Omit to leave the current value unchanged.", stringvalidator.OneOf(cifsProtocols...)),
			"nfs_protocol":  optString("NFS protocol version: `2`, `3`, or `4`. Omit to leave the current value unchanged.", stringvalidator.OneOf(nfsProtocols...)),
			"mount_options": optString("Extra mount options, in the form VergeOS stores them. Omit to leave the current value unchanged."),
			"read_ahead_kb": optString("Read ahead size in KB. One of `0`, `64`, `128`, `256`, `512`, `1024`, `2048`, or `4096`. Omit to leave the current value unchanged.", stringvalidator.OneOf(readAheadValues...)),
			"created":       createdAttr("Time the volume was created, as seconds since the epoch."),
			"modified":      timestampAttr("Last modification time, as seconds since the epoch."),
			"creator":       stableString("User that created the volume."),
			"drive":         stableInt("Machine drive id for this volume."),
			"is_snapshot":   stableBool("Whether this volume is a snapshot."),
			"fs_type":       stableString("Filesystem type VergeOS assigned."),
		},
	}
}

func (r *volumeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configure(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *volumeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config volumeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createVolume(ctx, &plan, volumeSecretsFrom(&config, nil, true)); err != nil {
		resp.Diagnostics.AddError("Error Creating NAS Volume", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *volumeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data volumeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readVolume(ctx, &data); err != nil {
		if missing(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading NAS Volume", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *volumeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config volumeModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateVolume(ctx, &plan, &state, volumeSecretsFrom(&config, &state, false)); err != nil {
		resp.Diagnostics.AddError("Error Updating NAS Volume", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *volumeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data volumeModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteVolume(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Deleting NAS Volume", err.Error())
	}
}

func (r *volumeResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Volume id. Import vergeio_nas_volume with this value.")
}

func (r *volumeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if strings.TrimSpace(req.ID) == "" && req.Identity == nil {
		resp.Diagnostics.AddError("Invalid NAS Volume Import ID", "Import vergeio_nas_volume with the volume id.")
		return
	}
	shared.ImportByID(ctx, req, resp, "Invalid NAS Volume Import ID", "Import vergeio_nas_volume with the volume id.")
}
