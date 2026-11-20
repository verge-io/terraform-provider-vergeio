// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestDeleteIPSecConnection404KeepsStateWhenRowRemains(t *testing.T) {
	var phase1Deleted bool
	item := configuredVPNResource(t, NewNetworkIPSecConnectionResource(), func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeJSON(t, w, http.StatusOK, []map[string]any{{
				"$key": 8, "phase1": 7, "name": "lan-to-lan", "local": "10.251.9.0/24",
			}})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8":
			writeRefusedDelete(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 8, "name": "lan-to-lan"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7":
			phase1Deleted = true
			writeJSON(t, w, http.StatusOK, map[string]any{})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	resp := deleteImported(t, item, "7")
	assertRefusedDiagnostic(t, resp.Diagnostics, "IPsec phase 2 8 (lan-to-lan)")
	if got := stateID(t, resp.State); got != "7" {
		t.Fatalf("state id = %q, want the connection kept", got)
	}
	if phase1Deleted {
		t.Fatal("phase 1 was deleted while phase 2 remained")
	}
}

func TestDeleteIPSecConnection404SucceedsWhenRowGone(t *testing.T) {
	item := configuredVPNResource(t, NewNetworkIPSecConnectionResource(), func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeJSON(t, w, http.StatusOK, []map[string]any{{
				"$key": 8, "phase1": 7, "name": "lan-to-lan", "local": "10.251.9.0/24",
			}})
		case r.Method == http.MethodDelete && (r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8" || r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7"):
			writeRefusedDelete(t, w)
		case r.Method == http.MethodGet && (r.URL.Path == "/api/v4/vnet_ipsec_phase2s/8" || r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7"):
			writeJSON(t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	resp := deleteImported(t, item, "7")
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
}

func TestDeleteIPSec404KeepsStateWhenRowRemains(t *testing.T) {
	item := configuredVPNResource(t, NewNetworkIPSecResource(), func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			writeJSON(t, w, http.StatusOK, []any{})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeRefusedDelete(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 5, "vnet": 14, "enabled": true})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	resp := deleteImported(t, item, "5")
	assertRefusedDiagnostic(t, resp.Diagnostics, "IPsec configuration 5")
	if got := stateID(t, resp.State); got != "5" {
		t.Fatalf("state id = %q, want the configuration kept", got)
	}
}

func TestDeleteIPSec404SucceedsWhenRowGone(t *testing.T) {
	item := configuredVPNResource(t, NewNetworkIPSecResource(), func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s":
			writeJSON(t, w, http.StatusOK, []any{})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeRefusedDelete(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeJSON(t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	resp := deleteImported(t, item, "5")
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
}

func TestDeleteWireGuardPeer404WhenRowRemains(t *testing.T) {
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers/9":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"$key": 9, "wireguard": 3, "name": "office", "peer_ip": "192.168.255.2",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguard_peers/9":
			writeRefusedDelete(t, w)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	_, err := api.deleteWireGuardPeer(t.Context(), &wireGuardPeerModel{
		ID:          types.StringValue("9"),
		WireGuardID: types.StringValue("3"),
		Name:        types.StringValue("office"),
		Apply:       types.BoolValue(true),
	})
	assertRefusedDelete(t, err, "WireGuard peer 9 (office)")
}

func TestDeleteWireGuardPeer404SucceedsWhenRowGone(t *testing.T) {
	reads := 0
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers/9":
			reads++
			if reads == 1 {
				writeJSON(t, w, http.StatusOK, map[string]any{
					"$key": 9, "wireguard": 3, "name": "office", "peer_ip": "192.168.255.2",
				})
				return
			}
			writeJSON(t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguard_peers/9":
			writeRefusedDelete(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 3, "vnet": 12, "name": "wg0"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 12, "name": "lan", "running": true, "status": "running"})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			writeJSON(t, w, http.StatusOK, map[string]any{})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	if _, err := api.deleteWireGuardPeer(t.Context(), &wireGuardPeerModel{
		ID:          types.StringValue("9"),
		WireGuardID: types.StringValue("3"),
		Apply:       types.BoolValue(true),
	}); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteWireGuardInterface404WhenRowRemains(t *testing.T) {
	api := newTestVPN(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguard_peers":
			writeJSON(t, w, http.StatusOK, []any{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 3, "vnet": 12, "name": "wg0", "enabled": true})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeJSON(t, w, http.StatusOK, map[string]any{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/12":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 12, "name": "lan", "running": false, "status": "stopped"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_wireguards/3":
			writeRefusedDelete(t, w)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	_, err := api.deleteWireGuard(t.Context(), &wireGuardModel{
		ID:        types.StringValue("3"),
		NetworkID: types.StringValue("12"),
		Apply:     types.BoolValue(true),
	})
	assertRefusedDelete(t, err, "WireGuard interface 3")
}

func TestDeleteDNSRecord404WhenRowRemains(t *testing.T) {
	api := newDNSTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_dns_zone_records/3":
			writeRefusedDelete(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_dns_zone_records/3":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 3, "zone": 2, "host": "www", "type": "A"})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	err := deleteDNSRecord(t.Context(), api.sdk, 3)
	assertRefusedDelete(t, err, "DNS record 3")
}

func TestDeleteDNSRecord404SucceedsWhenRowGone(t *testing.T) {
	api := newDNSTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_dns_zone_records/3":
			writeRefusedDelete(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_dns_zone_records/3":
			writeJSON(t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	if err := deleteDNSRecord(t.Context(), api.sdk, 3); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteFirewallRule404WhenRowRemains(t *testing.T) {
	rule := seededRule(t, apiRule(9, "allow-ssh", 1))
	api := newRuleTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_rules/9":
			writeJSON(t, w, http.StatusOK, rule)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_rules/9":
			writeRefusedDelete(t, w)
		default:
			unexpectedAPI(t, w, r)
		}
	})

	_, err := api.deleteRule(t.Context(), 3, 9, true)
	assertRefusedDelete(t, err, "firewall rule 9 (allow-ssh)")
}

func TestDeleteFirewallRule404SucceedsWhenRowGone(t *testing.T) {
	rule := seededRule(t, apiRule(9, "allow-ssh", 1))
	reads := 0
	api := newRuleTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_rules/9":
			reads++
			if reads == 1 {
				writeJSON(t, w, http.StatusOK, rule)
				return
			}
			writeJSON(t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/vnet_rules/9":
			writeRefusedDelete(t, w)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/3":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"$key": 3, "name": "lan", "running": false, "status": "stopped",
			})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	notice, err := api.deleteRule(t.Context(), 3, 9, true)
	if err != nil {
		t.Fatal(err)
	}
	if notice != nil {
		t.Fatalf("notice = %#v", notice)
	}
}

func TestReadIPSecWarnsWhenParentNetworkGone(t *testing.T) {
	item := configuredVPNResource(t, NewNetworkIPSecResource(), func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 5, "vnet": 14, "enabled": true, "mode": "normal"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/14":
			writeJSON(t, w, http.StatusNotFound, map[string]any{"err": "not found"})
		default:
			unexpectedAPI(t, w, r)
		}
	})

	resp := readImported(t, item, "5")
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	assertParentWarning(t, resp.Diagnostics, "IPsec configuration 5", "14")
	if got := stateID(t, resp.State); got != "5" {
		t.Fatalf("state id = %q", got)
	}
}

func TestReadIPSecConnectionWarnsWhenParentNetworkGone(t *testing.T) {
	item := configuredVPNResource(t, NewNetworkIPSecConnectionResource(), ipsecConnectionReadHandler(t, http.StatusNotFound))

	resp := readImported(t, item, "7")
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	assertParentWarning(t, resp.Diagnostics, "IPsec phase 1 7", "14")
	if got := stateID(t, resp.State); got != "7" {
		t.Fatalf("state id = %q", got)
	}
}

func TestReadIPSecSkipsWarningWhenParentLookupIsUnclear(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{name: "network present", status: http.StatusOK},
		{name: "network lookup failed", status: http.StatusInternalServerError},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			item := configuredVPNResource(t, NewNetworkIPSecResource(), func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs/5":
					writeJSON(t, w, http.StatusOK, map[string]any{"$key": 5, "vnet": 14, "enabled": true, "mode": "normal"})
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/14":
					if tt.status == http.StatusOK {
						writeJSON(t, w, http.StatusOK, map[string]any{"$key": 14, "name": "lan", "running": false})
						return
					}
					writeJSON(t, w, tt.status, map[string]any{"err": "lookup failed"})
				default:
					unexpectedAPI(t, w, r)
				}
			})
			resp := readImported(t, item, "5")
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			if resp.Diagnostics.WarningsCount() != 0 {
				t.Fatalf("warnings = %v", resp.Diagnostics)
			}
		})
	}
}

func ipsecConnectionReadHandler(t *testing.T, networkStatus int) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase1s/7":
			writeJSON(t, w, http.StatusOK, map[string]any{
				"$key": 7, "ipsec": 5, "name": "bugtest-conn", "enabled": true, "remote_gateway": "203.0.113.10",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_phase2s":
			writeJSON(t, w, http.StatusOK, []map[string]any{{
				"$key": 8, "phase1": 7, "name": "lan-to-lan", "enabled": true,
				"local": "10.251.9.0/24", "remote": "198.51.100.0/24",
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsec_connections":
			writeJSON(t, w, http.StatusOK, []any{})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnet_ipsecs/5":
			writeJSON(t, w, http.StatusOK, map[string]any{"$key": 5, "vnet": 14, "enabled": true})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets/14":
			if networkStatus == http.StatusOK {
				writeJSON(t, w, http.StatusOK, map[string]any{"$key": 14, "name": "lan"})
				return
			}
			writeJSON(t, w, networkStatus, map[string]any{"err": "not found"})
		default:
			unexpectedAPI(t, w, r)
		}
	}
}

func configuredVPNResource(t *testing.T, item fwresource.Resource, h http.HandlerFunc) fwresource.Resource {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		if r.URL.Path == "/version.json" {
			writeJSON(t, w, http.StatusOK, map[string]any{"version": "26.0.0"})
			return
		}
		h(w, r)
	}))
	t.Cleanup(server.Close)
	resp := &fwresource.ConfigureResponse{}
	item.(fwresource.ResourceWithConfigure).Configure(t.Context(), fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "user", "pass", true),
	}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return item
}

func deleteImported(t *testing.T, item fwresource.Resource, id string) *fwresource.DeleteResponse {
	t.Helper()
	imported := importResourceState(t, item, id)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	resp := &fwresource.DeleteResponse{State: imported.State}
	item.Delete(t.Context(), fwresource.DeleteRequest{State: imported.State}, resp)
	return resp
}

func readImported(t *testing.T, item fwresource.Resource, id string) *fwresource.ReadResponse {
	t.Helper()
	imported := importResourceState(t, item, id)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	resp := &fwresource.ReadResponse{State: imported.State, Identity: nullNetworkIdentity(t, item.(fwresource.ResourceWithIdentity))}
	item.Read(t.Context(), fwresource.ReadRequest{State: imported.State, Identity: resp.Identity}, resp)
	return resp
}

func stateID(t *testing.T, state tfsdk.State) string {
	t.Helper()
	if state.Raw.IsNull() {
		t.Fatal("resource was removed from state")
	}
	var id types.String
	if diags := state.GetAttribute(t.Context(), path.Root("id"), &id); diags.HasError() {
		t.Fatal(diags)
	}
	return id.ValueString()
}

func assertRefusedDelete(t *testing.T, err error, row string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected the delete to fail while the row remains")
	}
	assertRefusedText(t, err.Error(), row)
}

func assertRefusedDiagnostic(t *testing.T, diags diag.Diagnostics, row string) {
	t.Helper()
	if !diags.HasError() {
		t.Fatal("expected an error so Terraform keeps the resource")
	}
	assertRefusedText(t, diags.Errors()[0].Detail(), row)
}

func assertRefusedText(t *testing.T, text, row string) {
	t.Helper()
	for _, want := range []string{
		"VergeOS refused the delete",
		"probably because the parent network was deleted outside Terraform",
		row,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("error missing %q: %s", want, text)
		}
	}
}

func assertParentWarning(t *testing.T, diags diag.Diagnostics, row, networkID string) {
	t.Helper()
	warnings := diags.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v", warnings)
	}
	if warnings[0].Summary() != "Parent network was deleted outside Terraform" {
		t.Fatalf("summary = %q", warnings[0].Summary())
	}
	detail := warnings[0].Detail()
	for _, want := range []string{"Network " + networkID, row, "deleted outside Terraform"} {
		if !strings.Contains(detail, want) {
			t.Errorf("warning missing %q: %s", want, detail)
		}
	}
}

func writeRefusedDelete(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	writeJSON(t, w, http.StatusNotFound, map[string]any{
		"err": "Error from put in trigger 'put:vnets/{vnet}?need_fw_apply=true' - No such file or directory",
	})
}

func seededRule(t *testing.T, rule vergeos.VNetRule) map[string]any {
	t.Helper()
	body, err := json.Marshal(rule)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["$key"] = rule.Key.Int()
	return decoded
}
