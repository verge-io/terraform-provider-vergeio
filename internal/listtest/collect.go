// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package listtest

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Collect runs a list resource and returns every result.
// lister must already be configured. managed supplies the resource and identity schemas.
func Collect(ctx context.Context, lister list.ListResource, managed resource.Resource, query shared.ListQuery, include bool, limit int64) ([]list.ListResult, error) {
	schemaResp := &list.ListResourceSchemaResponse{}
	lister.ListResourceConfigSchema(ctx, list.ListResourceSchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		return nil, fmt.Errorf("list schema: %v", schemaResp.Diagnostics)
	}
	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &query); diags.HasError() {
		return nil, fmt.Errorf("list config: %v", diags)
	}
	resSchema := &resource.SchemaResponse{}
	managed.Schema(ctx, resource.SchemaRequest{}, resSchema)
	if resSchema.Diagnostics.HasError() {
		return nil, fmt.Errorf("resource schema: %v", resSchema.Diagnostics)
	}
	withIdentity, ok := managed.(resource.ResourceWithIdentity)
	if !ok {
		return nil, fmt.Errorf("%T has no identity schema", managed)
	}
	idSchema := &resource.IdentitySchemaResponse{}
	withIdentity.IdentitySchema(ctx, resource.IdentitySchemaRequest{}, idSchema)
	if idSchema.Diagnostics.HasError() {
		return nil, fmt.Errorf("identity schema: %v", idSchema.Diagnostics)
	}
	stream := &list.ListResultsStream{}
	lister.List(ctx, list.ListRequest{
		Config:                 tfsdk.Config{Raw: state.Raw, Schema: schemaResp.Schema},
		IncludeResource:        include,
		Limit:                  limit,
		ResourceSchema:         resSchema.Schema,
		ResourceIdentitySchema: idSchema.IdentitySchema,
	}, stream)
	if stream.Results == nil {
		return nil, fmt.Errorf("list returned no result stream")
	}
	var got []list.ListResult
	for result := range stream.Results {
		got = append(got, result)
		if result.Diagnostics.HasError() {
			return got, fmt.Errorf("%v", result.Diagnostics)
		}
	}
	return got, nil
}

// IdentityID reads the identity id attribute.
func IdentityID(ctx context.Context, result list.ListResult) (string, error) {
	if result.Identity == nil {
		return "", fmt.Errorf("result %q has no identity", result.DisplayName)
	}
	var id types.String
	if diags := result.Identity.GetAttribute(ctx, path.Root("id"), &id); diags.HasError() {
		return "", fmt.Errorf("%v", diags)
	}
	return id.ValueString(), nil
}
