// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package acctest

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

const fixtureTenantAdmin = "Tf-acc-tenant-password1"

// TenantRecipeFixture is a throwaway tenant, snapshot, catalog, and tenant
// recipe. The tenant and snapshot use govergeos. Catalogs and tenant recipes
// have no create method there, so those rows are posted over HTTP.
type TenantRecipeFixture struct {
	RecipeID   string
	AnswersHCL string

	ctx        context.Context
	sdk        *vergeos.Client
	http       *vergeio.Client
	tenantID   int
	snapshotID int
	catalogID  string
}

// NewTenantRecipeFixture creates the source tenant, snapshots it, posts a
// private catalog in the local repository, and posts a tenant recipe from
// that snapshot. Required questions the fixture can answer are returned as
// HCL inside an answers map. A required question it cannot answer is disabled,
// and the recipe is republished.
func NewTenantRecipeFixture(t *testing.T) *TenantRecipeFixture {
	t.Helper()
	ctx := context.Background()
	sdk, err := SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	httpClient, err := acceptanceHTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	fixture := &TenantRecipeFixture{ctx: ctx, sdk: sdk, http: httpClient}
	t.Cleanup(func() { fixture.Destroy(t) })
	fixture.createRecipe(t)
	fixture.prepareAnswers(t)
	return fixture
}

func (f *TenantRecipeFixture) createRecipe(t *testing.T) {
	t.Helper()
	tenantName := Name("tenant-recipe-source")
	snapName := Name("tenant-recipe-source-snap")
	catalogName := Name("tenant-recipe-catalog")
	recipeName := Name("tenant-recipe-def")
	for _, name := range []string{tenantName, snapName, catalogName, recipeName} {
		if err := RequirePrefix(name); err != nil {
			t.Fatal(err)
		}
	}

	created, err := f.sdk.Tenants.Create(f.ctx, &vergeos.TenantCreateRequest{
		Name:        tenantName,
		Password:    fixtureTenantAdmin,
		Description: "acceptance tenant recipe source",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.tenantID = created.Key.Int()

	snap, err := f.sdk.TenantSnapshots.Create(f.ctx, &vergeos.TenantSnapshotCreateRequest{
		Tenant:      f.tenantID,
		Name:        snapName,
		Description: "acceptance tenant recipe source",
		Type:        vergeos.TenantSnapshotTypeFull,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.snapshotID = snap.Key.Int()

	repository, err := f.http.LocalCatalogRepositoryID(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	catalogID, err := f.http.CreateCatalog(f.ctx, vergeio.CatalogCreate{
		Name:            catalogName,
		Repository:      repository,
		Description:     "acceptance tenant recipes",
		PublishingScope: "private",
		Enabled:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	f.catalogID = catalogID

	recipeID, err := f.http.CreateTenantRecipe(f.ctx, vergeio.TenantRecipeCreate{
		Name:           recipeName,
		Description:    "acceptance tenant recipe",
		Catalog:        catalogID,
		Tenant:         f.tenantID,
		TenantSnapshot: f.snapshotID,
		Version:        "1.0.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.RecipeID = recipeID
}

func (f *TenantRecipeFixture) prepareAnswers(t *testing.T) {
	t.Helper()
	questions, err := f.sdk.TenantRecipes.Questions(f.ctx, f.RecipeID)
	if err != nil {
		t.Fatal(err)
	}
	answers, disable := tenantRecipeFixturePlan(questions)
	for _, id := range disable {
		if err := f.http.DisableRecipeQuestion(f.ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if len(disable) > 0 {
		if err := f.http.RepublishTenantRecipe(f.ctx, f.RecipeID); err != nil {
			t.Fatal(err)
		}
	}
	f.AnswersHCL = tenantRecipeAnswersHCL(answers)
}

// Destroy removes the recipe, the catalog, the snapshot, and the source tenant.
// The deployed tenant belongs to vergeio_tenant_recipe_instance.
func (f *TenantRecipeFixture) Destroy(t *testing.T) {
	t.Helper()
	if f == nil {
		return
	}
	if f.RecipeID != "" && f.http != nil {
		if err := f.http.DeleteTenantRecipe(f.ctx, f.RecipeID); err != nil {
			t.Errorf("delete tenant recipe %s: %v", f.RecipeID, err)
		}
	}
	if f.catalogID != "" && f.http != nil {
		if err := f.http.DeleteCatalog(f.ctx, f.catalogID); err != nil {
			t.Errorf("delete catalog %s: %v", f.catalogID, err)
		}
	}
	if f.snapshotID > 0 && f.sdk != nil {
		if err := f.sdk.TenantSnapshots.Delete(f.ctx, f.snapshotID); err != nil && !vergeos.IsNotFoundError(err) {
			t.Errorf("delete tenant snapshot %d: %v", f.snapshotID, err)
		}
	}
	if f.tenantID > 0 && f.sdk != nil {
		if err := f.sdk.Tenants.Delete(f.ctx, f.tenantID); err != nil && !vergeos.IsNotFoundError(err) {
			t.Errorf("delete tenant %d: %v", f.tenantID, err)
		}
	}
}

// tenantRecipeFixturePlan returns answers for required questions the fixture
// can fill, and the question ids that must be disabled because they need a
// network, address, or other row this test does not have.
func tenantRecipeFixturePlan(questions []vergeos.RecipeQuestion) (map[string]string, []int) {
	answers := map[string]string{}
	var disable []int
	for i := range questions {
		question := &questions[i]
		if !question.Enabled || !question.Required || !recipeDefaultEmpty(question.Default) {
			continue
		}
		value, ok := tenantRecipeAnswer(question)
		if ok {
			answers[question.Name] = value
			continue
		}
		if id := question.Key.Int(); id > 0 {
			disable = append(disable, id)
		}
	}
	return answers, disable
}

func tenantRecipeAnswer(question *vergeos.RecipeQuestion) (string, bool) {
	kind := strings.ToLower(strings.TrimSpace(question.Type))
	name := strings.ToLower(question.Name)
	switch kind {
	case "string", "text", "textarea", "password":
		if strings.Contains(name, "password") || kind == "password" {
			return fixtureTenantAdmin, true
		}
		if strings.Contains(name, "email") {
			return "tf-acc@example.com", true
		}
		return "tf-acc", true
	case "bool", "boolean":
		return "true", true
	case "num", "number", "integer", "int":
		if question.Min.Set && question.Min.N > 0 {
			return fmt.Sprintf("%d", question.Min.N), true
		}
		return "1", true
	case "ram":
		if question.Min.Set && question.Min.N > 0 {
			return fmt.Sprintf("%d", question.Min.N), true
		}
		return "2048", true
	case "disksize":
		return "53687091200", true
	case "list":
		return firstRecipeChoice(question.Choices)
	default:
		return "", false
	}
}

func firstRecipeChoice(choices vergeos.RecipeChoices) (string, bool) {
	if len(choices) == 0 {
		return "", false
	}
	keys := make([]string, 0, len(choices))
	for key := range choices {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys[0], true
}

func recipeDefaultEmpty(raw json.RawMessage) bool {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" || text == `""` || text == "0" || text == "false" {
		return true
	}
	var decoded string
	if json.Unmarshal(raw, &decoded) == nil && strings.TrimSpace(decoded) == "" {
		return true
	}
	return false
}

func tenantRecipeAnswersHCL(answers map[string]string) string {
	if len(answers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(answers))
	for key := range answers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&b, "    %s = %q\n", key, answers[key])
	}
	return strings.TrimRight(b.String(), "\n")
}
