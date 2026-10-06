// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"strings"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const cifsDescription = "A CIFS share on a vergeio_nas_volume. Set volume_id to that volume id so Terraform removes this share before the volume. User and host lists use the form VergeOS stores, one entry on each line. If creating the share succeeds and a later read fails, Terraform deletes the share so it cannot block volume destroy."

var (
	_ resource.Resource                = &cifsResource{}
	_ resource.ResourceWithConfigure   = &cifsResource{}
	_ resource.ResourceWithImportState = &cifsResource{}
	_ resource.ResourceWithIdentity    = &cifsResource{}
)

func NewCIFSShareResource() resource.Resource {
	return &cifsResource{}
}

// cifsResource is vergeio_nas_cifs_share.
type cifsResource struct {
	api *API
}

type cifsModel struct {
	ID             types.String `tfsdk:"id"`
	VolumeID       types.String `tfsdk:"volume_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	Enabled        types.Bool   `tfsdk:"enabled"`
	SharePath      types.String `tfsdk:"share_path"`
	Comment        types.String `tfsdk:"comment"`
	ValidUsers     types.String `tfsdk:"valid_users"`
	ValidGroups    types.String `tfsdk:"valid_groups"`
	AdminUsers     types.String `tfsdk:"admin_users"`
	AdminGroups    types.String `tfsdk:"admin_groups"`
	HostAllow      types.String `tfsdk:"host_allow"`
	HostDeny       types.String `tfsdk:"host_deny"`
	ForceUser      types.String `tfsdk:"force_user"`
	ForceGroup     types.String `tfsdk:"force_group"`
	Browseable     types.Bool   `tfsdk:"browseable"`
	ReadOnly       types.Bool   `tfsdk:"read_only"`
	GuestOK        types.Bool   `tfsdk:"guest_ok"`
	GuestOnly      types.Bool   `tfsdk:"guest_only"`
	Advanced       types.String `tfsdk:"advanced"`
	VFSShadowCopy2 types.Bool   `tfsdk:"vfs_shadow_copy2"`
	Created        types.Int64  `tfsdk:"created"`
	Modified       types.Int64  `tfsdk:"modified"`
	Status         types.Int64  `tfsdk:"status"`
}

func (r *cifsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nas_cifs_share"
}

func (r *cifsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: cifsDescription,
		Attributes: map[string]schema.Attribute{
			"id": idAttr("Share id."),
			"volume_id": schema.StringAttribute{
				MarkdownDescription: "Volume id, the same value as vergeio_nas_volume.id. Changing it replaces the share.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Share name, unique on the volume.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description":      optString("What this share is for. Omit to leave the current value unchanged."),
			"enabled":          enabledAttr("Whether the share is enabled. Defaults to true."),
			"share_path":       optString("Path inside the volume. Empty shares the whole volume. Omit to leave the current value unchanged."),
			"comment":          optString("Short comment, up to 64 characters. Omit to leave the current value unchanged.", stringvalidator.LengthAtMost(64)),
			"valid_users":      optString("Users who can connect, one name on each line, in the form VergeOS stores. Omit to leave the current list unchanged."),
			"valid_groups":     optString("Groups who can connect, one name on each line. Omit to leave the current list unchanged."),
			"admin_users":      optString("Users with admin access, one name on each line. Omit to leave the current list unchanged."),
			"admin_groups":     optString("Groups with admin access, one name on each line. Omit to leave the current list unchanged."),
			"host_allow":       optString("Hosts that are allowed, one entry on each line. Omit to leave the current list unchanged."),
			"host_deny":        optString("Hosts that are denied, one entry on each line. Omit to leave the current list unchanged."),
			"force_user":       optString("Run every operation as this user. Omit to leave the current value unchanged."),
			"force_group":      optString("Primary group for every connection. Omit to leave the current value unchanged."),
			"browseable":       optBool("Show the share when a client browses the server. Omit to leave the current value unchanged."),
			"read_only":        optBool("Keep the share read only. Omit to leave the current value unchanged."),
			"guest_ok":         optBool("Allow guest access. Omit to leave the current value unchanged."),
			"guest_only":       optBool("Allow only guest connections. Omit to leave the current value unchanged."),
			"advanced":         optString("Extra smb.conf options for this share. Omit to leave the current value unchanged."),
			"vfs_shadow_copy2": optBool("Expose previous versions from mounted snapshots. Omit to leave the current value unchanged."),
			"created":          createdAttr("Time the share was created, as seconds since the epoch."),
			"modified":         timestampAttr("Last modification time, as seconds since the epoch."),
			"status":           computedInt("Share status id reported by VergeOS."),
		},
	}
}

func enabledAttr(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		Default:             booldefault.StaticBool(true),
	}
}

func (r *cifsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configure(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *cifsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data cifsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createCIFS(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Creating NAS CIFS Share", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *cifsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data cifsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readCIFS(ctx, &data); err != nil {
		if missing(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading NAS CIFS Share", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *cifsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state cifsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateCIFS(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error Updating NAS CIFS Share", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *cifsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data cifsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteCIFSShare(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Deleting NAS CIFS Share", err.Error())
	}
}

func (r *cifsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("CIFS share id. Import vergeio_nas_cifs_share with this value.")
}

func (r *cifsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importObjectID(ctx, req, resp, "Invalid NAS CIFS Share Import ID", "Import vergeio_nas_cifs_share with the share id.")
}

func importObjectID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary, detail string) {
	if strings.TrimSpace(req.ID) == "" && req.Identity == nil {
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	shared.ImportByID(ctx, req, resp, summary, detail)
}
