package mediasource

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

func TestReadMediasourcesEscapesNameFilterAndKeepsExactName(t *testing.T) {
	for _, name := range []string{"O'Brien", "Core{x}", `Core\x`} {
		t.Run(name, func(t *testing.T) {
			decoy := "other"
			if name == "Core{x}" {
				decoy = "Core"
			}
			var gotFilter string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/version.json":
					_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
				case "/api/v4/files":
					gotFilter = r.URL.Query().Get("filter")
					body, err := json.Marshal([]map[string]any{
						{"$key": 1, "name": decoy},
						{"$key": 2, "name": name},
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

			api := NewMediasourceApi(vergeio.NewClient(server.URL, "user", "pass", true))
			data := &MediasourceDataSourceModel{FilterName: types.StringValue(name)}
			if err := api.readMediasources(t.Context(), data); err != nil {
				t.Fatal(err)
			}
			wantFilter := fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))
			if gotFilter != wantFilter {
				t.Fatalf("filter = %q, want %q", gotFilter, wantFilter)
			}
			if len(data.Mediasources) != 1 || data.Mediasources[0].Name.ValueString() != name {
				t.Fatalf("got %d mediasources, want the one named %q", len(data.Mediasources), name)
			}
		})
	}
}
