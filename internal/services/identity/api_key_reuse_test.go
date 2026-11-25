// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

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

func TestAPIKeyOwnedSameUserDifferentCreated(t *testing.T) {
	data := &APIKeyResourceModel{
		UserID:  types.Int32Value(10),
		Created: types.Int64Value(1700000000),
	}
	same := &vergeos.UserAPIKey{Key: 3, User: 10, Created: 1700000000, Name: "renamed"}
	if err := ensureAPIKeyOwned(data, same); err != nil {
		t.Fatal(err)
	}
	reused := &vergeos.UserAPIKey{Key: 3, User: 10, Created: 1800000000, Name: "other"}
	if !vergeos.IsNotFoundError(ensureAPIKeyOwned(data, reused)) {
		t.Fatal("same user with a different created time should be not found")
	}
	otherUser := &vergeos.UserAPIKey{Key: 3, User: 11, Created: 1700000000}
	if !vergeos.IsNotFoundError(ensureAPIKeyOwned(data, otherUser)) {
		t.Fatal("different user should stay not found")
	}
	imported := &APIKeyResourceModel{Id: types.StringValue("3")}
	if err := ensureAPIKeyOwned(imported, reused); err != nil {
		t.Fatal(err)
	}
}

func TestAPIKeyReadDropsReusedKeyForSameUser(t *testing.T) {
	var puts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/user_api_keys/3":
			_, _ = w.Write([]byte(`{"$key":3,"user":10,"name":"other","description":"belongs to someone else","created":1800000000}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/user_api_keys/3":
			puts++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredAPIKeyResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &APIKeyResourceModel{
		Id:      types.StringValue("3"),
		UserID:  types.Int32Value(10),
		Name:    types.StringValue("mine"),
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
		t.Fatal("a reused API key for the same user should leave state")
	}

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags = plan.Set(ctx, &APIKeyResourceModel{
		Id:      types.StringValue("3"),
		UserID:  types.Int32Value(10),
		Name:    types.StringValue("mine"),
		Created: types.Int64Value(1700000000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	updated := &fwresource.UpdateResponse{State: state}
	item.Update(ctx, fwresource.UpdateRequest{Plan: plan, State: state}, updated)
	if !updated.Diagnostics.HasError() {
		t.Fatal("update should refuse a reused API key")
	}
	if puts != 0 {
		t.Fatalf("update wrote the foreign API key %d times", puts)
	}
	if !vergeos.IsNotFoundError(item.api.readAPIKey(ctx, &APIKeyResourceModel{
		Id:      types.StringValue("3"),
		UserID:  types.Int32Value(10),
		Created: types.Int64Value(1700000000),
	})) {
		t.Fatal("read should report the reused key as not found")
	}
}

func TestAPIKeyReadKeepsSameCreatedAndImport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.2"}`))
		case "/api/v4/user_api_keys/3":
			_, _ = w.Write([]byte(`{"$key":3,"user":10,"name":"renamed","description":"edited outside","created":1700000000}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredAPIKeyResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &APIKeyResourceModel{
		Id:      types.StringValue("3"),
		UserID:  types.Int32Value(10),
		Name:    types.StringValue("mine"),
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
	var got APIKeyResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Name.ValueString() != "renamed" || got.Created.ValueInt64() != 1700000000 || got.UserID.ValueInt32() != 10 {
		t.Fatalf("same created time should refresh in place, got name %s created %s user %s", got.Name, got.Created, got.UserID)
	}

	imported := tfsdk.State{Schema: schemaResp.Schema}
	diags = imported.Set(ctx, &APIKeyResourceModel{Id: types.StringValue("3")})
	if diags.HasError() {
		t.Fatal(diags)
	}
	importedResp := &fwresource.ReadResponse{State: imported}
	item.Read(ctx, fwresource.ReadRequest{State: imported}, importedResp)
	if importedResp.Diagnostics.HasError() || importedResp.State.Raw.IsNull() {
		t.Fatalf("import read should keep the row: %v null=%t", importedResp.Diagnostics, importedResp.State.Raw.IsNull())
	}
	var filled APIKeyResourceModel
	importedResp.Diagnostics.Append(importedResp.State.Get(ctx, &filled)...)
	if filled.Created.ValueInt64() != 1700000000 || filled.Name.ValueString() != "renamed" || filled.UserID.ValueInt32() != 10 {
		t.Fatalf("import read = name %s created %s user %s", filled.Name, filled.Created, filled.UserID)
	}
}

func TestAPIKeyDeleteChecksCreatedAndUser(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		userID     int32
		created    int64
		wantDelete bool
		wantErr    bool
	}{
		{name: "same user different created", status: http.StatusOK, body: `{"$key":3,"user":10,"name":"other","created":1800000000}`, userID: 10, created: 1700000000},
		{name: "different user same created", status: http.StatusOK, body: `{"$key":3,"user":11,"name":"other","created":1700000000}`, userID: 10, created: 1700000000},
		{name: "same", status: http.StatusOK, body: `{"$key":3,"user":10,"name":"renamed","created":1700000000}`, userID: 10, created: 1700000000, wantDelete: true},
		{name: "missing", status: http.StatusNotFound, userID: 10, created: 1700000000, wantDelete: true},
		{name: "read error", status: http.StatusInternalServerError, userID: 10, created: 1700000000, wantErr: true},
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
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/user_api_keys/3":
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(tc.body))
				case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/user_api_keys/3":
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
			item := configuredAPIKeyResource(t, server.URL)
			schemaResp := &fwresource.SchemaResponse{}
			item.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, &APIKeyResourceModel{
				Id:      types.StringValue("3"),
				UserID:  types.Int32Value(tc.userID),
				Name:    types.StringValue("mine"),
				Created: types.Int64Value(tc.created),
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

func configuredAPIKeyResource(t *testing.T, url string) *APIKeyResource {
	t.Helper()
	item := &APIKeyResource{}
	configured := &fwresource.ConfigureResponse{}
	item.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(url, "user", "pass", true),
	}, configured)
	if configured.Diagnostics.HasError() {
		t.Fatal(configured.Diagnostics)
	}
	return item
}
