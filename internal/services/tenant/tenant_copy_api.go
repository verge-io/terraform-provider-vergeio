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
	name := strings.TrimSpace(data.Name.ValueString())
	if name == "" {
		return fmt.Errorf("name is required")
	}
	existing, err := a.sdk.Tenants.GetByName(ctx, name)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	if existing != nil && existing.Key.Int() > 0 {
		return fmt.Errorf("tenant %q already exists", name)
	}
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
	copyID := clone.Key.Int()
	if boolKnownTrue(data.Isolate) && !clone.Isolate {
		if err := a.sdk.Tenants.IsolateOn(ctx, copyID); err != nil {
			return a.deleteFailedTenantCopy(ctx, copyID, fmt.Errorf("isolate tenant %d: %w", copyID, err))
		}
		refreshed, err := a.sdk.Tenants.Get(ctx, copyID)
		if err != nil {
			return a.deleteFailedTenantCopy(ctx, copyID, err)
		}
		if !refreshed.Isolate {
			return a.deleteFailedTenantCopy(ctx, copyID, fmt.Errorf("tenant %d is not isolated", copyID))
		}
		clone = refreshed
	}
	data.Id = idString(clone.Key.Int())
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
	sourceID, err := optionalTenantID(data.TenantID)
	if err != nil {
		return err
	}
	if sourceID > 0 && id == sourceID {
		return fmt.Errorf("copy id %d is the source tenant", id)
	}
	got, err := a.sdk.Tenants.Get(ctx, id)
	if err != nil {
		return err
	}
	if got.IsSnapshot {
		return &vergeos.NotFoundError{Resource: "Tenant", ID: id}
	}
	if !data.Name.IsNull() && !data.Name.IsUnknown() && got.Name != strings.TrimSpace(data.Name.ValueString()) {
		return &vergeos.NotFoundError{Resource: "Tenant", ID: id}
	}
	data.Id = idString(got.Key.Int())
	data.Name = types.StringValue(got.Name)
	// isolate is the configured choice to post isolateon. false does not
	// post isolateoff, so the copied tenant can stay isolated. Writing that
	// live flag back would not match the configuration and RequiresReplace
	// would clone again on every plan. A missing value uses the default true.
	if data.Isolate.IsNull() || data.Isolate.IsUnknown() {
		data.Isolate = types.BoolValue(true)
	}
	return nil
}

func (a *API) deleteTenantCopy(ctx context.Context, data *TenantCopyResourceModel) error {
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		return err
	}
	sourceID, err := optionalTenantID(data.TenantID)
	if err != nil {
		return err
	}
	if sourceID > 0 && id == sourceID {
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

// deleteFailedTenantCopy removes a copy that was created and then failed
// isolation. The id is not stored, so a later apply cannot see it.
func (a *API) deleteFailedTenantCopy(ctx context.Context, id int, cause error) error {
	if err := a.deleteTenant(ctx, &TenantResourceModel{Id: idString(id)}); err != nil {
		return fmt.Errorf("%w; delete tenant %d: %v", cause, id, err)
	}
	return cause
}

// optionalTenantID reads tenant_id. Null and unknown are valid after import,
// when only the copy id is known.
func optionalTenantID(value types.String) (int, error) {
	if value.IsNull() || value.IsUnknown() {
		return 0, nil
	}
	return parseID(value, "tenant")
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
