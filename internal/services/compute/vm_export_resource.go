// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const vmExportDescription = "The VM export configuration for one NAS volume. There is one row per volume. " +
	"quiesced freezes guest filesystems during an export, create_current keeps a current folder for the latest export, and max_exports is how many generations to keep. " +
	"This resource does not start an export. A run is a procedure: use vergeio.vergeos.vm_export in the Ansible collection. " +
	"Destroy removes the configuration and leaves files already written on the volume."

var (
	_ resource.Resource                   = &vmExportResource{}
	_ resource.ResourceWithImportState    = &vmExportResource{}
	_ resource.ResourceWithIdentity       = &vmExportResource{}
	_ resource.ResourceWithValidateConfig = &vmExportResource{}
)

func NewVMExportResource() resource.Resource {
	return &vmExportResource{}
}

type vmExportResource struct {
	api *exportAPI
}

func (r *vmExportResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_export"
}

func (r *vmExportResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: vmExportDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "volume_vm_exports row id.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"volume": schema.StringAttribute{
				MarkdownDescription: "NAS volume key the export files are written to, a 40 character hex string. Changing it replaces the configuration.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"volume_name": schema.StringAttribute{
				MarkdownDescription: "Name of the NAS volume.",
				Computed:            true,
			},
			"quiesced": schema.BoolAttribute{
				MarkdownDescription: "Freeze guest filesystems during an export. Omit to leave the platform default.",
				Optional:            true,
				Computed:            true,
			},
			"create_current": schema.BoolAttribute{
				MarkdownDescription: "Keep a current folder that points at the latest export. Omit to leave the platform default.",
				Optional:            true,
				Computed:            true,
			},
			"max_exports": schema.Int64Attribute{
				MarkdownDescription: "How many export generations to keep, from 1 to 100. Omit to leave the platform default.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.Int64{
					int64validator.Between(1, 100),
				},
			},
			"status": schema.StringAttribute{
				MarkdownDescription: "Export status: idle, building, error, or cleaning.",
				Computed:            true,
			},
			"status_info": schema.StringAttribute{
				MarkdownDescription: "Detail VergeOS reported for status.",
				Computed:            true,
			},
		},
	}
}

func (r *vmExportResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	conn, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	api, err := newExportAPI(conn)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.api = api
}

func (r *vmExportResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data exportModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateExportModel(&data)...)
}

func (r *vmExportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data exportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.create(ctx, &data); err != nil {
		if exportIDSet(&data) {
			r.remember(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating VM export", err.Error())
		return
	}
	r.remember(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmExportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data exportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.read(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VM export", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *vmExportResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state exportModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.update(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating VM export", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *vmExportResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data exportModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := exportID(data.ID)
	if err != nil {
		resp.Diagnostics.AddError("Error deleting VM export", err.Error())
		return
	}
	if err := r.api.delete(ctx, id); err != nil {
		resp.Diagnostics.AddError("Error deleting VM export", err.Error())
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted vm export %d", id))
}

func (r *vmExportResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("volume_vm_exports row id, a positive integer.")
}

func (r *vmExportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if id == "" && req.Identity != nil {
		var got types.String
		resp.Diagnostics.Append(req.Identity.GetAttribute(ctx, path.Root("id"), &got)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !got.IsNull() && !got.IsUnknown() {
			id = strings.TrimSpace(got.ValueString())
		}
	}
	if _, err := positiveDecimal(id); err != nil {
		resp.Diagnostics.AddError(
			"Invalid VM Export Import ID",
			"Import vergeio_vm_export with the volume_vm_exports id, a positive integer.",
		)
		return
	}
	shared.ImportByID(ctx, resource.ImportStateRequest{ID: id, Identity: req.Identity}, resp,
		"Invalid VM Export Import ID",
		"Import vergeio_vm_export with the volume_vm_exports id, a positive integer.",
	)
}

func (r *vmExportResource) remember(ctx context.Context, resp *resource.CreateResponse, data *exportModel) {
	if !exportIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}
