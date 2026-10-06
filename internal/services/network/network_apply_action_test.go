// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestNetworkApplyActionSchema(t *testing.T) {
	item := NewNetworkApplyAction()
	meta := &action.MetadataResponse{}
	item.Metadata(context.Background(), action.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
	if meta.TypeName != "vergeio_network_apply" {
		t.Fatalf("type = %s", meta.TypeName)
	}
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	if diags := schemaResp.Schema.ValidateImplementation(context.Background()); diags.HasError() {
		t.Fatal(diags)
	}
	networkID := schemaResp.Schema.Attributes["network_id"]
	if !networkID.IsRequired() {
		t.Fatal("network_id should be required")
	}
	target := schemaResp.Schema.Attributes["target"]
	if target.IsRequired() || !target.IsOptional() {
		t.Fatal("target should be optional")
	}
}

func TestNetworkApplyActionValidateConfig(t *testing.T) {
	item := NewNetworkApplyAction()
	if diags := validateNetworkApply(t, item, &networkApplyActionModel{
		NetworkID: types.StringValue("4"),
		Target:    types.StringNull(),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := validateNetworkApply(t, item, &networkApplyActionModel{
		NetworkID: types.StringUnknown(),
		Target:    types.StringValue(networkApplyDNS),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	diags := validateNetworkApply(t, item, &networkApplyActionModel{
		NetworkID: types.StringValue("nope"),
		Target:    types.StringNull(),
	})
	if !diags.HasError() {
		t.Fatal("invalid network_id was accepted")
	}
}

func TestNetworkApplyActionInvoke(t *testing.T) {
	tests := []struct {
		name   string
		target types.String
		want   int
		dns    bool
	}{
		{name: "default rules", target: types.StringNull(), want: 1, dns: false},
		{name: "rules", target: types.StringValue(networkApplyRules), want: 1, dns: false},
		{name: "dns", target: types.StringValue(networkApplyDNS), want: 1, dns: true},
		{name: "all", target: types.StringValue(networkApplyAll), want: 2, dns: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var posts []string
			server := networkApplyServer(t, true, func(body string) {
				posts = append(posts, body)
			})
			item := configuredNetworkApply(t, server.URL)
			resp := &action.InvokeResponse{}
			item.Invoke(context.Background(), action.InvokeRequest{Config: networkApplyConfig(t, item, &networkApplyActionModel{
				NetworkID: types.StringValue("12"),
				Target:    test.target,
			})}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			if len(posts) != test.want {
				t.Fatalf("posts = %#v, want %d", posts, test.want)
			}
			for i, body := range posts {
				if !strings.Contains(body, `"action":"refresh"`) || !strings.Contains(body, `"vnet":12`) {
					t.Fatalf("post %d = %s", i, body)
				}
			}
			lastDNS := strings.Contains(posts[len(posts)-1], `"dnsonly"`)
			if lastDNS != test.dns {
				t.Fatalf("last post dns=%v, body %s", lastDNS, posts[len(posts)-1])
			}
			if test.want == 2 && strings.Contains(posts[0], `"dnsonly"`) {
				t.Fatalf("rules post included dnsonly: %s", posts[0])
			}
		})
	}
}

func TestNetworkApplyActionStoppedNetwork(t *testing.T) {
	var posts int
	server := networkApplyServer(t, false, func(string) { posts++ })
	item := configuredNetworkApply(t, server.URL)
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: networkApplyConfig(t, item, &networkApplyActionModel{
		NetworkID: types.StringValue("12"),
		Target:    types.StringValue(networkApplyRules),
	})}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("stopped network was applied")
	}
	if posts != 0 {
		t.Fatalf("posts = %d, want none", posts)
	}
	if resp.Diagnostics.Errors()[0].Summary() != "Network Is Stopped" {
		t.Fatalf("summary = %s", resp.Diagnostics.Errors()[0].Summary())
	}
}

func TestNetworkApplyActionRequiresClient(t *testing.T) {
	item := &NetworkApplyAction{}
	resp := &action.InvokeResponse{}
	item.Invoke(context.Background(), action.InvokeRequest{Config: networkApplyConfig(t, item, &networkApplyActionModel{
		NetworkID: types.StringValue("12"),
		Target:    types.StringNull(),
	})}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a missing client diagnostic")
	}
}

func validateNetworkApply(t *testing.T, item action.Action, model *networkApplyActionModel) diag.Diagnostics {
	t.Helper()
	withValidate := item.(action.ActionWithValidateConfig)
	resp := &action.ValidateConfigResponse{}
	withValidate.ValidateConfig(context.Background(), action.ValidateConfigRequest{Config: networkApplyConfig(t, item, model)}, resp)
	return resp.Diagnostics
}

func networkApplyConfig(t *testing.T, item action.Action, model *networkApplyActionModel) tfsdk.Config {
	t.Helper()
	schemaResp := &action.SchemaResponse{}
	item.Schema(context.Background(), action.SchemaRequest{}, schemaResp)
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	return tfsdk.Config{Raw: plan.Raw, Schema: schemaResp.Schema}
}

func configuredNetworkApply(t *testing.T, host string) *NetworkApplyAction {
	t.Helper()
	item := &NetworkApplyAction{}
	resp := &action.ConfigureResponse{}
	item.Configure(context.Background(), action.ConfigureRequest{ProviderData: vergeio.NewClient(host, "user", "pass", true)}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}

func networkApplyServer(t *testing.T, running bool, record func(string)) *httptest.Server {
	t.Helper()
	flag := "false"
	if running {
		flag = "true"
	}
	body := `{"$key":12,"name":"lan","running":` + flag + `,"status":"` + map[bool]string{true: "running", false: "stopped"}[running] + `"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			_, _ = w.Write([]byte(body))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			payload, _ := io.ReadAll(r.Body)
			record(string(payload))
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
