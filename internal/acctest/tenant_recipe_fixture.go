// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package acctest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

const fixtureTenantAdmin = "Tf-acc-tenant-password1"

const (
	envTenantRecipeID             = "TF_ACC_VERGEIO_TENANT_RECIPE_ID"
	envTenantRecipeSourceTenantID = "TF_ACC_VERGEIO_TENANT_RECIPE_SOURCE_TENANT_ID"
)

// tenantRecipeLabSkip is the acceptance skip when neither a published recipe
// nor a configured source tenant is named. VergeOS creates a recipe from a
// source tenant that was configured inside the tenant. That work is not
// tenant_status.started, db_version, or a host-side power-on, and no
// host-side field records it. The source tenant is dedicated to recipe use
// and is powered off when the recipe is created.
const tenantRecipeLabSkip = "set TF_ACC_VERGEIO_TENANT_RECIPE_ID to a published tenant recipe, or TF_ACC_VERGEIO_TENANT_RECIPE_SOURCE_TENANT_ID to an existing tenant that was configured inside the tenant and is powered off"

// TenantRecipeFixture is a throwaway catalog and tenant recipe.
// The source tenant already exists and is left unchanged: this fixture
// does not power it, edit it, or delete it. VergeOS copies it into
// tenant_snapshot. The fixture deletes that snapshot copy, the tf-acc-
// recipe, and the tf-acc- catalog. The deployed instance tenant belongs
// to vergeio_tenant_recipe_instance, which removes its nodes highest
// nodeid first.
type TenantRecipeFixture struct {
	RecipeID   string
	AnswersHCL string

	ctx            context.Context
	sdk            *vergeos.Client
	http           *vergeio.Client
	sourceTenantID int
	catalogID      string
}

// PublishedTenantRecipeID returns TF_ACC_VERGEIO_TENANT_RECIPE_ID.
// An empty value means the caller should build a recipe from
// TF_ACC_VERGEIO_TENANT_RECIPE_SOURCE_TENANT_ID, or skip.
func PublishedTenantRecipeID() string {
	return strings.TrimSpace(os.Getenv(envTenantRecipeID))
}

// SkipWithoutTenantRecipeLab skips when neither lab variable is set.
func SkipWithoutTenantRecipeLab(t *testing.T) {
	t.Helper()
	if PublishedTenantRecipeID() != "" {
		return
	}
	if strings.TrimSpace(os.Getenv(envTenantRecipeSourceTenantID)) == "" {
		t.Skip(tenantRecipeLabSkip)
	}
}

// NewTenantRecipeFixture posts a tf-acc- catalog and a tf-acc- recipe for
// the tenant named by TF_ACC_VERGEIO_TENANT_RECIPE_SOURCE_TENANT_ID.
// That tenant must already be configured inside the tenant and powered off.
// The fixture does not power, edit, or delete it. A 405 from the recipe
// POST is returned as the test failure. Required questions the fixture can
// answer are returned as HCL inside an answers map. A required question it
// cannot answer is disabled on this recipe, and the recipe is republished.
func NewTenantRecipeFixture(t *testing.T) *TenantRecipeFixture {
	t.Helper()
	raw := strings.TrimSpace(os.Getenv(envTenantRecipeSourceTenantID))
	if raw == "" {
		t.Skip(tenantRecipeLabSkip)
	}
	sourceID, err := strconv.Atoi(raw)
	if err != nil || sourceID <= 0 {
		t.Fatalf("%s = %q, want a positive integer", envTenantRecipeSourceTenantID, raw)
	}
	ctx := context.Background()
	sdk, err := SDKClient()
	if err != nil {
		t.Fatal(err)
	}
	httpClient, err := acceptanceHTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	fixture := &TenantRecipeFixture{
		ctx:            ctx,
		sdk:            sdk,
		http:           httpClient,
		sourceTenantID: sourceID,
	}
	t.Cleanup(func() { fixture.Destroy(t) })
	fixture.createRecipe(t)
	fixture.prepareAnswers(t)
	return fixture
}

func (f *TenantRecipeFixture) createRecipe(t *testing.T) {
	t.Helper()
	catalogName := Name("tenant-recipe-catalog")
	recipeName := Name("tenant-recipe-def")
	for _, name := range []string{catalogName, recipeName} {
		if err := RequirePrefix(name); err != nil {
			t.Fatal(err)
		}
	}
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
		Name:        recipeName,
		Description: "acceptance tenant recipe",
		Catalog:     catalogID,
		Tenant:      f.sourceTenantID,
		Version:     "1.0.0",
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

// Destroy removes the tf-acc- recipe, the tf-acc- catalog, and the snapshot
// tenant VergeOS created for the recipe. Nodes on that snapshot tenant are
// deleted highest nodeid first. The source tenant is not powered, edited,
// or deleted.
func (f *TenantRecipeFixture) Destroy(t *testing.T) {
	t.Helper()
	if f == nil {
		return
	}
	copyID := f.recipeSnapshotTenant()
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
	if copyID == f.sourceTenantID {
		t.Errorf("recipe snapshot tenant %d is the source tenant", copyID)
		return
	}
	if err := deleteSnapshotTenant(f.ctx, f.sdk, copyID); err != nil {
		t.Errorf("delete recipe snapshot tenant %d: %v", copyID, err)
	}
}

func (f *TenantRecipeFixture) recipeSnapshotTenant() int {
	if f == nil || f.sdk == nil || f.RecipeID == "" {
		return 0
	}
	recipe, err := f.sdk.TenantRecipes.Get(f.ctx, f.RecipeID)
	if err != nil || recipe == nil || recipe.TenantSnapshot == nil {
		return 0
	}
	id := recipe.TenantSnapshot.Int()
	if id <= 0 || id == f.sourceTenantID {
		return 0
	}
	return id
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
