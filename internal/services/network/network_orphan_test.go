// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestNetworkCreateSurvivesCanceledRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	item := newNetworkResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			cancel()
			writeJSONStatus(t, w, http.StatusCreated, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/9":
			writeJSONStatus(t, w, http.StatusOK, networkRowJSON("9", "tf-acc-net-int", false))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs":
			writeJSONStatus(t, w, http.StatusOK, `[]`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	})
	resp := createNetworkResource(t, item, ctx, networkPlan("tf-acc-net-int"))
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	got := networkFromState(t, resp.State)
	if got.Id.ValueString() != "9" || got.Name.ValueString() != "tf-acc-net-int" {
		t.Fatalf("state id=%q name=%q", got.Id.ValueString(), got.Name.ValueString())
	}
}

func TestNetworkCreateKeepsIDWhenReadFails(t *testing.T) {
	var gets int
	item := newNetworkResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			writeJSONStatus(t, w, http.StatusCreated, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/9":
			gets++
			if gets > 1 {
				http.Error(w, `{"err":"read failed"}`, http.StatusInternalServerError)
				return
			}
			writeJSONStatus(t, w, http.StatusOK, networkRowJSON("9", "tf-acc-net-read", false))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs":
			writeJSONStatus(t, w, http.StatusOK, `[]`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	})
	resp := createNetworkResource(t, item, context.Background(), networkPlan("tf-acc-net-read"))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the follow-up read to fail")
	}
	got := networkFromState(t, resp.State)
	if got.Id.ValueString() != "9" || got.Name.ValueString() != "tf-acc-net-read" {
		t.Fatalf("state id=%q name=%q", got.Id.ValueString(), got.Name.ValueString())
	}
}

func TestNetworkCreateAdoptsMatchingName(t *testing.T) {
	posts := 0
	item := newNetworkResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			posts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"err":"This name is already in use"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
			writeJSONStatus(t, w, http.StatusOK, `[`+networkRowJSON("4", "tf-acc-net-adopt", false)+`]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/4":
			writeJSONStatus(t, w, http.StatusOK, networkRowJSON("4", "tf-acc-net-adopt", false))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	})
	resp := createNetworkResource(t, item, context.Background(), networkPlan("tf-acc-net-adopt"))
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	got := networkFromState(t, resp.State)
	if got.Id.ValueString() != "4" || got.Network.ValueString() != "10.251.4.0/24" {
		t.Fatalf("state id=%q network=%q", got.Id.ValueString(), got.Network.ValueString())
	}
	if posts != 1 {
		t.Fatalf("create posts = %d, want 1", posts)
	}
}

func TestNetworkCreateRefusesDifferentCIDR(t *testing.T) {
	item := newNetworkResource(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"err":"This name is already in use"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
			writeJSONStatus(t, w, http.StatusOK, `[`+networkRowJSON("4", "tf-acc-net-adopt", false)+`]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/4":
			row := strings.Replace(networkRowJSON("4", "tf-acc-net-adopt", false), "10.251.4.0/24", "10.9.9.0/24", 1)
			writeJSONStatus(t, w, http.StatusOK, row)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	})
	resp := createNetworkResource(t, item, context.Background(), networkPlan("tf-acc-net-adopt"))
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected the different network to be left in place")
	}
	text := joinDiagnostics(resp.Diagnostics)
	if !strings.Contains(text, "terraform import vergeio_network") || !strings.Contains(text, "id 4") {
		t.Fatalf("diagnostic = %s", text)
	}
	if networkFromState(t, resp.State).Id.ValueString() != "" {
		t.Fatal("mismatched network was stored")
	}
}

func TestNetworkPlanMatchesPower(t *testing.T) {
	stopped := &vergeos.Network{Running: false, Name: "lan", Type: "internal", Network: "10.251.4.0/24", IPAddress: "10.251.4.1"}
	running := &vergeos.Network{Running: true, Name: "lan", Type: "internal", Network: "10.251.4.0/24", IPAddress: "10.251.4.1"}
	planOff := networkPlan("lan")
	if ok, reason := networkPlanMatches(planOff, stopped); !ok {
		t.Fatalf("stopped network should match powerstate false: %s", reason)
	}
	if ok, _ := networkPlanMatches(planOff, running); ok {
		t.Fatal("running network matched powerstate false")
	}
}

func newNetworkResource(t *testing.T, handler http.HandlerFunc) fwresource.Resource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		if r.URL.Path == "/version.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"version":"26.2.2"}`)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	item := NewNetworkResource()
	configure := &fwresource.ConfigureResponse{}
	item.(fwresource.ResourceWithConfigure).Configure(context.Background(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatal(configure.Diagnostics)
	}
	return item
}

func networkPlan(name string) *NetworkResourceModel {
	return &NetworkResourceModel{
		Name:                 types.StringValue(name),
		Type:                 types.StringValue("internal"),
		Network:              types.StringValue("10.251.4.0/24"),
		IPaddress:            types.StringValue("10.251.4.1"),
		PowerState:           types.BoolValue(false),
		Bond_Interfaces_Args: types.ListNull(types.Int32Type),
	}
}

func networkRowJSON(id, name string, running bool) string {
	run := "false"
	if running {
		run = "true"
	}
	return fmt.Sprintf(`{"$key":%s,"name":%q,"type":"internal","network":"10.251.4.0/24","ipaddress":"10.251.4.1","running":%s,"enabled":true}`, id, name, run)
}

func createNetworkResource(t *testing.T, item fwresource.Resource, ctx context.Context, model *NetworkResourceModel) *fwresource.CreateResponse {
	t.Helper()
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(context.Background(), fwresource.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.CreateResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	item.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)
	return resp
}

func networkFromState(t *testing.T, state tfsdk.State) NetworkResourceModel {
	t.Helper()
	if state.Raw.Type() == nil || state.Raw.IsNull() {
		return NetworkResourceModel{}
	}
	var got NetworkResourceModel
	if diags := state.Get(context.Background(), &got); diags.HasError() {
		t.Fatal(diags)
	}
	return got
}

func writeJSONStatus(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}

func joinDiagnostics(diags diag.Diagnostics) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Detail())
		b.WriteByte('\n')
	}
	return b.String()
}
