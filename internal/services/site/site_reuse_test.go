// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

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

func TestSiteReadDropsReusedKey(t *testing.T) {
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/sites/1":
			_, _ = w.Write([]byte(`{"$key":1,"name":"other","url":"https://other.invalid","created":1800000000,"enabled":false}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/sites/1":
			puts++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredSite(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &siteModel{
		ID:      types.StringValue("1"),
		Name:    types.StringValue("mine"),
		URL:     types.StringValue("https://bugtest.invalid"),
		Enabled: types.BoolValue(false),
		Created: types.Int64Value(1700000000),
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
		t.Fatal("a reused site key with a different created time should leave state")
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = plan.Set(ctx, &siteModel{
		ID:      types.StringValue("1"),
		Name:    types.StringValue("mine"),
		URL:     types.StringValue("https://bugtest.invalid"),
		Enabled: types.BoolValue(false),
		Created: types.Int64Value(1700000000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	config := tfsdk.Config{Schema: schemaResp.Schema, Raw: state.Raw}
	updated := &fwresource.UpdateResponse{State: state}
	item.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state, Config: config}, updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("update should refuse a reused site key")
	}
	if puts != 0 {
		t.Fatalf("update wrote the foreign site %d times", puts)
	}
	if !vergeos.IsNotFoundError(item.api.readSite(ctx, &siteModel{
		ID:      types.StringValue("1"),
		Created: types.Int64Value(1700000000),
	})) {
		t.Fatal("read should report the reused key as not found")
	}
}

func TestSiteReadKeepsSameCreatedAndImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case "/api/v4/sites/1":
			_, _ = w.Write([]byte(`{"$key":1,"name":"renamed","url":"https://renamed.invalid","created":1700000000,"enabled":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredSite(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &siteModel{
		ID:      types.StringValue("1"),
		Name:    types.StringValue("mine"),
		URL:     types.StringValue("https://bugtest.invalid"),
		Enabled: types.BoolValue(false),
		Created: types.Int64Value(1700000000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	item.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got siteModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Name.ValueString() != "renamed" || got.Created.ValueInt64() != 1700000000 || !got.Enabled.ValueBool() {
		t.Fatalf("same created time should refresh in place, got name %s created %s enabled %s", got.Name, got.Created, got.Enabled)
	}

	imported := tfsdk.State{Schema: schemaResp.Schema}
	diags = imported.Set(ctx, &siteModel{ID: types.StringValue("1")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	importedResp := &fwresource.ReadResponse{State: imported}
	item.Read(ctx, fwresource.ReadRequest{State: imported}, importedResp)
	if importedResp.Diagnostics.HasError() || importedResp.State.Raw.IsNull() {
		t.Fatalf("import read should keep the row: %v null=%t", importedResp.Diagnostics, importedResp.State.Raw.IsNull())
	}
	var filled siteModel
	importedResp.Diagnostics.Append(importedResp.State.Get(ctx, &filled)...)
	if filled.Created.ValueInt64() != 1700000000 || filled.Name.ValueString() != "renamed" {
		t.Fatalf("import read = name %s created %s", filled.Name, filled.Created)
	}
}

func configuredSite(t *testing.T, host string) *siteResource {
	t.Helper()
	item := &siteResource{}
	resp := &fwresource.ConfigureResponse{}
	item.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}
