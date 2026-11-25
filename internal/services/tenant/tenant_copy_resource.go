// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                   = &TenantCopyResource{}
	_ resource.ResourceWithImportState    = &TenantCopyResource{}
	_ resource.ResourceWithValidateConfig = &TenantCopyResource{}
)

const tenantCopyDescription = "A copy of a VergeOS tenant's current state, kept in state. Destroy deletes the copy. The source tenant is only read. The pinned govergeos client clones a tenant with Tenants.Clone. That call takes name, no_vnet, no_storage, and no_nodes. It has no snapshot argument, so this resource copies the tenant as it is now, not a snapshot. tenant_actions restore sends params.snapshot and overwrites the source tenant. This resource does not call it. A copy from a system snapshot needs cloud_snapshot_tenant_actions action recover. The pinned client does not expose that call. isolate defaults to true and posts isolateon on the copy. It does not post isolateoff. An apply that is interrupted after VergeOS stores the copy keeps that key in state. A later destroy removes it instead of leaving a second tenant with the same name. created is the time VergeOS stored the copy. Refresh drops this resource when the key's name or created time differs. Destroy without a refresh leaves that other tenant."

func NewTenantCopyResource() resource.Resource {
	return &TenantCopyResource{}
}

// TenantCopyResource is vergeio_tenant_copy.
type TenantCopyResource struct {
	api *API
}

// TenantCopyResourceModel is vergeio_tenant_copy.
type TenantCopyResourceModel struct {
	Id       types.String `tfsdk:"id"`
	TenantID types.String `tfsdk:"tenant_id"`
	Name     types.String `tfsdk:"name"`
	Isolate  types.Bool   `tfsdk:"isolate"`
	Created  types.Int64  `tfsdk:"created"`
}

func (r *TenantCopyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_copy"
}

func (r *TenantCopyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: tenantCopyDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Tenant key of the copy.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Source tenant key, the same value as vergeio_tenant.id. Changing it replaces the copy. This tenant is not modified.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the copy. Changing it replaces the copy.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"isolate": schema.BoolAttribute{
				MarkdownDescription: "Post isolateon on the copy when it is not already isolated. Defaults to true. false leaves the connectivity the clone copied, including isolation copied from the source. This resource does not post isolateoff, and refresh does not replace this value with the tenant isolate field.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
					boolplanmodifier.RequiresReplace(),
				},
			},
			"created": schema.Int64Attribute{
				MarkdownDescription: "Unix time when VergeOS created the copy. Distinct from id, which VergeOS can reuse. A different name or created time means the key now points at another tenant, and refresh drops this resource. Destroy without a refresh leaves the other tenant. Import leaves this empty, and the first read stores it.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *TenantCopyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.api = api
}

func (r *TenantCopyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data TenantCopyResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !data.TenantID.IsUnknown() {
		if _, err := parseID(data.TenantID, "tenant"); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("tenant_id"), "Invalid Tenant ID", err.Error())
		}
	}
	if !data.Name.IsUnknown() && strings.TrimSpace(data.Name.ValueString()) == "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid Copy Name", "name is required")
	}
}

