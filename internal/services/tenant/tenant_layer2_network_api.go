// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func tenantLayer2CreateRequest(data *TenantLayer2NetworkResourceModel) (*vergeos.TenantLayer2NetworkCreateRequest, error) {
	tenantID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return nil, err
	}
	networkID, err := parseID(data.NetworkID, "network")
	if err != nil {
		return nil, err
	}
	enabled := true
	if !data.Enabled.IsNull() && !data.Enabled.IsUnknown() {
		enabled = data.Enabled.ValueBool()
	}
	return &vergeos.TenantLayer2NetworkCreateRequest{
		Tenant:  tenantID,
		VNet:    networkID,
		Enabled: &enabled,
	}, nil
}

func assignTenantLayer2Network(data *TenantLayer2NetworkResourceModel, row *vergeos.TenantLayer2Network) {
	data.Id = idString(row.Key.Int())
	if tenantID := row.Tenant.Int(); tenantID > 0 {
		data.TenantID = idString(tenantID)
	}
	if networkID := row.VNet.Int(); networkID > 0 {
		data.NetworkID = idString(networkID)
	}
	data.Enabled = types.BoolValue(row.Enabled)
}

// ensureTenantLayer2Owned returns NotFound when the stored key now points at
// another assignment. Skip each check when that attribute is unset (import).
func ensureTenantLayer2Owned(data *TenantLayer2NetworkResourceModel, row *vergeos.TenantLayer2Network) error {
	if tenantID, ok, err := configuredPositiveID(data.TenantID); err != nil {
		return err
	} else if ok && row.Tenant.Int() != tenantID {
		return &vergeos.NotFoundError{Resource: "TenantLayer2Network", ID: row.Key.Int()}
	}
	if networkID, ok, err := configuredPositiveID(data.NetworkID); err != nil {
		return err
	} else if ok && row.VNet.Int() != networkID {
		return &vergeos.NotFoundError{Resource: "TenantLayer2Network", ID: row.Key.Int()}
	}
	return nil
}

func (a *API) createTenantLayer2Network(ctx context.Context, data *TenantLayer2NetworkResourceModel) error {
	req, err := tenantLayer2CreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.TenantLayer2Networks.Create(ctx, req)
	if created != nil {
		assignTenantLayer2Network(data, created)
		tflog.Debug(ctx, fmt.Sprintf("created tenant layer 2 network %d", created.Key.Int()))
	}
	return err
}

func (a *API) readTenantLayer2Network(ctx context.Context, data *TenantLayer2NetworkResourceModel) error {
	id, err := parseID(data.Id, "tenant layer 2 network")
	if err != nil {
		return err
	}
	row, err := a.sdk.TenantLayer2Networks.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureTenantLayer2Owned(data, row); err != nil {
		return err
	}
	assignTenantLayer2Network(data, row)
	return nil
}

func (a *API) updateTenantLayer2Network(ctx context.Context, plan *TenantLayer2NetworkResourceModel) error {
	id, err := parseID(plan.Id, "tenant layer 2 network")
	if err != nil {
		return err
	}
	if plan.Enabled.IsNull() || plan.Enabled.IsUnknown() {
		return fmt.Errorf("enabled is required")
	}
	enabled := plan.Enabled.ValueBool()
	updated, err := a.sdk.TenantLayer2Networks.Update(ctx, id, &vergeos.TenantLayer2NetworkUpdateRequest{Enabled: &enabled})
	if err != nil {
		return err
	}
	if err := ensureTenantLayer2Owned(plan, updated); err != nil {
		return err
	}
	assignTenantLayer2Network(plan, updated)
	tflog.Debug(ctx, fmt.Sprintf("updated tenant layer 2 network %d enabled=%t", id, enabled))
	return nil
}

// deleteTenantLayer2Network disables the assignment, then deletes it.
// VergeOS rejects a delete that skips the disable. A missing row is success.
func (a *API) deleteTenantLayer2Network(ctx context.Context, data *TenantLayer2NetworkResourceModel) error {
	id, err := parseID(data.Id, "tenant layer 2 network")
	if err != nil {
		return err
	}
	if err := a.sdk.TenantLayer2Networks.Disable(ctx, id); err != nil {
		missing, checkErr := a.tenantLayer2Missing(ctx, id)
		if checkErr != nil {
			return fmt.Errorf("disable tenant layer 2 network %d before delete: %w", id, err)
		}
		if missing {
			tflog.Debug(ctx, fmt.Sprintf("tenant layer 2 network %d already deleted", id))
			return nil
		}
		return fmt.Errorf("disable tenant layer 2 network %d before delete: %w", id, err)
	}
	if err := a.sdk.TenantLayer2Networks.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			tflog.Debug(ctx, fmt.Sprintf("tenant layer 2 network %d already deleted", id))
			return nil
		}
		return fmt.Errorf("disabled tenant layer 2 network %d, then delete failed: %w", id, err)
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant layer 2 network %d", id))
	return nil
}

func (a *API) tenantLayer2Missing(ctx context.Context, id int) (bool, error) {
	_, err := a.sdk.TenantLayer2Networks.Get(ctx, id)
	if vergeos.IsNotFoundError(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}
