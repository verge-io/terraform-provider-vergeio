// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestReadUsersEscapesNameFilterAndKeepsExactName(t *testing.T) {
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
				case "/api/v4/users":
					gotFilter = r.URL.Query().Get("filter")
					body, err := json.Marshal([]map[string]any{
						{"$key": 1, "name": decoy, "displayname": "", "email": "", "type": "normal", "enabled": true},
						{"$key": 2, "name": name, "displayname": "Shown", "email": "a@example.com", "type": "api", "enabled": false},
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

			api := mustAPI(NewUsersApi(vergeio.NewClient(server.URL, "user", "pass", true)))
			data := &UsersDataSourceModel{FilterName: types.StringValue(name)}
			if err := api.readUsers(t.Context(), data); err != nil {
				t.Fatal(err)
			}
			wantFilter := fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))
			if gotFilter != wantFilter {
				t.Fatalf("filter = %q, want %q", gotFilter, wantFilter)
			}
			if len(data.Users) != 1 || data.Users[0].Name.ValueString() != name {
				t.Fatalf("got %d users, want the one named %q", len(data.Users), name)
			}
			if data.Users[0].Id.ValueInt32() != 2 || data.Users[0].Enabled.ValueBool() || data.Users[0].Type.ValueString() != "api" {
				t.Fatalf("user = %#v", data.Users[0])
			}
		})
	}
}
