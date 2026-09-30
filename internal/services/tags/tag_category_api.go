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

// tagCategoryCreateRequest sends only flags the configuration set.
// A null or unknown bool stays nil, and encoding/json omits that field.
// An explicit false is sent. Defaulting an omitted taggable_* flag to false
// turns tagging off for that object type.
func tagCategoryCreateRequest(data *TagCategoryResourceModel) *vergeos.TagCategoryCreateRequest {
	req := &vergeos.TagCategoryCreateRequest{
		Name: data.Name.ValueString(),
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	req.SingleTagSelection = vergeio.KnownBool(data.SingleTagSelection)
	req.TaggableVolumes = vergeio.KnownBool(data.TaggableVolumes)
	req.TaggableVNets = vergeio.KnownBool(data.TaggableVNets)
	req.TaggableVNetRules = vergeio.KnownBool(data.TaggableVNetRules)
	req.TaggableVMwareContainers = vergeio.KnownBool(data.TaggableVMwareContainers)
	req.TaggableVMs = vergeio.KnownBool(data.TaggableVMs)
	req.TaggableUsers = vergeio.KnownBool(data.TaggableUsers)
	req.TaggableTenantNodes = vergeio.KnownBool(data.TaggableTenantNodes)
	req.TaggableSites = vergeio.KnownBool(data.TaggableSites)
	req.TaggableNodes = vergeio.KnownBool(data.TaggableNodes)
	req.TaggableGroups = vergeio.KnownBool(data.TaggableGroups)
	req.TaggableClusters = vergeio.KnownBool(data.TaggableClusters)
	req.TaggableTenants = vergeio.KnownBool(data.TaggableTenants)
	return req
}

// tagCategoryUpdateRequest sends a flag only when the plan sets a new value.
// Unchanged flags, including an explicit false already stored in state, are
// left out. An omitted flag is null in the plan and is left out as well.
func tagCategoryUpdateRequest(plan, state *TagCategoryResourceModel) *vergeos.TagCategoryUpdateRequest {
	req := &vergeos.TagCategoryUpdateRequest{
		Name:                     vergeio.ChangedString(plan.Name, state.Name),
		Description:              vergeio.ChangedString(plan.Description, state.Description),
		SingleTagSelection:       vergeio.ChangedBool(plan.SingleTagSelection, state.SingleTagSelection),
		TaggableVolumes:          vergeio.ChangedBool(plan.TaggableVolumes, state.TaggableVolumes),
		TaggableVNets:            vergeio.ChangedBool(plan.TaggableVNets, state.TaggableVNets),
		TaggableVNetRules:        vergeio.ChangedBool(plan.TaggableVNetRules, state.TaggableVNetRules),
		TaggableVMwareContainers: vergeio.ChangedBool(plan.TaggableVMwareContainers, state.TaggableVMwareContainers),
		TaggableVMs:              vergeio.ChangedBool(plan.TaggableVMs, state.TaggableVMs),
		TaggableUsers:            vergeio.ChangedBool(plan.TaggableUsers, state.TaggableUsers),
		TaggableTenantNodes:      vergeio.ChangedBool(plan.TaggableTenantNodes, state.TaggableTenantNodes),
		TaggableSites:            vergeio.ChangedBool(plan.TaggableSites, state.TaggableSites),
		TaggableNodes:            vergeio.ChangedBool(plan.TaggableNodes, state.TaggableNodes),
		TaggableGroups:           vergeio.ChangedBool(plan.TaggableGroups, state.TaggableGroups),
		TaggableClusters:         vergeio.ChangedBool(plan.TaggableClusters, state.TaggableClusters),
		TaggableTenants:          vergeio.ChangedBool(plan.TaggableTenants, state.TaggableTenants),
	}
	if apiRequestEmpty(req) {
		return nil
	}
	return req
}

func applyTagCategory(data *TagCategoryResourceModel, category *vergeos.TagCategory) {
	data.Id = idString(category.Key.Int())
	data.Name = types.StringValue(category.Name)
	data.Description = types.StringValue(category.Description)
	data.SingleTagSelection = types.BoolValue(category.SingleTagSelection)
	data.TaggableVolumes = types.BoolValue(category.TaggableVolumes)
	data.TaggableVNets = types.BoolValue(category.TaggableVNets)
	data.TaggableVNetRules = types.BoolValue(category.TaggableVNetRules)
	data.TaggableVMwareContainers = types.BoolValue(category.TaggableVMwareContainers)
	data.TaggableVMs = types.BoolValue(category.TaggableVMs)
	data.TaggableUsers = types.BoolValue(category.TaggableUsers)
	data.TaggableTenantNodes = types.BoolValue(category.TaggableTenantNodes)
	data.TaggableSites = types.BoolValue(category.TaggableSites)
	data.TaggableNodes = types.BoolValue(category.TaggableNodes)
	data.TaggableGroups = types.BoolValue(category.TaggableGroups)
	data.TaggableClusters = types.BoolValue(category.TaggableClusters)
	data.TaggableTenants = types.BoolValue(category.TaggableTenants)
}

func (ta *TagsApi) createTagCategory(ctx context.Context, data *TagCategoryResourceModel) error {
	created, err := ta.sdk.TagCategories.Create(ctx, tagCategoryCreateRequest(data))
	if err != nil {
		return err
	}
	applyTagCategory(data, created)
	tflog.Debug(ctx, fmt.Sprintf("created tag category %s", data.Id.ValueString()))
	return nil
}

func (ta *TagsApi) readTagCategory(ctx context.Context, data *TagCategoryResourceModel) error {
	id, err := parseID(data.Id, "tag category")
	if err != nil {
		return err
	}
	category, err := ta.sdk.TagCategories.Get(ctx, id)
	if err != nil {
		return err
	}
	applyTagCategory(data, category)
	tflog.Debug(ctx, fmt.Sprintf("read tag category %d", id))
	return nil
}

func (ta *TagsApi) updateTagCategory(ctx context.Context, plan, state *TagCategoryResourceModel) error {
	id, err := parseID(state.Id, "tag category")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	req := tagCategoryUpdateRequest(plan, state)
	if req == nil {
		tflog.Debug(ctx, fmt.Sprintf("tag category %d is unchanged", id))
		return nil
	}
	updated, err := ta.sdk.TagCategories.Update(ctx, id, req)
	if err != nil {
		return err
	}
	applyTagCategory(plan, updated)
	tflog.Debug(ctx, fmt.Sprintf("updated tag category %d", id))
	return nil
}

func (ta *TagsApi) deleteTagCategory(ctx context.Context, data *TagCategoryResourceModel) error {
	id, err := parseID(data.Id, "tag category")
	if err != nil {
		return err
	}
	if err := ta.sdk.TagCategories.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tag category %d", id))
	return nil
}
