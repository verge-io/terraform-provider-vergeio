// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestCompileNamePattern(t *testing.T) {
	exact, err := CompileNamePattern("web")
	if err != nil {
		t.Fatal(err)
	}
	if !exact.Matches("web") || exact.Matches("web-1") {
		t.Fatal("exact pattern should match only web")
	}
	if exact.FilterClause() != "name eq 'web'" {
		t.Fatalf("clause = %q", exact.FilterClause())
	}

	quoted, err := CompileNamePattern("O'Brien")
	if err != nil {
		t.Fatal(err)
	}
	if quoted.FilterClause() != `name eq 'O\'Brien'` {
		t.Fatalf("clause = %q", quoted.FilterClause())
	}

	glob, err := CompileNamePattern("web-*")
	if err != nil {
		t.Fatal(err)
	}
	if glob.FilterClause() != "" {
		t.Fatal("a glob should not be sent as a name equality filter")
	}
	for _, name := range []string{"web-", "web-1", "web-app"} {
		if !glob.Matches(name) {
			t.Fatalf("%q should match web-*", name)
		}
	}
	if glob.Matches("other") || glob.Matches("webs") {
		t.Fatal("web-* matched a name it should not")
	}

	one, err := CompileNamePattern("vm-?")
	if err != nil {
		t.Fatal(err)
	}
	if !one.Matches("vm-a") || one.Matches("vm-ab") {
		t.Fatal("? should match one character")
	}

	all, err := CompileNamePattern("  ")
	if err != nil {
		t.Fatal(err)
	}
	if !all.Matches("anything") {
		t.Fatal("an empty pattern should match every name")
	}
}

func TestSelectionTagKeys(t *testing.T) {
	sel := Selection{Name: NameMatch{all: true}, Keys: map[string]struct{}{"2": {}}}
	if !sel.Allow("web", "2") || sel.Allow("web", "3") {
		t.Fatal("tag keys should keep only the assigned id")
	}
	empty := Selection{Name: NameMatch{all: true}, Keys: map[string]struct{}{}}
	if empty.Allow("web", "2") {
		t.Fatal("an empty tag assignment set should match nothing")
	}
}

func TestTenantClauseAndTagMembers(t *testing.T) {
	var tagFilter, memberFilter string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/version.json", "/api/v4/version.json":
			_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		case "/api/v4/clusters":
			_, _ = w.Write([]byte(`[{"$key":1}]`))
		case "/api/v4/tenants":
			_, _ = w.Write([]byte(`[{"$key":9,"name":"customer-a"}]`))
		case "/api/v4/tenants/9":
			_, _ = w.Write([]byte(`{"$key":9,"name":"customer-a"}`))
		case "/api/v4/tags":
			tagFilter = r.URL.Query().Get("filter")
			_, _ = w.Write([]byte(`[{"$key":7,"name":"prod","category":1}]`))
		case "/api/v4/tag_categories":
			_, _ = w.Write([]byte(`[{"$key":1,"name":"env"}]`))
		case "/api/v4/tag_members":
			memberFilter = r.URL.Query().Get("filter")
			_, _ = w.Write([]byte(`[{"$key":1,"tag":7,"member":"vms/15"},{"$key":2,"tag":7,"member":"vnets/3"}]`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	sdk, err := vergeos.NewClient(
		vergeos.WithBaseURL(server.URL),
		vergeos.WithCredentials("user", "pass"),
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	clause, err := TenantClause(ctx, sdk, "9")
	if err != nil {
		t.Fatal(err)
	}
	if clause != "tenant eq 9" {
		t.Fatalf("clause = %q", clause)
	}
	named, err := TenantClause(ctx, sdk, "customer-a")
	if err != nil {
		t.Fatal(err)
	}
	if named != "tenant eq 9" {
		t.Fatalf("named clause = %q", named)
	}
	if _, err := TenantClause(ctx, sdk, "0"); err == nil {
		t.Fatal("tenant 0 should be rejected")
	}

	keys, err := TagMemberKeys(ctx, sdk, "prod", "vms")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := keys["15"]; !ok || len(keys) != 1 {
		t.Fatalf("vm keys = %#v", keys)
	}
	if !strings.Contains(tagFilter, "prod") {
		t.Fatalf("tag filter = %q", tagFilter)
	}
	if memberFilter != "tag eq 7" {
		t.Fatalf("member filter = %q", memberFilter)
	}

	byCategory, err := TagMemberKeys(ctx, sdk, "env/prod", "vms")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := byCategory["15"]; !ok {
		t.Fatalf("category keys = %#v", byCategory)
	}

	_, sel, diags := BuildSelection(ctx, sdk, ListQuery{Tenant: types.StringValue("customer-a")}, "tags", false)
	if !diags.HasError() {
		t.Fatal("tags should reject a tenant filter")
	}
	if sel.Keys != nil {
		t.Fatal("a rejected tenant filter should not build a selection")
	}
}
