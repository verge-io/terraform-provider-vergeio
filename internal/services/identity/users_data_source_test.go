// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestUsersDataSourceMetadata(t *testing.T) {
	resp := &datasource.MetadataResponse{}
	NewUsersDataSource().Metadata(context.Background(), datasource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
	if resp.TypeName != "vergeio_users" {
		t.Fatalf("type = %s", resp.TypeName)
	}
}

func TestUsersDataSourceSchema(t *testing.T) {
	resp := &datasource.SchemaResponse{}
	NewUsersDataSource().Schema(context.Background(), datasource.SchemaRequest{}, resp)
	filter, ok := resp.Schema.Attributes["filter_name"]
	if !ok || !filter.IsOptional() {
		t.Fatal("filter_name should be optional")
	}
	users, ok := resp.Schema.Attributes["users"]
	if !ok || !users.IsComputed() {
		t.Fatal("users should be computed")
	}
}

func TestUsersDataSourceConfigure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	source := &UsersDataSource{}
	resp := &datasource.ConfigureResponse{}
	source.Configure(context.Background(), datasource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() || source.api == nil || source.api.Name() != "Users Api" {
		t.Fatalf("configure failed: %v", resp.Diagnostics)
	}

	source = &UsersDataSource{}
	resp = &datasource.ConfigureResponse{}
	source.Configure(context.Background(), datasource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() || source.api != nil {
		t.Fatal("invalid client should fail configure")
	}

	source = &UsersDataSource{}
	resp = &datasource.ConfigureResponse{}
	source.Configure(context.Background(), datasource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || source.api != nil {
		t.Fatal("nil provider data should leave the data source unconfigured")
	}
}

func TestUserModelTypes(t *testing.T) {
	model := &UserModel{
		Id:          types.Int32Value(7),
		Name:        types.StringValue("ada"),
		DisplayName: types.StringValue("Ada"),
		Email:       types.StringValue("ada@example.com"),
		Type:        types.StringValue("normal"),
		Enabled:     types.BoolValue(true),
	}
	if model.Id.ValueInt32() != 7 || model.Name.ValueString() != "ada" || !model.Enabled.ValueBool() {
		t.Fatalf("model = %#v", model)
	}
}
