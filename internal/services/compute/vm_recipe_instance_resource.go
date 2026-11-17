// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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
	_ resource.Resource                = &VMRecipeInstanceResource{}
	_ resource.ResourceWithImportState = &VMRecipeInstanceResource{}
	_ resource.ResourceWithIdentity    = &VMRecipeInstanceResource{}
)

func NewVMRecipeInstanceResource() resource.Resource {
	return &VMRecipeInstanceResource{}
}

// VMRecipeInstanceResource is vergeio_vm_recipe_instance.
type VMRecipeInstanceResource struct {
	api *VMRecipeInstanceApi
}

// VMRecipeInstanceResourceModel is the Terraform model for vergeio_vm_recipe_instance.
type VMRecipeInstanceResourceModel struct {
	Id         types.String         `tfsdk:"id"`
	Name       types.String         `tfsdk:"name"`
	RecipeID   types.String         `tfsdk:"recipe_id"`
	Answers    types.Map            `tfsdk:"answers"`
	AutoUpdate types.Bool           `tfsdk:"auto_update"`
	VMID       types.Int64          `tfsdk:"vm_id"`
	RecipeName types.String         `tfsdk:"recipe_name"`
	Version    types.String         `tfsdk:"version"`
	Build      types.Int64          `tfsdk:"build"`
	Timeouts   *recipeTimeoutsModel `tfsdk:"timeouts"`
}

// recipeTimeoutsModel is the destroy wait. Update has no recipe-instance API.
type recipeTimeoutsModel struct {
	Delete types.String `tfsdk:"delete"`
}

func (r *VMRecipeInstanceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_recipe_instance"
}

func (r *VMRecipeInstanceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Deploys a VM from a catalog recipe. Look the recipe up with vergeio_vm_recipes. answers is a map of question name to string. A bool question accepts true, false, yes, no, on, off, 1, or 0. Any other value is rejected before the request is sent, because VergeOS would store it as false. A disksize answer is bytes. 50 GB is 53687091200. Zero keeps the recipe default. A value above zero and under 1 MB is rejected, because VergeOS would keep the image disk and still report success. A network answer is a network name, a vnet key, or __new_internal__. Changing name, recipe_id, answers, or auto_update replaces the VM. Destroy sends one ACPI poweroff, waits timeouts.delete (default 2 minutes), kills the guest if it is still running, deletes the VM, then deletes the recipe instance. A missing guest does not block that delete. A missing recipe instance still stops and deletes the VM stored in vm_id. This resource always deploys. It has no simulate argument.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Recipe instance key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the VM to create. Changing it replaces the VM.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"recipe_id": schema.StringAttribute{
				MarkdownDescription: "Recipe key, 40 hexadecimal characters. vergeio_vm_recipes returns it as id. Changing it replaces the VM.",
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
				MarkdownDescription: "Answers keyed by question name. Values are strings. A bool question accepts true, false, yes, no, on, off, 1, or 0. A disksize question is a byte count, so 50 GB is 53687091200. A network question is a network name, a vnet key, or __new_internal__. The map is stored and may contain a guest password. It is not read back: VergeOS coerces values and omits some passwords. Changing it replaces the VM.",
				ElementType:         types.StringType,
				Optional:            true,
				Sensitive:           true,
				PlanModifiers: []planmodifier.Map{
					mapplanmodifier.RequiresReplace(),
				},
			},
			"auto_update": schema.BoolAttribute{
				MarkdownDescription: "Update this VM when the recipe is updated. Omit to leave the platform default. Changing it replaces the VM.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.RequiresReplace(),
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"vm_id": schema.Int64Attribute{
				MarkdownDescription: "Key of the VM created from the recipe.",
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
		Blocks: map[string]schema.Block{
			"timeouts": schema.SingleNestedBlock{
				MarkdownDescription: "How long destroy waits for the guest to stop.",
				Attributes: map[string]schema.Attribute{
					"delete": schema.StringAttribute{
						MarkdownDescription: "How long destroy waits after one ACPI poweroff before killing the VM. A duration such as \"90s\" or \"2m\". Defaults to 2m.",
						Optional:            true,
						Validators: []validator.String{
							durationValidator{},
						},
					},
				},
			},
		},
	}
}

func (r *VMRecipeInstanceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	api, err := NewVMRecipeInstanceApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.api = api
}

func (r *VMRecipeInstanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VMRecipeInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deploy(ctx, &data); err != nil {
		if recipeInstanceIDSet(&data) {
			r.rememberInstance(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating VM from recipe", err.Error())
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

func (r *VMRecipeInstanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VMRecipeInstanceResourceModel
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

func (r *VMRecipeInstanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan VMRecipeInstanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.Id)
}

func (r *VMRecipeInstanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VMRecipeInstanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.delete(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting recipe instance", err.Error())
		return
	}
	tflog.Debug(ctx, "recipe instance deleted")
}

func (r *VMRecipeInstanceResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Recipe instance key. Import vergeio_vm_recipe_instance with this value.")
}

func (r *VMRecipeInstanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
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
			"Invalid Recipe Instance Import ID",
			"Import vergeio_vm_recipe_instance with the instance key, a positive integer.",
		)
		return
	}
	shared.ImportByID(ctx, req, resp, "Invalid Recipe Instance Import ID", "Import vergeio_vm_recipe_instance with the instance key, a positive integer.")
}

func (r *VMRecipeInstanceResource) rememberInstance(ctx context.Context, resp *resource.CreateResponse, data *VMRecipeInstanceResourceModel) {
	if !recipeInstanceIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.Id)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored recipe instance %s in state before read", data.Id.ValueString()))
}

func recipeInstanceIDSet(data *VMRecipeInstanceResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && strings.TrimSpace(data.Id.ValueString()) != ""
}
