// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestTagCategoryReadDropsReusedKey(t *testing.T) {
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tag_categories/3":
			_, _ = w.Write([]byte(`{"$key":3,"name":"other","description":"belongs to someone else","created":1800000000}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/tag_categories/3":
			puts++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredTagCategoryResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TagCategoryResourceModel{
		Id:          types.StringValue("3"),
		Name:        types.StringValue("mine"),
		Description: types.StringValue("mine"),
		Created:     types.Int64Value(1700000000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	item.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("a reused category key with a different created time should leave state")
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = plan.Set(ctx, &TagCategoryResourceModel{
		Id:          types.StringValue("3"),
		Name:        types.StringValue("mine"),
		Description: types.StringValue("mine"),
		Created:     types.Int64Value(1700000000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	updated := &fwresource.UpdateResponse{State: state}
	item.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state}, updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("update should refuse a reused category key")
	}
	if puts != 0 {
		t.Fatalf("update wrote the foreign category %d times", puts)
	}
	if !vergeos.IsNotFoundError(item.tagsApi.readTagCategory(ctx, &TagCategoryResourceModel{
		Id:      types.StringValue("3"),
		Created: types.Int64Value(1700000000),
	})) {
		t.Fatal("read should report the reused key as not found")
	}
}

func TestTagCategoryReadKeepsSameCreatedAndImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case "/api/v4/tag_categories/3":
			_, _ = w.Write([]byte(`{"$key":3,"name":"renamed","description":"edited outside","created":1700000000}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredTagCategoryResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TagCategoryResourceModel{
		Id:          types.StringValue("3"),
		Name:        types.StringValue("mine"),
		Description: types.StringValue("mine"),
		Created:     types.Int64Value(1700000000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	item.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got TagCategoryResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Name.ValueString() != "renamed" || got.Created.ValueInt64() != 1700000000 {
		t.Fatalf("same created time should refresh in place, got name %s created %s", got.Name, got.Created)
	}

	imported := tfsdk.State{Schema: schemaResp.Schema}
	diags = imported.Set(ctx, &TagCategoryResourceModel{Id: types.StringValue("3")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	importedResp := &fwresource.ReadResponse{State: imported}
	item.Read(ctx, fwresource.ReadRequest{State: imported}, importedResp)
	if importedResp.Diagnostics.HasError() || importedResp.State.Raw.IsNull() {
		t.Fatalf("import read should keep the row: %v null=%t", importedResp.Diagnostics, importedResp.State.Raw.IsNull())
	}
	var filled TagCategoryResourceModel
	importedResp.Diagnostics.Append(importedResp.State.Get(ctx, &filled)...)
	if filled.Created.ValueInt64() != 1700000000 || filled.Name.ValueString() != "renamed" {
		t.Fatalf("import read = name %s created %s", filled.Name, filled.Created)
	}
}
