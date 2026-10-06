// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"strconv"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ list.ListResource              = &GroupListResource{}
	_ list.ListResourceWithConfigure = &GroupListResource{}
)

func NewGroupListResource() list.ListResource {
	r := &GroupListResource{}
	r.Suffix = "group"
	r.Description = "Groups on this VergeOS system. Filter with name_pattern or tag. tenant is rejected; groups live on the system the provider is pointed at."
	r.Collection = "groups"
	r.TenantSupported = false
	r.ListFn = r.list
	return r
}

// GroupListResource lists vergeio_group.
type GroupListResource struct {
	shared.ObjectLister
	api *GroupApi
}

func (r *GroupListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.ObjectLister.Configure(ctx, req, resp)
	if resp.Diagnostics.HasError() || r.Client == nil {
		return
	}
	api, err := NewGroupApi(r.Client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.api = api
}

func (r *GroupListResource) list(ctx context.Context, filter string, sel shared.Selection, include bool) ([]shared.Listed, diag.Diagnostics) {
	var diags diag.Diagnostics
	sdk, more := shared.SDKClient(r.Client)
	diags.Append(more...)
	if diags.HasError() {
		return nil, diags
	}
	rows, err := sdk.Groups.List(ctx, shared.ListOptions(filter)...)
	if err != nil {
		diags.AddError("Error listing groups", err.Error())
		return nil, diags
	}
	items := make([]shared.Listed, 0, len(rows))
	for _, row := range rows {
		if row.Key.Int() <= 0 {
			continue
		}
		id := strconv.Itoa(row.Key.Int())
		if !sel.Allow(row.Name, id) {
			continue
		}
		item := shared.Listed{DisplayName: row.Name, ID: id}
		if include {
			model, err := r.read(ctx, id)
			if err != nil {
				diags.AddError("Error reading group "+row.Name, err.Error())
				return nil, diags
			}
			item.Resource = model
		}
		items = append(items, item)
	}
	return items, diags
}

func (r *GroupListResource) read(ctx context.Context, id string) (GroupResourceModel, error) {
	if r.api == nil {
		return GroupResourceModel{}, errListAPI
	}
	model := GroupResourceModel{Id: types.StringValue(id)}
	if err := r.api.readGroup(ctx, &model); err != nil {
		return GroupResourceModel{}, err
	}
	return groupForState(&model), nil
}
