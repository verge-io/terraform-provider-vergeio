// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const nfsDescription = "An NFS share on a vergeio_nas_volume. Set volume_id to that volume id so Terraform removes this share before the volume. allowed_hosts is a comma separated list. If creating the share succeeds and a later read fails, Terraform deletes the share so it cannot block volume destroy."

var (
	_ resource.Resource                = &nfsResource{}
	_ resource.ResourceWithConfigure   = &nfsResource{}
	_ resource.ResourceWithImportState = &nfsResource{}
	_ resource.ResourceWithIdentity    = &nfsResource{}

	squashValues = []string{"", "root_squash", "all_squash", "no_root_squash"}
	accessValues = []string{"", "ro", "rw"}
)

func NewNFSShareResource() resource.Resource {
	return &nfsResource{}
}

// nfsResource is vergeio_nas_nfs_share.
type nfsResource struct {
	api *API
}

type nfsModel struct {
	ID           types.String `tfsdk:"id"`
	VolumeID     types.String `tfsdk:"volume_id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Enabled      types.Bool   `tfsdk:"enabled"`
	SharePath    types.String `tfsdk:"share_path"`
	AllowedHosts types.String `tfsdk:"allowed_hosts"`
	AllowAll     types.Bool   `tfsdk:"allow_all"`
	FSID         types.String `tfsdk:"fsid"`
	AnonUID      types.String `tfsdk:"anonuid"`
	AnonGID      types.String `tfsdk:"anongid"`
	NoACL        types.Bool   `tfsdk:"no_acl"`
	Insecure     types.Bool   `tfsdk:"insecure"`
	Async        types.Bool   `tfsdk:"async"`
	Squash       types.String `tfsdk:"squash"`
	DataAccess   types.String `tfsdk:"data_access"`
	Created      types.Int64  `tfsdk:"created"`
	Modified     types.Int64  `tfsdk:"modified"`
	Status       types.Int64  `tfsdk:"status"`
}

func (r *nfsResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nas_nfs_share"
}

func (r *nfsResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: nfsDescription,
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
			"description":   optString("What this share is for. Omit to leave the current value unchanged."),
			"enabled":       enabledAttr("Whether the share is enabled. Defaults to true."),
			"share_path":    optString("Path inside the volume. Empty shares the whole volume. Omit to leave the current value unchanged."),
			"allowed_hosts": optString("Hosts and networks that can mount the share, separated by commas. Omit to leave the current list unchanged."),
			"allow_all":     optBool("Let every host mount the share. Omit to leave the current value unchanged."),
			"fsid":          optString("Filesystem id. It must be unique. A number, `root`, or `uuid`. Omit to leave the current value unchanged."),
			"anonuid":       optString("Anonymous user id. Omit to leave the current value unchanged."),
			"anongid":       optString("Anonymous group id. Omit to leave the current value unchanged."),
			"no_acl":        optBool("Turn access control lists off. Omit to leave the current value unchanged."),
			"insecure":      optBool("Allow mount requests from ports above 1024. Omit to leave the current value unchanged."),
			"async":         optBool("Reply before data is on disk. Omit to leave the current value unchanged."),
			"squash":        optString("How user ids are mapped: `root_squash`, `all_squash`, or `no_root_squash`. Omit to leave the current value unchanged.", stringvalidator.OneOf(squashValues...)),
			"data_access":   optString("Read and write access: `ro` or `rw`. Omit to leave the current value unchanged.", stringvalidator.OneOf(accessValues...)),
			"created":       createdAttr("Time the share was created, as seconds since the epoch."),
			"modified":      timestampAttr("Last modification time, as seconds since the epoch."),
			"status":        computedInt("Share status id reported by VergeOS."),
		},
	}
}

func (r *nfsResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configure(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *nfsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data nfsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createNFS(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Creating NAS NFS Share", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *nfsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data nfsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readNFS(ctx, &data); err != nil {
		if missing(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading NAS NFS Share", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *nfsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nfsModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateNFS(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error Updating NAS NFS Share", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *nfsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data nfsModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteNFSShare(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Deleting NAS NFS Share", err.Error())
	}
}

func (r *nfsResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("NFS share id. Import vergeio_nas_nfs_share with this value.")
}

func (r *nfsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importObjectID(ctx, req, resp, "Invalid NAS NFS Share Import ID", "Import vergeio_nas_nfs_share with the share id.")
}
