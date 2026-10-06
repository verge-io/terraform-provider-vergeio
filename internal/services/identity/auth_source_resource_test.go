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
	"sync"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestAuthSourceResourceSchema(t *testing.T) {
	resource := NewAuthSourceResource()
	meta := &fwresource.MetadataResponse{}
	resource.Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_auth_source" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	resp := &fwresource.SchemaResponse{}
	resource.Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	secret, ok := resp.Schema.Attributes["client_secret_wo"].(resschema.StringAttribute)
	if !ok || !secret.WriteOnly || !secret.Sensitive || !secret.Optional {
		t.Fatal("client_secret_wo should be an optional write-only secret")
	}
	if _, ok := resp.Schema.Attributes["client_secret"]; ok {
		t.Fatal("schema must not store client_secret")
	}
	settings, ok := resp.Schema.Attributes["settings"].(resschema.StringAttribute)
	if !ok || settings.Sensitive || !settings.Optional {
		t.Fatal("settings should be optional and not sensitive")
	}
	driver, ok := resp.Schema.Attributes["driver"].(resschema.StringAttribute)
	if !ok || !driver.IsRequired() || len(driver.PlanModifiers) == 0 {
		t.Fatal("driver should be required and replaced when it changes")
	}
}

func TestAuthSourceCreateDoesNotStoreSecret(t *testing.T) {
	const secret = "super-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/auth_sources":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), secret) || !strings.Contains(string(body), `"driver":"azure"`) {
				t.Errorf("create body = %s", body)
			}
			_, _ = w.Write([]byte(`{"$key":7}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/auth_sources/7":
			_, _ = w.Write([]byte(`{"$key":7,"name":"Corporate Azure","driver":"azure","menu":false,"debug":false,"button_fa_icon":"bi-microsoft","settings":{"client_id":"app","tenant_id":"tenant","client_secret":"` + secret + `","debug":false}}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	resource := configuredAuthSource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	planModel := &AuthSourceResourceModel{
		Name:                  types.StringValue("Corporate Azure"),
		Driver:                types.StringValue("azure"),
		Settings:              types.StringValue(`{"tenant_id":"tenant","client_id":"app"}`),
		ClientSecretWO:        types.StringNull(),
		ClientSecretWOVersion: types.Int64Value(1),
		ButtonFAIcon:          types.StringValue("bi-microsoft"),
	}
	configModel := *planModel
	configModel.ClientSecretWO = types.StringValue(secret)

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, planModel); diags.HasError() {
		t.Fatal(diags)
	}
	configState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := configState.Set(ctx, &configModel); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	resource.Create(ctx, fwresource.CreateRequest{
		Plan:   plan,
		Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if strings.Contains(fmt.Sprint(resp.State.Raw), secret) {
		t.Fatal("client secret was written to state")
	}
	var got AuthSourceResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if got.Id.ValueString() != "7" || got.ClientSecretWOVersion.ValueInt64() != 1 {
		t.Fatalf("id=%s version=%s", got.Id, got.ClientSecretWOVersion)
	}
	if got.Settings.ValueString() != `{"client_id":"app","tenant_id":"tenant"}` {
		t.Fatalf("settings = %s", got.Settings.ValueString())
	}
	if !got.ClientSecretWO.IsNull() {
		t.Fatal("client_secret_wo was stored")
	}
}

func TestAuthSourceUpdateMergesStoredSettings(t *testing.T) {
	const storedSecret = "stored-secret"
	var (
		mu           sync.Mutex
		puts         []string
		settingsGets int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/auth_sources/7":
			if r.URL.Query().Get("fields") == "$key,settings" {
				mu.Lock()
				settingsGets++
				mu.Unlock()
				_, _ = w.Write([]byte(`{"$key":7,"settings":{"client_id":"app","scope":"openid","client_secret":"` + storedSecret + `","debug":false}}`))
				return
			}
			_, _ = w.Write([]byte(`{"$key":7,"name":"Corporate Azure","driver":"azure","menu":true,"settings":{"client_id":"app","scope":"openid","debug":false}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/auth_sources/7":
			body, _ := io.ReadAll(r.Body)
			mu.Lock()
			puts = append(puts, string(body))
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewAuthSourceApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	state := &AuthSourceResourceModel{
		Id:       types.StringValue("7"),
		Name:     types.StringValue("Corporate Azure"),
		Driver:   types.StringValue("azure"),
		Settings: types.StringValue(`{"client_id":"app","scope":"openid"}`),
		Menu:     types.BoolValue(false),
	}
	plan := &AuthSourceResourceModel{
		Id:       types.StringValue("7"),
		Name:     types.StringValue("Corporate Azure"),
		Driver:   types.StringValue("azure"),
		Settings: types.StringValue(`{"client_id":"app"}`),
		Menu:     types.BoolValue(true),
	}
	if err := api.updateAuthSource(context.Background(), plan, state, ""); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if settingsGets != 1 || len(puts) != 1 {
		t.Fatalf("settings gets=%d puts=%d", settingsGets, len(puts))
	}
	body := puts[0]
	for _, want := range []string{storedSecret, `"client_id":"app"`, `"scope":"openid"`, `"debug":false`, `"menu":true`} {
		if !strings.Contains(body, want) {
			t.Fatalf("merged update missing %s: %s", want, body)
		}
	}
	if strings.Contains(plan.Settings.ValueString(), "scope") || strings.Contains(plan.Settings.ValueString(), storedSecret) {
		t.Fatalf("state settings = %s", plan.Settings.ValueString())
	}
}

func TestAuthSourceReadRemovesMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/version.json" {
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
			return
		}
		http.Error(w, `{"err":"not found"}`, http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	resource := configuredAuthSource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	resource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &AuthSourceResourceModel{
		Id:     types.StringValue("7"),
		Name:   types.StringValue("Corporate Azure"),
		Driver: types.StringValue("azure"),
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
		t.Fatal("missing auth source was kept in state")
	}
}

func configuredAuthSource(t *testing.T, host string) *AuthSourceResource {
	t.Helper()
	resource := &AuthSourceResource{}
	resp := &fwresource.ConfigureResponse{}
	resource.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() || resource.api == nil {
		t.Fatalf("configure: %v", resp.Diagnostics)
	}
	return resource
}
