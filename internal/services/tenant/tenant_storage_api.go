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

func tenantStorageCreateRequest(data *TenantStorageResourceModel) (*vergeos.TenantStorageCreateRequest, error) {
	tenantID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return nil, err
	}
	if data.Tier.IsNull() || data.Tier.IsUnknown() {
		return nil, fmt.Errorf("tier is required")
	}
	if data.Provisioned.IsNull() || data.Provisioned.IsUnknown() {
		return nil, fmt.Errorf("provisioned is required")
	}
	return &vergeos.TenantStorageCreateRequest{
		Tenant:      tenantID,
		Tier:        int(data.Tier.ValueInt32()),
		Provisioned: data.Provisioned.ValueInt64(),
	}, nil
}

func tenantStorageUpdateRequest(plan, state *TenantStorageResourceModel) *vergeos.TenantStorageUpdateRequest {
	req := &vergeos.TenantStorageUpdateRequest{
		Provisioned: vergeio.ChangedInt64(plan.Provisioned, state.Provisioned),
	}
	if req.Provisioned == nil {
		return nil
	}
	return req
}

func (a *API) createTenantStorage(ctx context.Context, data *TenantStorageResourceModel) error {
	req, err := tenantStorageCreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.TenantStorage.Create(ctx, req)
	if err != nil {
		return err
	}
	data.Id = idString(created.Key.Int())
	tflog.Debug(ctx, fmt.Sprintf("created tenant storage %d", created.Key.Int()))
	return nil
}

func (a *API) updateTenantStorage(ctx context.Context, plan, state *TenantStorageResourceModel) error {
	id, err := parseID(state.Id, "tenant storage")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	plan.TenantID = state.TenantID
	req := tenantStorageUpdateRequest(plan, state)
	if req == nil {
		return nil
	}
	if _, err := a.sdk.TenantStorage.Update(ctx, id, req); err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("updated tenant storage %d", id))
	return nil
}

func (a *API) readTenantStorage(ctx context.Context, data *TenantStorageResourceModel) error {
	id, err := parseID(data.Id, "tenant storage")
	if err != nil {
		return err
	}
	storage, err := a.sdk.TenantStorage.Get(ctx, id)
	if err != nil {
		return err
	}
	assignTenantStorage(data, storage)
	return nil
}

func assignTenantStorage(data *TenantStorageResourceModel, storage *vergeos.TenantStorage) {
	data.Id = idString(storage.Key.Int())
	if storage.Tenant.Int() > 0 {
		data.TenantID = idString(storage.Tenant.Int())
	}
	// Tier 0 is a real VergeOS tier. Do not treat 0 as unset.
	data.Tier = types.Int32Value(int32(storage.Tier.Int()))
	data.Provisioned = types.Int64Value(storage.Provisioned)
	data.Used = types.Int64Value(storage.Used)
	data.Allocated = types.Int64Value(storage.Allocated)
	data.UsedPct = types.Int32Value(int32(storage.UsedPct))
	data.LastUpdate = timestamp(storage.LastUpdate)
}

func (a *API) deleteTenantStorage(ctx context.Context, data *TenantStorageResourceModel) error {
	id, err := parseID(data.Id, "tenant storage")
	if err != nil {
		return err
	}
	if err := a.sdk.TenantStorage.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant storage %d", id))
	return nil
}
