// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	vergeio "terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

// ListQuery is the config block for a list resource.
type ListQuery struct {
	NamePattern types.String `tfsdk:"name_pattern"`
	Tag         types.String `tfsdk:"tag"`
	Tenant      types.String `tfsdk:"tenant"`
}

// ListQueryAttributes returns the list config attributes.
// tenant is present on every list resource. BuildSelection rejects it when the table has no tenant column.
func ListQueryAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"name_pattern": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Name glob. `*` matches any run of characters and `?` matches one. A value with no glob is an exact name.",
		},
		"tag": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Tag name, or `category/name` when the same tag name exists in more than one category. Only objects assigned that tag are listed.",
		},
		"tenant": schema.StringAttribute{
			Optional:            true,
			MarkdownDescription: "Tenant key, or tenant name. Sent as `tenant eq` for VMs and networks. Other list resources reject it.",
		},
	}
}

// NameMatch compares an object name to a list name_pattern.
type NameMatch struct {
	pattern string
	exact   bool
	all     bool
}

// CompileNamePattern compiles pattern. An empty pattern matches every name.
func CompileNamePattern(pattern string) (NameMatch, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return NameMatch{all: true}, nil
	}
	if strings.ContainsAny(pattern, "*?") {
		if _, err := globRegexp(pattern); err != nil {
			return NameMatch{}, err
		}
		return NameMatch{pattern: pattern}, nil
	}
	return NameMatch{pattern: pattern, exact: true}, nil
}

// Matches reports whether name satisfies the pattern.
func (m NameMatch) Matches(name string) bool {
	if m.all || m.pattern == "" {
		return true
	}
	if m.exact {
		return name == m.pattern
	}
	re, err := globRegexp(m.pattern)
	if err != nil {
		return false
	}
	return re.MatchString(name)
}

// FilterClause is a VergeOS name filter for an exact pattern.
// A glob is applied after the list returns.
func (m NameMatch) FilterClause() string {
	if !m.exact || m.pattern == "" {
		return ""
	}
	return fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(m.pattern))
}

func globRegexp(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// Selection is the local filter applied after List.
type Selection struct {
	Name NameMatch
	Keys map[string]struct{}
}

// Allow reports whether name and key survive the name pattern and tag filter.
// A nil Keys map means tag was not set. An empty map means the tag matched nothing.
func (s Selection) Allow(name, key string) bool {
	if !s.Name.Matches(name) {
		return false
	}
	if s.Keys == nil {
		return true
	}
	_, ok := s.Keys[key]
	return ok
}

// BuildSelection resolves name_pattern, tag, and tenant into an API filter and a local selection.
// collection is the tag member prefix, such as vms or vnets.
func BuildSelection(ctx context.Context, sdk *vergeos.Client, q ListQuery, collection string, tenantSupported bool) (string, Selection, diag.Diagnostics) {
	var diags diag.Diagnostics
	sel := Selection{Name: NameMatch{all: true}}
	var clauses []string

	if pattern, ok := queryString(q.NamePattern); ok {
		match, err := CompileNamePattern(pattern)
		if err != nil {
			diags.AddError("Invalid name_pattern", err.Error())
			return "", sel, diags
		}
		sel.Name = match
		if clause := match.FilterClause(); clause != "" {
			clauses = append(clauses, clause)
		}
	}
	if tag, ok := queryString(q.Tag); ok {
		keys, err := TagMemberKeys(ctx, sdk, tag, collection)
		if err != nil {
			diags.AddError("Invalid tag filter", err.Error())
			return "", sel, diags
		}
		sel.Keys = keys
	}
	if tenant, ok := queryString(q.Tenant); ok {
		if !tenantSupported {
			diags.AddError(
				"Tenant filter is not supported",
				fmt.Sprintf("%s has no tenant column. Filter with name_pattern or tag, or point the provider host at that tenant.", collection),
			)
			return "", sel, diags
		}
		clause, err := TenantClause(ctx, sdk, tenant)
		if err != nil {
			diags.AddError("Invalid tenant filter", err.Error())
			return "", sel, diags
		}
		clauses = append(clauses, clause)
	}
	return JoinFilter(clauses...), sel, diags
}

func queryString(v types.String) (string, bool) {
	if v.IsNull() || v.IsUnknown() {
		return "", false
	}
	text := strings.TrimSpace(v.ValueString())
	if text == "" {
		return "", false
	}
	return text, true
}

// JoinFilter joins non-empty VergeOS filter clauses with and.
func JoinFilter(clauses ...string) string {
	kept := make([]string, 0, len(clauses))
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause != "" {
			kept = append(kept, clause)
		}
	}
	return strings.Join(kept, " and ")
}

