// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

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
	_ list.ListResource              = &TenantListResource{}
	_ list.ListResourceWithConfigure = &TenantListResource{}
)

func NewTenantListResource() list.ListResource {
	r := &TenantListResource{}
	r.Suffix = "tenant"
	r.Description = "Tenants on this VergeOS system. Snapshot tenants are omitted. Filter with name_pattern or tag. tenant is rejected."
	r.Collection = "tenants"
	r.TenantSupported = false
	r.ListFn = r.list
	return r
}

// TenantListResource lists vergeio_tenant.
type TenantListResource struct {
	shared.ObjectLister
	api *API
}

func (r *TenantListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.ObjectLister.Configure(ctx, req, resp)
	if resp.Diagnostics.HasError() || r.Client == nil {
		return
	}
	api, err := NewAPI(r.Client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.api = api
}

func (r *TenantListResource) list(ctx context.Context, filter string, sel shared.Selection, include bool) ([]shared.Listed, diag.Diagnostics) {
	var diags diag.Diagnostics
	sdk, more := shared.SDKClient(r.Client)
	diags.Append(more...)
	if diags.HasError() {
		return nil, diags
	}
	rows, err := sdk.Tenants.List(ctx, shared.ListOptions(filter)...)
	if err != nil {
		diags.AddError("Error listing tenants", err.Error())
		return nil, diags
	}
	items := make([]shared.Listed, 0, len(rows))
	for _, row := range rows {
		if row.IsSnapshot || row.Key.Int() <= 0 {
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
				diags.AddError("Error reading tenant "+row.Name, err.Error())
				return nil, diags
			}
			item.Resource = model
		}
		items = append(items, item)
	}
	return items, diags
}

func (r *TenantListResource) read(ctx context.Context, id string) (TenantResourceModel, error) {
	if r.api == nil {
		return TenantResourceModel{}, errListAPI
	}
	model := TenantResourceModel{Id: types.StringValue(id)}
	if err := r.api.readTenant(ctx, &model); err != nil {
		return TenantResourceModel{}, err
	}
	return tenantForState(&model), nil
}
