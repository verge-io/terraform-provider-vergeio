// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"terraform-provider-vergeio/internal/client"
)

const (
	missingPositiveID = "987654321"
	missingSHA1ID     = "0123456789abcdef0123456789abcdef01234567"
)

func TestNASImportStateRejectsBadID(t *testing.T) {
	cases := []struct {
		name     string
		resource fwresource.Resource
		id       string
		summary  string
		detail   string
	}{
		{
			name:     "service empty",
			resource: NewServiceResource(),
			summary:  "Invalid NAS Service Import ID",
			detail:   "Import vergeio_nas_service with the service id, a positive integer.",
		},
		{
			name:     "service letters",
			resource: NewServiceResource(),
			id:       "abc",
			summary:  "Invalid NAS Service Import ID",
			detail:   "Import vergeio_nas_service with the service id, a positive integer.",
		},
		{
			name:     "volume empty",
			resource: NewVolumeResource(),
			summary:  "Invalid NAS Volume Import ID",
			detail:   "Import vergeio_nas_volume with the volume id, a 40 character SHA1 hex string.",
		},
		{
			name:     "volume letters",
			resource: NewVolumeResource(),
			id:       "abc",
			summary:  "Invalid NAS Volume Import ID",
			detail:   "Import vergeio_nas_volume with the volume id, a 40 character SHA1 hex string.",
		},
		{
			name:     "volume digits",
			resource: NewVolumeResource(),
			id:       missingPositiveID,
			summary:  "Invalid NAS Volume Import ID",
			detail:   "Import vergeio_nas_volume with the volume id, a 40 character SHA1 hex string.",
		},
		{
			name:     "cifs letters",
			resource: NewCIFSShareResource(),
			id:       "abc",
			summary:  "Invalid NAS CIFS Share Import ID",
			detail:   "Import vergeio_nas_cifs_share with the share id, a 40 character SHA1 hex string.",
		},
		{
			name:     "nfs letters",
			resource: NewNFSShareResource(),
			id:       "abc",
			summary:  "Invalid NAS NFS Share Import ID",
			detail:   "Import vergeio_nas_nfs_share with the share id, a 40 character SHA1 hex string.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := importNASState(t, tc.resource, tc.id)
			if !resp.Diagnostics.HasError() {
				t.Fatal("import id should fail")
			}
			got := resp.Diagnostics.Errors()[0]
			if got.Summary() != tc.summary {
				t.Fatalf("summary = %q, want %q", got.Summary(), tc.summary)
			}
			if got.Detail() != tc.detail {
				t.Fatalf("detail = %q, want %q", got.Detail(), tc.detail)
			}
			text := got.Summary() + "\n" + got.Detail()
			if strings.Contains(text, "Value Conversion Error") || strings.Contains(text, "Missing Resource Identity") || strings.Contains(text, "This is always an error in the provider") || strings.Contains(text, "This is always an issue in the Terraform Provider") {
				t.Fatalf("import id returned a framework error: %s", text)
			}
		})
	}
}

func TestNASImportStateKeepsWellFormedID(t *testing.T) {
	cases := []struct {
		resource fwresource.Resource
		id       string
	}{
		{NewServiceResource(), missingPositiveID},
		{NewVolumeResource(), missingSHA1ID},
		{NewCIFSShareResource(), missingSHA1ID},
		{NewNFSShareResource(), missingSHA1ID},
	}
	for _, tc := range cases {
		resp := importNASState(t, tc.resource, tc.id)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		var got types.String
		if diags := resp.State.GetAttribute(context.Background(), path.Root("id"), &got); diags.HasError() {
			t.Fatal(diags)
		}
		if got.ValueString() != tc.id {
			t.Fatalf("id = %q, want %q", got.ValueString(), tc.id)
		}
	}
}

func TestNASReadMissingSetsIdentityAndRemovesState(t *testing.T) {
	cases := []struct {
		name     string
		resource fwresource.Resource
		id       string
	}{
		{"service", NewServiceResource(), missingPositiveID},
		{"volume", NewVolumeResource(), missingSHA1ID},
		{"cifs", NewCIFSShareResource(), missingSHA1ID},
		{"nfs", NewNFSShareResource(), missingSHA1ID},
	}
	for _, tc := range cases {
		t.Run(tc.name+"/import", func(t *testing.T) {
			assertMissingRead(t, tc.resource, tc.id, false)
		})
		t.Run(tc.name+"/refresh", func(t *testing.T) {
			assertMissingRead(t, tc.resource, tc.id, true)
		})
	}
}

func assertMissingRead(t *testing.T, r fwresource.Resource, id string, identitySet bool) {
	t.Helper()
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if vergeio.AnswerCredentialCheck(w, req) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if req.URL.Path == "/version.json" {
			if _, err := w.Write([]byte(`{"version":"26.0.0"}`)); err != nil {
				t.Errorf("write version: %v", err)
			}
			return
		}
		w.WriteHeader(http.StatusNotFound)
		if _, err := w.Write([]byte(`{"err":"not found"}`)); err != nil {
			t.Errorf("write not found: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	configured := &fwresource.ConfigureResponse{}
	r.(fwresource.ResourceWithConfigure).Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configured)
	if configured.Diagnostics.HasError() {
		t.Fatal(configured.Diagnostics)
	}

	imported := importNASState(t, r, id)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	identity := nullNASIdentity(t, r.(fwresource.ResourceWithIdentity))
	if identitySet {
		if diags := identity.Set(ctx, &nasIdentityID{ID: types.StringValue(id)}); diags.HasError() {
			t.Fatal(diags)
		}
	}
	resp := &fwresource.ReadResponse{State: imported.State, Identity: identity}
	r.Read(ctx, fwresource.ReadRequest{State: imported.State, Identity: identity}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing object was kept in state")
	}
	var got nasIdentityID
	if diags := resp.Identity.Get(ctx, &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.ID.ValueString() != id {
		t.Fatalf("identity = %q, want %q", got.ID.ValueString(), id)
	}
}

type nasIdentityID struct {
	ID types.String `tfsdk:"id"`
}

func nullNASIdentity(t *testing.T, r fwresource.ResourceWithIdentity) *tfsdk.ResourceIdentity {
	t.Helper()
	ctx := context.Background()
	schemaResp := &fwresource.IdentitySchemaResponse{}
	r.IdentitySchema(ctx, fwresource.IdentitySchemaRequest{}, schemaResp)
	return &tfsdk.ResourceIdentity{
		Schema: schemaResp.IdentitySchema,
		Raw:    tftypes.NewValue(schemaResp.IdentitySchema.Type().TerraformType(ctx), nil),
	}
}

func importNASState(t *testing.T, r fwresource.Resource, id string) *fwresource.ImportStateResponse {
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
