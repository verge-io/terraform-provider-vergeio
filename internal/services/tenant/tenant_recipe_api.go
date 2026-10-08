// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func NewTenantRecipeAPI(c *vergeio.Client) (*TenantRecipeAPI, error) {
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	return &TenantRecipeAPI{
		name:    "Tenant Recipe Api",
		http:    c,
		sdk:     sdk,
		tenants: &API{name: "Tenant Api", sdk: sdk},
	}, nil
}

// TenantRecipeAPI lists tenant recipes and deploys a tenant from one.
type TenantRecipeAPI struct {
	name    string
	http    *vergeio.Client
	sdk     *vergeos.Client
	tenants *API
}

func (api *TenantRecipeAPI) Name() string {
	return api.name
}

func (api *TenantRecipeAPI) readTenantRecipes(ctx context.Context, data *TenantRecipesDataSourceModel) error {
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
				data.Recipes = []TenantRecipeModel{}
				return nil
			}
			return err
		}
		catalogID = catalogHex(catalog.Key, catalog.ID)
	}

	var rows []vergeos.TenantRecipe
	var err error
	if catalogID != "" {
		// ListByCatalog already sets the catalog filter. A second WithFilter
		// replaces it, so the name check stays in KeepExact.
		rows, err = api.sdk.TenantRecipes.ListByCatalog(ctx, catalogID)
	} else if name != "" {
		rows, err = api.sdk.TenantRecipes.List(ctx, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))))
	} else {
		rows, err = api.sdk.TenantRecipes.List(ctx)
	}
	if err != nil {
		return err
	}
	rows = vergeio.KeepExact(rows, name, func(row vergeos.TenantRecipe) string {
		return row.Name
	})

	recipes := make([]TenantRecipeModel, 0, len(rows))
	for i := range rows {
		key := catalogHex(rows[i].Key, rows[i].ID)
		questions, err := api.sdk.TenantRecipes.Questions(ctx, key)
		if err != nil {
			return err
		}
		model, err := tenantRecipeModel(&rows[i], questions)
		if err != nil {
			return err
		}
		recipes = append(recipes, model)
	}
	data.Recipes = recipes
	tflog.Debug(ctx, fmt.Sprintf("read %d tenant recipes", len(recipes)))
	return nil
}

// deploy creates the tenant. It calls Deploy. The request has no simulate flag.
func (api *TenantRecipeAPI) deploy(ctx context.Context, data *TenantRecipeInstanceResourceModel) error {
	req, err := tenantRecipeDeployRequest(ctx, data)
	if err != nil {
		return err
	}
	instance, err := api.sdk.TenantRecipeInstances.Deploy(ctx, req)
	if err != nil {
		if vergeos.IsValidationError(err) {
			return err
		}
		found, getErr := api.sdk.TenantRecipeInstances.GetByName(ctx, data.Name.ValueString())
		if getErr != nil || found.Recipe != data.RecipeID.ValueString() {
			return err
		}
		applyTenantRecipeInstance(data, found)
		return err
	}
	applyTenantRecipeInstance(data, instance)
	tflog.Debug(ctx, fmt.Sprintf("deployed tenant recipe instance %s tenant %s", data.Id.ValueString(), data.TenantID.String()))
	return nil
}

// waitForLinkedTenant polls until the instance row names a tenant that Get can read.
func (api *TenantRecipeAPI) waitForLinkedTenant(ctx context.Context, instanceID int) error {
	deadline := time.Now().Add(tenantPowerTimeout)
	var pending string
	for {
		instance, err := api.sdk.TenantRecipeInstances.Get(ctx, instanceID)
		if err != nil {
			return err
		}
		tenantID := 0
		if instance != nil {
			tenantID = instance.Tenant.Int()
		}
		if tenantID > 0 {
			if _, err := api.sdk.Tenants.Get(ctx, tenantID); err != nil {
				if !vergeos.IsNotFoundError(err) {
					return err
				}
				pending = fmt.Sprintf("tenant %d is not visible yet", tenantID)
			} else {
				return nil
			}
		} else {
			pending = "the instance has no tenant yet"
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timed out waiting for recipe instance %d to link a visible tenant: %s", instanceID, pending)
		}
		if err := sleepPower(ctx); err != nil {
			return err
		}
	}
}

