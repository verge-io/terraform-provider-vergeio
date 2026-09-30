// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func TestTagCreateRequest(t *testing.T) {
	req := tagCreateRequest(&TagResourceModel{
		Category:    types.Int32Value(4),
		Name:        types.StringValue("production"),
		Description: types.StringNull(),
	})
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"category":4,"name":"production"}` {
		t.Fatalf("create body = %s", raw)
	}

	req = tagCreateRequest(&TagResourceModel{
		Category:    types.Int32Value(4),
		Name:        types.StringValue("production"),
		Description: types.StringValue("workloads"),
	})
	raw, err = json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"category":4,"name":"production","description":"workloads"}`
	if string(raw) != want {
		t.Fatalf("create body = %s, want %s", raw, want)
	}
}

func TestTagUpdateRequestDoesNotSendCategory(t *testing.T) {
	state := &TagResourceModel{
		Category:    types.Int32Value(4),
		Name:        types.StringValue("production"),
		Description: types.StringValue("old"),
	}
	plan := *state
	plan.Category = types.Int32Value(9)
	plan.Name = types.StringValue("staging")
	plan.Description = types.StringValue("")

	req := tagUpdateRequest(&plan, state)
	if req == nil {
		t.Fatal("expected an update body")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"name":"staging","description":""}`
	if string(raw) != want {
		t.Fatalf("update body = %s, want %s", raw, want)
	}
	if tagUpdateRequest(&plan, &plan) != nil {
		t.Fatal("unchanged tag should not produce an update body")
	}
}

func TestTagCreateReadUpdateDelete(t *testing.T) {
	var gotPost, gotPut map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/tags":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			if err := json.Unmarshal(raw, &gotPost); err != nil {
				t.Errorf("unmarshal body: %v", err)
			}
			_, _ = w.Write([]byte(`{"$key":8}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tags/8":
			_, _ = w.Write([]byte(`{"$key":8,"name":"production","description":"workloads","category":4,"category_display":"environment"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/tags/8":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			if err := json.Unmarshal(raw, &gotPut); err != nil {
				t.Errorf("unmarshal body: %v", err)
			}
			_, _ = w.Write([]byte(`{"response":"OK"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v4/tags/8":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"response":"OK"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := NewTagsApi(vergeio.NewClient(server.URL, "user", "pass", true))
	data := &TagResourceModel{
		Category:    types.Int32Value(4),
		Name:        types.StringValue("production"),
		Description: types.StringValue("workloads"),
	}
	if err := api.createTag(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() != "8" || data.CategoryName.ValueString() != "environment" {
		t.Fatalf("created tag = %#v", data)
	}
	if gotPost["category"] != float64(4) || gotPost["name"] != "production" {
		t.Fatalf("create body = %#v", gotPost)
	}

	if err := api.readTag(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	plan := &TagResourceModel{
		Category:    data.Category,
		Name:        types.StringValue("staging"),
		Description: data.Description,
	}
	if err := api.updateTag(t.Context(), plan, data); err != nil {
		t.Fatal(err)
	}
	if _, present := gotPut["category"]; present {
		t.Fatalf("update sent category: %#v", gotPut)
	}
	if gotPut["name"] != "staging" {
		t.Fatalf("update body = %#v", gotPut)
	}
	if err := api.deleteTag(t.Context(), data); err != nil {
		t.Fatal(err)
	}
}

func TestReadTagNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/tags/9":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := NewTagsApi(vergeio.NewClient(server.URL, "user", "pass", true))
	err := api.readTag(t.Context(), &TagResourceModel{Id: types.StringValue("9")})
	if err == nil {
		t.Fatal("expected not found")
	}
}
