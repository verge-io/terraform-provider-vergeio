// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"regexp"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ datasource.DataSource = &VMRecipesDataSource{}

func NewVMRecipesDataSource() datasource.DataSource {
	return &VMRecipesDataSource{}
}

// VMRecipesDataSource is vergeio_vm_recipes.
type VMRecipesDataSource struct {
	api *RecipeCatalogApi
}

// VMRecipeModel is one recipe returned by vergeio_vm_recipes.
type VMRecipeModel struct {
	Id              types.String          `tfsdk:"id"`
	Name            types.String          `tfsdk:"name"`
	Description     types.String          `tfsdk:"description"`
	Version         types.String          `tfsdk:"version"`
	Build           types.Int64           `tfsdk:"build"`
	CatalogID       types.String          `tfsdk:"catalog_id"`
	CatalogName     types.String          `tfsdk:"catalog_name"`
	Downloaded      types.Bool            `tfsdk:"downloaded"`
	UpdateAvailable types.Bool            `tfsdk:"update_available"`
	Questions       []RecipeQuestionModel `tfsdk:"questions"`
}

// RecipeQuestionModel is one question on a VM recipe.
type RecipeQuestionModel struct {
	Name        types.String `tfsdk:"name"`
	Display     types.String `tfsdk:"display"`
	Type        types.String `tfsdk:"type"`
	Required    types.Bool   `tfsdk:"required"`
	Enabled     types.Bool   `tfsdk:"enabled"`
	Default     types.String `tfsdk:"default"`
	Help        types.String `tfsdk:"help"`
	Note        types.String `tfsdk:"note"`
	Hint        types.String `tfsdk:"hint"`
	SectionName types.String `tfsdk:"section_name"`
	Min         types.Int64  `tfsdk:"min"`
	Max         types.Int64  `tfsdk:"max"`
	DontStore   types.Bool   `tfsdk:"dont_store"`
	Choices     types.Map    `tfsdk:"choices"`
}

// VMRecipesDataSourceModel is the Terraform model for vergeio_vm_recipes.
type VMRecipesDataSourceModel struct {
	FilterName  types.String    `tfsdk:"filter_name"`
	CatalogID   types.String    `tfsdk:"catalog_id"`
	CatalogName types.String    `tfsdk:"catalog_name"`
	Recipes     []VMRecipeModel `tfsdk:"recipes"`
}

func (d *VMRecipesDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_recipes"
}

func (d *VMRecipesDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists VergeOS VM recipes and their questions. filter_name keeps an exact recipe name. catalog_id or catalog_name limits the list to one catalog. The same recipe name in two catalogs is two rows. Pass recipes[0].id to vergeio_vm_recipe_instance.recipe_id. A disksize question wants bytes. A bool question accepts true, false, yes, no, on, off, 1, or 0.",
		Attributes: map[string]schema.Attribute{
			"filter_name": schema.StringAttribute{
				MarkdownDescription: "Exact recipe name. Omit to list every recipe in the selected catalog, or every recipe when no catalog is set.",
				Optional:            true,
			},
			"catalog_id": schema.StringAttribute{
				MarkdownDescription: "Catalog key, 40 hexadecimal characters. Cannot be set with catalog_name.",
				Optional:            true,
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						regexp.MustCompile(`(?i)^[0-9a-f]{40}$`),
						"must be a 40-character hexadecimal catalog key",
					),
				},
			},
			"catalog_name": schema.StringAttribute{
				MarkdownDescription: "Exact catalog name. Two catalogs with this name is an error. Cannot be set with catalog_id. A name that matches nothing returns an empty list.",
				Optional:            true,
			},
			"recipes": schema.ListNestedAttribute{
				MarkdownDescription: "VM recipes visible to this provider configuration.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":               schema.StringAttribute{MarkdownDescription: "Recipe key, 40 hexadecimal characters. Use this as vergeio_vm_recipe_instance.recipe_id.", Computed: true},
						"name":             schema.StringAttribute{MarkdownDescription: "Recipe name.", Computed: true},
						"description":      schema.StringAttribute{MarkdownDescription: "Recipe description.", Computed: true},
						"version":          schema.StringAttribute{MarkdownDescription: "Recipe version.", Computed: true},
						"build":            schema.Int64Attribute{MarkdownDescription: "Recipe build number.", Computed: true},
						"catalog_id":       schema.StringAttribute{MarkdownDescription: "Parent catalog key.", Computed: true},
						"catalog_name":     schema.StringAttribute{MarkdownDescription: "Parent catalog name.", Computed: true},
						"downloaded":       schema.BoolAttribute{MarkdownDescription: "Whether the recipe has been downloaded and can be deployed.", Computed: true},
						"update_available": schema.BoolAttribute{MarkdownDescription: "Whether the catalog has a newer build.", Computed: true},
						"questions": schema.ListNestedAttribute{
							MarkdownDescription: "Questions the recipe asks at deploy time, in form order. name is the answers key. type disksize is bytes. type bool accepts true, false, yes, no, on, off, 1, or 0.",
							Computed:            true,
							NestedObject: schema.NestedAttributeObject{
								Attributes: map[string]schema.Attribute{
									"name":         schema.StringAttribute{MarkdownDescription: "Answer key.", Computed: true},
									"display":      schema.StringAttribute{MarkdownDescription: "Label shown on the form.", Computed: true},
									"type":         schema.StringAttribute{MarkdownDescription: "Question type, such as string, bool, num, disksize, network, or list.", Computed: true},
									"required":     schema.BoolAttribute{MarkdownDescription: "Whether the question must be answered when it has no default.", Computed: true},
									"enabled":      schema.BoolAttribute{MarkdownDescription: "Whether the question is on the form.", Computed: true},
									"default":      schema.StringAttribute{MarkdownDescription: "Platform default, when the question publishes one. Null, an empty string, 0, and false count as empty for a required question.", Computed: true},
									"help":         schema.StringAttribute{MarkdownDescription: "Tooltip text.", Computed: true},
									"note":         schema.StringAttribute{MarkdownDescription: "Text shown under the field.", Computed: true},
									"hint":         schema.StringAttribute{MarkdownDescription: "Placeholder text.", Computed: true},
									"section_name": schema.StringAttribute{MarkdownDescription: "Section name. $database marks recipe plumbing rather than an operator question.", Computed: true},
									"min":          schema.Int64Attribute{MarkdownDescription: "Declared minimum. For a number it is a value floor. For a string it is a length floor. Null when the question did not set one. A set zero is a real floor.", Computed: true},
									"max":          schema.Int64Attribute{MarkdownDescription: "Declared maximum. Null when the question did not set one. Zero means there is no upper limit.", Computed: true},
									"dont_store":   schema.BoolAttribute{MarkdownDescription: "Whether VergeOS keeps the answer off the instance row. Passwords are often marked this way and are still sent on deploy.", Computed: true},
									"choices":      schema.MapAttribute{MarkdownDescription: "Inline list choices, keyed by the value answers must use. Null when the question has no inline list.", ElementType: types.StringType, Computed: true},
								},
							},
						},
					},
				},
			},
		},
	}
}

func (d *VMRecipesDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	api, err := NewRecipeCatalogApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	d.api = api
}

func (d *VMRecipesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tflog.Trace(ctx, "Start reading VM recipes data source")
	var data VMRecipesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := d.api.readVMRecipes(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Fetching VM Recipes", err.Error())
		return
	}
	tflog.Trace(ctx, "End reading VM recipes data source")
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
