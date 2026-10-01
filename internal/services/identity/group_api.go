// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var _ vergeio.IClient = &GroupApi{}

func NewGroupApi(c *vergeio.Client) (*GroupApi, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &GroupApi{
		name: "Group Api",
		sdk:  sdk,
	}, nil
}

// GroupApi is the govergeos GroupService client for vergeio_group.
type GroupApi struct {
	name string
	sdk  *vergeos.Client
}

func (api *GroupApi) Name() string {
	return api.name
}

func groupCreateRequest(data *GroupResourceModel) *vergeos.GroupCreateRequest {
	req := &vergeos.GroupCreateRequest{
		Name: data.Name.ValueString(),
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	req.Enabled = vergeio.KnownBool(data.Enabled)
	return req
}

func groupUpdateRequest(plan, state *GroupResourceModel) *vergeos.GroupUpdateRequest {
	req := &vergeos.GroupUpdateRequest{
		Name:        vergeio.ChangedString(plan.Name, state.Name),
		Description: vergeio.ChangedString(plan.Description, state.Description),
		Enabled:     vergeio.ChangedBool(plan.Enabled, state.Enabled),
	}
	if req.Name == nil && req.Description == nil && req.Enabled == nil {
		return nil
	}
	return req
}

func applyGroup(data *GroupResourceModel, group *vergeos.Group) {
	data.Id = idString(group.Key.Int())
	data.Name = types.StringValue(group.Name)
	data.Description = types.StringValue(group.Description)
	data.Enabled = types.BoolValue(group.Enabled)
}

func (api *GroupApi) createGroup(ctx context.Context, data *GroupResourceModel) error {
	created, err := api.sdk.Groups.Create(ctx, groupCreateRequest(data))
	if err != nil {
		return err
	}
	applyGroup(data, created)
	tflog.Debug(ctx, fmt.Sprintf("created group %s", data.Id.ValueString()))
	return nil
}

func (api *GroupApi) readGroup(ctx context.Context, data *GroupResourceModel) error {
	id, err := parseID(data.Id, "group")
	if err != nil {
		return err
	}
	group, err := api.sdk.Groups.Get(ctx, id)
	if err != nil {
		return err
	}
	applyGroup(data, group)
	tflog.Debug(ctx, fmt.Sprintf("read group %d", id))
	return nil
}

func (api *GroupApi) updateGroup(ctx context.Context, plan, state *GroupResourceModel) error {
	id, err := parseID(state.Id, "group")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	req := groupUpdateRequest(plan, state)
	if req == nil {
		tflog.Debug(ctx, fmt.Sprintf("group %d is unchanged", id))
		return nil
	}
	updated, err := api.sdk.Groups.Update(ctx, id, req)
	if err != nil {
		return err
	}
	applyGroup(plan, updated)
	tflog.Debug(ctx, fmt.Sprintf("updated group %d", id))
	return nil
}

func (api *GroupApi) deleteGroup(ctx context.Context, data *GroupResourceModel) error {
	id, err := parseID(data.Id, "group")
	if err != nil {
		return err
	}
	if err := api.sdk.Groups.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted group %d", id))
	return nil
}