// read refreshes one instance. knownTenant is the tenant id already stored in
// state. The instance row is deleted only when that same id returns 404.
func (api *TenantRecipeAPI) read(ctx context.Context, data *TenantRecipeInstanceResourceModel, knownTenant int) error {
	id, err := parseRecipeInstanceID(data.Id)
	if err != nil {
		return err
	}
	instance, err := api.sdk.TenantRecipeInstances.Get(ctx, id)
	if err != nil {
		return err
	}
	if instance == nil {
		return &vergeos.NotFoundError{Resource: "TenantRecipeInstance", ID: id}
	}
	tenantID := instance.Tenant.Int()
	if tenantID > 0 {
		if _, err := api.sdk.Tenants.Get(ctx, tenantID); err != nil {
			if !vergeos.IsNotFoundError(err) {
				return err
			}
			if knownTenant > 0 && knownTenant == tenantID {
				if delErr := api.http.DeleteTenantRecipeInstance(ctx, id); delErr != nil {
					return delErr
				}
				return err
			}
			return fmt.Errorf("tenant %d for recipe instance %d is not visible yet", tenantID, id)
		}
	}
	answers := data.Answers
	applyTenantRecipeInstance(data, instance)
	data.Answers = answers
	if tenantID <= 0 && knownTenant > 0 {
		data.TenantID = types.Int64Value(int64(knownTenant))
	}
	tflog.Debug(ctx, fmt.Sprintf("read tenant recipe instance %d", id))
	return nil
}

func (api *TenantRecipeAPI) delete(ctx context.Context, data *TenantRecipeInstanceResourceModel) error {
	id, err := parseRecipeInstanceID(data.Id)
	if err != nil {
		return err
	}
	instance, err := api.sdk.TenantRecipeInstances.Get(ctx, id)
	rowMissing := false
	if err != nil {
		if !vergeos.IsNotFoundError(err) {
			return err
		}
		rowMissing = true
		instance = nil
	}
	tenantID := 0
	if instance != nil {
		tenantID = instance.Tenant.Int()
	}
	if tenantID <= 0 && rowMissing {
		tenantID = recipeStateTenantID(data)
	}
	if tenantID > 0 {
		if err := api.deleteRecipeTenant(ctx, tenantID); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("deleted tenant %d for recipe instance %d", tenantID, id))
	}
	if err := api.http.DeleteTenantRecipeInstance(ctx, id); err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant recipe instance %d", id))
	return nil
}

// deleteRecipeTenant removes the copy a recipe deploy stamps out.
// That copy has its own nodes and storage. Tenants.Delete returns 405
// while those rows still exist, so they go first.
func (api *TenantRecipeAPI) deleteRecipeTenant(ctx context.Context, tenantID int) error {
	if err := api.deleteStampedSnapshots(ctx, tenantID); err != nil {
		return err
	}
	if err := api.deleteStampedNodes(ctx, tenantID); err != nil {
		return err
	}
	if err := api.deleteStampedStorage(ctx, tenantID); err != nil {
		return err
	}
	return api.tenants.deleteTenant(ctx, &TenantResourceModel{Id: idString(tenantID)})
}