func (r *TenantCopyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantCopyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.api.createTenantCopy(ctx, &data)
	if tenantCopyIDSet(&data) {
		r.keepTenantCopy(ctx, resp, &data)
	}
	if err != nil {
		if tenantCopyIDSet(&data) && tenantCopyStateID(ctx, resp) == "" {
			resp.Diagnostics.AddError("Error saving tenant copy state", tenantCopyNotInState(&data))
		}
		resp.Diagnostics.AddError("Error creating tenant copy", err.Error())
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
}

func tenantCopyIDSet(data *TenantCopyResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && strings.TrimSpace(data.Id.ValueString()) != ""
}

// keepTenantCopy writes the copy once its key is known. Terraform keeps that
// state when create also returns an error, so the next apply destroys this
// copy instead of posting the same name again.
func (r *TenantCopyResource) keepTenantCopy(ctx context.Context, resp *resource.CreateResponse, data *TenantCopyResourceModel) {
	if resp == nil || !tenantCopyIDSet(data) {
		return
	}
	stored := tenantCopyForState(data)
	resp.Diagnostics.Append(resp.State.Set(shared.WritableContext(ctx), &stored)...)
}

func tenantCopyForState(data *TenantCopyResourceModel) TenantCopyResourceModel {
	stored := *data
	stored.Id = tenantCopyKnownString(data.Id)
	stored.TenantID = tenantCopyKnownString(data.TenantID)
	stored.Name = tenantCopyKnownString(data.Name)
	stored.Isolate = tenantCopyKnownBool(data.Isolate)
	stored.Created = tenantCopyKnownInt64(data.Created)
	return stored
}

func tenantCopyKnownString(v types.String) types.String {
	if v.IsNull() || v.IsUnknown() {
		return types.StringNull()
	}
	return v
}

func tenantCopyKnownBool(v types.Bool) types.Bool {
	if v.IsNull() || v.IsUnknown() {
		return types.BoolNull()
	}
	return v
}

func tenantCopyKnownInt64(v types.Int64) types.Int64 {
	if v.IsNull() || v.IsUnknown() {
		return types.Int64Null()
	}
	return v
}

func tenantCopyNotInState(data *TenantCopyResourceModel) string {
	name := ""
	id := ""
	if data != nil {
		name = data.Name.ValueString()
		id = data.Id.ValueString()
	}
	return fmt.Sprintf("Tenant copy %q (id %s) exists in VergeOS but could not be stored in Terraform state. Import it with `terraform import vergeio_tenant_copy.<name> %s`, or delete it in VergeOS and apply again.", name, id, id)
}

func tenantCopyStateID(ctx context.Context, resp *resource.CreateResponse) string {
	ctx = shared.WritableContext(ctx)
	if resp == nil || resp.State.Raw.Type() == nil || resp.State.Raw.IsNull() {
		return ""
	}
	var got TenantCopyResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		return ""
	}
	if !tenantCopyIDSet(&got) {
		return ""
	}
	return got.Id.ValueString()
}

func (r *TenantCopyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantCopyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readTenantCopy(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading tenant copy", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *TenantCopyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("Tenant Copy Cannot Be Updated", "Changing tenant_id, name, or isolate replaces the copy.")
}

func (r *TenantCopyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantCopyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteTenantCopy(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting tenant copy", err.Error())
	}
}

func (r *TenantCopyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	sourceID, copyID, err := tenantCopyImportID(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Tenant Copy Import ID", err.Error())
		return
	}
	if sourceID != "" {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("tenant_id"), sourceID)...)
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), copyID)...)
	// The schema default. Refresh keeps this value. It is not the tenant isolate field.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("isolate"), true)...)
}

// tenantCopyImportID accepts the copy key, or source/copy so refresh has
// the source tenant id read compares against.
func tenantCopyImportID(raw string) (string, string, error) {
	text := strings.TrimSpace(raw)
	parts := strings.Split(text, "/")
	switch len(parts) {
	case 1:
		if _, err := parseID(types.StringValue(parts[0]), "tenant"); err != nil {
			return "", "", fmt.Errorf("import id must be the copy key, or source/copy")
		}
		return "", parts[0], nil
	case 2:
		if _, err := parseID(types.StringValue(parts[0]), "tenant"); err != nil {
			return "", "", fmt.Errorf("import id must be the copy key, or source/copy")
		}
		if _, err := parseID(types.StringValue(parts[1]), "tenant"); err != nil {
			return "", "", fmt.Errorf("import id must be the copy key, or source/copy")
		}
		return parts[0], parts[1], nil
	default:
		return "", "", fmt.Errorf("import id must be the copy key, or source/copy")
	}
}
