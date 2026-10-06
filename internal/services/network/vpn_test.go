// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestVPNSchemasDocumentDestroyOrder(t *testing.T) {
	cases := []struct {
		name string
		new  func() fwresource.Resource
		want []string
	}{
		{
			name: "ipsec",
			new:  NewNetworkIPSecResource,
			want: []string{"phase 2", "phase 1", "vergeio_network.id", "fail permanently"},
		},
		{
			name: "connection",
			new:  NewNetworkIPSecConnectionResource,
			want: []string{"phase 2", "phase 1", "vnet_ipsec_connections", "keeps failing"},
		},
		{
			name: "wireguard",
			new:  NewNetworkWireGuardResource,
			want: []string{"Accept WireGuard", "apply", "peer", "disables the interface"},
		},
		{
			name: "peer",
			new:  NewNetworkWireGuardPeerResource,
			want: []string{"wireguard_id", "Accept WireGuard", "site-to-site"},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			item := tt.new()
			resp := &fwresource.SchemaResponse{}
			item.Schema(t.Context(), fwresource.SchemaRequest{}, resp)
			for _, want := range tt.want {
				if !strings.Contains(resp.Schema.MarkdownDescription, want) {
					t.Errorf("description missing %q: %s", want, resp.Schema.MarkdownDescription)
				}
			}
			meta := &fwresource.MetadataResponse{}
			item.Metadata(t.Context(), fwresource.MetadataRequest{ProviderTypeName: "vergeio"}, meta)
			if !strings.HasPrefix(meta.TypeName, "vergeio_network_") {
				t.Fatalf("type name = %s", meta.TypeName)
			}
		})
	}

	network := &NetworkResource{}
	resp := &fwresource.SchemaResponse{}
	network.Schema(t.Context(), fwresource.SchemaRequest{}, resp)
	if !strings.Contains(resp.Schema.MarkdownDescription, "IPsec or WireGuard") {
		t.Fatalf("network description = %s", resp.Schema.MarkdownDescription)
	}
}

func TestNetworkIPSecImportRejectsName(t *testing.T) {
	resp := &fwresource.ImportStateResponse{}
	NewNetworkIPSecResource().(fwresource.ResourceWithImportState).ImportState(t.Context(), fwresource.ImportStateRequest{ID: "branch"}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected a rejected import id")
	}
}

