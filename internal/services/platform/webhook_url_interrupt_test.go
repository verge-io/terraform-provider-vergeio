// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestWebhookURLCreateSurvivesCanceledRequest(t *testing.T) {
	fake := newPlatformFake()
	ctx, cancel := context.WithCancel(context.Background())
	fake.pause = func(r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/v4/webhook_urls" {
			cancel()
		}
	}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	model := &webhookURLModel{
		Name: types.StringValue("tf-acc-hook-int"),
		URL:  types.StringValue("https://example.com/tf-acc-hook-int"),
	}
	resp := createWebhookURL(t, server.URL, ctx, model)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got webhookURLModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.ID.ValueString() == "" || got.Name.ValueString() != model.Name.ValueString() || got.URL.ValueString() != model.URL.ValueString() {
		t.Fatalf("state = id %q name %q url %q", got.ID.ValueString(), got.Name.ValueString(), got.URL.ValueString())
	}
	if len(fake.urls) != 1 {
		t.Fatalf("destinations stored = %d", len(fake.urls))
	}
}

func TestWebhookURLCreateAdoptsMatchingName(t *testing.T) {
	fake := newPlatformFake()
	fake.seedURL("tf-acc-hook-adopt", "https://example.com/tf-acc-hook-adopt")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	model := &webhookURLModel{
		Name: types.StringValue("tf-acc-hook-adopt"),
		URL:  types.StringValue("https://example.com/tf-acc-hook-adopt"),
	}
	resp := createWebhookURL(t, server.URL, context.Background(), model)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got webhookURLModel
	if diags := resp.State.Get(context.Background(), &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.ID.ValueString() != "1" || got.URL.ValueString() != model.URL.ValueString() {
		t.Fatalf("state id=%q url=%q", got.ID.ValueString(), got.URL.ValueString())
	}
	if len(fake.urls) != 1 {
		t.Fatalf("destinations stored = %d, want the existing row only", len(fake.urls))
	}
}

func TestWebhookURLCreateRefusesDifferentURL(t *testing.T) {
	fake := newPlatformFake()
	fake.seedURL("tf-acc-hook-adopt", "https://example.com/other")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	model := &webhookURLModel{
		Name: types.StringValue("tf-acc-hook-adopt"),
		URL:  types.StringValue("https://example.com/tf-acc-hook-adopt"),
	}
	resp := createWebhookURL(t, server.URL, context.Background(), model)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the different destination to be left in place")
	}
	text := resp.Diagnostics.Errors()[0].Detail()
	if !strings.Contains(text, "terraform import vergeio_webhook_url") || !strings.Contains(text, "id 1") {
		t.Fatalf("diagnostic = %s", text)
	}
	if resp.State.Raw.Type() != nil && !resp.State.Raw.IsNull() {
		var got webhookURLModel
		if diags := resp.State.Get(context.Background(), &got); !diags.HasError() && got.ID.ValueString() != "" {
			t.Fatalf("stored id %q for a destination that does not match", got.ID.ValueString())
		}
	}
	if len(fake.urls) != 1 {
		t.Fatalf("destinations stored = %d", len(fake.urls))
	}
}

func createWebhookURL(t *testing.T, host string, ctx context.Context, model *webhookURLModel) *resource.CreateResponse {
	t.Helper()
	item := NewWebhookURLResource()
	configure := &resource.ConfigureResponse{}
	item.(resource.ResourceWithConfigure).Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &resource.SchemaResponse{}
	item.Schema(context.Background(), resource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	configState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := configState.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &resource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	item.Create(ctx, resource.CreateRequest{
		Plan:   plan,
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: configState.Raw},
	}, resp)
	return resp
}
