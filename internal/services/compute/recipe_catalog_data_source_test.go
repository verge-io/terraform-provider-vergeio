// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestCatalogsDataSourceSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewCatalogsDataSource().Schema(t.Context(), datasource.SchemaRequest{}, resp)
	if resp.Schema.Attributes["filter_name"] == nil || !resp.Schema.Attributes["filter_name"].IsOptional() {
		t.Fatal("filter_name should be optional")
	}
	catalogs := resp.Schema.Attributes["catalogs"]
	if catalogs == nil || !catalogs.IsComputed() {
		t.Fatal("catalogs should be computed")
	}
}

func TestVMRecipesDataSourceSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewVMRecipesDataSource().Schema(t.Context(), datasource.SchemaRequest{}, resp)
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

func TestCatalogsDataSourceKeepsExactName(t *testing.T) {
	fake := newCatalogFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	source := configuredCatalogs(t, server.URL)
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	configState := tfsdk.State{Schema: schemaResp.Schema}
	diags := configState.Set(ctx, &CatalogsDataSourceModel{FilterName: types.StringValue("Operating Systems")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	source.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got CatalogsDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if len(got.Catalogs) != 1 || got.Catalogs[0].Name.ValueString() != "Operating Systems" || got.Catalogs[0].Id.ValueString() != testCatalogKey {
		t.Fatalf("catalogs = %+v", got.Catalogs)
	}
	if got.Catalogs[0].PublishingScope.ValueString() != "global" || !got.Catalogs[0].Enabled.ValueBool() {
		t.Fatalf("catalog = %+v", got.Catalogs[0])
	}
}

func TestVMRecipesDataSourceReadsQuestions(t *testing.T) {
	fake := newCatalogFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	source := configuredVMRecipes(t, server.URL)
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	configState := tfsdk.State{Schema: schemaResp.Schema}
	diags := configState.Set(ctx, &VMRecipesDataSourceModel{
		FilterName: types.StringValue("Windows Server"),
		CatalogID:  types.StringValue(testCatalogKey),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	source.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got VMRecipesDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if len(got.Recipes) != 1 {
		t.Fatalf("recipes = %+v", got.Recipes)
	}
	recipe := got.Recipes[0]
	if recipe.Id.ValueString() != testRecipeKey || recipe.CatalogID.ValueString() != testCatalogKey || !recipe.Downloaded.ValueBool() {
		t.Fatalf("recipe = %+v", recipe)
	}
	if len(recipe.Questions) != 2 {
		t.Fatalf("questions = %+v", recipe.Questions)
	}
	disk := recipe.Questions[1]
	if disk.Name.ValueString() != "YB_DRIVE_OS_SIZE" || disk.Type.ValueString() != "disksize" || !disk.Required.ValueBool() {
		t.Fatalf("disk question = %+v", disk)
	}
	if disk.Min.ValueInt64() != 0 || !disk.Max.IsNull() {
		t.Fatalf("bounds min=%v max null=%v", disk.Min, disk.Max.IsNull())
	}
	if fake.questionCalls != 1 {
		t.Fatalf("question calls = %d", fake.questionCalls)
	}
}

func TestVMRecipesDataSourceRejectsBothCatalogFilters(t *testing.T) {
	fake := newCatalogFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	source := configuredVMRecipes(t, server.URL)
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	configState := tfsdk.State{Schema: schemaResp.Schema}
	diags := configState.Set(ctx, &VMRecipesDataSourceModel{
		CatalogID:   types.StringValue(testCatalogKey),
		CatalogName: types.StringValue("Operating Systems"),
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

func TestVMRecipesDataSourceMissingCatalogNameIsEmpty(t *testing.T) {
	fake := newCatalogFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := t.Context()
	source := configuredVMRecipes(t, server.URL)
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	configState := tfsdk.State{Schema: schemaResp.Schema}
	diags := configState.Set(ctx, &VMRecipesDataSourceModel{CatalogName: types.StringValue("Missing")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	source.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got VMRecipesDataSourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if len(got.Recipes) != 0 {
		t.Fatalf("recipes = %+v", got.Recipes)
	}
}

func configuredCatalogs(t *testing.T, host string) *CatalogsDataSource {
	t.Helper()
	source := &CatalogsDataSource{}
	resp := &datasource.ConfigureResponse{}
	source.Configure(t.Context(), datasource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return source
}

func configuredVMRecipes(t *testing.T, host string) *VMRecipesDataSource {
	t.Helper()
	source := &VMRecipesDataSource{}
	resp := &datasource.ConfigureResponse{}
	source.Configure(t.Context(), datasource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return source
}

type catalogFake struct {
	mu            sync.Mutex
	questionCalls int
}

func newCatalogFake() *catalogFake {
	return &catalogFake{}
}

func (f *catalogFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}
	_, _ = io.Copy(io.Discard, r.Body)
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
		_, _ = w.Write([]byte(`[
			{"$key":"` + testCatalogKey + `","id":"` + testCatalogKey + `","name":"Operating Systems","description":"OS images","publishing_scope":"global","enabled":true,"repository":4,"repository_name":"Marketplace","created":1710000000000000},
			{"$key":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","name":"Operating Systems Extra","publishing_scope":"private","enabled":false}
		]`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vm_recipes":
		_, _ = w.Write([]byte(`[
			{"$key":"` + testRecipeKey + `","id":"` + testRecipeKey + `","name":"Windows Server","description":"Windows with cloudbase-init","version":"2022","build":7,"catalog":"` + testCatalogKey + `","catalog_name":"Operating Systems","downloaded":true,"update_available":false},
			{"$key":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","id":"eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee","name":"Windows Server Preview","catalog":"` + testCatalogKey + `","catalog_name":"Operating Systems","downloaded":false}
		]`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/recipe_questions":
		f.questionCalls++
		_, _ = w.Write([]byte(`[
			{"name":"HOSTNAME","display":"Hostname","type":"string","required":true,"enabled":true,"default":"","section_name":"Guest"},
			{"name":"YB_DRIVE_OS_SIZE","display":"OS disk","type":"disksize","required":true,"enabled":true,"default":0,"min":0,"max":null,"section_name":"Storage"}
		]`))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"err":"unexpected ` + r.Method + ` ` + r.URL.RequestURI() + `"}`))
	}
}
