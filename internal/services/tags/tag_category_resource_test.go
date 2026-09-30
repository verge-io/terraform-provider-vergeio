// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"terraform-provider-vergeio/internal/client"
)

func TestTagCategoryResourceMetadata(t *testing.T) {
	resp := &fwresource.MetadataResponse{}
	NewTagCategoryResource().Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
	if resp.TypeName != "vergeio_tag_category" {
		t.Fatalf("type = %s", resp.TypeName)
	}
}

func TestTagCategoryResourceSchema(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewTagCategoryResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if !strings.Contains(resp.Schema.MarkdownDescription, "every assignment of those tags") {
		t.Fatalf("description should state the cascade delete: %s", resp.Schema.MarkdownDescription)
	}
	if !strings.Contains(resp.Schema.MarkdownDescription, "not sent as false") {
		t.Fatalf("description should state omitted flags are not sent as false: %s", resp.Schema.MarkdownDescription)
	}

	name, ok := resp.Schema.Attributes["name"]
	if !ok || !name.IsRequired() {
		t.Fatal("name should be required")
	}
	id, ok := resp.Schema.Attributes["id"]
	if !ok || !id.IsComputed() {
		t.Fatal("id should be computed")
	}

	wantModifier := boolplanmodifier.UseStateForUnknown().Description(context.Background())
	flags := []string{
		"single_tag_selection",
		"taggable_volumes",
		"taggable_vnets",
		"taggable_vnet_rules",
		"taggable_vmware_containers",
		"taggable_vms",
		"taggable_users",
		"taggable_tenant_nodes",
		"taggable_sites",
		"taggable_nodes",
		"taggable_groups",
		"taggable_clusters",
		"taggable_tenants",
	}
	for _, name := range flags {
		attr, ok := resp.Schema.Attributes[name].(resschema.BoolAttribute)
		if !ok {
			t.Fatalf("%s should be a bool attribute", name)
		}
		if !attr.Optional || attr.Required {
			t.Fatalf("%s should be optional", name)
		}
		if attr.Default != nil {
			t.Fatalf("%s has a default; an omitted flag would be sent as that default", name)
		}
		if len(attr.PlanModifiers) != 1 {
			t.Fatalf("%s plan modifiers = %d, want 1", name, len(attr.PlanModifiers))
		}
		if got := attr.PlanModifiers[0].Description(context.Background()); got != wantModifier {
			t.Fatalf("%s plan modifier %q, want %q", name, got, wantModifier)
		}
	}
}

func TestTagCategoryResourceConfigure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	category := &TagCategoryResource{}
	resp := &fwresource.ConfigureResponse{}
	category.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() || category.tagsApi == nil || category.tagsApi.Name() != "Tags Api" {
		t.Fatalf("configure failed: %v api=%v", resp.Diagnostics, category.tagsApi)
	}

	category = &TagCategoryResource{}
	resp = &fwresource.ConfigureResponse{}
	category.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() || category.tagsApi != nil {
		t.Fatal("invalid client should fail configure")
	}

	category = &TagCategoryResource{}
	resp = &fwresource.ConfigureResponse{}
	category.Configure(context.Background(), fwresource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || category.tagsApi != nil {
		t.Fatal("nil provider data should leave the resource unconfigured")
	}
}

func TestTagCategoryImportState(t *testing.T) {
	ctx := context.Background()
	category := &TagCategoryResource{}
	schemaResp := &fwresource.SchemaResponse{}
	category.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	resp := &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	category.ImportState(ctx, fwresource.ImportStateRequest{ID: "15"}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TagCategoryResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Id.ValueString() != "15" {
		t.Fatalf("id = %q, want 15", got.Id.ValueString())
	}
}

func TestTagCategoryCreateKeepsIDWhenRefreshFails(t *testing.T) {
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/tag_categories":
			_, _ = w.Write([]byte(`{"$key":4}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tag_categories/4":
			if gets.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"$key":4,"name":"env","description":"classification","taggable_vms":true}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"err":"unavailable"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	category := configuredTagCategoryResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	category.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &TagCategoryResourceModel{
		Name:        types.StringValue("env"),
		Description: types.StringValue("classification"),
		TaggableVMs: types.BoolValue(true),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	category.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the follow-up read to fail")
	}
	var got TagCategoryResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if got.Id.ValueString() != "4" {
		t.Fatalf("id = %q, want 4", got.Id.ValueString())
	}
}

func TestTagCategoryReadRemovesMissingCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/tag_categories/9":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	category := configuredTagCategoryResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	category.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TagCategoryResourceModel{
		Id:   types.StringValue("9"),
		Name: types.StringValue("env"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	category.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing category should be removed from state")
	}
}

func TestTagCategoryDeleteWarnsAboutCascade(t *testing.T) {
	var deleted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/tag_categories/4":
			deleted.Store(true)
			_, _ = w.Write([]byte(`{"response":"OK"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	category := configuredTagCategoryResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	category.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TagCategoryResourceModel{
		Id:   types.StringValue("4"),
		Name: types.StringValue("env"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.DeleteResponse{State: state}
	category.Delete(ctx, fwresource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !deleted.Load() {
		t.Fatal("category was not deleted")
	}
	if resp.Diagnostics.WarningsCount() == 0 {
		t.Fatal("delete should warn that the category cascade removes tags and assignments")
	}
	warned := false
	for _, diag := range resp.Diagnostics {
		if strings.Contains(diag.Summary(), "deletes its tags") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("delete warning = %v", resp.Diagnostics)
	}
}

func configuredTagCategoryResource(t *testing.T, url string) *TagCategoryResource {
	t.Helper()
	category := &TagCategoryResource{}
	resp := &fwresource.ConfigureResponse{}
	category.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(url, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return category
}
