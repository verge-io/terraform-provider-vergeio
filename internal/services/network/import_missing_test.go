// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

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

func TestNetworkFamilyImportStateRejectsBadID(t *testing.T) {
	cases := []struct {
		name     string
		resource fwresource.Resource
		summary  string
		detail   string
	}{
		{
			name:     "dns view",
			resource: NewNetworkDNSViewResource(),
			summary:  "Invalid Network DNS View Import ID",
			detail:   "Import vergeio_network_dns_view with the view id.",
		},
		{
			name:     "dns zone",
			resource: NewNetworkDNSZoneResource(),
			summary:  "Invalid Network DNS Zone Import ID",
			detail:   "Import vergeio_network_dns_zone with the zone id.",
		},
		{
			name:     "dns record",
			resource: NewNetworkDNSRecordResource(),
			summary:  "Invalid Network DNS Record Import ID",
			detail:   "Import vergeio_network_dns_record with the record id.",
		},
		{
			name:     "ipsec",
			resource: NewNetworkIPSecResource(),
			summary:  "Invalid Network IPsec Import ID",
			detail:   "Import vergeio_network_ipsec with the IPsec configuration id.",
		},
		{
			name:     "ipsec connection",
			resource: NewNetworkIPSecConnectionResource(),
			summary:  "Invalid Network IPsec Connection Import ID",
			detail:   "Import vergeio_network_ipsec_connection with the phase 1 id.",
		},
		{
			name:     "wireguard",
			resource: NewNetworkWireGuardResource(),
			summary:  "Invalid Network WireGuard Import ID",
			detail:   "Import vergeio_network_wireguard with the interface id.",
		},
		{
			name:     "wireguard peer",
			resource: NewNetworkWireGuardPeerResource(),
			summary:  "Invalid Network WireGuard Peer Import ID",
			detail:   "Import vergeio_network_wireguard_peer with the peer id.",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, id := range []string{"", "abc"} {
				resp := importResourceState(t, tc.resource, id)
				if !resp.Diagnostics.HasError() {
					t.Fatalf("import id %q should fail", id)
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
			}
		})
	}
}

func TestNetworkFamilyReadMissingSetsIdentityAndRemovesState(t *testing.T) {
	const id = "987654321"
	resources := []fwresource.Resource{
		NewNetworkDNSViewResource(),
		NewNetworkDNSZoneResource(),
		NewNetworkDNSRecordResource(),
		NewNetworkIPSecResource(),
		NewNetworkIPSecConnectionResource(),
		NewNetworkWireGuardResource(),
		NewNetworkWireGuardPeerResource(),
	}
	ctx := context.Background()
	for _, r := range resources {
		meta := &fwresource.MetadataResponse{}
		r.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
		t.Run(meta.TypeName, func(t *testing.T) {
			imported := importResourceState(t, r, id)
			if imported.Diagnostics.HasError() {
				t.Fatal(imported.Diagnostics)
			}
			var stored types.String
			if diags := imported.State.GetAttribute(ctx, path.Root("id"), &stored); diags.HasError() {
				t.Fatal(diags)
			}
			if stored.ValueString() != id {
				t.Fatalf("id = %q, want %q", stored.ValueString(), id)
			}
			t.Run("import", func(t *testing.T) {
				assertNetworkMissingRead(t, r, id, false)
			})
			t.Run("refresh", func(t *testing.T) {
				assertNetworkMissingRead(t, r, id, true)
			})
		})
	}
}

func assertNetworkMissingRead(t *testing.T, r fwresource.Resource, id string, identitySet bool) {
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

	imported := importResourceState(t, r, id)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	identity := nullNetworkIdentity(t, r.(fwresource.ResourceWithIdentity))
	if identitySet {
		if diags := identity.Set(ctx, &networkIdentityID{ID: types.StringValue(id)}); diags.HasError() {
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
	var got networkIdentityID
	if diags := resp.Identity.Get(ctx, &got); diags.HasError() {
		t.Fatal(diags)
	}
	if got.ID.ValueString() != id {
		t.Fatalf("identity = %q, want %q", got.ID.ValueString(), id)
	}
}

type networkIdentityID struct {
	ID types.String `tfsdk:"id"`
}

func nullNetworkIdentity(t *testing.T, r fwresource.ResourceWithIdentity) *tfsdk.ResourceIdentity {
	t.Helper()
	ctx := context.Background()
	schemaResp := &fwresource.IdentitySchemaResponse{}
	r.IdentitySchema(ctx, fwresource.IdentitySchemaRequest{}, schemaResp)
	return &tfsdk.ResourceIdentity{
		Schema: schemaResp.IdentitySchema,
		Raw:    tftypes.NewValue(schemaResp.IdentitySchema.Type().TerraformType(ctx), nil),
	}
}
