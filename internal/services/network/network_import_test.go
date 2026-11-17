// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

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

func TestNetworkImportStateEmptyID(t *testing.T) {
	resp := importResourceState(t, NewNetworkResource(), "")
	assertEmptyImportID(t, resp, "Invalid Network Import ID", "Import vergeio_network with the network id.")
}

func TestNetworkImportStateKeepsNonEmptyID(t *testing.T) {
	resp := importResourceState(t, NewNetworkResource(), "abc")
	assertImportIDPassthrough(t, resp, "abc")
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
