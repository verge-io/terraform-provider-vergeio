// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/listtest"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestNetworkListIncludeResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.0"}`))
		case "/api/v4/vnets":
			_, _ = w.Write([]byte(`[
				{"$key":4,"name":"lan","description":"inside","enabled":true,"type":"internal","network":"192.168.0.0/24","ipaddress":"192.168.0.1","mtu":1500,"layer2_id":0,"layer2_type":"none","interface_vnet":0,"enable_bonding":false,"running":false,"need_restart":false},
				{"$key":5,"name":"wan","description":"uplink","enabled":true,"type":"external","enable_bonding":true,"bond_interfaces_args":[4,5],"running":true,"need_restart":false}
			]`))
		case "/api/v4/vnets/4":
			_, _ = w.Write([]byte(`{"$key":4,"name":"lan","description":"inside","enabled":true,"type":"internal","network":"192.168.0.0/24","ipaddress":"192.168.0.1","ipaddress_type":"static","mtu":1500,"layer2_id":0,"layer2_type":"none","interface_vnet":0,"enable_bonding":false,"running":false,"need_restart":false,"on_power_loss":"last_state"}`))
		case "/api/v4/vnets/5":
			_, _ = w.Write([]byte(`{"$key":5,"name":"wan","description":"uplink","enabled":true,"type":"external","enable_bonding":true,"bond_interfaces_args":[4,5],"running":true,"need_restart":false,"ipaddress_type":"static","on_power_loss":"last_state"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	lister := configuredNetworkList(t, server.URL)
	results, err := listtest.Collect(ctx, lister, NewNetworkResource(), shared.ListQuery{
		NamePattern: types.StringValue("lan"),
	}, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].DisplayName != "lan" {
		t.Fatalf("results = %#v", results)
	}
	id, err := listtest.IdentityID(ctx, results[0])
	if err != nil {
		t.Fatal(err)
	}
	if id != "4" {
		t.Fatalf("identity id = %q", id)
	}
	if results[0].Resource == nil {
		t.Fatal("include_resource returned no network")
	}
	var model NetworkResourceModel
	if diags := results[0].Resource.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Name.ValueString() != "lan" || model.Id.ValueString() != "4" || model.Network.ValueString() != "192.168.0.0/24" {
		t.Fatalf("model = %#v", model)
	}
	if !model.Bond_Interfaces_Args.IsNull() || !model.Bond_Interfaces_Args.ElementType(ctx).Equal(types.Int32Type) {
		t.Fatalf("bond_interfaces_args = %s", model.Bond_Interfaces_Args)
	}

	bonded, err := listtest.Collect(ctx, lister, NewNetworkResource(), shared.ListQuery{
		NamePattern: types.StringValue("wan"),
	}, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bonded) != 1 {
		t.Fatalf("bonded results = %#v", bonded)
	}
	var wan NetworkResourceModel
	if diags := bonded[0].Resource.Get(ctx, &wan); diags.HasError() {
		t.Fatal(diags)
	}
	var got []int32
	if diags := wan.Bond_Interfaces_Args.ElementsAs(ctx, &got, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("bond_interfaces_args = %v", got)
	}
}

func TestReadNetworkKeepsConfiguredBondInterfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.0"}`))
		case "/api/v4/vnets/5":
			_, _ = w.Write([]byte(`{"$key":5,"name":"wan","enabled":true,"type":"external","enable_bonding":true,"bond_interfaces_args":[9],"running":false,"ipaddress_type":"static"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	configured, diags := types.ListValueFrom(ctx, types.Int32Type, []int32{4, 5})
	if diags.HasError() {
		t.Fatal(diags)
	}
	api := mustAPI(NewNetworkApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &NetworkResourceModel{
		Id:                   types.StringValue("5"),
		Bond_Interfaces_Args: configured,
	}
	if err := api.readNetwork(ctx, data); err != nil {
		t.Fatal(err)
	}
	var got []int32
	if diags := data.Bond_Interfaces_Args.ElementsAs(ctx, &got, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(got) != 2 || got[0] != 4 || got[1] != 5 {
		t.Fatalf("bond_interfaces_args = %v, want configured [4 5]", got)
	}
}

func configuredNetworkList(t *testing.T, host string) *NetworkListResource {
	t.Helper()
	lister := NewNetworkListResource().(*NetworkListResource)
	resp := &resource.ConfigureResponse{}
	lister.Configure(t.Context(), resource.ConfigureRequest{ProviderData: vergeio.NewClient(host, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return lister
}
