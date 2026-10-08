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
	"time"

	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

const fixtureTenantAdmin = "Tf-acc-tenant-password1"

// TenantRecipeFixture is a throwaway tenant, catalog, and tenant recipe.
// The tenant uses govergeos. Catalogs and tenant recipes have no create
// method there, so those rows are posted over HTTP. The source tenant stays
// powered on while its node machine status stays running, and the recipe
// POST is retried until VergeOS accepts it or recipeBootLimit elapses.
// VergeOS fills tenant_snapshot itself. The fixture does not send that field.
type TenantRecipeFixture struct {
	RecipeID   string
	AnswersHCL string

	ctx       context.Context
	sdk       *vergeos.Client
	http      *vergeio.Client
	tenantID  int
	catalogID string
}

// NewTenantRecipeFixture creates the source tenant, gives it a node and
// storage, posts a private catalog in the local repository, powers the
// tenant on, and posts a tenant recipe once the node machine has stayed
// running. Required questions the fixture can answer are returned as HCL
// inside an answers map. A required question it cannot answer is disabled,
// and the recipe is republished. An empty TF_ACC_VERGEIO_TENANT_RECIPE_ID
// uses this fixture. That is not a skip.
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
	nodeName := Name("tenant-recipe-node")
	catalogName := Name("tenant-recipe-catalog")
	recipeName := Name("tenant-recipe-def")
	for _, name := range []string{tenantName, nodeName, catalogName, recipeName} {
		if err := RequirePrefix(name); err != nil {
			t.Fatal(err)
		}
	}
	f.createSourceTenant(t, tenantName)
	f.addSourceCapacity(t, nodeName)

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
	f.postRecipeAfterBoot(t, recipeName)
}

func (f *TenantRecipeFixture) createSourceTenant(t *testing.T, name string) {
	t.Helper()
	created, err := f.sdk.Tenants.Create(f.ctx, &vergeos.TenantCreateRequest{
		Name:        name,
		Password:    fixtureTenantAdmin,
		Description: "acceptance tenant recipe source",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.tenantID = created.Key.Int()
}

// addSourceCapacity gives the tenant a node and storage. Power-on with no
// node cannot reach running. 4 cores and 16 GB are the tenant-node defaults.
func (f *TenantRecipeFixture) addSourceCapacity(t *testing.T, nodeName string) {
	t.Helper()
	enabled := true
	if _, err := f.sdk.TenantNodes.Create(f.ctx, &vergeos.TenantNodeCreateRequest{
		Tenant:   f.tenantID,
		Name:     nodeName,
		CPUCores: 4,
		RAM:      16384,
		Enabled:  &enabled,
	}); err != nil {
		t.Fatal(err)
	}
	tiers, err := f.sdk.StorageTiers.List(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tiers) == 0 {
		t.Fatal("no storage tiers on the lab")
	}
	if _, err := f.sdk.TenantStorage.Create(f.ctx, &vergeos.TenantStorageCreateRequest{
		Tenant:      f.tenantID,
		Tier:        tiers[0].Key,
		Provisioned: 1073741824,
	}); err != nil {
		t.Fatal(err)
	}
}

func (f *TenantRecipeFixture) postRecipeAfterBoot(t *testing.T, name string) {
	t.Helper()
	boot := newRecipeBoot(time.Now, sleepCtx, t.Logf, fixtureRecipePost{f: f, name: name})
	id, err := boot.run(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	f.RecipeID = id
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

// Destroy removes the recipe, the catalog, a snapshot tenant VergeOS created
// for the recipe, and the source tenant. Nodes and storage on those tenants
// are deleted first. The deployed tenant belongs to
// vergeio_tenant_recipe_instance.
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
	if err := deleteSnapshotTenant(f.ctx, f.sdk, copyID); err != nil {
		t.Errorf("delete recipe snapshot tenant %d: %v", copyID, err)
	}
	if err := removeManagedTenant(f.ctx, f.sdk, f.tenantID); err != nil {
		t.Errorf("delete tenant %d: %v", f.tenantID, err)
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
	if id <= 0 || id == f.tenantID {
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
