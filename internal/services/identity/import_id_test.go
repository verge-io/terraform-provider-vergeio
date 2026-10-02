// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestIdentityImportStateEmptyID(t *testing.T) {
	cases := []struct {
		name     string
		resource fwresource.Resource
		summary  string
		detail   string
	}{
		{
			name:     "user",
			resource: NewUserResource(),
			summary:  "Invalid User Import ID",
			detail:   "Import vergeio_user with the user id.",
		},
		{
			name:     "group",
			resource: NewGroupResource(),
			summary:  "Invalid Group Import ID",
			detail:   "Import vergeio_group with the group key.",
		},
		{
			name:     "member",
			resource: NewMemberResource(),
			summary:  "Invalid Member Import ID",
			detail:   "Import vergeio_member with the member id.",
		},
		{
			name:     "permission",
			resource: NewPermissionResource(),
			summary:  "Invalid Permission Import ID",
			detail:   "Import vergeio_permission with the permission key.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := importResourceState(t, tc.resource, "")
			assertEmptyImportID(t, resp, tc.summary, tc.detail)
		})
	}
}

func TestIdentityImportStateKeepsNonEmptyID(t *testing.T) {
	for _, r := range []fwresource.Resource{
		NewUserResource(),
		NewGroupResource(),
		NewMemberResource(),
		NewPermissionResource(),
	} {
		resp := importResourceState(t, r, "abc")
		assertImportIDPassthrough(t, resp, "abc")
	}
}

func importResourceState(t *testing.T, r fwresource.Resource, id string) *fwresource.ImportStateResponse {
	t.Helper()
	ctx := context.Background()
	schemaResp := &fwresource.SchemaResponse{}
	r.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)
	resp := &fwresource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	r.(fwresource.ResourceWithImportState).ImportState(ctx, fwresource.ImportStateRequest{ID: id}, resp)
	return resp
}

func assertEmptyImportID(t *testing.T, resp *fwresource.ImportStateResponse, summary, detail string) {
	t.Helper()
	if !resp.Diagnostics.HasError() {
		t.Fatal("empty import id should fail")
	}
	got := resp.Diagnostics.Errors()[0]
	if got.Summary() != summary {
		t.Fatalf("summary = %q, want %q", got.Summary(), summary)
	}
	if got.Detail() != detail {
		t.Fatalf("detail = %q, want %q", got.Detail(), detail)
	}
	text := got.Summary() + "\n" + got.Detail()
	if strings.Contains(text, "Value Conversion Error") || strings.Contains(text, "This is always an error in the provider") {
		t.Fatalf("empty import id returned a framework conversion error: %s", text)
	}
}

func assertImportIDPassthrough(t *testing.T, resp *fwresource.ImportStateResponse, id string) {
	t.Helper()
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got types.String
	if diags := resp.State.GetAttribute(context.Background(), path.Root("id"), &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.ValueString() != id {
		t.Fatalf("id = %q, want %q", got.ValueString(), id)
	}
}
