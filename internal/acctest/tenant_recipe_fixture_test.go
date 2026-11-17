// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package acctest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestTenantRecipeFixturePlan(t *testing.T) {
	min := vergeos.RecipeBound{Set: true, N: 2}
	questions := []vergeos.RecipeQuestion{
		{Key: 1, Name: "YB_USER_PASSWORD", Type: "password", Required: true, Enabled: true},
		{Key: 2, Name: "YB_USER_NAME", Type: "string", Required: true, Enabled: true, Default: json.RawMessage(`"admin"`)},
		{Key: 3, Name: "YB_NODE_1_CPU_CORES", Type: "num", Required: true, Enabled: true, Min: min},
		{Key: 4, Name: "YB_NET_1_IP", Type: "vnet_address", Required: true, Enabled: true},
		{Key: 5, Name: "YB_EXPOSE_CLOUD_SNAPSHOTS", Type: "bool", Required: false, Enabled: true},
		{Key: 6, Name: "YB_THEME_ACCESS", Type: "list", Required: true, Enabled: true, Choices: vergeos.RecipeChoices{"host_only": "Host", "tenant": "Tenant"}},
	}
	answers, disable := tenantRecipeFixturePlan(questions)
	if answers["YB_USER_PASSWORD"] != "Tf-acc-tenant-password1" {
		t.Fatalf("password = %#v", answers["YB_USER_PASSWORD"])
	}
	if _, ok := answers["YB_USER_NAME"]; ok {
		t.Fatal("a question with a default does not need an answer")
	}
	if answers["YB_NODE_1_CPU_CORES"] != "2" {
		t.Fatalf("cores = %#v", answers["YB_NODE_1_CPU_CORES"])
	}
	if answers["YB_THEME_ACCESS"] != "host_only" {
		t.Fatalf("theme = %#v", answers["YB_THEME_ACCESS"])
	}
	if len(disable) != 1 || disable[0] != 4 {
		t.Fatalf("disable = %#v", disable)
	}

	hcl := tenantRecipeAnswersHCL(answers)
	for _, part := range []string{
		`YB_NODE_1_CPU_CORES = "2"`,
		`YB_THEME_ACCESS = "host_only"`,
		`YB_USER_PASSWORD = "Tf-acc-tenant-password1"`,
	} {
		if !strings.Contains(hcl, part) {
			t.Fatalf("hcl = %s missing %s", hcl, part)
		}
	}
}

func TestDestroyErrorsWhenSnapshotIsSourceTenant(t *testing.T) {
	const sourceID = 44
	const recipeID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	var mu sync.Mutex
	var mutated []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenant_recipes":
			body := `[{"$key":"` + recipeID + `","id":"` + recipeID + `","tenant":` + strconv.Itoa(sourceID) + `,"tenant_snapshot":` + strconv.Itoa(sourceID) + `}]`
			_, _ = w.Write([]byte(body))
		default:
			mu.Lock()
			mutated = append(mutated, r.Method+" "+r.URL.Path)
			mu.Unlock()
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)

	sdk, err := vergeos.NewClient(
		vergeos.WithBaseURL(server.URL),
		vergeos.WithCredentials("user", "pass"),
		vergeos.WithInsecureTLS(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &TenantRecipeFixture{
		ctx:            context.Background(),
		sdk:            sdk,
		http:           vergeio.NewClient(server.URL, "user", "pass", true),
		sourceTenantID: sourceID,
		RecipeID:       recipeID,
		catalogID:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	err = fixture.Destroy(t)
	if err == nil || !strings.Contains(err.Error(), "source tenant") || !strings.Contains(err.Error(), strconv.Itoa(sourceID)) {
		t.Fatalf("err = %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(mutated) != 0 {
		t.Fatalf("destroy changed the lab: %#v", mutated)
	}
}

func TestTenantRecipeLabSkipNamesBothVariables(t *testing.T) {
	for _, name := range []string{
		"TF_ACC_VERGEIO_TENANT_RECIPE_ID",
		"TF_ACC_VERGEIO_TENANT_RECIPE_SOURCE_TENANT_ID",
		"configured inside the tenant",
	} {
		if !strings.Contains(tenantRecipeLabSkip, name) {
			t.Fatalf("skip message = %q missing %s", tenantRecipeLabSkip, name)
		}
	}
}
