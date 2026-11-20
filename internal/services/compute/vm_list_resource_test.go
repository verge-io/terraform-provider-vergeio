// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

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

func TestVMListSkipsSnapshotsAndFiltersName(t *testing.T) {
	var filter string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/vms":
			filter = r.URL.Query().Get("filter")
			_, _ = w.Write([]byte(`[
				{"$key":1,"name":"web-1","is_snapshot":false},
				{"$key":2,"name":"web-1-snap","is_snapshot":true},
				{"$key":3,"name":"db","is_snapshot":false}
			]`))
		case "/api/v4/tenants":
			_, _ = w.Write([]byte(`[{"$key":9,"name":"customer-a"}]`))
		case "/api/v4/tenants/9":
			_, _ = w.Write([]byte(`{"$key":9,"name":"customer-a"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	lister := NewVMListResource().(*VMListResource)
	resp := &resource.ConfigureResponse{}
	lister.Configure(ctx, resource.ConfigureRequest{ProviderData: vergeio.NewClient(server.URL, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	results, err := listtest.Collect(ctx, lister, NewVMResource(), shared.ListQuery{
		NamePattern: types.StringValue("web-*"),
		Tenant:      types.StringValue("customer-a"),
	}, false, 0)
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
	if id != "1" {
		t.Fatalf("identity id = %q", id)
	}
	if filter != "tenant eq 9" {
		t.Fatalf("filter = %q", filter)
	}

	limited, err := listtest.Collect(ctx, lister, NewVMResource(), shared.ListQuery{}, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("limit listed %d", len(limited))
	}
}

func TestVMListIncludeResource(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.0"}`))
		case "/api/v4/vms":
			_, _ = w.Write([]byte(`[
				{"$key":1,"name":"web-1","is_snapshot":false,"machine":3},
				{"$key":2,"name":"web-1-snap","is_snapshot":true,"machine":4}
			]`))
		case "/api/v4/vms/1":
			_, _ = w.Write([]byte(`{"$key":1,"machine":3,"name":"web-1","description":"app","enabled":true,"machine_type":"pc-q35-10.0","cpu_cores":2,"ram":2048,"guest_agent":true,"powerstate":false,"cloudinit_datasource":"none"}`))
		case "/api/v4/cloudinit_files":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	lister := NewVMListResource().(*VMListResource)
	resp := &resource.ConfigureResponse{}
	lister.Configure(ctx, resource.ConfigureRequest{ProviderData: vergeio.NewClient(server.URL, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	results, err := listtest.Collect(ctx, lister, NewVMResource(), shared.ListQuery{
		NamePattern: types.StringValue("web-*"),
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
	if id != "1" {
		t.Fatalf("identity id = %q", id)
	}
	if results[0].Resource == nil {
		t.Fatal("include_resource returned no VM")
	}
	var model VMResourceModel
	if diags := results[0].Resource.Get(ctx, &model); diags.HasError() {
		t.Fatal(diags)
	}
	if model.Name.ValueString() != "web-1" || model.Id.ValueString() != "1" || model.CPUCores.ValueInt32() != 2 {
		t.Fatalf("model = %#v", model)
	}
	if !model.GuestAgentIPs.IsNull() || !model.GuestAgentIPs.ElementType(ctx).Equal(types.StringType) {
		t.Fatalf("guest_agent_ips = %s", model.GuestAgentIPs)
	}
}

func TestReadVMKeepsGuestAgentIPs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.2.0"}`))
		case "/api/v4/vms/1":
			_, _ = w.Write([]byte(`{"$key":1,"machine":3,"name":"web-1","enabled":true,"cpu_cores":1,"ram":512,"powerstate":false}`))
		case "/api/v4/cloudinit_files":
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	ctx := t.Context()
	ips, diags := types.ListValueFrom(ctx, types.StringType, []string{"10.0.0.8"})
	if diags.HasError() {
		t.Fatal(diags)
	}
	api := mustAPI(NewVMApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &VMResourceModel{
		Id:            types.StringValue("1"),
		GuestAgentIPs: ips,
	}
	if _, err := api.readVM(ctx, data, false); err != nil {
		t.Fatal(err)
	}
	var got []string
	if diags := data.GuestAgentIPs.ElementsAs(ctx, &got, false); diags.HasError() {
		t.Fatal(diags)
	}
	if len(got) != 1 || got[0] != "10.0.0.8" {
		t.Fatalf("guest_agent_ips = %v", got)
	}
}
