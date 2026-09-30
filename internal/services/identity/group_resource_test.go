// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"terraform-provider-vergeio/internal/client"
)

func TestGroupResourceMetadata(t *testing.T) {
	resp := &fwresource.MetadataResponse{}
	NewGroupResource().Metadata(context.Background(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, resp)
	if resp.TypeName != "vergeio_group" {
		t.Fatalf("type = %s", resp.TypeName)
	}
}

func TestGroupResourceSchema(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewGroupResource().Schema(context.Background(), fwresource.SchemaRequest{}, resp)
	name, ok := resp.Schema.Attributes["name"]
	if !ok || !name.IsRequired() {
		t.Fatal("name should be required")
	}
	id, ok := resp.Schema.Attributes["id"]
	if !ok || !id.IsComputed() {
		t.Fatal("id should be computed")
	}
	for _, attr := range []string{"description", "enabled"} {
		got, ok := resp.Schema.Attributes[attr]
		if !ok || !got.IsOptional() {
			t.Fatalf("%s should be optional", attr)
		}
	}
}

func TestGroupResourceConfigure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	resource := &GroupResource{}
	resp := &fwresource.ConfigureResponse{}
	resource.Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() || resource.api == nil || resource.api.Name() != "Group Api" {
		t.Fatalf("configure failed: %v api=%v", resp.Diagnostics, resource.api)
	}

	resource = &GroupResource{}
	resp = &fwresource.ConfigureResponse{}
	resource.Configure(context.Background(), fwresource.ConfigureRequest{ProviderData: "nope"}, resp)
	if !resp.Diagnostics.HasError() || resource.api != nil {
		t.Fatal("invalid client should fail configure")
	}

	resource = &GroupResource{}
	resp = &fwresource.ConfigureResponse{}
	resource.Configure(context.Background(), fwresource.ConfigureRequest{}, resp)
	if resp.Diagnostics.HasError() || resource.api != nil {
		t.Fatal("nil provider data should leave the resource unconfigured")
	}
}

func TestGroupCreateKeepsIDWhenRefreshFails(t *testing.T) {
	var gets atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups":
			_, _ = w.Write([]byte(`{"$key":4}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/4":
			if gets.Add(1) == 1 {
				_, _ = w.Write([]byte(`{"$key":4,"name":"ops","description":"operators","enabled":true}`))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"err":"unavailable"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	group := &GroupResource{}
	configure := &fwresource.ConfigureResponse{}
	group.Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	schemaResp := &fwresource.SchemaResponse{}
	group.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &GroupResourceModel{
		Id:          types.StringNull(),
		Name:        types.StringValue("ops"),
		Description: types.StringValue("operators"),
		Enabled:     types.BoolValue(true),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	group.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the follow-up read to fail")
	}
	if resp.State.Raw.IsNull() || resp.State.Raw.Type().Is(tftypes.DynamicPseudoType) && resp.State.Raw.IsNull() {
		t.Fatal("group id was not kept after the read failed")
	}
	var got GroupResourceModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() && got.Id.ValueString() == "" {
		t.Fatalf("state: %v", resp.Diagnostics)
	}
	if got.Id.ValueString() != "4" {
		t.Fatalf("id = %q, want 4", got.Id.ValueString())
	}
}

func TestGroupReadRemovesMissingGroup(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/groups/9":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	ctx := context.Background()
	group := &GroupResource{}
	configure := &fwresource.ConfigureResponse{}
	group.Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configure)
	schemaResp := &fwresource.SchemaResponse{}
	group.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	diags := state.Set(ctx, &GroupResourceModel{
		Id:          types.StringValue("9"),
		Name:        types.StringValue("ops"),
		Description: types.StringNull(),
		Enabled:     types.BoolNull(),
	})
	if diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.ReadResponse{State: state}
	group.Read(ctx, fwresource.ReadRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing group should be removed from state")
	}
}