func TestDeleteNetworkVPNRowsOrdersTeardown(t *testing.T) {
	var calls []string
	phase1Deleted := false
	phase2Deleted := map[string]bool{}
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete || r.Method == http.MethodPut || (r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions") {
			calls = append(calls, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards":
			writeBody(w, `[{"$key":3,"vnet":12,"name":"wg0","enabled":true}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0","enabled":true}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/3":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"enabled":false`) {
				t.Errorf("disable body = %s", body)
			}
			writeBody(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			writeBody(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":true,"status":"running"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[{"$key":9,"wireguard":3,"name":"office"}]`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguard_peers/9":
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs" && !strings.Contains(r.URL.Path, "/api/v4/vnet_ipsecs/"):
			writeBody(w, `[{"$key":5,"vnet":12,"enabled":true,"mode":"normal"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeBody(w, `{"$key":5,"vnet":12,"enabled":true,"mode":"normal"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			if phase1Deleted {
				writeBody(w, `[]`)
				return
			}
			writeBody(w, `[{"$key":7,"ipsec":5,"name":"branch","remote_gateway":"203.0.113.10"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			var left []string
			if !phase2Deleted["/api/v4/vnet_ipsec_phase2s/8"] {
				left = append(left, `{"$key":8,"phase1":7,"name":"lan","local":"192.168.0.0/24","remote":"198.51.100.0/24"}`)
			}
			if !phase2Deleted["/api/v4/vnet_ipsec_phase2s/11"] {
				left = append(left, `{"$key":11,"phase1":7,"name":"extra","local":"192.168.1.0/24","remote":"198.51.100.0/24"}`)
			}
			writeBody(w, "["+strings.Join(left, ",")+"]")
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/vnet_ipsec_phase2s/"):
			phase2Deleted[r.URL.Path] = true
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7":
			phase1Deleted = true
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	if err := DeleteNetworkVPNRows(t.Context(), api.sdk, 12); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DELETE /api/v4/vnet_wireguard_peers/9",
		"PUT /api/v4/vnet_wireguards/3",
		"POST /api/v4/vnet_actions",
		"DELETE /api/v4/vnet_wireguards/3",
		"DELETE /api/v4/vnet_ipsec_phase2s/8",
		"DELETE /api/v4/vnet_ipsec_phase2s/11",
		"DELETE /api/v4/vnet_ipsec_phase1s/7",
		"DELETE /api/v4/vnet_ipsecs/5",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("delete order = %v, want %v", calls, want)
	}
}

func TestPhase2DeleteFailureLeavesPhase1(t *testing.T) {
	var calls []string
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			calls = append(calls, r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeBody(w, `[{"$key":8,"phase1":7,"name":"lan","local":"192.168.0.0/24"}]`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8":
			http.Error(w, "trigger failed", http.StatusInternalServerError)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	err := api.deleteIPSecConnection(t.Context(), &ipsecConnectionModel{ID: types.StringValue("7")})
	if err == nil {
		t.Fatal("expected phase 2 delete to fail")
	}
	if !strings.Contains(err.Error(), "phase 1 7 was left in place") {
		t.Fatalf("error = %v", err)
	}
	if len(calls) != 1 || calls[0] != "/api/v4/vnet_ipsec_phase2s/8" {
		t.Fatalf("deletes = %v, want only the phase 2", calls)
	}
}

func TestDeleteIPSecRefusesWhilePhase1Remains(t *testing.T) {
	var deleted bool
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			writeBody(w, `[{"$key":7,"ipsec":5,"name":"branch"}]`)
		case r.Method == http.MethodDelete:
			deleted = true
			unexpectedAPI(t, w, r)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	err := api.deleteIPSec(t.Context(), &ipsecModel{ID: types.StringValue("5")})
	if err == nil || !strings.Contains(err.Error(), "phase 1") {
		t.Fatalf("error = %v", err)
	}
	if deleted {
		t.Fatal("IPsec configuration was deleted while phase 1 remained")
	}
}

func TestPhase2CreateFailureRemovesPhase2BeforePhase1(t *testing.T) {
	var calls []string
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			calls = append(calls, r.URL.Path)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			writeBody(w, `{"$key":7}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7":
			writeBody(w, `{"$key":7,"ipsec":5,"name":"branch","remote_gateway":"203.0.113.10"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeBody(w, `{"$key":8}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8":
			http.Error(w, "read failed", http.StatusInternalServerError)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeBody(w, `[{"$key":8,"phase1":7,"name":"lan","local":"192.168.0.0/24"}]`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8":
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7":
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	err := api.createIPSecConnection(t.Context(), &ipsecConnectionModel{
		IPSecID:       types.StringValue("5"),
		Name:          types.StringValue("branch"),
		RemoteGateway: types.StringValue("203.0.113.10"),
		Phase2: &ipsecPhase2Model{
			Name:  types.StringValue("lan"),
			Local: types.StringValue("192.168.0.0/24"),
		},
	})
	if err == nil {
		t.Fatal("expected phase 2 create to fail")
	}
	want := []string{"/api/v4/vnet_ipsec_phase2s/8", "/api/v4/vnet_ipsec_phase1s/7"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("cleanup deletes = %v, want phase 2 then phase 1", calls)
	}
}

func TestCreateIPSecConnectionReadsStatus(t *testing.T) {
	var sawStatus bool
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			writeBody(w, `{"$key":7}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7":
			writeBody(w, `{"$key":7,"ipsec":5,"name":"branch","enabled":true,"remote_gateway":"203.0.113.10","keyexchange":"ikev2","auth":"psk","auto":"start"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeBody(w, `{"$key":8}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8":
			writeBody(w, `{"$key":8,"phase1":7,"name":"lan","enabled":true,"mode":"tunnel","local":"192.168.0.0/24","remote":"198.51.100.0/24","protocol":"esp"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeBody(w, `[{"$key":8,"phase1":7,"name":"lan","enabled":true,"mode":"tunnel","local":"192.168.0.0/24","remote":"198.51.100.0/24","protocol":"esp"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_connections":
			sawStatus = true
			writeBody(w, `[]`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	data := &ipsecConnectionModel{
		IPSecID:       types.StringValue("5"),
		Name:          types.StringValue("branch"),
		Enabled:       types.BoolValue(true),
		RemoteGateway: types.StringValue("203.0.113.10"),
		PSK:           types.StringValue("secret-key"),
		Phase2: &ipsecPhase2Model{
			Name:     types.StringValue("lan"),
			Enabled:  types.BoolValue(true),
			Mode:     types.StringValue("tunnel"),
			Local:    types.StringValue("192.168.0.0/24"),
			Remote:   types.StringValue("198.51.100.0/24"),
			Protocol: types.StringValue("esp"),
		},
	}
	if err := api.createIPSecConnection(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if !sawStatus {
		t.Fatal("create did not read vnet_ipsec_connections")
	}
	if data.ID.ValueString() != "7" || data.Phase2.ID.ValueString() != "8" {
		t.Fatalf("ids = %s / %s", data.ID.ValueString(), data.Phase2.ID.ValueString())
	}
	if data.PSK.ValueString() != "secret-key" {
		t.Fatalf("psk = %q, want the configured value kept", data.PSK.ValueString())
	}
}

func TestPhase1CreateRequestSendsIKE(t *testing.T) {
	unset, err := phase1CreateRequest(&ipsecConnectionModel{
		IPSecID:       types.StringValue("5"),
		Name:          types.StringValue("branch"),
		RemoteGateway: types.StringValue("203.0.113.10"),
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(unset)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"ike":"aes256-sha256-modp2048"`) {
		t.Fatalf("create body = %s, want the required ike proposal", body)
	}

	custom := "aes128-sha256-modp2048"
	set, err := phase1CreateRequest(&ipsecConnectionModel{
		IPSecID:       types.StringValue("5"),
		Name:          types.StringValue("branch"),
		RemoteGateway: types.StringValue("203.0.113.10"),
		IKE:           types.StringValue(custom),
	})
	if err != nil {
		t.Fatal(err)
	}
	if set.IKE == nil || *set.IKE != custom {
		t.Fatalf("ike = %#v, want %s", set.IKE, custom)
	}
}

func TestCreateWireGuardAppliesFirewall(t *testing.T) {
	var calls []string
	var createBody string
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_wireguards":
			body, _ := io.ReadAll(r.Body)
			createBody = string(body)
			writeBody(w, `{"$key":3}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0","enabled":true,"ip":"192.168.255.1/24","listenport":51820,"public_key":"pub"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":true,"status":"running","need_fw_apply":true}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"action":"refresh"`) || !strings.Contains(string(body), `"vnet":12`) {
				t.Errorf("apply body = %s", body)
			}
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	data := &wireGuardModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("wg0"),
		IP:        types.StringValue("192.168.255.1/24"),
		Enabled:   types.BoolValue(true),
		Apply:     types.BoolValue(true),
	}
	notice, err := api.createWireGuard(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("notice = %#v, want the rules applied", notice)
	}
	if !strings.Contains(createBody, `"auto_apply_firewall":true`) {
		t.Fatalf("create body = %s", createBody)
	}
	if data.PublicKey.ValueString() != "pub" {
		t.Fatalf("public key = %q", data.PublicKey.ValueString())
	}
	if !containsCall(calls, "POST /api/v4/vnet_actions") {
		t.Fatalf("calls = %v, want a firewall refresh", calls)
	}
}

func TestCreateWireGuardLeavesRulesStagedWhenApplyIsFalse(t *testing.T) {
	var refreshed bool
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_wireguards":
			body, _ := io.ReadAll(r.Body)
			if strings.Contains(string(body), `"auto_apply_firewall":true`) {
				t.Errorf("apply false still asked the platform to apply: %s", body)
			}
			writeBody(w, `{"$key":3}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0","enabled":true,"ip":"192.168.255.1/24","listenport":51820}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":true,"status":"running","need_fw_apply":true}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			refreshed = true
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	notice, err := api.createWireGuard(t.Context(), &wireGuardModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("wg0"),
		IP:        types.StringValue("192.168.255.1/24"),
		Apply:     types.BoolValue(false),
	})
	if err != nil {
		t.Fatal(err)
	}
	if refreshed {
		t.Fatal("apply false refreshed the network")
	}
	if notice == nil || !strings.Contains(notice.Detail, "Accept WireGuard") {
		t.Fatalf("notice = %#v", notice)
	}
}

func TestCreateWireGuardPeerAppliesFirewall(t *testing.T) {
	var refreshed bool
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers/9":
			writeBody(w, `{"$key":9,"wireguard":3,"name":"office","enabled":true,"peer_ip":"192.168.255.2","public_key":"cHVibGlj","allowed_ips":"10.0.0.0/24","configure_firewall":"site-to-site","port":51820}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":true,"status":"running","need_fw_apply":true}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			refreshed = true
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	notice, err := api.createWireGuardPeer(t.Context(), &wireGuardPeerModel{
		WireGuardID:       types.StringValue("3"),
		Name:              types.StringValue("office"),
		PeerIP:            types.StringValue("192.168.255.2"),
		PublicKey:         types.StringValue("cHVibGlj"),
		AllowedIPs:        types.StringValue("10.0.0.0/24"),
		ConfigureFirewall: types.StringValue("site-to-site"),
		Apply:             types.BoolValue(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil || !refreshed {
		t.Fatalf("notice=%#v refreshed=%v", notice, refreshed)
	}
}

func TestIPSecPhase2IsABlock(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewNetworkIPSecConnectionResource().Schema(t.Context(), fwresource.SchemaRequest{}, resp)
	if _, ok := resp.Schema.Attributes["phase2"]; ok {
		t.Fatal("phase2 is an attribute; vnet_ipsec_phase2s is a separate row and the configuration is a block")
	}
	block, ok := resp.Schema.Blocks["phase2"].(schema.SingleNestedBlock)
	if !ok {
		t.Fatalf("phase2 type = %T", resp.Schema.Blocks["phase2"])
	}
	if len(block.Validators) == 0 {
		t.Fatal("phase2 block is not required")
	}
}

func TestWireGuardFirewallFieldsRequireReplace(t *testing.T) {
	resp := &fwresource.SchemaResponse{}
	NewNetworkWireGuardResource().Schema(t.Context(), fwresource.SchemaRequest{}, resp)
	firewall, ok := resp.Schema.Attributes["configure_firewall"].(schema.BoolAttribute)
	if !ok || !boolRequiresReplace(t, firewall.PlanModifiers) {
		t.Fatal("configure_firewall change should replace the interface")
	}
	external, ok := resp.Schema.Attributes["external_ip"].(schema.StringAttribute)
	if !ok || !stringRequiresReplace(t, external.PlanModifiers) {
		t.Fatal("external_ip change should replace the interface")
	}
}

func TestCreateIPSecRemovesRowWhenAdvancedUpdateFails(t *testing.T) {
	var deleted bool
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_ipsecs":
			writeBody(w, `{"$key":5}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeBody(w, `{"$key":5,"vnet":12,"enabled":true,"mode":"normal"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			http.Error(w, "advanced failed", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			deleted = true
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	err := api.createIPSec(t.Context(), &ipsecModel{
		NetworkID:  types.StringValue("12"),
		CiscoUnity: types.BoolValue(true),
	})
	if err == nil || !strings.Contains(err.Error(), "row was removed") {
		t.Fatalf("error = %v", err)
	}
	if !deleted {
		t.Fatal("created IPsec row was left in place")
	}
}

func TestCreateIPSecConnectionRemovesRowsWhenReadFails(t *testing.T) {
	phase1Reads := 0
	var calls []string
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			calls = append(calls, r.URL.Path)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			writeBody(w, `{"$key":7}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7":
			phase1Reads++
			if phase1Reads > 1 {
				http.Error(w, "read failed", http.StatusInternalServerError)
				return
			}
			writeBody(w, `{"$key":7,"ipsec":5,"name":"branch","enabled":true,"remote_gateway":"203.0.113.10"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeBody(w, `{"$key":8}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8":
			writeBody(w, `{"$key":8,"phase1":7,"name":"lan","local":"192.168.0.0/24"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeBody(w, `[{"$key":8,"phase1":7,"name":"lan","local":"192.168.0.0/24"}]`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8":
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7":
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	err := api.createIPSecConnection(t.Context(), &ipsecConnectionModel{
		IPSecID:       types.StringValue("5"),
		Name:          types.StringValue("branch"),
		RemoteGateway: types.StringValue("203.0.113.10"),
		Phase2: &ipsecPhase2Model{
			Name:  types.StringValue("lan"),
			Local: types.StringValue("192.168.0.0/24"),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "rows were removed") {
		t.Fatalf("error = %v", err)
	}
	want := []string{"/api/v4/vnet_ipsec_phase2s/8", "/api/v4/vnet_ipsec_phase1s/7"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("cleanup deletes = %v, want phase 2 then phase 1", calls)
	}
}

func TestCreateWireGuardRemovesInterfaceWhenApplyFails(t *testing.T) {
	var deleted bool
	networkReads := 0
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_wireguards":
			writeBody(w, `{"$key":3}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0","enabled":true,"ip":"192.168.255.1/24"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			networkReads++
			if networkReads == 1 {
				http.Error(w, "network read failed", http.StatusInternalServerError)
				return
			}
			writeBody(w, `{"$key":12,"name":"lan","running":true,"status":"running"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/3":
			deleted = true
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	_, err := api.createWireGuard(t.Context(), &wireGuardModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("wg0"),
		IP:        types.StringValue("192.168.255.1/24"),
		Apply:     types.BoolValue(true),
	})
	if err == nil || !strings.Contains(err.Error(), "row was removed") {
		t.Fatalf("error = %v", err)
	}
	if !deleted {
		t.Fatal("created WireGuard interface was left in place")
	}
}

func TestCreateWireGuardPeerRemovesPeerWhenApplyFails(t *testing.T) {
	var deleted bool
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `{"$key":9}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers/9":
			writeBody(w, `{"$key":9,"wireguard":3,"name":"office","enabled":true,"peer_ip":"192.168.255.2","public_key":"cHVibGlj","allowed_ips":"10.0.0.0/24"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			http.Error(w, "network read failed", http.StatusInternalServerError)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguard_peers/9":
			deleted = true
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	_, err := api.createWireGuardPeer(t.Context(), &wireGuardPeerModel{
		WireGuardID: types.StringValue("3"),
		Name:        types.StringValue("office"),
		PeerIP:      types.StringValue("192.168.255.2"),
		PublicKey:   types.StringValue("cHVibGlj"),
		AllowedIPs:  types.StringValue("10.0.0.0/24"),
		Apply:       types.BoolValue(true),
	})
	if err == nil || !strings.Contains(err.Error(), "row was removed") {
		t.Fatalf("error = %v", err)
	}
	if !deleted {
		t.Fatal("created WireGuard peer was left in place")
	}
}

func TestDeleteWireGuardDisablesAndApplies(t *testing.T) {
	var calls []string
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			calls = append(calls, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0","enabled":true}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/3":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"enabled":false`) {
				t.Errorf("disable body = %s", body)
			}
			writeBody(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":true,"status":"running"}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	notice, err := api.deleteWireGuard(t.Context(), &wireGuardModel{
		ID:        types.StringValue("3"),
		NetworkID: types.StringValue("12"),
		Apply:     types.BoolValue(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("notice = %#v", notice)
	}
	want := []string{
		"PUT /api/v4/vnet_wireguards/3",
		"POST /api/v4/vnet_actions",
		"DELETE /api/v4/vnet_wireguards/3",
		"POST /api/v4/vnet_actions",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want disable, apply, delete, apply", calls)
	}
}

func TestDeleteWireGuardRetriesUntilApplied(t *testing.T) {
	attempts := wireGuardDeleteAttempts
	poll := wireGuardDeletePoll
	t.Cleanup(func() {
		wireGuardDeleteAttempts = attempts
		wireGuardDeletePoll = poll
	})
	wireGuardDeleteAttempts = 2
	wireGuardDeletePoll = 0

	var calls []string
	deletes := 0
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			calls = append(calls, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/5":
			// A previous destroy can leave the row disabled while the
			// running router still has the interface enabled.
			writeBody(w, `{"$key":5,"vnet":12,"name":"wg0","enabled":false}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/5":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"enabled":false`) {
				t.Errorf("disable body = %s", body)
			}
			writeBody(w, `{"$key":5,"vnet":12,"name":"wg0","enabled":false}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"vnet":12`) || !strings.Contains(string(body), `"action":"refresh"`) {
				t.Errorf("apply body = %s", body)
			}
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/5":
			deletes++
			if deletes == 1 {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"err":"This WireGuard inteface must be disabled and applied before you can delete it"}`))
				return
			}
			writeBody(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":true,"status":"running"}`)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	notice, err := api.deleteWireGuard(t.Context(), &wireGuardModel{
		ID:        types.StringValue("5"),
		NetworkID: types.StringValue("12"),
		Apply:     types.BoolValue(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("notice = %#v", notice)
	}
	want := []string{
		"PUT /api/v4/vnet_wireguards/5",
		"POST /api/v4/vnet_actions",
		"DELETE /api/v4/vnet_wireguards/5",
		"PUT /api/v4/vnet_wireguards/5",
		"POST /api/v4/vnet_actions",
		"DELETE /api/v4/vnet_wireguards/5",
		"POST /api/v4/vnet_actions",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want disable, apply, rejected delete, disable, apply, delete, apply", calls)
	}
}

func TestDeleteWireGuardDoesNotRetryOtherErrors(t *testing.T) {
	var deletes int
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/5":
			writeBody(w, `{"$key":5,"vnet":12,"name":"wg0","enabled":true}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/5":
			writeBody(w, `{"$key":5,"vnet":12,"enabled":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":true,"status":"running"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/5":
			deletes++
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"err":"WireGuard interface is in use"}`))
		default:
			unexpectedAPI(t, w, r)
		}
	})

	_, err := api.deleteWireGuard(t.Context(), &wireGuardModel{
		ID:        types.StringValue("5"),
		NetworkID: types.StringValue("12"),
		Apply:     types.BoolValue(true),
	})
	if err == nil || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("error = %v", err)
	}
	if deletes != 1 {
		t.Fatalf("deletes = %d, want one attempt", deletes)
	}
}

func TestDeleteWireGuardOnStoppedNetworkSkipsApply(t *testing.T) {
	var calls []string
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			calls = append(calls, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0","enabled":true}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/3":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"enabled":false`) {
				t.Errorf("disable body = %s", body)
			}
			writeBody(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":false,"status":"stopped"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			t.Errorf("stopped network was applied")
			http.Error(w, "network is stopped", http.StatusUnprocessableEntity)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	notice, err := api.deleteWireGuard(t.Context(), &wireGuardModel{
		ID:        types.StringValue("3"),
		NetworkID: types.StringValue("12"),
		Apply:     types.BoolValue(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	if notice == nil || !strings.Contains(notice.Detail, "not running") {
		t.Fatalf("notice = %#v, want a stopped network warning", notice)
	}
	want := []string{
		"PUT /api/v4/vnet_wireguards/3",
		"DELETE /api/v4/vnet_wireguards/3",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want disable then delete", calls)
	}
}

func TestDeleteWireGuardOnStoppedNetworkDoesNotApplyAfterReject(t *testing.T) {
	var calls []string
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			calls = append(calls, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/5":
			writeBody(w, `{"$key":5,"vnet":12,"name":"wg0","enabled":true}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/5":
			writeBody(w, `{"$key":5,"vnet":12,"enabled":false}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":false,"status":"stopped"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/5":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"err":"This WireGuard inteface must be disabled and applied before you can delete it"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			t.Errorf("stopped network was applied")
			http.Error(w, "network is stopped", http.StatusUnprocessableEntity)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	_, err := api.deleteWireGuard(t.Context(), &wireGuardModel{
		ID:        types.StringValue("5"),
		NetworkID: types.StringValue("12"),
		Apply:     types.BoolValue(true),
	})
	if err == nil || !strings.Contains(err.Error(), "disabled and applied") {
		t.Fatalf("error = %v", err)
	}
	want := []string{
		"PUT /api/v4/vnet_wireguards/5",
		"DELETE /api/v4/vnet_wireguards/5",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want disable then one delete", calls)
	}
}

func TestDeleteNetworkVPNRowsSkipsApplyWhenStopped(t *testing.T) {
	var calls []string
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			calls = append(calls, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards":
			writeBody(w, `[{"$key":3,"vnet":12,"name":"wg0","enabled":true}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0","enabled":true}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[]`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":false,"status":"stopped"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs":
			writeBody(w, `[]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			t.Errorf("stopped network was applied")
			http.Error(w, "network is stopped", http.StatusUnprocessableEntity)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	if err := DeleteNetworkVPNRows(t.Context(), api.sdk, 12); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"PUT /api/v4/vnet_wireguards/3",
		"DELETE /api/v4/vnet_wireguards/3",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want disable then delete", calls)
	}
}

func TestCreateWireGuardRemovesStoppedInterfaceWhenReadFails(t *testing.T) {
	var calls []string
	reads := 0
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			calls = append(calls, r.Method+" "+r.URL.Path)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_wireguards":
			writeBody(w, `{"$key":3}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			reads++
			// Create reads the new row, then the provider reads it again.
			// Fail that second read so rollback runs. The delete reads once more.
			if reads == 2 {
				http.Error(w, "read failed", http.StatusInternalServerError)
				return
			}
			writeBody(w, `{"$key":3,"vnet":12,"name":"wg0","enabled":true}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeBody(w, `{"$key":12,"name":"lan","running":false,"status":"stopped"}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeBody(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			t.Errorf("stopped network was applied")
			http.Error(w, "network is stopped", http.StatusUnprocessableEntity)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	_, err := api.createWireGuard(t.Context(), &wireGuardModel{
		NetworkID: types.StringValue("12"),
		Name:      types.StringValue("wg0"),
		IP:        types.StringValue("192.168.255.1/24"),
		Apply:     types.BoolValue(true),
	})
	if err == nil || !strings.Contains(err.Error(), "row was removed") {
		t.Fatalf("error = %v", err)
	}
	want := []string{
		"POST /api/v4/vnet_wireguards",
		"PUT /api/v4/vnet_wireguards/3",
		"DELETE /api/v4/vnet_wireguards/3",
	}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("calls = %v, want create, disable, delete", calls)
	}
}

func TestDeleteMissingWireGuardReportsBadNetworkID(t *testing.T) {
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			http.NotFound(w, r)
		default:
			unexpectedAPI(t, w, r)
		}
	})
	_, err := api.deleteWireGuard(t.Context(), &wireGuardModel{
		ID:        types.StringValue("3"),
		NetworkID: types.StringValue("nope"),
		Apply:     types.BoolValue(true),
	})
	if err == nil || !strings.Contains(err.Error(), "network_id") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeleteWireGuardPeerRejectsBadWireGuardID(t *testing.T) {
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		unexpectedAPI(t, w, r)
	})
	_, err := api.deleteWireGuardPeer(t.Context(), &wireGuardPeerModel{
		ID:          types.StringValue("9"),
		WireGuardID: types.StringValue("nope"),
	})
	if err == nil || !strings.Contains(err.Error(), "wireguard_id") {
		t.Fatalf("error = %v", err)
	}
}

func TestNetworkDeleteRefusesWhileVPNRemains(t *testing.T) {
	var killed, deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			writeBody(w, `{"version":"26.0.0"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards":
			writeBody(w, `[{"$key":3,"vnet":12,"name":"wg0"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeBody(w, `[{"$key":9,"wireguard":3,"name":"office"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs" && r.URL.Path != "/api/v4/vnet_ipsecs/5":
			writeBody(w, `[{"$key":5,"vnet":12,"enabled":true}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeBody(w, `{"$key":5,"vnet":12,"enabled":true,"mode":"normal"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			writeBody(w, `[{"$key":7,"ipsec":5,"name":"branch"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeBody(w, `[{"$key":8,"phase1":7,"name":"lan"}]`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			killed = true
			writeBody(w, `{}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnets/12":
			deleted = true
			writeBody(w, `{}`)
		default:
			unexpectedAPI(t, w, r)
		}
	}))
	t.Cleanup(server.Close)

	item := configuredNetworkResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(t.Context(), fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(t.Context(), networkDeleteModel()); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.DeleteResponse{State: state}
	item.Delete(t.Context(), fwresource.DeleteRequest{State: state}, resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("delete should refuse while VPN rows remain")
	}
	detail := resp.Diagnostics.Errors()[0].Detail()
	for _, want := range []string{"WireGuard peer 9", "WireGuard interface 3", "IPsec phase 2 8", "IPsec phase 1 7", "IPsec configuration 5", "phase 2, then phase 1"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail missing %q: %s", want, detail)
		}
	}
	if killed || deleted {
		t.Fatalf("killed=%v deleted=%v, network should stay in place", killed, deleted)
	}
}

func TestNetworkDeleteProceedsWhenVPNIsGone(t *testing.T) {
	var deleted bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			writeBody(w, `{"version":"26.0.0"}`)
		case r.Method == http.MethodGet && (r.URL.Path == "/api/v4/vnet_wireguards" || r.URL.Path == "/api/v4/vnet_ipsecs"):
			writeBody(w, `[]`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
			writeBody(w, `[{"$key":12,"name":"lan","running":false}]`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnets/12":
			deleted = true
			writeBody(w, `{}`)
		case r.Method == http.MethodPost:
			t.Errorf("stopped network was powered: %s", r.URL.Path)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		default:
			unexpectedAPI(t, w, r)
		}
	}))
	t.Cleanup(server.Close)

	item := configuredNetworkResource(t, server.URL)
	schemaResp := &fwresource.SchemaResponse{}
	item.Schema(t.Context(), fwresource.SchemaRequest{}, schemaResp)
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(t.Context(), networkDeleteModel()); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &fwresource.DeleteResponse{State: state}
	item.Delete(t.Context(), fwresource.DeleteRequest{State: state}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !deleted {
		t.Fatal("network was not deleted")
	}
}

func networkDeleteModel() *NetworkResourceModel {
	return &NetworkResourceModel{
		Id:                   types.StringValue("12"),
		Name:                 types.StringValue("lan"),
		Bond_Interfaces_Args: types.ListNull(types.Int32Type),
	}
}

func configuredNetworkResource(t *testing.T, url string) *NetworkResource {
	t.Helper()
	item := &NetworkResource{}
	resp := &fwresource.ConfigureResponse{}
	item.Configure(t.Context(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(url, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}

func newTestVPN(t *testing.T, h http.HandlerFunc) *vpnAPI {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		if r.URL.Path == "/version.json" {
			w.Header().Set("Content-Type", "application/json")
			writeBody(w, `{"version":"26.0.0"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}))
	t.Cleanup(server.Close)
	sdk, err := vergeos.NewClient(
		vergeos.WithBaseURL(server.URL),
		vergeos.WithCredentials("user", "pass"),
		vergeos.WithInsecureTLS(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	return &vpnAPI{sdk: sdk}
}

func writeBody(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func unexpectedAPI(t *testing.T, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
	http.Error(w, "unexpected", http.StatusInternalServerError)
}

func boolRequiresReplace(t *testing.T, mods []planmodifier.Bool) bool {
	t.Helper()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	for _, mod := range mods {
		resp := &planmodifier.BoolResponse{PlanValue: types.BoolValue(false)}
		mod.PlanModifyBool(t.Context(), planmodifier.BoolRequest{
			StateValue:  types.BoolValue(true),
			PlanValue:   types.BoolValue(false),
			ConfigValue: types.BoolValue(false),
			State:       tfsdk.State{Raw: raw},
			Plan:        tfsdk.Plan{Raw: raw},
		}, resp)
		if resp.RequiresReplace {
			return true
		}
	}
	return false
}

func stringRequiresReplace(t *testing.T, mods []planmodifier.String) bool {
	t.Helper()
	raw := tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
	for _, mod := range mods {
		resp := &planmodifier.StringResponse{PlanValue: types.StringValue("b")}
		mod.PlanModifyString(t.Context(), planmodifier.StringRequest{
			StateValue:  types.StringValue("a"),
			PlanValue:   types.StringValue("b"),
			ConfigValue: types.StringValue("b"),
			State:       tfsdk.State{Raw: raw},
			Plan:        tfsdk.Plan{Raw: raw},
		}, resp)
		if resp.RequiresReplace {
			return true
		}
	}
	return false
}

func containsCall(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}
