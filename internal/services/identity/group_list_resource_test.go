// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/listtest"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestGroupListFiltersNameTagAndIdentity(t *testing.T) {
	var groupFilter string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/groups":
			groupFilter = r.URL.Query().Get("filter")
			_, _ = w.Write([]byte(`[{"$key":2,"name":"web-1","description":"one","enabled":true},{"$key":3,"name":"db","description":"","enabled":true}]`))
		case "/api/v4/groups/2":
			_, _ = w.Write([]byte(`{"$key":2,"name":"web-1","description":"one","enabled":true}`))
		case "/api/v4/tags":
			_, _ = w.Write([]byte(`[{"$key":7,"name":"prod","category":1}]`))
		case "/api/v4/tag_members":
			_, _ = w.Write([]byte(`[{"$key":1,"tag":7,"member":"groups/2"}]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	lister := configuredGroupList(t, server.URL)
	results, err := listtest.Collect(ctx, lister, NewGroupResource(), shared.ListQuery{
		NamePattern: types.StringValue("web-*"),
		Tag:         types.StringValue("prod"),
	}, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].DisplayName != "web-1" {
		t.Fatalf("results = %#v", results)
	}
	id, err := listtest.IdentityID(ctx, results[0])
	if err != nil {
		t.Fatal(err)
	}
	if id != "2" {
		t.Fatalf("identity id = %q", id)
	}
	if results[0].Resource == nil {
		t.Fatal("include_resource should populate the group")
	}
	var model GroupResourceModel
	if diags := results[0].Resource.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Name.ValueString() != "web-1" || model.Id.ValueString() != "2" {
		t.Fatalf("model = %#v", model)
	}
	if groupFilter != "" {
		t.Fatalf("a glob should not send a name filter, got %q", groupFilter)
	}

	exact, err := listtest.Collect(ctx, lister, NewGroupResource(), shared.ListQuery{
		NamePattern: types.StringValue("O'Brien"),
	}, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(exact) != 0 {
		t.Fatalf("exact missing name listed %d", len(exact))
	}
	if groupFilter != `name eq 'O\'Brien'` {
		t.Fatalf("filter = %q", groupFilter)
	}

	_, err = listtest.Collect(ctx, lister, NewGroupResource(), shared.ListQuery{
		Tenant: types.StringValue("1"),
	}, false, 0)
	if err == nil || !strings.Contains(err.Error(), "Tenant filter is not supported") {
		t.Fatalf("tenant filter error = %v", err)
	}
}

func TestGroupImportStateAcceptsIdentityID(t *testing.T) {
	ctx := t.Context()
	r := NewGroupResource()
	importer := r.(resource.ResourceWithImportState)
	schemaResp := &resource.SchemaResponse{}
	r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	ident := &tfsdk.ResourceIdentity{Schema: shared.KeyIdentitySchema("Group key.")}
	if diags := ident.Set(ctx, &struct {
		ID types.String `tfsdk:"id"`
	}{ID: types.StringValue("42")}); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &resource.ImportStateResponse{
		State: tfsdk.State{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
		},
		Identity: &tfsdk.ResourceIdentity{Schema: ident.Schema},
	}
	importer.ImportState(ctx, resource.ImportStateRequest{Identity: ident}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got GroupResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.Id.ValueString() != "42" {
		t.Fatalf("id = %q", got.Id.ValueString())
	}
}

func configuredGroupList(t *testing.T, host string) *GroupListResource {
	t.Helper()
	lister := NewGroupListResource().(*GroupListResource)
	resp := &resource.ConfigureResponse{}
	lister.Configure(t.Context(), resource.ConfigureRequest{ProviderData: vergeio.NewClient(host, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return lister
}
