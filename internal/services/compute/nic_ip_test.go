package compute

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

const nicIPTestMAC = "52:54:00:11:22:33"

func TestNICIPAddressStaysOptionalComputed(t *testing.T) {
	vmResource := NewVMResource()
	resp := &fwresource.SchemaResponse{}
	vmResource.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	attr := nestedStringAttr(t, resp.Schema.Blocks, "vergeio_nic", "ipaddress")
	if !attr.Optional || !attr.Computed {
		t.Fatalf("ipaddress optional=%v computed=%v, want both", attr.Optional, attr.Computed)
	}
}

func TestCreateNICPostsRequestedIP(t *testing.T) {
	const wantIP = "10.0.0.50"
	var posted []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_nics":
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":"98"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"nic0","interface":"virtio","vnet":26,"macaddress":"` + nicIPTestMAC + `"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_addresses":
			posted = body
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"response":"` + wantIP + `"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &NICApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	data := &nicResourceModel{
		Name:            types.StringValue("nic0"),
		Interface:       types.StringValue("virtio"),
		VNET:            types.Int32Value(26),
		AssignIPAddress: types.BoolValue(true),
		IPAddress:       types.StringValue(wantIP),
	}
	if err := api.createNIC(t.Context(), data); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{}
	if err := json.Unmarshal(posted, &body); err != nil {
		t.Fatalf("vnet_addresses body %q: %v", posted, err)
	}
	requireString(t, body, "ip", wantIP)
	requireString(t, body, "mac", nicIPTestMAC)
	requireString(t, body, "type", "static")
	requireNumber(t, body, "vnet", 26)
	if strings.Contains(string(posted), "N/A") {
		t.Fatalf("vnet_addresses body contained placeholder N/A: %s", posted)
	}
	if data.IPAddress.IsNull() || data.IPAddress.ValueString() != wantIP {
		t.Fatalf("state ipaddress = %#v, want %s", data.IPAddress, wantIP)
	}
}

func TestCreateNICAssignOmitsUnsetIP(t *testing.T) {
	var posted []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_nics":
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"$key":"98"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`{"machine":1,"name":"nic0","interface":"virtio","vnet":26,"macaddress":"` + nicIPTestMAC + `"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_addresses":
			posted = body
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{"response":"10.0.0.8"}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	api := &NICApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
	data := &nicResourceModel{
		Name:            types.StringValue("nic0"),
		Interface:       types.StringValue("virtio"),
		VNET:            types.Int32Value(26),
		AssignIPAddress: types.BoolValue(true),
		IPAddress:       types.StringNull(),
	}
	if err := api.createNIC(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	body := map[string]any{}
	if err := json.Unmarshal(posted, &body); err != nil {
		t.Fatalf("vnet_addresses body %q: %v", posted, err)
	}
	requireAbsent(t, body, "ip")
	if strings.Contains(string(posted), "N/A") {
		t.Fatalf("vnet_addresses body contained placeholder N/A: %s", posted)
	}
	if data.IPAddress.ValueString() != "10.0.0.8" {
		t.Fatalf("state ipaddress = %#v, want the address VergeOS assigned", data.IPAddress)
	}
}

func TestCreateNICLeavesIPNullWhenUnassigned(t *testing.T) {
	cases := []struct {
		name   string
		assign types.Bool
		ip     types.String
	}{
		{name: "unset flag and address", assign: types.BoolNull(), ip: types.StringNull()},
		{name: "false flag and unknown address", assign: types.BoolValue(false), ip: types.StringUnknown()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read body: %v", err)
				}
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/v4/machine_nics":
					w.WriteHeader(http.StatusCreated)
					if _, err := w.Write([]byte(`{"$key":"98"}`)); err != nil {
						t.Errorf("write response: %v", err)
					}
				case r.Method == http.MethodGet && r.URL.Path == "/api/v4/machine_nics/98":
					w.WriteHeader(http.StatusOK)
					if _, err := w.Write([]byte(`{"machine":1,"name":"nic0","interface":"virtio","vnet":26,"macaddress":"` + nicIPTestMAC + `"}`)); err != nil {
						t.Errorf("write response: %v", err)
					}
				default:
					t.Errorf("unexpected %s %s body %s", r.Method, r.URL.Path, body)
					http.Error(w, "unexpected", http.StatusInternalServerError)
				}
			}))
			defer server.Close()

			api := &NICApi{client: vergeio.NewClient(server.URL, "user", "pass", true)}
			data := &nicResourceModel{
				Name:            types.StringValue("nic0"),
				Interface:       types.StringValue("virtio"),
				VNET:            types.Int32Value(26),
				AssignIPAddress: tc.assign,
				IPAddress:       tc.ip,
			}
			if err := api.createNIC(t.Context(), data); err != nil {
				t.Fatal(err)
			}
			if !data.IPAddress.IsNull() || data.IPAddress.ValueString() == "N/A" {
				t.Fatalf("ipaddress = %#v, want null", data.IPAddress)
			}
		})
	}
}
