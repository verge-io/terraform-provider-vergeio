// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"terraform-provider-vergeio/internal/client"
)

func TestTagResourceMetadata(t *testing.T) {
	resp := &fwresource.MetadataResponse{}
	NewTagResource().Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
	if resp.TypeName != "vergeio_tag" {
		t.Fatalf("type = %s", resp.TypeName)
	}
}

func TestTagResourceSchema(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewTagResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if !strings.Contains(resp.Schema.MarkdownDescription, "removes every assignment") {
		t.Fatalf("description should state that deleting a tag removes assignments: %s", resp.Schema.MarkdownDescription)
	}
	name, ok := resp.Schema.Attributes["name"]
	if !ok || !name.IsRequired() {
		t.Fatal("name should be required")
	}
	category, ok := resp.Schema.Attributes["category"].(resschema.Int32Attribute)
	if !ok || !category.Required {
		t.Fatal("category should be a required int32")
	}
	if len(category.PlanModifiers) != 1 {
		t.Fatalf("category plan modifiers = %d, want 1", len(category.PlanModifiers))
	}
	want := int32planmodifier.RequiresReplace().Description(context.Background())
	if got := category.PlanModifiers[0].Description(context.Background()); got != want {
		t.Fatalf("category plan modifier %q, want %q", got, want)
	}
	assertTagMemberInt32RequiresReplace(t, "category", category.PlanModifiers[0], 4, 9)

	categoryName, ok := resp.Schema.Attributes["category_name"].(resschema.StringAttribute)
	if !ok || !categoryName.Computed || categoryName.Optional || categoryName.Required {
		t.Fatal("category_name should be computed only")
	}
	// A category rename does not change this tag's category id, but VergeOS
	// returns the new display name. UseStateForUnknown would keep the old
	// name in the plan and the apply would fail as an inconsistent result.
	keepPrior := stringplanmodifier.UseStateForUnknown().Description(context.Background())
	for _, mod := range categoryName.PlanModifiers {
		if mod.Description(context.Background()) == keepPrior {
			t.Fatal("category_name must not preserve the prior name across an update")
		}
	}
}

// TestTagImportReadReusesSDKClient is the acceptance import: ImportState
// configures one resource, then Read configures another. A second govergeos
// setup must not leave Read with a nil tag service.
func TestTagImportReadReusesSDKClient(t *testing.T) {
	var versionCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			versionCalls++
			if versionCalls > 1 {
				http.Error(w, "unavailable", http.StatusInternalServerError)
				return
			}
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/tags/8":
			_, _ = w.Write([]byte(`{"$key":8,"name":"production","description":"workloads","category":4,"category_display":"environment"}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	vergeClient := vergeio.NewClient(server.URL, "user", "pass", true)
	schemaResp := &fwresource.SchemaResponse{}
	NewTagResource().Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	importer := &TagResource{}
	configure := &fwresource.ConfigureResponse{}
	importer.Configure(ctx, fwresource.ConfigureRequest{ProviderData: vergeClient}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	importResp := &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	importer.ImportState(ctx, fwresource.ImportStateRequest{ID: "8"}, importResp)
	if importResp.Diagnostics.HasError() {
		t.Fatal(importResp.Diagnostics)
	}

	// Read uses a new resource instance, matching the framework server.
	reader := &TagResource{}
	configure = &fwresource.ConfigureResponse{}
	reader.Configure(ctx, fwresource.ConfigureRequest{ProviderData: vergeClient}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	if reader.tagsApi == nil || reader.tagsApi.sdk == nil || reader.tagsApi.sdk.Tags == nil {
		t.Fatal("import Read was configured without a tag client")
	}
	resp := &fwresource.ReadResponse{State: importResp.State}
	reader.Read(ctx, fwresource.ReadRequest{State: importResp.State}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TagResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Name.ValueString() != "production" || got.Category.ValueInt32() != 4 || got.CategoryName.ValueString() != "environment" {
		t.Fatalf("imported tag = name %q category %d category_name %q", got.Name.ValueString(), got.Category.ValueInt32(), got.CategoryName.ValueString())
	}
	if versionCalls != 1 {
		t.Fatalf("version checks = %d, want 1", versionCalls)
	}
}

func TestTagImportState(t *testing.T) {
	ctx := context.Background()
	tag := &TagResource{}
	schemaResp := &fwresource.SchemaResponse{}
	tag.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	resp := &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	tag.ImportState(ctx, fwresource.ImportStateRequest{ID: "8"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TagResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Id.ValueString() != "8" {
		t.Fatalf("id = %q, want 8", got.Id.ValueString())
	}
}

func TestTagReadRemovesMissingTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/tags/9":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	tag := &TagResource{}
	configure := &fwresource.ConfigureResponse{}
	tag.Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &fwresource.SchemaResponse{}
	tag.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TagResourceModel{
		Id:       types.StringValue("9"),
		Category: types.Int32Value(4),
		Name:     types.StringValue("production"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	tag.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing tag should be removed from state")
	}
}

// TestTagUpdateRefreshesCategoryName is the unit stand-in for the acceptance
// step that renames a category and a tag in one apply. The plan leaves
// category_name unknown. The read after update stores the name VergeOS
// returns, which is the renamed category.
func TestTagUpdateRefreshesCategoryName(t *testing.T) {
	const (
		oldCategory = "tf-acc-tagcat-abc"
		newCategory = "tf-acc-tagcat-abc-v2"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/tags/8":
			_, _ = w.Write([]byte(`{"response":"OK"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tags/8":
			_, _ = w.Write([]byte(`{"$key":8,"name":"production-v2","description":"workloads","category":4,"category_display":"` + newCategory + `"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	tag := &TagResource{}
	configure := &fwresource.ConfigureResponse{}
	tag.Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &fwresource.SchemaResponse{}
	tag.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TagResourceModel{
		Id:           types.StringValue("8"),
		Category:     types.Int32Value(4),
		Name:         types.StringValue("production"),
		Description:  types.StringValue("workloads"),
		CategoryName: types.StringValue(oldCategory),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = plan.Set(ctx, &TagResourceModel{
		Id:           types.StringValue("8"),
		Category:     types.Int32Value(4),
		Name:         types.StringValue("production-v2"),
		Description:  types.StringValue("workloads"),
		CategoryName: types.StringUnknown(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}

	resp := &fwresource.UpdateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	tag.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TagResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.CategoryName.ValueString() != newCategory {
		t.Fatalf("category_name = %q, want %q", got.CategoryName.ValueString(), newCategory)
	}
	if got.Category.ValueInt32() != 4 {
		t.Fatalf("category id = %d, want 4", got.Category.ValueInt32())
	}
	if got.Name.ValueString() != "production-v2" {
		t.Fatalf("name = %q", got.Name.ValueString())
	}
}
