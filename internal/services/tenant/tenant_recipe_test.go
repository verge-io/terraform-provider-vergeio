// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

const (
	testTenantRecipeKey  = "dddddddddddddddddddddddddddddddddddddddd"
	testTenantCatalogKey = "cccccccccccccccccccccccccccccccccccccccc"
)

func TestTenantRecipesDataSourceSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewTenantRecipesDataSource().Schema(t.Context(), datasource.SchemaRequest{}, resp)
	for _, name := range []string{"filter_name", "catalog_id", "catalog_name"} {
		attr := resp.Schema.Attributes[name]
		if attr == nil || !attr.IsOptional() {
			t.Fatalf("%s should be optional", name)
		}
	}
	if resp.Schema.Attributes["recipes"] == nil || !resp.Schema.Attributes["recipes"].IsComputed() {
		t.Fatal("recipes should be computed")
	}
	if !strings.Contains(resp.Schema.MarkdownDescription, "disksize") || !strings.Contains(resp.Schema.MarkdownDescription, "bool") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestTenantRecipesDataSourceReadsQuestions(t *testing.T) {
	fake := newTenantRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	source := configuredTenantRecipes(t, server.URL)
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	configState := tfsdk.State{Schema: schemaResp.Schema}
	diags := configState.Set(ctx, &TenantRecipesDataSourceModel{
		FilterName: types.StringValue("Trial Tenant"),
		CatalogID:  types.StringValue(testTenantCatalogKey),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	source.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TenantRecipesDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if len(got.Recipes) != 1 {
		t.Fatalf("recipes = %+v", got.Recipes)
	}
	recipe := got.Recipes[0]
	if recipe.Id.ValueString() != testTenantRecipeKey || recipe.CatalogID.ValueString() != testTenantCatalogKey || !recipe.Downloaded.ValueBool() {
		t.Fatalf("recipe = %+v", recipe)
	}
	if recipe.Version.ValueString() != "1.0.0" || recipe.Build.ValueInt64() != 4 {
		t.Fatalf("recipe version = %+v", recipe)
	}
	if len(recipe.Questions) != 5 {
		t.Fatalf("questions = %+v", recipe.Questions)
	}
	disk := recipe.Questions[2]
	if disk.Name.ValueString() != "YB_DRIVE_OS_SIZE" || disk.Type.ValueString() != "disksize" || !disk.Required.ValueBool() {
		t.Fatalf("disk question = %+v", disk)
	}
	if disk.Min.ValueInt64() != 0 || !disk.Max.IsNull() {
		t.Fatalf("bounds min=%v max null=%v", disk.Min, disk.Max.IsNull())
	}
	choices := map[string]string{}
	if diags := recipe.Questions[4].Choices.ElementsAs(ctx, &choices, false); diags.HasError() {
		t.Fatal(diags)
	}
	if choices["1"] != "One" || recipe.Questions[4].Type.ValueString() != "list" {
		t.Fatalf("choices = %#v question = %+v", choices, recipe.Questions[4])
	}
	if fake.questionCalls != 1 {
		t.Fatalf("question calls = %d", fake.questionCalls)
	}
}

func TestTenantRecipesDataSourceRejectsBothCatalogFilters(t *testing.T) {
	fake := newTenantRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	source := configuredTenantRecipes(t, server.URL)
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	configState := tfsdk.State{Schema: schemaResp.Schema}
	diags := configState.Set(ctx, &TenantRecipesDataSourceModel{
		CatalogID:   types.StringValue(testTenantCatalogKey),
		CatalogName: types.StringValue("Tenants"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	source.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema}}, resp)
	if !resp.Diagnostics.HasError() || !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "not both") {
		t.Fatalf("diagnostics = %v", resp.Diagnostics)
	}
}

func TestTenantRecipesDataSourceMissingCatalogNameIsEmpty(t *testing.T) {
	fake := newTenantRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	source := configuredTenantRecipes(t, server.URL)
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	configState := tfsdk.State{Schema: schemaResp.Schema}
	diags := configState.Set(ctx, &TenantRecipesDataSourceModel{CatalogName: types.StringValue("Missing")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	source.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TenantRecipesDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if len(got.Recipes) != 0 {
		t.Fatalf("recipes = %+v", got.Recipes)
	}
	if fake.questionCalls != 0 {
		t.Fatalf("question calls = %d", fake.questionCalls)
	}
}

func TestTenantRecipeInstanceResourceSchema(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewTenantRecipeInstanceResource().Schema(t.Context(), fwresource.SchemaRequest{}, resp)

	if _, exists := resp.Schema.Attributes["simulate"]; exists {
		t.Fatal("simulate must not be an argument; a dry run is not a deploy")
	}
	if _, exists := resp.Schema.Attributes["auto_update"]; exists {
		t.Fatal("tenant recipe deploy has no auto_update argument")
	}
	if _, exists := resp.Schema.Blocks["timeouts"]; exists {
		t.Fatal("tenant recipe destroy uses the tenant power wait")
	}
	name := resp.Schema.Attributes["name"]
	if name == nil || !name.IsRequired() {
		t.Fatal("name should be required")
	}
	recipe := resp.Schema.Attributes["recipe_id"]
	if recipe == nil || !recipe.IsRequired() {
		t.Fatal("recipe_id should be required")
	}
	answers, ok := resp.Schema.Attributes["answers"].(resschema.MapAttribute)
	if !ok || !answers.Optional || answers.Required || !answers.Sensitive {
		t.Fatal("answers should be an optional sensitive map")
	}
	if !strings.Contains(answers.MarkdownDescription, "byte") || !strings.Contains(answers.MarkdownDescription, "53687091200") {
		t.Fatalf("answers description = %s", answers.MarkdownDescription)
	}
	tenantID, ok := resp.Schema.Attributes["tenant_id"].(resschema.Int64Attribute)
	if !ok || !tenantID.Computed || tenantID.Required || tenantID.Optional {
		t.Fatal("tenant_id should be computed")
	}
	if !strings.Contains(resp.Schema.MarkdownDescription, "no simulate") {
		t.Fatalf("description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestTenantRecipeInstanceMetadata(t *testing.T) {
	resp := &fwresource.MetadataResponse{}
	NewTenantRecipeInstanceResource().Metadata(t.Context(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
	if resp.TypeName != "vergeio_tenant_recipe_instance" {
		t.Fatalf("type = %s", resp.TypeName)
	}
}

func TestTenantRecipeInstanceConfigure(t *testing.T) {
	resource := &TenantRecipeInstanceResource{}
	resp := &fwresource.ConfigureResponse{}
	resource.Configure(t.Context(), fwresource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || resource.api != nil {
		t.Fatal("nil provider data should leave the resource unconfigured")
	}

	resp = &fwresource.ConfigureResponse{}
	resource.Configure(t.Context(), fwresource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() || resource.api != nil {
		t.Fatal("invalid client should fail configure")
	}

	resource = &TenantRecipeInstanceResource{}
	resp = &fwresource.ConfigureResponse{}
	resource.Configure(t.Context(), fwresource.ConfigureRequest{ProviderData: versionClient(t, "25.0.0")}, resp)
	if !resp.Diagnostics.HasError() || resource.api != nil {
		t.Fatal("unsupported version should fail configure")
	}
	if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), "failed to create VergeOS client") {
		t.Fatalf("error = %s", resp.Diagnostics.Errors()[0].Detail())
	}
}

func TestTenantRecipeInstanceImportState(t *testing.T) {
	ctx := t.Context()
	resource := &TenantRecipeInstanceResource{}
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	resp := &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	resource.ImportState(ctx, fwresource.ImportStateRequest{ID: "9"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TenantRecipeInstanceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Id.ValueString() != "9" {
		t.Fatalf("id = %q", got.Id.ValueString())
	}

	for _, id := range []string{"", "recipe", "0", "-1"} {
		resp = &fwresource.ImportStateResponse{State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		}}
		resource.ImportState(ctx, fwresource.ImportStateRequest{ID: id}, resp)
		if !resp.Diagnostics.HasError() {
			t.Fatalf("import id %q should fail", id)
		}
	}
}

func TestTenantRecipeInstanceCreateDelete(t *testing.T) {
	fake := newTenantRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredTenantRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	answers := tenantRecipeStringMap(t, map[string]string{
		"YB_USER_NAME":              "admin",
		"YB_EXPOSE_CLOUD_SNAPSHOTS": "yes",
		"YB_DRIVE_OS_SIZE":          strconv.FormatInt(vergeos.RecipeDiskSize50GB, 10),
		"YB_NIC_ETH0":               "Internal",
	})
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &TenantRecipeInstanceResourceModel{
		Name:     types.StringValue("customer-a"),
		RecipeID: types.StringValue(testTenantRecipeKey),
		Answers:  answers,
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	created := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resource.Create(ctx, fwresource.CreateRequest{Plan: plan}, created)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	var got TenantRecipeInstanceResourceModel
	created.Diagnostics.Append(created.State.Get(ctx, &got)...)
	if created.Diagnostics.HasError() {
		t.Fatal(created.Diagnostics)
	}
	if got.Id.ValueString() != "9" || got.TenantID.ValueInt64() != 44 || got.RecipeName.ValueString() != "Trial Tenant" {
		t.Fatalf("instance = %+v", got)
	}
	if got.Version.ValueString() != "1.0.0" || got.Build.ValueInt64() != 3 {
		t.Fatalf("recipe metadata = %+v", got)
	}
	stored := map[string]string{}
	if diags := got.Answers.ElementsAs(ctx, &stored, false); diags.HasError() {
		t.Fatal(diags)
	}
	if stored["YB_EXPOSE_CLOUD_SNAPSHOTS"] != "yes" || stored["YB_NIC_ETH0"] != "Internal" {
		t.Fatalf("answers were rewritten from the API: %#v", stored)
	}

	body := fake.deployBody()
	if strings.Contains(body, "simulate") {
		t.Fatalf("deploy body contained simulate: %s", body)
	}
	var posted map[string]any
	if err := json.Unmarshal([]byte(body), &posted); err != nil {
		t.Fatal(err)
	}
	if posted["recipe"] != testTenantRecipeKey || posted["name"] != "customer-a" {
		t.Fatalf("body = %#v", posted)
	}
	if _, ok := posted["auto_update"]; ok {
		t.Fatalf("body included auto_update: %#v", posted)
	}
	sent := posted["answers"].(map[string]any)
	if sent["YB_EXPOSE_CLOUD_SNAPSHOTS"] != true {
		t.Fatalf("bool = %#v", sent["YB_EXPOSE_CLOUD_SNAPSHOTS"])
	}
	if sent["YB_NIC_ETH0"] != float64(12) {
		t.Fatalf("network = %#v", sent["YB_NIC_ETH0"])
	}
	if sent["YB_DRIVE_OS_SIZE"] != float64(vergeos.RecipeDiskSize50GB) {
		t.Fatalf("disk = %#v", sent["YB_DRIVE_OS_SIZE"])
	}
	if fake.deployCount != 1 {
		t.Fatalf("deploys = %d", fake.deployCount)
	}

	updatePlan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = updatePlan.Set(ctx, &got)
	if diags.HasError() {
		t.Fatal(diags)
	}
	updated := &fwresource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resource.Update(ctx, fwresource.UpdateRequest{Plan: updatePlan, State: created.State}, updated)
	if updated.Diagnostics.HasError() {
		t.Fatal(updated.Diagnostics)
	}
	if fake.deployCount != 1 {
		t.Fatal("update deployed again")
	}

	fake.online = true
	fake.vnetRunning = true
	deleted := &fwresource.DeleteResponse{}
	resource.Delete(ctx, fwresource.DeleteRequest{State: updated.State}, deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	if fake.tenantDeletes != 1 || fake.instanceDeletes != 1 {
		t.Fatalf("tenant deletes = %d instance deletes = %d", fake.tenantDeletes, fake.instanceDeletes)
	}
	if !fake.sawAction("poweroff") || !fake.sawAction("kill") {
		t.Fatalf("actions = %#v, want poweroff and vnet kill", fake.actionBodies())
	}
	if !fake.tenantDeletedBeforeInstance {
		t.Fatal("instance row was deleted before the tenant")
	}
}

func TestTenantRecipeInstanceRefusesBoolAndDiskTraps(t *testing.T) {
	cases := []struct {
		name    string
		answers map[string]string
		want    string
	}{
		{
			name: "unrecognized bool",
			answers: map[string]string{
				"YB_USER_NAME":              "admin",
				"YB_EXPOSE_CLOUD_SNAPSHOTS": "enabled",
				"YB_DRIVE_OS_SIZE":          strconv.FormatInt(vergeos.RecipeDiskSize50GB, 10),
			},
			want: "not a recognized boolean",
		},
		{
			name: "disk size in gigabytes",
			answers: map[string]string{
				"YB_USER_NAME":     "admin",
				"YB_DRIVE_OS_SIZE": "50",
			},
			want: "bytes",
		},
		{
			name: "unknown question",
			answers: map[string]string{
				"YB_USER_NAME": "admin",
				"NO_SUCH":      "1",
			},
			want: "unknown answer",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newTenantRecipeFake()
			server := httptest.NewServer(fake)
			t.Cleanup(server.Close)

			ctx := t.Context()
			resource := configuredTenantRecipeInstance(t, server.URL)
			schemaResp := &fwresource.SchemaResponse{}
			resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
			plan := tfsdk.Plan{Schema: schemaResp.Schema}
			diags := plan.Set(ctx, &TenantRecipeInstanceResourceModel{
				Name:     types.StringValue("trap"),
				RecipeID: types.StringValue(testTenantRecipeKey),
				Answers:  tenantRecipeStringMap(t, tc.answers),
			})
			if diags.HasError() {
				t.Fatal(diags)
			}
			resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
			resource.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected deploy to be refused")
			}
			if !strings.Contains(resp.Diagnostics.Errors()[0].Detail(), tc.want) {
				t.Fatalf("error = %s", resp.Diagnostics.Errors()[0].Detail())
			}
			if fake.deployCount != 0 {
				t.Fatalf("refused answer was deployed %d times", fake.deployCount)
			}
		})
	}
}

func TestTenantRecipeInstanceSendsNewInternalNetwork(t *testing.T) {
	fake := newTenantRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredTenantRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &TenantRecipeInstanceResourceModel{
		Name:     types.StringValue("internal"),
		RecipeID: types.StringValue(testTenantRecipeKey),
		Answers: tenantRecipeStringMap(t, map[string]string{
			"YB_USER_NAME":     "admin",
			"YB_DRIVE_OS_SIZE": "0",
			"YB_NIC_ETH0":      "__new_internal__",
		}),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resource.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if fake.networkLists != 0 {
		t.Fatalf("network lists = %d, __new_internal__ does not need a lookup", fake.networkLists)
	}
	var posted map[string]any
	if err := json.Unmarshal([]byte(fake.deployBody()), &posted); err != nil {
		t.Fatal(err)
	}
	answers := posted["answers"].(map[string]any)
	if answers["YB_NIC_ETH0"] != "__new_internal__" {
		t.Fatalf("network = %#v", answers["YB_NIC_ETH0"])
	}
	if _, ok := posted["simulate"]; ok {
		t.Fatal("deploy JSON included simulate")
	}
}

func TestTenantRecipeInstanceDeleteMissingTenantStillRemovesRow(t *testing.T) {
	fake := newTenantRecipeFake()
	fake.tenantMissing = true
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredTenantRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	deleted := &fwresource.DeleteResponse{}
	resource.Delete(ctx, fwresource.DeleteRequest{State: tenantRecipeInstanceState(t, schemaResp.Schema, 44)}, deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	if fake.instanceDeletes != 1 {
		t.Fatalf("instance deletes = %d", fake.instanceDeletes)
	}
	if fake.sawAction("poweroff") {
		t.Fatalf("actions = %#v, a missing tenant is already gone", fake.actionBodies())
	}
}

func TestTenantRecipeInstanceDeleteMissingRowStopsStateTenant(t *testing.T) {
	fake := newTenantRecipeFake()
	fake.instanceMissing = true
	fake.online = true
	fake.vnetRunning = true
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredTenantRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	deleted := &fwresource.DeleteResponse{}
	resource.Delete(ctx, fwresource.DeleteRequest{State: tenantRecipeInstanceState(t, schemaResp.Schema, 44)}, deleted)
	if deleted.Diagnostics.HasError() {
		t.Fatal(deleted.Diagnostics)
	}
	if fake.tenantDeletes != 1 || fake.instanceDeletes != 1 {
		t.Fatalf("tenant deletes = %d instance deletes = %d", fake.tenantDeletes, fake.instanceDeletes)
	}
	if !fake.sawAction("poweroff") {
		t.Fatalf("actions = %#v, want poweroff", fake.actionBodies())
	}
	if !fake.tenantDeletedBeforeInstance {
		t.Fatal("instance row was deleted before the tenant")
	}
}

func TestTenantRecipeInstanceDeleteStatusErrorKeepsRow(t *testing.T) {
	fake := newTenantRecipeFake()
	fake.statusFailure = true
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredTenantRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	deleted := &fwresource.DeleteResponse{}
	resource.Delete(ctx, fwresource.DeleteRequest{State: tenantRecipeInstanceState(t, schemaResp.Schema, 44)}, deleted)
	if !deleted.Diagnostics.HasError() {
		t.Fatal("expected a tenant status error to fail destroy")
	}
	if fake.instanceDeletes != 0 || fake.tenantDeletes != 0 {
		t.Fatalf("tenant deletes = %d instance deletes = %d", fake.tenantDeletes, fake.instanceDeletes)
	}
}

func TestTenantRecipeInstanceReadDropsMissingTenant(t *testing.T) {
	fake := newTenantRecipeFake()
	fake.tenantMissing = true
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredTenantRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TenantRecipeInstanceResourceModel{
		Id:       types.StringValue("9"),
		Name:     types.StringValue("customer-a"),
		RecipeID: types.StringValue(testTenantRecipeKey),
		Answers:  tenantRecipeStringMap(t, map[string]string{"YB_USER_NAME": "admin"}),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	resource.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing tenant should remove the recipe instance from state")
	}
	if fake.instanceDeletes != 1 {
		t.Fatalf("instance deletes = %d, the row should go with the tenant", fake.instanceDeletes)
	}
}

func TestTenantRecipeInstanceReadStoresNameDrift(t *testing.T) {
	fake := newTenantRecipeFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	resource := configuredTenantRecipeInstance(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TenantRecipeInstanceResourceModel{
		Id:       types.StringValue("9"),
		Name:     types.StringValue("someone-else"),
		RecipeID: types.StringValue("eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"),
		Answers:  types.MapNull(types.StringType),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	resource.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TenantRecipeInstanceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Name.ValueString() != "customer-a" || got.RecipeID.ValueString() != testTenantRecipeKey {
		t.Fatalf("state name = %s recipe_id = %s, refresh should store the VergeOS values", got.Name.ValueString(), got.RecipeID.ValueString())
	}
	if fake.instanceDeletes != 0 {
		t.Fatal("a name difference is drift, not a missing instance")
	}
}

func configuredTenantRecipes(t *testing.T, host string) *TenantRecipesDataSource {
	t.Helper()
	source := &TenantRecipesDataSource{}
	resp := &datasource.ConfigureResponse{}
	source.Configure(t.Context(), datasource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return source
}

func configuredTenantRecipeInstance(t *testing.T, host string) *TenantRecipeInstanceResource {
	t.Helper()
	resource := &TenantRecipeInstanceResource{}
	resp := &fwresource.ConfigureResponse{}
	resource.Configure(t.Context(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resource
}

func tenantRecipeInstanceState(t *testing.T, schema resschema.Schema, tenantID int64) tfsdk.State {
	t.Helper()
	state := tfsdk.State{Schema: schema}
	diags := state.Set(t.Context(), &TenantRecipeInstanceResourceModel{
		Id:       types.StringValue("9"),
		Name:     types.StringValue("customer-a"),
		RecipeID: types.StringValue(testTenantRecipeKey),
		Answers:  types.MapNull(types.StringType),
		TenantID: types.Int64Value(tenantID),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	return state
}

func tenantRecipeStringMap(t *testing.T, values map[string]string) types.Map {
	t.Helper()
	elems := make(map[string]attr.Value, len(values))
	for key, value := range values {
		elems[key] = types.StringValue(value)
	}
	mapped, diags := types.MapValue(types.StringType, elems)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return mapped
}

type tenantRecipeFake struct {
	mu                          sync.Mutex
	online                      bool
	vnetRunning                 bool
	tenantMissing               bool
	instanceMissing             bool
	statusFailure               bool
	deployCount                 int
	networkLists                int
	questionCalls               int
	tenantDeletes               int
	instanceDeletes             int
	tenantDeletedBeforeInstance bool
	actions                     []string
	deploy                      string
}

func newTenantRecipeFake() *tenantRecipeFake {
	return &tenantRecipeFake{}
}

func (f *tenantRecipeFake) deployBody() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deploy
}

func (f *tenantRecipeFake) actionBodies() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.actions))
	copy(out, f.actions)
	return out
}

func (f *tenantRecipeFake) sawAction(action string) bool {
	for _, body := range f.actionBodies() {
		if strings.Contains(body, `"action":"`+action+`"`) {
			return true
		}
	}
	return false
}

func (f *tenantRecipeFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/version.json":
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/catalogs":
		filter := r.URL.Query().Get("filter")
		if strings.Contains(filter, "Missing") {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`[{"$key":"` + testTenantCatalogKey + `","id":"` + testTenantCatalogKey + `","name":"Tenants","description":"Tenant recipes","publishing_scope":"global","enabled":true}]`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenant_recipes":
		_, _ = w.Write([]byte(`[
			{"$key":"` + testTenantRecipeKey + `","id":"` + testTenantRecipeKey + `","name":"Trial Tenant","description":"A tenant from a snapshot","version":"1.0.0","build":4,"catalog":"` + testTenantCatalogKey + `","catalog_name":"Tenants","downloaded":true,"update_available":false},
			{"$key":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","id":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","name":"Trial Tenant Preview","catalog":"` + testTenantCatalogKey + `","catalog_name":"Tenants","downloaded":false}
		]`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/recipe_questions":
		f.questionCalls++
		_, _ = w.Write([]byte(`[
			{"name":"YB_USER_NAME","display":"Admin user","type":"string","required":true,"enabled":true,"default":"","section_name":"Access","min":1,"max":64},
			{"name":"YB_EXPOSE_CLOUD_SNAPSHOTS","display":"Cloud snapshots","type":"bool","required":false,"enabled":true,"default":false,"section_name":"Options"},
			{"name":"YB_DRIVE_OS_SIZE","display":"OS disk","type":"disksize","required":true,"enabled":true,"default":0,"min":0,"max":null,"section_name":"Storage"},
			{"name":"YB_NIC_ETH0","display":"Network","type":"network","required":false,"enabled":true,"section_name":"Network"},
			{"name":"YB_NODE_COUNT","display":"Nodes","type":"list","required":false,"enabled":true,"list":{"1":"One","2":"Two"},"section_name":"Compute"}
		]`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
		f.networkLists++
		_, _ = w.Write([]byte(`[{"$key":12,"name":"Internal"}]`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/84":
		running := "false"
		if f.vnetRunning {
			running = "true"
		}
		_, _ = w.Write([]byte(`{"$key":84,"name":"tenant_customer-a","running":` + running + `}`))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
		f.actions = append(f.actions, string(body))
		if strings.Contains(string(body), `"action":"kill"`) {
			f.vnetRunning = false
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/tenant_recipe_instances":
		f.deployCount++
		f.deploy = string(body)
		_, _ = w.Write([]byte(`{"$key":9}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenant_recipe_instances/9":
		if f.instanceMissing {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"$key":9,"recipe":"` + testTenantRecipeKey + `","recipe_name":"Trial Tenant","name":"customer-a","tenant":44,"tenant_name":"customer-a","version":"1.0.0","build":3,"answers":{"YB_EXPOSE_CLOUD_SNAPSHOTS":true,"YB_USER_NAME":"admin"}}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenant_status":
		if f.statusFailure {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"err":"status unavailable"}`))
			return
		}
		if f.tenantMissing {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		running := "false"
		status := "offline"
		if f.online {
			running = "true"
			status = "online"
		}
		_, _ = w.Write([]byte(`[{"$key":44,"tenant":44,"running":` + running + `,"starting":false,"stopping":false,"status":"` + status + `"}]`))
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/tenant_actions":
		f.actions = append(f.actions, string(body))
		if strings.Contains(string(body), `"action":"poweroff"`) {
			f.online = false
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenants/44":
		if f.tenantMissing {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{"$key":44,"name":"customer-a","vnet":84,"is_snapshot":false}`))
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/tenants/44":
		f.tenantDeletes++
		if f.tenantMissing {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
			return
		}
		if f.vnetRunning {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_, _ = w.Write([]byte(`{"err":"Tenant network must be powered off to delete tenant"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/tenant_recipe_instances/9":
		f.instanceDeletes++
		f.tenantDeletedBeforeInstance = f.tenantDeletes >= 1
		if f.instanceMissing {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"err":"unexpected ` + r.Method + ` ` + r.URL.RequestURI() + `"}`))
	}
}
