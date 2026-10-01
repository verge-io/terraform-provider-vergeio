// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

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

func TestTagCategoryCreateRequestOmitsUnsetFlags(t *testing.T) {
	req := tagCategoryCreateRequest(&TagCategoryResourceModel{
		Name:               types.StringValue("env"),
		Description:        types.StringNull(),
		SingleTagSelection: types.BoolUnknown(),
		TaggableVMs:        types.BoolUnknown(),
	})
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != `{"name":"env"}` {
		t.Fatalf("create body = %s, want {\"name\":\"env\"}", raw)
	}
}

func TestTagCategoryCreateRequestSendsExplicitFalse(t *testing.T) {
	req := tagCategoryCreateRequest(&TagCategoryResourceModel{
		Name:               types.StringValue("env"),
		Description:        types.StringValue("classification"),
		SingleTagSelection: types.BoolValue(false),
		TaggableVMs:        types.BoolValue(false),
		TaggableVNets:      types.BoolValue(true),
	})
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"name":"env","description":"classification","single_tag_selection":false,"taggable_vnets":true,"taggable_vms":false}`
	if string(raw) != want {
		t.Fatalf("create body = %s, want %s", raw, want)
	}
}

func TestTagCategoryUpdateDoesNotSendUnchangedFalseFlags(t *testing.T) {
	state := withTaggableFlags(TagCategoryResourceModel{
		Name:        types.StringValue("env"),
		Description: types.StringValue("old"),
	}, types.BoolValue(false))
	plan := state
	plan.Description = types.StringValue("new")

	req := tagCategoryUpdateRequest(&plan, &state)
	if req == nil {
		t.Fatal("expected an update body")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"description":"new"}`
	if string(raw) != want {
		t.Fatalf("update body = %s, want %s", raw, want)
	}
	if tagCategoryUpdateRequest(&plan, &plan) != nil {
		t.Fatal("unchanged category should not produce an update body")
	}
}

func TestTagCategoryUpdateSendsExplicitFalseWhenNewlySet(t *testing.T) {
	state := &TagCategoryResourceModel{
		Name:        types.StringValue("env"),
		Description: types.StringValue("old"),
		TaggableVMs: types.BoolNull(),
	}
	plan := *state
	plan.TaggableVMs = types.BoolValue(false)

	req := tagCategoryUpdateRequest(&plan, state)
	if req == nil {
		t.Fatal("expected an update body")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"taggable_vms":false}`
	if string(raw) != want {
		t.Fatalf("update body = %s, want %s", raw, want)
	}
}

func TestTagCategoryCreateOmitsUnsetFlagsOnWire(t *testing.T) {
	body := postTagCategory(t, &TagCategoryResourceModel{
		Name:        types.StringValue("env"),
		TaggableVMs: types.BoolValue(true),
	})
	if body["name"] != "env" {
		t.Fatalf("name = %#v", body["name"])
	}
	enabled, ok := body["taggable_vms"].(bool)
	if !ok || !enabled {
		t.Fatalf("taggable_vms = %#v, want true", body["taggable_vms"])
	}
	for _, key := range []string{
		"taggable_volumes",
		"taggable_vnets",
		"taggable_vnet_rules",
		"taggable_vmware_containers",
		"taggable_users",
		"taggable_tenant_nodes",
		"taggable_sites",
		"taggable_nodes",
		"taggable_groups",
		"taggable_clusters",
		"taggable_tenants",
		"single_tag_selection",
		"description",
	} {
		if _, present := body[key]; present {
			t.Errorf("create body included omitted flag %s: %#v", key, body[key])
		}
	}
}

func TestTagCategoryCreateSendsExplicitFalseOnWire(t *testing.T) {
	body := postTagCategory(t, &TagCategoryResourceModel{
		Name:        types.StringValue("env"),
		TaggableVMs: types.BoolValue(false),
	})
	enabled, ok := body["taggable_vms"].(bool)
	if !ok || enabled {
		t.Fatalf("taggable_vms = %#v, want false", body["taggable_vms"])
	}
}

func TestTagCategoryUpdateOmitsUnchangedFalseOnWire(t *testing.T) {
	var gotPut map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v4/tag_categories/4":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			if err := json.Unmarshal(raw, &gotPut); err != nil {
				t.Errorf("unmarshal body: %v", err)
			}
			_, _ = w.Write([]byte(`{"response":"OK"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tag_categories/4":
			_, _ = w.Write([]byte(`{"$key":4,"name":"env","description":"new","taggable_vms":false}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewTagsApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	state := withTaggableFlags(TagCategoryResourceModel{
		Id:          types.StringValue("4"),
		Name:        types.StringValue("env"),
		Description: types.StringValue("old"),
	}, types.BoolValue(false))
	plan := state
	plan.Description = types.StringValue("new")
	if err := api.updateTagCategory(t.Context(), &plan, &state); err != nil {
		t.Fatal(err)
	}
	if _, present := gotPut["taggable_vms"]; present {
		t.Fatalf("update sent unchanged taggable_vms: %#v", gotPut)
	}
	if gotPut["description"] != "new" {
		t.Fatalf("update body = %#v", gotPut)
	}
	if len(gotPut) != 1 {
		t.Fatalf("update body = %#v, want description only", gotPut)
	}
}

func TestTagCategoryReadNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/tag_categories/9":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewTagsApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	err := api.readTagCategory(t.Context(), &TagCategoryResourceModel{Id: types.StringValue("9")})
	if err == nil {
		t.Fatal("expected not found")
	}
}

func TestTagCategoryDeleteNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/tag_categories/9":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"err":"not found"}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewTagsApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	if err := api.deleteTagCategory(t.Context(), &TagCategoryResourceModel{Id: types.StringValue("9")}); err != nil {
		t.Fatal(err)
	}
}

func postTagCategory(t *testing.T, data *TagCategoryResourceModel) map[string]any {
	t.Helper()
	var got map[string]any
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/tag_categories":
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
			}
			mu.Lock()
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Errorf("unmarshal body: %v", err)
			}
			mu.Unlock()
			_, _ = w.Write([]byte(`{"$key":4}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tag_categories/4":
			_, _ = w.Write([]byte(`{"$key":4,"name":"env","taggable_vms":true}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	api := mustAPI(NewTagsApi(vergeio.NewClient(server.URL, "user", "pass", true)))
	if err := api.createTagCategory(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if got == nil {
		t.Fatal("create did not post a body")
	}
	return got
}

func withTaggableFlags(m TagCategoryResourceModel, v types.Bool) TagCategoryResourceModel {
	m.SingleTagSelection = v
	m.TaggableVolumes = v
	m.TaggableVNets = v
	m.TaggableVNetRules = v
	m.TaggableVMwareContainers = v
	m.TaggableVMs = v
	m.TaggableUsers = v
	m.TaggableTenantNodes = v
	m.TaggableSites = v
	m.TaggableNodes = v
	m.TaggableGroups = v
	m.TaggableClusters = v
	m.TaggableTenants = v
	return m
}
