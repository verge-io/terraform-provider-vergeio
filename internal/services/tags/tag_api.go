// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func tagCreateRequest(data *TagResourceModel) *vergeos.TagCreateRequest {
	req := &vergeos.TagCreateRequest{
		Category: int(data.Category.ValueInt32()),
		Name:     data.Name.ValueString(),
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	return req
}

// tagUpdateRequest does not include category. VergeOS does not move a tag
// between categories; the resource replaces the tag instead.
func tagUpdateRequest(plan, state *TagResourceModel) *vergeos.TagUpdateRequest {
	req := &vergeos.TagUpdateRequest{
		Name:        vergeio.ChangedString(plan.Name, state.Name),
		Description: vergeio.ChangedString(plan.Description, state.Description),
	}
	if apiRequestEmpty(req) {
		return nil
	}
	return req
}

func applyTag(data *TagResourceModel, tag *vergeos.Tag) {
	data.Id = idString(tag.Key.Int())
	data.Name = types.StringValue(tag.Name)
	data.Description = types.StringValue(tag.Description)
	data.Category = types.Int32Value(int32(tag.Category.Int()))
	data.CategoryName = types.StringValue(tag.CategoryDisplay)
}

func (ta *TagsApi) createTag(ctx context.Context, data *TagResourceModel) error {
	created, err := ta.sdk.Tags.Create(ctx, tagCreateRequest(data))
	if err != nil {
		return err
	}
	applyTag(data, created)
	tflog.Debug(ctx, fmt.Sprintf("created tag %s", data.Id.ValueString()))
	return nil
}

func (ta *TagsApi) readTag(ctx context.Context, data *TagResourceModel) error {
	id, err := parseID(data.Id, "tag")
	if err != nil {
		return err
	}
	tag, err := ta.sdk.Tags.Get(ctx, id)
	if err != nil {
		return err
	}
	applyTag(data, tag)
	tflog.Debug(ctx, fmt.Sprintf("read tag %d", id))
	return nil
}

func (ta *TagsApi) updateTag(ctx context.Context, plan, state *TagResourceModel) error {
	id, err := parseID(state.Id, "tag")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	req := tagUpdateRequest(plan, state)
	if req == nil {
		tflog.Debug(ctx, fmt.Sprintf("tag %d is unchanged", id))
		return nil
	}
	updated, err := ta.sdk.Tags.Update(ctx, id, req)
	if err != nil {
		return err
	}
	applyTag(plan, updated)
	tflog.Debug(ctx, fmt.Sprintf("updated tag %d", id))
	return nil
}

func (ta *TagsApi) deleteTag(ctx context.Context, data *TagResourceModel) error {
	id, err := parseID(data.Id, "tag")
	if err != nil {
		return err
	}
	if err := ta.sdk.Tags.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tag %d", id))
	return nil
}
