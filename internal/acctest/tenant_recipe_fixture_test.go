// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package acctest

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/verge-io/govergeos"
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
