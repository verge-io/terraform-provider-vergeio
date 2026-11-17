// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestGroupCreateRequestKeepsDisabledAndDescription(t *testing.T) {
	req := groupCreateRequest(&GroupResourceModel{
		Name:        types.StringValue("ops"),
		Description: types.StringValue("operators"),
		Enabled:     types.BoolValue(false),
	})
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body["name"] != "ops" {
		t.Fatalf("name = %#v", body["name"])
	}
	if body["description"] != "operators" {
		t.Fatalf("description = %#v", body["description"])
	}
	enabled, ok := body["enabled"].(bool)
	if !ok || enabled {
		t.Fatalf("enabled = %#v, want false", body["enabled"])
	}
}

func TestGroupUpdateRequestSendsOnlyChanges(t *testing.T) {
	plan := &GroupResourceModel{
		Name:        types.StringValue("ops"),
		Description: types.StringValue(""),
		Enabled:     types.BoolValue(false),
	}
	state := &GroupResourceModel{
		Name:        types.StringValue("ops"),
		Description: types.StringValue("operators"),
		Enabled:     types.BoolValue(true),
	}
	req := groupUpdateRequest(plan, state)
	if req == nil {
		t.Fatal("expected an update body")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"description":"","enabled":false}`
	if string(raw) != want {
		t.Fatalf("update body = %s, want %s", raw, want)
	}
	if groupUpdateRequest(plan, plan) != nil {
		t.Fatal("unchanged group should not produce an update body")
	}
}

func TestGroupCreateReadUpdateDelete(t *testing.T) {
	var gotPost map[string]any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/groups":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			mu.Lock()
			_ = json.Unmarshal(body, &gotPost)
			mu.Unlock()
			_, _ = w.Write([]byte(`{"$key":4}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/groups/4":
			_, _ = w.Write([]byte(`{"$key":4,"name":"ops","description":"operators","enabled":false}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/groups/4":
			_, _ = w.Write([]byte(`{"response":"OK"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/groups/4":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"response":"OK"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewGroupApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	data := &GroupResourceModel{
		Name:        types.StringValue("ops"),
		Description: types.StringValue("operators"),
		Enabled:     types.BoolValue(false),
	}
	if err := api.createGroup(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() != "4" || data.Enabled.ValueBool() {
		t.Fatalf("created group = %#v", data)
	}
	mu.Lock()
	enabled, _ := gotPost["enabled"].(bool)
	mu.Unlock()
	if enabled {
		t.Fatal("create posted enabled true, want false")
	}

	if err := api.readGroup(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	plan := &GroupResourceModel{
		Name:        types.StringValue("ops"),
		Description: types.StringValue("renamed"),
		Enabled:     types.BoolValue(true),
	}
	if err := api.updateGroup(t.Context(), plan, data); err != nil {
		t.Fatal(err)
	}
	if err := api.deleteGroup(t.Context(), data); err != nil {
		t.Fatal(err)
	}
}

func TestReadGroupNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/groups/9":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewGroupApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	err := api.readGroup(t.Context(), &GroupResourceModel{Id: types.StringValue("9")})
	if err == nil {
		t.Fatal("expected not found")
	}
}
