// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// tenantCopyPoll is how often a copied tenant name is read.
// tenantCopyTimeout is the whole wait.
var (
	tenantCopyPoll    = 2 * time.Second
	tenantCopyTimeout = 2 * time.Minute
)

func (a *API) createTenantCopy(ctx context.Context, data *TenantCopyResourceModel) error {
	if a == nil || a.sdk == nil {
		return fmt.Errorf("tenant client is not configured")
	}
	tenantID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return err
	}
	snapshotID, err := parseID(data.SnapshotID, "tenant snapshot")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(data.Name.ValueString())
	if name == "" {
		return fmt.Errorf("name is required")
	}
	snap, err := a.sdk.TenantSnapshots.Get(ctx, snapshotID)
	if err != nil {
		return err
	}
	if snap.Tenant.Int() != tenantID {
		return fmt.Errorf("snapshot %d belongs to tenant %d, not %d", snapshotID, snap.Tenant.Int(), tenantID)
	}
	existing, err := a.sdk.Tenants.GetByName(ctx, name)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	if existing != nil && existing.Key.Int() > 0 {
		return fmt.Errorf("tenant %q already exists", name)
	}
	// Tenants.Clone copies the current tenant. TenantCloneOptions has name,
	// no_vnet, no_storage, and no_nodes. It has no snapshot field.
	// tenant_actions restore with params.snapshot overwrites tenant_id.
	if err := a.sdk.Tenants.Clone(ctx, tenantID, &vergeos.TenantCloneOptions{Name: name}); err != nil {
		return err
	}
	clone, err := waitTenantByName(ctx, a.sdk, name)
	if err != nil {
		return err
	}
	if clone.Key.Int() == tenantID || clone.IsSnapshot {
		return fmt.Errorf("copy %q resolved to tenant %d, which is not a new tenant", name, clone.Key.Int())
	}
	if boolKnownTrue(data.Isolate) && !clone.Isolate {
		if err := a.sdk.Tenants.IsolateOn(ctx, clone.Key.Int()); err != nil {
			return fmt.Errorf("isolate tenant %d: %w", clone.Key.Int(), err)
		}
		clone, err = a.sdk.Tenants.Get(ctx, clone.Key.Int())
		if err != nil {
			return err
		}
		if !clone.Isolate {
			return fmt.Errorf("tenant %d is not isolated", clone.Key.Int())
		}
	}
	data.Id = idString(clone.Key.Int())
	data.SnapshotName = types.StringValue(snap.Name)
	tflog.Debug(ctx, fmt.Sprintf("copied tenant %d as tenant %d", tenantID, clone.Key.Int()))
	return nil
}

func (a *API) readTenantCopy(ctx context.Context, data *TenantCopyResourceModel) error {
	if a == nil || a.sdk == nil {
		return fmt.Errorf("tenant client is not configured")
	}
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		return err
	}
	sourceID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return err
	}
	if id == sourceID {
		return fmt.Errorf("copy id %d is the source tenant", id)
	}
	got, err := a.sdk.Tenants.Get(ctx, id)
	if err != nil {
		return err
	}
	if got.IsSnapshot || got.Name != strings.TrimSpace(data.Name.ValueString()) {
		return &vergeos.NotFoundError{Resource: "Tenant", ID: id}
	}
	data.Id = idString(got.Key.Int())
	data.Name = types.StringValue(got.Name)
	return nil
}

func (a *API) deleteTenantCopy(ctx context.Context, data *TenantCopyResourceModel) error {
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		return err
	}
	sourceID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return err
	}
	if id == sourceID {
		return fmt.Errorf("refusing to delete source tenant %d", sourceID)
	}
	got, err := a.sdk.Tenants.Get(ctx, id)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	if got.IsSnapshot {
		return fmt.Errorf("refusing to delete snapshot tenant %d", id)
	}
	return a.deleteTenant(ctx, &TenantResourceModel{Id: data.Id})
}

func waitTenantByName(ctx context.Context, sdk *vergeos.Client, name string) (*vergeos.Tenant, error) {
	deadline := time.Now().Add(tenantCopyTimeout)
	for {
		tenant, err := sdk.Tenants.GetByName(ctx, name)
		if err == nil && tenant != nil && tenant.Key.Int() > 0 && !tenant.IsSnapshot {
			return tenant, nil
		}
		if err != nil && !vergeos.IsNotFoundError(err) {
			return nil, err
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("tenant %q was not created", name)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		timer := time.NewTimer(tenantCopyPoll)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
