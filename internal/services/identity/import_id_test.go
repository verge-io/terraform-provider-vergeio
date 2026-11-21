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
		{
			name:     "api key",
			resource: NewAPIKeyResource(),
			summary:  "Invalid API Key Import ID",
			detail:   "Import vergeio_api_key with the API key id.",
		},
		{
			name:     "auth source",
			resource: NewAuthSourceResource(),
			summary:  "Invalid Auth Source Import ID",
			detail:   "Import vergeio_auth_source with the auth source key.",
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
		NewAPIKeyResource(),
		NewAuthSourceResource(),
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

func TestParseUserID(t *testing.T) {
	cases := []struct {
		name    string
		input   types.String
		want    int
		wantErr string
	}{
		{
			name:  "valid integer",
			input: types.StringValue("42"),
			want:  42,
		},
		{
			name:  "trimmed whitespace",
			input: types.StringValue(" 1 "),
			want:  1,
		},
		{
			name:    "non-numeric",
			input:   types.StringValue("abc"),
			wantErr: `user id "abc" is not a positive integer`,
		},
		{
			name:    "zero",
			input:   types.StringValue("0"),
			wantErr: `user id "0" is not a positive integer`,
		},
		{
			name:    "negative",
			input:   types.StringValue("-5"),
			wantErr: `user id "-5" is not a positive integer`,
		},
		{
			name:    "null",
			input:   types.StringNull(),
			wantErr: "user id is empty",
		},
		{
			name:    "unknown",
			input:   types.StringUnknown(),
			wantErr: "user id is empty",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseID(tc.input, "user")
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("parseID(%v) expected error %q, got nil", tc.input, tc.wantErr)
				}
				if err.Error() != tc.wantErr {
					t.Fatalf("parseID(%v) error = %q, want %q", tc.input, err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseID(%v) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("parseID(%v) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}
