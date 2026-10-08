// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &TenantRecipeInstanceResource{}
	_ resource.ResourceWithImportState = &TenantRecipeInstanceResource{}
	_ resource.ResourceWithIdentity    = &TenantRecipeInstanceResource{}
)

func NewTenantRecipeInstanceResource() resource.Resource {
	return &TenantRecipeInstanceResource{}
}

// TenantRecipeInstanceResource is vergeio_tenant_recipe_instance.
type TenantRecipeInstanceResource struct {
	api *TenantRecipeAPI
}

// TenantRecipeInstanceResourceModel is the Terraform model for vergeio_tenant_recipe_instance.
type TenantRecipeInstanceResourceModel struct {
	Id         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	RecipeID   types.String `tfsdk:"recipe_id"`
	Answers    types.Map    `tfsdk:"answers"`
	TenantID   types.Int64  `tfsdk:"tenant_id"`
	RecipeName types.String `tfsdk:"recipe_name"`
	Version    types.String `tfsdk:"version"`
	Build      types.Int64  `tfsdk:"build"`
}

func (r *TenantRecipeInstanceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_recipe_instance"
}

func (r *TenantRecipeInstanceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Deploys a tenant from a catalog recipe. Look the recipe up with vergeio_tenant_recipes. answers is a map of question name to string. A bool question accepts true, false, yes, no, on, off, 1, or 0. Any other value is rejected before the request is sent, because VergeOS would store it as false. A disksize answer is bytes. 50 GB is 53687091200. Zero keeps the recipe default. A value above zero and under 1 MB is rejected before the request is sent. A network answer is a network name, a vnet key, or __new_internal__. Changing name, recipe_id, or answers replaces the tenant. Destroy powers the tenant off, waits for the tenant network to stop, deletes the tenant, then deletes the recipe instance. A missing tenant does not block that delete. A missing recipe instance still powers off and deletes the tenant stored in tenant_id. This resource always deploys. It has no simulate argument.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Recipe instance key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the tenant to create. Changing it replaces the tenant.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"recipe_id": schema.StringAttribute{
				MarkdownDescription: "Recipe key, 40 hexadecimal characters. vergeio_tenant_recipes returns it as id. Changing it replaces the tenant.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`(?i)^[0-9a-f]{40}$`),
						"must be a 40-character hexadecimal recipe key",
					),
				},
			},
			"answers": schema.MapAttribute{
				MarkdownDescription: "Answers keyed by question name. Values are strings. A bool question accepts true, false, yes, no, on, off, 1, or 0. A disksize question is a byte count, so 50 GB is 53687091200. A network question is a network name, a vnet key, or __new_internal__. The map is stored and may contain a password. It is not read back: VergeOS coerces values and omits some passwords. Changing it replaces the tenant.",
				ElementType:         types.StringType,
				Optional:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"tenant_id": schema.Int64Attribute{
				MarkdownDescription: "Key of the tenant created from the recipe. vergeio_tenant.id is that key as a string. vergeio_tenant_node and the other tenant resources take the string, so use tostring.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"recipe_name": schema.StringAttribute{
				MarkdownDescription: "Recipe name reported by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"version": schema.StringAttribute{
				MarkdownDescription: "Recipe version at deploy time.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"build": schema.Int64Attribute{
				MarkdownDescription: "Recipe build at deploy time.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *TenantRecipeInstanceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	api, err := NewTenantRecipeAPI(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.api = api
}

func (r *TenantRecipeInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data TenantRecipeInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deploy(ctx, &data); err != nil {
		if tenantRecipeInstanceIDSet(&data) {
			r.rememberInstance(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating tenant from recipe", err.Error())
		return
	}
	r.rememberInstance(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.read(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading recipe instance", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
}

func (r *TenantRecipeInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data TenantRecipeInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.read(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading recipe instance", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
}

func (r *TenantRecipeInstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan TenantRecipeInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.Id)
}

func (r *TenantRecipeInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data TenantRecipeInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.delete(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting recipe instance", err.Error())
		return
	}
	tflog.Debug(ctx, "tenant recipe instance deleted")
}

func (r *TenantRecipeInstanceResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Recipe instance key. Import vergeio_tenant_recipe_instance with this value.")
}

func (r *TenantRecipeInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
	if _, err := parseRecipeInstanceIDText(id); err != nil {
		resp.Diagnostics.AddError(
			"Invalid Tenant Recipe Instance Import ID",
			"Import vergeio_tenant_recipe_instance with the instance key, a positive integer.",
		)
		return
	}
	shared.ImportByID(ctx, req, resp, "Invalid Tenant Recipe Instance Import ID", "Import vergeio_tenant_recipe_instance with the instance key, a positive integer.")
}

func (r *TenantRecipeInstanceResource) rememberInstance(ctx context.Context, resp *resource.CreateResponse, data *TenantRecipeInstanceResourceModel) {
	if !tenantRecipeInstanceIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored tenant recipe instance %s in state before read", data.Id.ValueString()))
}

func tenantRecipeInstanceIDSet(data *TenantRecipeInstanceResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && strings.TrimSpace(data.Id.ValueString()) != ""
}
