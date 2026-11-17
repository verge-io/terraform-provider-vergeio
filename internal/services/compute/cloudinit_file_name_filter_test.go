package compute

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

func TestReadCloudinitFilesEscapesNameFilterAndKeepsExactName(t *testing.T) {
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
				case "/api/v4/cloudinit_files":
					gotFilter = r.URL.Query().Get("filter")
					if r.URL.Query().Get("download") != "" {
						t.Errorf("list sent download=%q", r.URL.Query().Get("download"))
					}
					body, err := json.Marshal([]map[string]any{
						{"$key": 1, "name": decoy, "contents": "decoy-list"},
						{"$key": 2, "name": name, "contents": "from-list"},
					})
					if err != nil {
						t.Errorf("marshal: %v", err)
						return
					}
					_, _ = w.Write(body)
				case "/api/v4/cloudinit_files/2":
					if r.URL.Query().Get("download") != "1" {
						t.Errorf("download=%q, want 1", r.URL.Query().Get("download"))
					}
					_, _ = w.Write([]byte("from-download"))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			api := mustAPI(NewCloudinitFileApi(vergeio.NewClient(server.URL, "user", "pass", true)))
			data := &CloudinitFileDataSourceModel{FilterName: types.StringValue(name)}
			if err := api.readCloudinitFiles(t.Context(), data); err != nil {
				t.Fatal(err)
			}
			wantFilter := fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))
			if gotFilter != wantFilter {
				t.Fatalf("filter = %q, want %q", gotFilter, wantFilter)
			}
			if len(data.CloudinitFiles) != 1 || data.CloudinitFiles[0].Name.ValueString() != name {
				t.Fatalf("got %d cloudinit files, want the one named %q", len(data.CloudinitFiles), name)
			}
			if got := data.CloudinitFiles[0].Contents.ValueString(); got != "from-download" {
				t.Fatalf("contents = %q, want the download body", got)
			}
		})
	}
}

func TestReadCloudinitFilesEmptyNameFilterIsEmptyList(t *testing.T) {
	const filterName = "no-such-file"
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "no rows", body: `[]`},
		{name: "different name", body: `[{"$key":1,"name":"other"}]`},
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
				case "/api/v4/cloudinit_files":
					gotFilter = r.URL.Query().Get("filter")
					_, _ = w.Write([]byte(tc.body))
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			t.Cleanup(server.Close)

			api := mustAPI(NewCloudinitFileApi(vergeio.NewClient(server.URL, "user", "pass", true)))
			assertEmptyFilterList(t, &CloudinitFileDataSource{cloudinitFileApi: api}, &CloudinitFileDataSourceModel{FilterName: types.StringValue(filterName)}, "cloudinit_files", &gotFilter, filterName)
		})
	}
}

func TestReadCloudinitFilesNoFilterEmptyIsEmptyList(t *testing.T) {
	var gotFilter string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/cloudinit_files":
			gotFilter = r.URL.Query().Get("filter")
			_, _ = w.Write([]byte(`[]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewCloudinitFileApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	assertEmptyUnfilteredList(t, &CloudinitFileDataSource{cloudinitFileApi: api}, &CloudinitFileDataSourceModel{}, "cloudinit_files", &gotFilter)
}

func assertEmptyUnfilteredList(t *testing.T, source datasource.DataSource, model any, listName string, gotFilter *string) {
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

	if *gotFilter != "" {
		t.Fatalf("filter = %q, want none", *gotFilter)
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
