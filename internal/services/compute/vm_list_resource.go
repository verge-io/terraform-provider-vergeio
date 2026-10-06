// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

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
	_ list.ListResource              = &VMListResource{}
	_ list.ListResourceWithConfigure = &VMListResource{}
)

func NewVMListResource() list.ListResource {
	r := &VMListResource{}
	r.Suffix = "vm"
	r.Description = "VMs on this VergeOS system. Snapshot VMs are omitted. Filter with name_pattern, tag, or tenant."
	r.Collection = "vms"
	r.TenantSupported = true
	r.ListFn = r.list
	return r
}

// VMListResource lists vergeio_vm.
type VMListResource struct {
	shared.ObjectLister
	api *VMApi
}

func (r *VMListResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.ObjectLister.Configure(ctx, req, resp)
	if resp.Diagnostics.HasError() || r.Client == nil {
		return
	}
	api, err := NewVMApi(r.Client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.api = api
}

func (r *VMListResource) list(ctx context.Context, filter string, sel shared.Selection, include bool) ([]shared.Listed, diag.Diagnostics) {
	var diags diag.Diagnostics
	sdk, more := shared.SDKClient(r.Client)
	diags.Append(more...)
	if diags.HasError() {
		return nil, diags
	}
	rows, err := sdk.VMs.List(ctx, shared.ListOptions(filter)...)
	if err != nil {
		diags.AddError("Error listing VMs", err.Error())
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
				diags.AddError("Error reading VM "+row.Name, err.Error())
				return nil, diags
			}
			item.Resource = model
		}
		items = append(items, item)
	}
	return items, diags
}

func (r *VMListResource) read(ctx context.Context, id string) (VMResourceModel, error) {
	if r.api == nil {
		return VMResourceModel{}, errListAPI
	}
	model := VMResourceModel{Id: types.StringValue(id)}
	if _, err := r.api.readVM(ctx, &model, false); err != nil {
		return VMResourceModel{}, err
	}
	scrubVMWriteOnly(&model)
	return model, nil
}
