// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func (a *API) readTenants(ctx context.Context, data *TenantsDataSourceModel) error {
	var opts []vergeos.ListOption
	filterName := ""
	if name := vergeio.KnownString(data.FilterName); name != nil && *name != "" {
		filterName = *name
		opts = append(opts, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(filterName))))
	}
	tenants, err := a.sdk.Tenants.List(ctx, opts...)
	if err != nil {
		return err
	}
	tenants = vergeio.KeepExact(tenants, filterName, func(tenant vergeos.Tenant) string {
		return tenant.Name
	})
	models := make([]*TenantModel, 0, len(tenants))
	for i := range tenants {
		tenant := tenants[i]
		facts, err := a.tenantFacts(ctx, tenant.Key.Int(), tenant.UIAddress.Int())
		if err != nil {
			return err
		}
		models = append(models, tenantModel(&tenant, facts))
	}
	data.Tenants = models
	tflog.Debug(ctx, fmt.Sprintf("read %d tenants", len(models)))
	return nil
}

func tenantModel(tenant *vergeos.Tenant, facts tenantFacts) *TenantModel {
	return &TenantModel{
		Id:                   types.Int32Value(int32(tenant.Key.Int())),
		Name:                 types.StringValue(tenant.Name),
		Description:          types.StringValue(tenant.Description),
		URL:                  types.StringValue(tenant.URL),
		UUID:                 types.StringValue(tenant.UUID),
		VNet:                 flexID(tenant.VNet),
		UIAddressID:          facts.UIAddressID,
		UIAddress:            facts.UIAddress,
		Isolate:              types.BoolValue(tenant.Isolate),
		IsSnapshot:           types.BoolValue(tenant.IsSnapshot),
		ExposeCloudSnapshots: types.BoolValue(tenant.ExposeCloudSnapshots),
		AllowBranding:        types.BoolValue(tenant.AllowBranding),
		ThemeAccess:          enumString(tenant.ThemeAccess),
		PowerState:           facts.PowerState,
		Status:               facts.Status,
		State:                facts.State,
		Creator:              types.StringValue(tenant.Creator),
		Created:              timestamp(tenant.Created),
	}
}
