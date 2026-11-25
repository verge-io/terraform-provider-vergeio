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

func TestTagReadDropsReusedKey(t *testing.T) {
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tags/5":
			_, _ = w.Write([]byte(`{"$key":5,"name":"other","description":"belongs to someone else","category":4,"category_display":"environment","created":1800000000}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/tags/5":
			puts++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredTagResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TagResourceModel{
		Id:          types.StringValue("5"),
		Category:    types.Int32Value(4),
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
		t.Fatal("a reused tag key with a different created time should leave state")
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = plan.Set(ctx, &TagResourceModel{
		Id:          types.StringValue("5"),
		Category:    types.Int32Value(4),
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
		t.Fatal("update should refuse a reused tag key")
	}
	if puts != 0 {
		t.Fatalf("update wrote the foreign tag %d times", puts)
	}
	if !vergeos.IsNotFoundError(item.tagsApi.readTag(ctx, &TagResourceModel{
		Id:      types.StringValue("5"),
		Created: types.Int64Value(1700000000),
	})) {
		t.Fatal("read should report the reused key as not found")
	}
}

func TestTagReadKeepsSameCreatedAndImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case "/api/v4/tags/5":
			_, _ = w.Write([]byte(`{"$key":5,"name":"renamed","description":"edited outside","category":4,"category_display":"environment","created":1700000000}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredTagResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &TagResourceModel{
		Id:          types.StringValue("5"),
		Category:    types.Int32Value(4),
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
	var got TagResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Name.ValueString() != "renamed" || got.Created.ValueInt64() != 1700000000 {
		t.Fatalf("same created time should refresh in place, got name %s created %s", got.Name, got.Created)
	}

	imported := tfsdk.State{Schema: schemaResp.Schema}
	diags = imported.Set(ctx, &TagResourceModel{Id: types.StringValue("5")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	importedResp := &fwresource.ReadResponse{State: imported}
	item.Read(ctx, fwresource.ReadRequest{State: imported}, importedResp)
	if importedResp.Diagnostics.HasError() || importedResp.State.Raw.IsNull() {
		t.Fatalf("import read should keep the row: %v null=%t", importedResp.Diagnostics, importedResp.State.Raw.IsNull())
	}
	var filled TagResourceModel
	importedResp.Diagnostics.Append(importedResp.State.Get(ctx, &filled)...)
	if filled.Created.ValueInt64() != 1700000000 || filled.Name.ValueString() != "renamed" {
		t.Fatalf("import read = name %s created %s", filled.Name, filled.Created)
	}
}

func TestTagDeleteChecksCreated(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		created    int64
		wantDelete bool
		wantErr    bool
	}{
		{name: "foreign", status: http.StatusOK, body: `{"$key":5,"name":"other","category":4,"created":1800000000}`, created: 1700000000},
		{name: "same", status: http.StatusOK, body: `{"$key":5,"name":"renamed","category":4,"created":1700000000}`, created: 1700000000, wantDelete: true},
		{name: "missing", status: http.StatusNotFound, created: 1700000000, wantDelete: true},
		{name: "read error", status: http.StatusInternalServerError, created: 1700000000, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var deletes int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tags/5":
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/tags/5":
					deletes++
					if tc.status == http.StatusNotFound {
						w.WriteHeader(http.StatusNotFound)
						return
					}
					_, _ = w.Write([]byte(`{"response":"OK"}`))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)

			ctx := context.Background()
			item := configuredTagResource(t, server.URL)
			schemaResp := &fwresource.SchemaResponse{}
			item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, &TagResourceModel{
				Id:       types.StringValue("5"),
				Name:     types.StringValue("mine"),
				Category: types.Int32Value(4),
				Created:  types.Int64Value(tc.created),
			})
			if diags.HasError() {
				t.Fatal(diags)
			}
			resp := &fwresource.DeleteResponse{}
			item.Delete(ctx, fwresource.DeleteRequest{State: state}, resp)
			if resp.Diagnostics.HasError() != tc.wantErr {
				t.Fatalf("error = %v, want error %t", resp.Diagnostics, tc.wantErr)
			}
			if deletes > 0 != tc.wantDelete {
				t.Fatalf("deletes = %d, want delete %t", deletes, tc.wantDelete)
			}
		})
	}
}

func configuredTagResource(t *testing.T, url string) *TagResource {
	t.Helper()
	item := &TagResource{}
	configured := &fwresource.ConfigureResponse{}
	item.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(url, "user", "pass", true),
	}, configured)
	if configured.Diagnostics.HasError() {
		t.Fatal(configured.Diagnostics)
	}
	return item
}
