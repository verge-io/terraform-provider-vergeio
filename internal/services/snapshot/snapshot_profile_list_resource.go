// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

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
	_ list.ListResource              = &SnapshotProfileListResource{}
	_ list.ListResourceWithConfigure = &SnapshotProfileListResource{}
)

func NewSnapshotProfileListResource() list.ListResource {
	r := &SnapshotProfileListResource{}
	r.Suffix = "snapshot_profile"
	r.Description = "Snapshot profiles on this VergeOS system. Filter with name_pattern or tag. tenant is rejected."
	r.Collection = "snapshot_profiles"
	r.TenantSupported = false
	r.ListFn = r.list
	return r
}

// SnapshotProfileListResource lists vergeio_snapshot_profile.
type SnapshotProfileListResource struct {
	shared.ObjectLister
	api *SnapshotProfileApi
}

func (r *SnapshotProfileListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.ObjectLister.Configure(ctx, req, resp)
	if resp.Diagnostics.HasError() || r.Client == nil {
		return
	}
	api, err := NewSnapshotProfileApi(r.Client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.api = api
}

func (r *SnapshotProfileListResource) list(ctx context.Context, filter string, sel shared.Selection, include bool) ([]shared.Listed, diag.Diagnostics) {
	var diags diag.Diagnostics
	sdk, more := shared.SDKClient(r.Client)
	diags.Append(more...)
	if diags.HasError() {
		return nil, diags
	}
	rows, err := sdk.SnapshotProfiles.List(ctx, shared.ListOptions(filter)...)
	if err != nil {
		diags.AddError("Error listing snapshot profiles", err.Error())
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
				diags.AddError("Error reading snapshot profile "+row.Name, err.Error())
				return nil, diags
			}
			item.Resource = model
		}
		items = append(items, item)
	}
	return items, diags
}

func (r *SnapshotProfileListResource) read(ctx context.Context, id string) (SnapshotProfileResourceModel, error) {
	if r.api == nil {
		return SnapshotProfileResourceModel{}, errListAPI
	}
	model := SnapshotProfileResourceModel{Id: types.StringValue(id)}
	if err := r.api.readProfile(ctx, &model); err != nil {
		return SnapshotProfileResourceModel{}, err
	}
	return profileForState(&model), nil
}