// ListOptions returns a govergeos filter option when filter is set.
func ListOptions(filter string) []vergeos.ListOption {
	if strings.TrimSpace(filter) == "" {
		return nil
	}
	return []vergeos.ListOption{vergeos.WithFilter(filter)}
}

// TenantClause resolves tenant to `tenant eq <key>`.
// A positive integer is a key. Anything else is a tenant name.
func TenantClause(ctx context.Context, sdk *vergeos.Client, tenant string) (string, error) {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return "", nil
	}
	if n, err := strconv.Atoi(tenant); err == nil {
		if n <= 0 {
			return "", fmt.Errorf("tenant %q is not a positive integer", tenant)
		}
		return fmt.Sprintf("tenant eq %d", n), nil
	}
	if sdk == nil || sdk.Tenants == nil {
		return "", fmt.Errorf("tenant client is not configured")
	}
	row, err := sdk.Tenants.GetByName(ctx, tenant)
	if err != nil {
		return "", err
	}
	if row == nil || row.Key.Int() <= 0 {
		return "", fmt.Errorf("tenant %q was not found", tenant)
	}
	return fmt.Sprintf("tenant eq %d", row.Key.Int()), nil
}

// TagMemberKeys returns the object keys assigned tag in collection.
// tag is a name or category/name. The returned map is empty when nothing is assigned.
func TagMemberKeys(ctx context.Context, sdk *vergeos.Client, tag, collection string) (map[string]struct{}, error) {
	if sdk == nil || sdk.Tags == nil || sdk.TagMembers == nil {
		return nil, fmt.Errorf("tag client is not configured")
	}
	tagID, err := resolveTagID(ctx, sdk, tag)
	if err != nil {
		return nil, err
	}
	members, err := sdk.TagMembers.ListByTag(ctx, tagID)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]struct{})
	prefix := strings.TrimSpace(collection) + "/"
	for _, member := range members {
		ref := strings.TrimSpace(member.Member)
		key, ok := strings.CutPrefix(ref, prefix)
		if !ok || key == "" || strings.Contains(key, "/") {
			continue
		}
		keys[key] = struct{}{}
	}
	return keys, nil
}

func resolveTagID(ctx context.Context, sdk *vergeos.Client, ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	category, name, hasCategory := strings.Cut(ref, "/")
	if hasCategory {
		category = strings.TrimSpace(category)
		name = strings.TrimSpace(name)
		if category == "" || name == "" || strings.Contains(name, "/") {
			return 0, fmt.Errorf("tag %q must be a name or category/name", ref)
		}
		if sdk.TagCategories == nil {
			return 0, fmt.Errorf("tag client is not configured")
		}
		cat, err := sdk.TagCategories.GetByName(ctx, category)
		if err != nil {
			return 0, err
		}
		if cat == nil || cat.Key.Int() <= 0 {
			return 0, fmt.Errorf("tag category %q was not found", category)
		}
		row, err := sdk.Tags.GetByName(ctx, cat.Key.Int(), name)
		if err != nil {
			return 0, err
		}
		if row == nil || row.Key.Int() <= 0 {
			return 0, fmt.Errorf("tag %q was not found", ref)
		}
		return row.Key.Int(), nil
	}
	rows, err := sdk.Tags.List(ctx, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(ref))))
	if err != nil {
		return 0, err
	}
	rows = vergeio.KeepExact(rows, ref, func(tag vergeos.Tag) string { return tag.Name })
	if len(rows) == 0 {
		return 0, fmt.Errorf("tag %q was not found", ref)
	}
	if len(rows) > 1 {
		return 0, fmt.Errorf("tag %q matches more than one category; set category/name", ref)
	}
	if rows[0].Key.Int() <= 0 {
		return 0, fmt.Errorf("tag %q was not found", ref)
	}
	return rows[0].Key.Int(), nil
}
