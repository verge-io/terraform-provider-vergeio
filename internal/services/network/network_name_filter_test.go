package network

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestReadNetworksEscapesNameFilterAndKeepsExactName(t *testing.T) {
	for _, name := range []string{"O'Brien", "Core{x}", `Core\x`} {
		t.Run(name, func(t *testing.T) {
			decoy := "other"
			if name == "Core{x}" {
				decoy = "Core"
			}
			var gotFilter string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}

				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				case "/api/v4/vnets":
					gotFilter = r.URL.Query().Get("filter")
					body, err := json.Marshal([]map[string]any{
						{"$key": 1, "name": decoy, "type": "internal"},
						{"$key": 2, "name": name, "type": "internal"},
					})
					if err != nil {
						t.Errorf("marshal: %v", err)
						return
					}
					_, _ = w.Write(body)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			api := mustAPI(NewNetworkApi(vergeio.NewClient(server.URL, "user", "pass", true)))
			data := &NetworkDataSourceModel{FilterName: types.StringValue(name)}
			if err := api.readNetworks(t.Context(), data); err != nil {
				t.Fatal(err)
			}
			wantFilter := fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))
			if gotFilter != wantFilter {
				t.Fatalf("filter = %q, want %q", gotFilter, wantFilter)
			}
			if len(data.Networks) != 1 || data.Networks[0].Name.ValueString() != name {
				t.Fatalf("got %d networks, want the one named %q", len(data.Networks), name)
			}
		})
	}
}

func TestReadNetworksEscapesTypeFilterAndKeepsExactType(t *testing.T) {
	for _, networkType := range []string{"O'Brien", "internal{x}", `ext\ernal`} {
		t.Run(networkType, func(t *testing.T) {
			decoy := "other"
			if networkType == "internal{x}" {
				decoy = "internal"
			}
			var gotFilter string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}

				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				case "/api/v4/vnets":
					gotFilter = r.URL.Query().Get("filter")
					body, err := json.Marshal([]map[string]any{
						{"$key": 1, "name": "dropped", "type": decoy},
						{"$key": 2, "name": "kept", "type": networkType},
					})
					if err != nil {
						t.Errorf("marshal: %v", err)
						return
					}
					_, _ = w.Write(body)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			api := mustAPI(NewNetworkApi(vergeio.NewClient(server.URL, "user", "pass", true)))
			data := &NetworkDataSourceModel{FilterType: types.StringValue(networkType)}
			if err := api.readNetworks(t.Context(), data); err != nil {
				t.Fatal(err)
			}
			wantFilter := fmt.Sprintf("type eq '%s'", vergeio.EscapeFilterValue(networkType))
			if gotFilter != wantFilter {
				t.Fatalf("filter = %q, want %q", gotFilter, wantFilter)
			}
			if len(data.Networks) != 1 || data.Networks[0].Name.ValueString() != "kept" {
				t.Fatalf("got %d networks, want the one with type %q", len(data.Networks), networkType)
			}
		})
	}
}

func TestReadNetworksEmptyNameFilterIsEmptyList(t *testing.T) {
	const filterName = "no-such-network"
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "no rows", body: `[]`},
		{name: "different name", body: `[{"$key":1,"name":"other","type":"internal"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var gotFilter string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if vergeio.AnswerCredentialCheck(w, r) {
					return
				}

				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				case "/api/v4/vnets":
					gotFilter = r.URL.Query().Get("filter")
					_, _ = w.Write([]byte(tc.body))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			api := mustAPI(NewNetworkApi(vergeio.NewClient(server.URL, "user", "pass", true)))
			assertEmptyFilterList(t, &NetworkDataSource{networkApi: api}, &NetworkDataSourceModel{FilterName: types.StringValue(filterName)}, "networks", &gotFilter, filterName)
		})
	}
}

func assertEmptyFilterList(t *testing.T, source datasource.DataSource, model any, listName string, gotFilter *string, filterName string) {
	t.Helper()
	ctx := t.Context()
	schemaResp := &datasource.SchemaResponse{}
	source.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %v", schemaResp.Diagnostics)
	}

	configState := tfsdk.State{Schema: schemaResp.Schema}
	if diags := configState.Set(ctx, model); diags.HasError() {
		t.Fatalf("config: %v", diags)
	}
	resp := &datasource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	source.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Raw: configState.Raw, Schema: schemaResp.Schema}}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("read: %v", resp.Diagnostics)
	}

	wantFilter := fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(filterName))
	if *gotFilter != wantFilter {
		t.Fatalf("filter = %q, want %q", *gotFilter, wantFilter)
	}

	var list types.List
	if diags := resp.State.GetAttribute(ctx, path.Root(listName), &list); diags.HasError() {
		t.Fatalf("%s: %v", listName, diags)
	}
	if list.IsNull() || list.IsUnknown() {
		t.Fatalf("%s is null, want an empty list", listName)
	}
	if len(list.Elements()) != 0 {
		t.Fatalf("%s has %d elements, want none", listName, len(list.Elements()))
	}
}
