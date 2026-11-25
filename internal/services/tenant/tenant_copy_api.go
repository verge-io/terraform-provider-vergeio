// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/shared"

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

// tenantCopyCreatedSkew is how far behind this process a VergeOS clock can be
// and still count as the copy Clone just stored.
const tenantCopyCreatedSkew = 2 * time.Minute

// tenantCopyLookupTimeout bounds the read after a wait failure. The caller
// context may already be canceled.
const tenantCopyLookupTimeout = 30 * time.Second

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
	started := time.Now()
	if err := a.sdk.Tenants.Clone(ctx, tenantID, &vergeos.TenantCloneOptions{Name: name}); err != nil {
		return err
	}
	clone, err := waitTenantByName(ctx, a.sdk, name)
	if err != nil {
		found, findErr := a.lookupTenantByName(ctx, name)
		if findErr != nil {
			return fmt.Errorf("%w; looking up tenant %q: %v", err, name, findErr)
		}
		if !tenantCopyIsNew(found, name, started, tenantID) {
			return err
		}
		assignTenantCopy(data, found)
		return err
	}
	if !tenantCopyIsCopy(clone, name, tenantID) {
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
	assignTenantCopy(data, clone)
	tflog.Debug(ctx, fmt.Sprintf("copied tenant %d as tenant %d", tenantID, clone.Key.Int()))
	return nil
}

func assignTenantCopy(data *TenantCopyResourceModel, clone *vergeos.Tenant) {
	data.Id = idString(clone.Key.Int())
	data.Created = timestamp(clone.Created)
}

// tenantCopyIsCopy reports that tenant is a new tenant with the requested name.
// A snapshot and the source tenant are not.
func tenantCopyIsCopy(tenant *vergeos.Tenant, name string, sourceID int) bool {
	if tenant == nil || tenant.Key.Int() <= 0 || tenant.IsSnapshot || tenant.Name != name {
		return false
	}
	if sourceID > 0 && tenant.Key.Int() == sourceID {
		return false
	}
	return true
}

// tenantCopyIsNew is tenantCopyIsCopy plus the created time. A row that
// VergeOS stored before this clone is not the copy the wait missed.
func tenantCopyIsNew(tenant *vergeos.Tenant, name string, started time.Time, sourceID int) bool {
	if !tenantCopyIsCopy(tenant, name, sourceID) {
		return false
	}
	if tenant.Created > 0 && tenant.Created < started.Add(-tenantCopyCreatedSkew).Unix() {
		return false
	}
	return true
}

func (a *API) lookupTenantByName(ctx context.Context, name string) (*vergeos.Tenant, error) {
	lookupCtx, cancel := tenantCopyLookupContext(ctx)
	defer cancel()
	tenant, err := a.sdk.Tenants.GetByName(lookupCtx, name)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	return tenant, nil
}

func tenantCopyLookupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithTimeout(context.WithoutCancel(ctx), tenantCopyLookupTimeout)
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
	if err := ensureTenantCopyOwned(data, got); err != nil {
		return err
	}
	data.Id = idString(got.Key.Int())
	data.Name = types.StringValue(got.Name)
	data.Created = timestamp(got.Created)
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
	foreign, err := shared.ForeignRow(nil, ensureTenantCopyOwned(data, got))
	if err != nil {
		return err
	}
	if foreign {
		label := ""
		if tenantCopyNameKnown(data.Name) {
			label = strings.TrimSpace(data.Name.ValueString())
		}
		tflog.Debug(ctx, fmt.Sprintf("tenant %d is not copy %q; leaving it in place", id, label))
		return nil
	}
	return a.deleteTenant(ctx, &TenantResourceModel{Id: data.Id})
}

// ensureTenantCopyOwned returns NotFound when the stored key now points at
// another tenant. Name and created are both checked when state has them.
// Created is skipped when it was not stored (import, or state from before
// that attribute existed). A snapshot row is not the copy.
func ensureTenantCopyOwned(data *TenantCopyResourceModel, tenant *vergeos.Tenant) error {
	if data == nil || tenant == nil {
		return fmt.Errorf("tenant copy read returned no row")
	}
	if tenant.IsSnapshot {
		return &vergeos.NotFoundError{Resource: "Tenant", ID: tenant.Key.Int()}
	}
	if tenantCopyNameKnown(data.Name) && tenant.Name != strings.TrimSpace(data.Name.ValueString()) {
		return &vergeos.NotFoundError{Resource: "Tenant", ID: tenant.Key.Int()}
	}
	if tenantCopyCreatedKnown(data.Created) && tenant.Created != data.Created.ValueInt64() {
		return &vergeos.NotFoundError{Resource: "Tenant", ID: tenant.Key.Int()}
	}
	return nil
}

func tenantCopyNameKnown(value types.String) bool {
	return !value.IsNull() && !value.IsUnknown() && strings.TrimSpace(value.ValueString()) != ""
}

func tenantCopyCreatedKnown(value types.Int64) bool {
	return !value.IsNull() && !value.IsUnknown() && value.ValueInt64() != 0
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
