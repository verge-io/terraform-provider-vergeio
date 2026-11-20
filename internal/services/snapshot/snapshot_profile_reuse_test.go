// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

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

func TestSnapshotProfileReadDropsReusedKey(t *testing.T) {
	var puts, periodLists int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/snapshot_profiles/9":
			_, _ = w.Write([]byte(`{"$key":9,"name":"other","description":"belongs to someone else"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/snapshot_profile_periods":
			periodLists++
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/snapshot_profiles/9":
			puts++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredSnapshotProfile(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &SnapshotProfileResourceModel{
		Id:          types.StringValue("9"),
		Name:        types.StringValue("mine"),
		Description: types.StringValue("mine"),
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
		t.Fatal("a reused profile key with a different name should leave state")
	}
	if periodLists != 0 {
		t.Fatalf("reused key listed periods %d times", periodLists)
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = plan.Set(ctx, &SnapshotProfileResourceModel{
		Id:          types.StringValue("9"),
		Name:        types.StringValue("mine"),
		Description: types.StringValue("mine"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	updated := &fwresource.UpdateResponse{State: state}
	item.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state}, updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("update should refuse a reused profile key")
	}
	if puts != 0 {
		t.Fatalf("update wrote the foreign profile %d times", puts)
	}
	if !vergeos.IsNotFoundError(item.api.readProfile(ctx, &SnapshotProfileResourceModel{
		Id:   types.StringValue("9"),
		Name: types.StringValue("mine"),
	})) {
		t.Fatal("read should report the reused key as not found")
	}
}

func TestSnapshotProfileReadKeepsSameNameAndImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case "/api/v4/snapshot_profiles/9":
			_, _ = w.Write([]byte(`{"$key":9,"name":"mine","description":"edited outside"}`))
		case "/api/v4/snapshot_profile_periods":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredSnapshotProfile(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &SnapshotProfileResourceModel{
		Id:          types.StringValue("9"),
		Name:        types.StringValue("mine"),
		Description: types.StringValue("mine"),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	item.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got SnapshotProfileResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Name.ValueString() != "mine" || got.Description.ValueString() != "edited outside" {
		t.Fatalf("same name should refresh in place, got name %s description %s", got.Name, got.Description)
	}

	imported := tfsdk.State{Schema: schemaResp.Schema}
	diags = imported.Set(ctx, &SnapshotProfileResourceModel{Id: types.StringValue("9")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	importedResp := &fwresource.ReadResponse{State: imported}
	item.Read(ctx, fwresource.ReadRequest{State: imported}, importedResp)
	if importedResp.Diagnostics.HasError() || importedResp.State.Raw.IsNull() {
		t.Fatalf("import read should keep the row: %v null=%t", importedResp.Diagnostics, importedResp.State.Raw.IsNull())
	}
}