func recipeListGone(err error) bool {
	if err == nil {
		return false
	}
	if vergeos.IsNotFoundError(err) {
		return true
	}
	var apiErr *vergeos.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

func (api *TenantRecipeAPI) deleteStampedSnapshots(ctx context.Context, tenantID int) error {
	snaps, err := api.sdk.TenantSnapshots.ListByTenant(ctx, tenantID)
	if recipeListGone(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for i := range snaps {
		id := snaps[i].Key.Int()
		if id <= 0 {
			continue
		}
		if err := api.sdk.TenantSnapshots.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("delete tenant snapshot %d: %w", id, err)
		}
	}
	return nil
}

func (api *TenantRecipeAPI) deleteStampedNodes(ctx context.Context, tenantID int) error {
	nodes, err := api.sdk.TenantNodes.ListByTenant(ctx, tenantID)
	if recipeListGone(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for i := range nodes {
		id := nodes[i].Key.Int()
		if id <= 0 {
			continue
		}
		if err := api.tenants.deleteTenantNode(ctx, &TenantNodeResourceModel{Id: idString(id)}); err != nil {
			return err
		}
	}
	return nil
}

func (api *TenantRecipeAPI) deleteStampedStorage(ctx context.Context, tenantID int) error {
	rows, err := api.sdk.TenantStorage.ListByTenant(ctx, tenantID)
	if recipeListGone(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for i := range rows {
		id := rows[i].Key.Int()
		if id <= 0 {
			continue
		}
		if err := api.sdk.TenantStorage.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("delete tenant storage %d: %w", id, err)
		}
	}
	return nil
}

func tenantRecipeDeployRequest(ctx context.Context, data *TenantRecipeInstanceResourceModel) (*vergeos.TenantRecipeDeployRequest, error) {
	if data == nil {
		return nil, fmt.Errorf("recipe instance is missing")
	}
	recipe := strings.TrimSpace(data.RecipeID.ValueString())
	name := strings.TrimSpace(data.Name.ValueString())
	if recipe == "" {
		return nil, fmt.Errorf("recipe_id is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	answers, err := tenantRecipeAnswerMap(ctx, data.Answers)
	if err != nil {
		return nil, err
	}
	return &vergeos.TenantRecipeDeployRequest{
		Recipe:  recipe,
		Name:    name,
		Answers: answers,
	}, nil
}

func tenantRecipeAnswerMap(ctx context.Context, answers types.Map) (vergeos.RecipeAnswers, error) {
	if answers.IsNull() || answers.IsUnknown() {
		return nil, nil
	}
	raw := map[string]string{}
	if diags := answers.ElementsAs(ctx, &raw, false); diags.HasError() {
		return nil, fmt.Errorf("answers: %s", diags)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(vergeos.RecipeAnswers, len(raw))
	for key, value := range raw {
		out[key] = value
	}
	return out, nil
}

func applyTenantRecipeInstance(data *TenantRecipeInstanceResourceModel, instance *vergeos.TenantRecipeInstance) {
	if data == nil || instance == nil {
		return
	}
	if key := instance.Key.Int(); key > 0 {
		data.Id = types.StringValue(fmt.Sprintf("%d", key))
	}
	if instance.Name != "" {
		data.Name = types.StringValue(instance.Name)
	}
	if instance.Recipe != "" {
		data.RecipeID = types.StringValue(instance.Recipe)
	}
	data.TenantID = types.Int64Value(int64(instance.Tenant.Int()))
	data.RecipeName = types.StringValue(instance.RecipeName)
	data.Version = types.StringValue(instance.Version)
	data.Build = types.Int64Value(int64(instance.Build.Int()))
}

func recipeStateTenantID(data *TenantRecipeInstanceResourceModel) int {
	if data == nil {
		return 0
	}
	id := vergeio.KnownInt64(data.TenantID)
	if id == nil || *id <= 0 {
		return 0
	}
	return int(*id)
}

func parseRecipeInstanceID(id types.String) (int, error) {
	return parseID(id, "recipe instance")
}

func parseRecipeInstanceIDText(id string) (int, error) {
	return parseID(types.StringValue(strings.TrimSpace(id)), "recipe instance")
}

func tenantRecipeModel(row *vergeos.TenantRecipe, questions []vergeos.RecipeQuestion) (TenantRecipeModel, error) {
	items := make([]TenantRecipeQuestionModel, 0, len(questions))
	for i := range questions {
		item, err := tenantRecipeQuestionModel(&questions[i])
		if err != nil {
			return TenantRecipeModel{}, err
		}
		items = append(items, item)
	}
	return TenantRecipeModel{
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

func tenantRecipeQuestionModel(question *vergeos.RecipeQuestion) (TenantRecipeQuestionModel, error) {
	choices, err := tenantRecipeChoiceMap(question.Choices)
	if err != nil {
		return TenantRecipeQuestionModel{}, err
	}
	return TenantRecipeQuestionModel{
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

func tenantRecipeChoiceMap(choices vergeos.RecipeChoices) (types.Map, error) {
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
