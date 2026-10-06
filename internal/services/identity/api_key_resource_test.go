// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestAPIKeyResourceMetadataAndSchema(t *testing.T) {
	resource := NewAPIKeyResource()
	meta := &fwresource.MetadataResponse{}
	resource.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_api_key" {
		t.Fatalf("type = %s", meta.TypeName)
	}

	resp := &fwresource.SchemaResponse{}
	resource.Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	if _, ok := resp.Schema.Attributes["token"]; ok {
		t.Fatal("managed API key schema must not include token")
	}
	for _, name := range []string{"user_id", "name"} {
		if !resp.Schema.Attributes[name].IsRequired() {
			t.Fatalf("%s should be required", name)
		}
	}
	expires, ok := resp.Schema.Attributes["expires"].(resschema.Int64Attribute)
	if !ok || !expires.IsOptional() || expires.IsComputed() {
		t.Fatal("expires should be optional and not computed, so removing it clears the expiry")
	}
	for _, name := range []string{"lastlogin_stamp", "created"} {
		attr, ok := resp.Schema.Attributes[name].(resschema.Int64Attribute)
		if !ok || len(attr.PlanModifiers) == 0 {
			t.Fatalf("%s should keep its refreshed value in the plan", name)
		}
	}
	if attr, ok := resp.Schema.Attributes["lastlogin_ip"].(resschema.StringAttribute); !ok || len(attr.PlanModifiers) == 0 {
		t.Fatal("lastlogin_ip should keep its refreshed value in the plan")
	}
	userID, ok := resp.Schema.Attributes["user_id"].(resschema.Int32Attribute)
	if !ok || len(userID.PlanModifiers) == 0 {
		t.Fatal("user_id should require replace")
	}
}

func TestAPIKeyResourceConfigure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	resource := &APIKeyResource{}
	resp := &fwresource.ConfigureResponse{}
	resource.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() || resource.api == nil || resource.api.Name() != "API Key Api" {
		t.Fatalf("configure failed: %v api=%v", resp.Diagnostics, resource.api)
	}

	resource = &APIKeyResource{}
	resp = &fwresource.ConfigureResponse{}
	resource.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() || resource.api != nil {
		t.Fatal("invalid client should fail configure")
	}
}

func TestAPIKeyCreateDoesNotStoreToken(t *testing.T) {
	const token = "secret-token-123"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/user_api_keys":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"expires_type":"date"`) || !strings.Contains(string(body), `"ip_allow_list":"192.0.2.0/24"`) {
				t.Errorf("create body = %s", body)
			}
			_, _ = fmt.Fprintf(w, `{"$key":5,"response":{"token":%q}}`, token)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/user_api_keys/5":
			_, _ = w.Write([]byte(`{"$key":5,"user":10,"name":"ci","description":"runner","ip_allow_list":"192.0.2.0/24","ip_deny_list":"","created":1700000000,"expires":1893456000,"lastlogin_stamp":0,"lastlogin_ip":""}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	resource := &APIKeyResource{}
	configure := &fwresource.ConfigureResponse{}
	resource.Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &APIKeyResourceModel{
		UserID:      types.Int32Value(10),
		Name:        types.StringValue("ci"),
		Description: types.StringValue("runner"),
		IPAllowList: types.StringValue("192.0.2.0/24"),
		Expires:     types.Int64Value(1893456000),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resource.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if strings.Contains(fmt.Sprint(resp.State.Raw), token) {
		t.Fatal("API key token was written to state")
	}
	var got APIKeyResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Id.ValueString() != "5" || got.Expires.ValueInt64() != 1893456000 || got.IPAllowList.ValueString() != "192.0.2.0/24" {
		t.Fatalf("state = id %s expires %s allow %s", got.Id, got.Expires, got.IPAllowList)
	}
}

func TestAPIKeyReadRemovesMissingAndReused(t *testing.T) {
	var mode string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch mode {
		case "missing":
			if r.URL.Path == "/version.json" {
				_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				return
			}
			http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
		default:
			if r.URL.Path == "/version.json" {
				_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				return
			}
			_, _ = w.Write([]byte(`{"$key":5,"user":99,"name":"other"}`))
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	for _, mode = range []string{"missing", "reused"} {
		t.Run(mode, func(t *testing.T) {
			resource := &APIKeyResource{}
			configure := &fwresource.ConfigureResponse{}
			resource.Configure(ctx, fwresource.ConfigureRequest{
				ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
			}, configure)
			if configure.Diagnostics.HasError() {
				t.Fatal(configure.Diagnostics)
			}
			schemaResp := &fwresource.SchemaResponse{}
			resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
			state := tfsdk.State{Schema: schemaResp.Schema}
			diags := state.Set(ctx, &APIKeyResourceModel{
				Id:     types.StringValue("5"),
				UserID: types.Int32Value(10),
				Name:   types.StringValue("ci"),
			})
			if diags.HasError() {
				t.Fatal(diags)
			}
			resp := &fwresource.ReadResponse{State: state}
			resource.Read(ctx, fwresource.ReadRequest{State: state}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			if !resp.State.Raw.IsNull() {
				t.Fatal("stale API key was kept in state")
			}
		})
	}
}
