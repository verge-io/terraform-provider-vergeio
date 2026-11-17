// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func NewRecipeCatalogApi(c *vergeio.Client) (*RecipeCatalogApi, error) {
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	return &RecipeCatalogApi{
		name: "Recipe Catalog Api",
		sdk:  sdk,
	}, nil
}

// RecipeCatalogApi reads catalogs and VM recipes.
type RecipeCatalogApi struct {
	name string
	sdk  *vergeos.Client
}

func (api *RecipeCatalogApi) Name() string {
	return api.name
}

func (api *RecipeCatalogApi) readCatalogs(ctx context.Context, data *CatalogsDataSourceModel) error {
	name := trimmedString(data.FilterName)
	var opts []vergeos.ListOption
	if name != "" {
		opts = append(opts, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))))
	}
	rows, err := api.sdk.Catalogs.List(ctx, opts...)
	if err != nil {
		return err
	}
	rows = vergeio.KeepExact(rows, name, func(row vergeos.Catalog) string {
		return row.Name
	})
	catalogs := make([]CatalogModel, 0, len(rows))
	for i := range rows {
		catalogs = append(catalogs, catalogModel(&rows[i]))
	}
	data.Catalogs = catalogs
	tflog.Debug(ctx, fmt.Sprintf("read %d catalogs", len(catalogs)))
	return nil
}

func (api *RecipeCatalogApi) readVMRecipes(ctx context.Context, data *VMRecipesDataSourceModel) error {
	name := trimmedString(data.FilterName)
	catalogID := trimmedString(data.CatalogID)
	catalogName := trimmedString(data.CatalogName)
	if catalogID != "" && catalogName != "" {
		return fmt.Errorf("set catalog_id or catalog_name, not both")
	}
	if catalogName != "" {
		catalog, err := api.sdk.Catalogs.GetByName(ctx, catalogName)
		if err != nil {
			if vergeos.IsNotFoundError(err) {
				data.Recipes = []VMRecipeModel{}
				return nil
			}
			return err
		}
		catalogID = catalogHex(catalog.Key, catalog.ID)
	}

	var rows []vergeos.VMRecipe
	var err error
	if catalogID != "" {
		// ListByCatalog already sets the catalog filter. A second WithFilter
		// replaces it, so the name check stays in KeepExact.
		rows, err = api.sdk.VMRecipes.ListByCatalog(ctx, catalogID)
	} else if name != "" {
		rows, err = api.sdk.VMRecipes.List(ctx, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))))
	} else {
		rows, err = api.sdk.VMRecipes.List(ctx)
	}
	if err != nil {
		return err
	}
	rows = vergeio.KeepExact(rows, name, func(row vergeos.VMRecipe) string {
		return row.Name
	})

	recipes := make([]VMRecipeModel, 0, len(rows))
	for i := range rows {
		questions, err := api.sdk.VMRecipes.Questions(ctx, catalogHex(rows[i].Key, rows[i].ID))
		if err != nil {
			return err
		}
		model, err := vmRecipeModel(&rows[i], questions)
		if err != nil {
			return err
		}
		recipes = append(recipes, model)
	}
	data.Recipes = recipes
	tflog.Debug(ctx, fmt.Sprintf("read %d VM recipes", len(recipes)))
	return nil
}

func catalogModel(row *vergeos.Catalog) CatalogModel {
	repository := types.Int64Null()
	if row.Repository.Int() != 0 {
		repository = types.Int64Value(int64(row.Repository.Int()))
	}
	created := types.Int64Null()
	if row.Created != 0 {
		created = types.Int64Value(row.Created)
	}
	return CatalogModel{
		Id:              types.StringValue(catalogHex(row.Key, row.ID)),
		Name:            types.StringValue(row.Name),
		Description:     types.StringValue(row.Description),
		PublishingScope: types.StringValue(row.PublishingScope),
		Enabled:         types.BoolValue(row.Enabled),
		Repository:      repository,
		RepositoryName:  types.StringValue(row.RepositoryName),
		Created:         created,
	}
}

func vmRecipeModel(row *vergeos.VMRecipe, questions []vergeos.RecipeQuestion) (VMRecipeModel, error) {
	items := make([]RecipeQuestionModel, 0, len(questions))
	for i := range questions {
		item, err := recipeQuestionModel(&questions[i])
		if err != nil {
			return VMRecipeModel{}, err
		}
		items = append(items, item)
	}
	return VMRecipeModel{
		Id:              types.StringValue(catalogHex(row.Key, row.ID)),
		Name:            types.StringValue(row.Name),
		Description:     types.StringValue(row.Description),
		Version:         types.StringValue(row.Version),
		Build:           types.Int64Value(int64(row.Build.Int())),
		CatalogID:       types.StringValue(row.Catalog),
		CatalogName:     types.StringValue(row.CatalogName),
		Downloaded:      types.BoolValue(row.Downloaded),
		UpdateAvailable: types.BoolValue(row.UpdateAvailable),
		Questions:       items,
	}, nil
}

func recipeQuestionModel(question *vergeos.RecipeQuestion) (RecipeQuestionModel, error) {
	choices, err := choiceMap(question.Choices)
	if err != nil {
		return RecipeQuestionModel{}, err
	}
	return RecipeQuestionModel{
		Name:        types.StringValue(question.Name),
		Display:     types.StringValue(question.Display),
		Type:        types.StringValue(question.Type),
		Required:    types.BoolValue(question.Required),
		Enabled:     types.BoolValue(question.Enabled),
		Default:     recipeDefaultString(question.Default),
		Help:        types.StringValue(question.Help),
		Note:        types.StringValue(question.Note),
		Hint:        types.StringValue(question.Hint),
		SectionName: types.StringValue(question.SectionName),
		Min:         recipeBoundValue(question.Min),
		Max:         recipeBoundValue(question.Max),
		DontStore:   types.BoolValue(question.DontStore),
		Choices:     choices,
	}, nil
}

func choiceMap(choices vergeos.RecipeChoices) (types.Map, error) {
	if len(choices) == 0 {
		return types.MapNull(types.StringType), nil
	}
	elems := make(map[string]attr.Value, len(choices))
	for key, value := range choices {
		elems[key] = types.StringValue(value)
	}
	mapped, diags := types.MapValue(types.StringType, elems)
	if diags.HasError() {
		return types.MapNull(types.StringType), fmt.Errorf("choices: %s", diags)
	}
	return mapped, nil
}

func recipeDefaultString(raw json.RawMessage) types.String {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return types.StringNull()
	}
	var decoded string
	if json.Unmarshal(raw, &decoded) == nil {
		return types.StringValue(decoded)
	}
	return types.StringValue(text)
}

func recipeBoundValue(bound vergeos.RecipeBound) types.Int64 {
	if !bound.Set {
		return types.Int64Null()
	}
	return types.Int64Value(bound.N)
}

func catalogHex(key, id string) string {
	if key != "" {
		return key
	}
	return id
}

func trimmedString(value types.String) string {
	if value.IsNull() || value.IsUnknown() {
		return ""
	}
	return strings.TrimSpace(value.ValueString())
}
