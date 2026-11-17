// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                   = &TenantSnapshotResource{}
	_ resource.ResourceWithImportState    = &TenantSnapshotResource{}
	_ resource.ResourceWithValidateConfig = &TenantSnapshotResource{}
)

const tenantSnapshotImportDetail = "Import vergeio_tenant_snapshot with the snapshot key, such as 15, or with tenant_id/name, such as 7/before-change."

func NewTenantSnapshotResource() resource.Resource {
	return &TenantSnapshotResource{}
}

// TenantSnapshotResource is vergeio_tenant_snapshot.
type TenantSnapshotResource struct {
	api *API
}

// TenantSnapshotResourceModel is the Terraform model for vergeio_tenant_snapshot.
type TenantSnapshotResourceModel struct {
	Id           types.String `tfsdk:"id"`
	TenantID     types.String `tfsdk:"tenant_id"`
	Name         types.String `tfsdk:"name"`
	Description  types.String `tfsdk:"description"`
	Type         types.String `tfsdk:"type"`
	Expires      types.Int64  `tfsdk:"expires"`
	NeverExpires types.Bool   `tfsdk:"never_expires"`
	Created      types.Int64  `tfsdk:"created"`
}

func (r *TenantSnapshotResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_snapshot"
}

func (r *TenantSnapshotResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One snapshot of a VergeOS tenant, kept in Terraform state. Create takes the snapshot. description updates in place. expires and never_expires update the expiration in place. Changing tenant_id, name, or type replaces the snapshot, because VergeOS does not allow those to change after creation. The vergeio_tenant_snapshot action takes a snapshot and does not store it. This resource does.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Snapshot key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Key of the parent vergeio_tenant. Changing it replaces the snapshot.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			// UseStateForUnknown before RequiresReplace. An omitted name is unknown on update.
			"name": schema.StringAttribute{
				MarkdownDescription: "Snapshot name. Omit it and VergeOS assigns one. VergeOS does not rename a snapshot, so changing the name replaces it.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Snapshot description. Omit it to leave the current description unchanged. Set it, including to an empty string, to change it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"type": schema.StringAttribute{
				MarkdownDescription: "Snapshot coverage: full, partial_include, or partial_exclude. Omit it and VergeOS uses full. Changing it replaces the snapshot. This is the coverage stored on the snapshot, not the Provider or Local label from the tenant cloud snapshot UI.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(
						vergeos.TenantSnapshotTypeFull,
						vergeos.TenantSnapshotTypePartialInclude,
						vergeos.TenantSnapshotTypePartialExclude,
					),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
					stringplanmodifier.RequiresReplace(),
				},
			},
			"expires": schema.Int64Attribute{
				MarkdownDescription: "Unix timestamp when the snapshot expires. Set this or never_expires, and only one of them.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"never_expires": schema.BoolAttribute{
				MarkdownDescription: "When true, the snapshot does not expire. Set this or expires. Defaults to false, which requires expires.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
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

func (r *TenantSnapshotResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *TenantSnapshotResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data TenantSnapshotResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !data.TenantID.IsUnknown() {
		if _, err := parseID(data.TenantID, "tenant"); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("tenant_id"), "Invalid Tenant ID", err.Error())
		}
	}
	if name := knownString(data.Name); !name.IsNull() && strings.TrimSpace(name.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid Snapshot Name", "Omit name to let VergeOS assign one. A name that is only spaces is not a snapshot name.")
	}
	if data.Expires.IsUnknown() || data.NeverExpires.IsUnknown() {
		return
	}
	never := boolKnownTrue(data.NeverExpires)
	hasExpires := !data.Expires.IsNull()
	if never && hasExpires {
		resp.Diagnostics.AddError("Invalid Snapshot Expiration", "Set expires to a Unix timestamp, or set never_expires to true, and only one of them.")
		return
	}
	if !never && !hasExpires {
		resp.Diagnostics.AddError("Invalid Snapshot Expiration", "Set expires to a Unix timestamp, or set never_expires to true.")
	}
}

func (r *TenantSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createTenantSnapshot(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error creating tenant snapshot", err.Error())
		return
	}
	if err := r.api.readTenantSnapshot(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading tenant snapshot", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenantSnapshot(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant snapshot", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state TenantSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateTenantSnapshot(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating tenant snapshot", err.Error())
		return
	}
	if err := r.api.readTenantSnapshot(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading tenant snapshot", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *TenantSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteTenantSnapshot(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tenant snapshot", err.Error())
		return
	}
}

func (r *TenantSnapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	key, tenantID, name, err := parseTenantSnapshotImport(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Tenant Snapshot Import ID", tenantSnapshotImportDetail)
		return
	}
	if key == 0 {
		if r.api == nil || r.api.sdk == nil {
			resp.Diagnostics.AddError("VergeOS Client Missing", "The provider has not been configured.")
			return
		}
		snap, err := r.api.sdk.TenantSnapshots.GetByName(ctx, tenantID, name)
		if err != nil {
			resp.Diagnostics.AddError("Error importing tenant snapshot", err.Error())
			return
		}
		key = snap.Key.Int()
		if key <= 0 {
			resp.Diagnostics.AddError("Error importing tenant snapshot", "VergeOS did not return a snapshot key.")
			return
		}
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), strconv.Itoa(key))...)
}

func parseTenantSnapshotImport(id string) (key int, tenantID int, name string, err error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, 0, "", fmt.Errorf("empty import id")
	}
	if !strings.Contains(id, "/") {
		n, convErr := strconv.Atoi(id)
		if convErr != nil || n <= 0 {
			return 0, 0, "", fmt.Errorf("snapshot key")
		}
		return n, 0, "", nil
	}
	tenantText, snapshotName, _ := strings.Cut(id, "/")
	n, convErr := strconv.Atoi(strings.TrimSpace(tenantText))
	snapshotName = strings.TrimSpace(snapshotName)
	if convErr != nil || n <= 0 || snapshotName == "" {
		return 0, 0, "", fmt.Errorf("tenant and name")
	}
	return 0, n, snapshotName, nil
}
