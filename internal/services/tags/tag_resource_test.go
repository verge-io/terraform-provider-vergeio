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

	categoryName, ok := resp.Schema.Attributes["category_name"]
	if !ok || !categoryName.IsComputed() || categoryName.IsRequired() {
		t.Fatal("category_name should be computed")
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
