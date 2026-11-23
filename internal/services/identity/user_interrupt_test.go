// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestUserCreateDropsUnknownsWhenReadIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = io.WriteString(w, `{"version":"26.2.2"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/users":
			cancel()
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `{"$key":7}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/7":
			gets++
			if gets > 1 {
				t.Errorf("user read was sent after the request was canceled")
			}
			_, _ = io.WriteString(w, `{"$key":7,"name":"tf-acc-user-int","enabled":true,"email":"ada@example.com","type":"normal"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	user := &UserResource{}
	configure := &fwresource.ConfigureResponse{}
	user.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &fwresource.SchemaResponse{}
	user.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)
	model := &UserResourceModel{
		Name:    types.StringValue("tf-acc-user-int"),
		Enabled: types.BoolUnknown(),
		Email:   types.StringUnknown(),
		Type:    types.StringUnknown(),
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	configState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := configState.Set(context.Background(), &UserResourceModel{Name: model.Name}); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	user.Create(ctx, fwresource.CreateRequest{
		Plan:   plan,
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: configState.Raw},
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got UserResourceModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.Id.ValueString() != "7" || got.Name.ValueString() != "tf-acc-user-int" {
		t.Fatalf("id=%q name=%q", got.Id.ValueString(), got.Name.ValueString())
	}
	if unknownOrSet(got.Enabled) || unknownOrSetString(got.Email) || unknownOrSetString(got.Type) {
		t.Fatalf("enabled=%#v email=%#v type=%#v, want null", got.Enabled, got.Email, got.Type)
	}
}

func unknownOrSet(v types.Bool) bool {
	return v.IsUnknown() || (!v.IsNull() && !v.IsUnknown())
}

func unknownOrSetString(v types.String) bool {
	return v.IsUnknown() || (!v.IsNull() && !v.IsUnknown())
}
