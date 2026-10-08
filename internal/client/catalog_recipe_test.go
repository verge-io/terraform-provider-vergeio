// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateCatalogAndTenantRecipe(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(body))
		switch r.URL.Path {
		case "/api/v4/catalog_repositories":
			_, _ = w.Write([]byte(`[{"$key":3,"name":"Marketplace","type":"yottabyte"},{"$key":"1","name":"Local","type":"local"}]`))
		case "/api/v4/catalogs":
			_, _ = w.Write([]byte(`{"$key":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
		case "/api/v4/tenant_recipes":
			if strings.Contains(string(body), `"tenant_snapshot"`) {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"err":"field 'tenant_snapshot' cannot be set"}`))
				return
			}
			_, _ = w.Write([]byte(`{"$key":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	httpClient := NewClient(server.URL, "user", "pass", true)
	ctx := context.Background()

	repo, err := httpClient.LocalCatalogRepositoryID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if repo != 1 {
		t.Fatalf("repository = %d", repo)
	}
	catalog, err := httpClient.CreateCatalog(ctx, CatalogCreate{
		Name:            "tf-acc-catalog",
		Repository:      repo,
		Description:     "acceptance",
		PublishingScope: "private",
		Enabled:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if catalog != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("catalog = %s", catalog)
	}
	recipe, err := httpClient.CreateTenantRecipe(ctx, TenantRecipeCreate{
		Name:    "tf-acc-recipe",
		Catalog: catalog,
		Tenant:  7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recipe != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("recipe = %s", recipe)
	}
	joined := strings.Join(calls, "\n")
	for _, part := range []string{
		"GET /api/v4/catalog_repositories",
		`"name":"tf-acc-catalog"`,
		`"repository":1`,
		`"publishing_scope":"private"`,
		`"catalog":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"`,
		`"tenant":7`,
		`"version":"1.0.0"`,
	} {
		if !strings.Contains(joined, part) {
			t.Fatalf("calls = %#v missing %s", calls, part)
		}
	}
	if strings.Contains(joined, "tenant_snapshot") {
		t.Fatalf("create sent tenant_snapshot: %s", joined)
	}
}

func TestCreateTenantRecipeRejectsTenantSnapshot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		body, _ := io.ReadAll(r.Body)
		if r.Method == http.MethodPost && strings.Contains(string(body), `"tenant_snapshot"`) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"err":"field 'tenant_snapshot' cannot be set"}`))
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"$key":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`))
	}))
	t.Cleanup(server.Close)
	httpClient := NewClient(server.URL, "user", "pass", true)
	resp, err := httpClient.Post(context.Background(), tenantRecipeEndpoint, bytes.NewBufferString(`{"catalog":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","name":"tf-acc-recipe","tenant":7,"tenant_snapshot":1,"version":"1.0.0"}`))
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "field 'tenant_snapshot' cannot be set") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeleteCatalogAndTenantRecipeNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"err":"not found"}`))
	}))
	t.Cleanup(server.Close)
	httpClient := NewClient(server.URL, "user", "pass", true)
	ctx := context.Background()
	if err := httpClient.DeleteCatalog(ctx, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatal(err)
	}
	if err := httpClient.DeleteTenantRecipe(ctx, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
}

func TestDisableAndRepublishTenantRecipe(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(body))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(server.Close)
	httpClient := NewClient(server.URL, "user", "pass", true)
	ctx := context.Background()
	if err := httpClient.DisableRecipeQuestion(ctx, 15); err != nil {
		t.Fatal(err)
	}
	if err := httpClient.RepublishTenantRecipe(ctx, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	for _, part := range []string{
		`PUT /api/v4/recipe_questions/15 {"enabled":false}`,
		`POST /api/v4/tenant_recipe_actions {"action":"republish","tenant_recipe":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`,
	} {
		if !strings.Contains(joined, part) {
			t.Fatalf("calls = %#v missing %s", calls, part)
		}
	}
}
