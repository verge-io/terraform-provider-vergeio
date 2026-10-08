// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// tenantSnapshotExpiration is the expiration write for one update.
// setNever calls SetNeverExpires. expires calls SetExpires.
// Both zero means the expiration is unchanged.
type tenantSnapshotExpiration struct {
	setNever bool
	expires  *int64
}

func tenantSnapshotCreateRequest(data *TenantSnapshotResourceModel) (*vergeos.TenantSnapshotCreateRequest, error) {
	tenantID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return nil, err
	}
	req := &vergeos.TenantSnapshotCreateRequest{Tenant: tenantID}
	if name := vergeio.KnownString(data.Name); name != nil && strings.TrimSpace(*name) != "" {
		req.Name = *name
	}
	if desc := vergeio.KnownString(data.Description); desc != nil && *desc != "" {
		req.Description = *desc
	}
	if typ := vergeio.KnownString(data.Type); typ != nil && strings.TrimSpace(*typ) != "" {
		req.Type = strings.TrimSpace(*typ)
	}
	if boolKnownTrue(data.NeverExpires) {
		zero := int64(0)
		req.Expires = &zero
		return req, nil
	}
	expires := vergeio.KnownInt64(data.Expires)
	if expires == nil {
		return nil, fmt.Errorf("set expires or never_expires")
	}
	req.Expires = expires
	return req, nil
}

// tenantSnapshotExpirationPlan chooses SetNeverExpires or SetExpires.
// A snapshot that already never expires is left alone. Turning never_expires
// on wins over a timestamp still sitting in the plan.
func tenantSnapshotExpirationPlan(plan, state *TenantSnapshotResourceModel) tenantSnapshotExpiration {
	if boolKnownTrue(plan.NeverExpires) {
		if boolKnownTrue(state.NeverExpires) {
			return tenantSnapshotExpiration{}
		}
		return tenantSnapshotExpiration{setNever: true}
	}
	return tenantSnapshotExpiration{expires: vergeio.ChangedInt64(plan.Expires, state.Expires)}
}

func (a *API) createTenantSnapshot(ctx context.Context, data *TenantSnapshotResourceModel) error {
	req, err := tenantSnapshotCreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.TenantSnapshots.Create(ctx, req)
	if err != nil {
		return err
	}
	if created.Key.Int() <= 0 {
		return fmt.Errorf("VergeOS did not return a snapshot key")
	}
	data.Id = idString(created.Key.Int())
	tflog.Debug(ctx, fmt.Sprintf("created tenant snapshot %d", created.Key.Int()))
	return nil
}

func (a *API) updateTenantSnapshot(ctx context.Context, plan, state *TenantSnapshotResourceModel) error {
	id, err := parseID(state.Id, "tenant snapshot")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	if desc := vergeio.ChangedString(plan.Description, state.Description); desc != nil {
		if _, err := a.sdk.TenantSnapshots.Update(ctx, id, &vergeos.TenantSnapshotUpdateRequest{Description: desc}); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("updated tenant snapshot %d description", id))
	}
	switch exp := tenantSnapshotExpirationPlan(plan, state); {
	case exp.setNever:
		if _, err := a.sdk.TenantSnapshots.SetNeverExpires(ctx, id); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("tenant snapshot %d set to never expire", id))
	case exp.expires != nil:
		if _, err := a.sdk.TenantSnapshots.SetExpires(ctx, id, *exp.expires); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("tenant snapshot %d expires at %d", id, *exp.expires))
	}
	return nil
}

func (a *API) readTenantSnapshot(ctx context.Context, data *TenantSnapshotResourceModel) error {
	id, err := parseID(data.Id, "tenant snapshot")
	if err != nil {
		return err
	}
	snap, err := a.sdk.TenantSnapshots.Get(ctx, id)
	if err != nil {
		return err
	}
	// VergeOS reuses keys. A Get that returns another tenant's snapshot, or
	// a different name than the one in state, must not be adopted.
	if err := ensureTenantSnapshotOwned(data, snap); err != nil {
		return err
	}
	assignTenantSnapshot(data, snap)
	return nil
}

func ensureTenantSnapshotOwned(data *TenantSnapshotResourceModel, snap *vergeos.TenantSnapshot) error {
	if !data.TenantID.IsNull() && !data.TenantID.IsUnknown() {
		want, err := parseID(data.TenantID, "tenant")
		if err != nil {
			return err
		}
		if snap.Tenant.Int() != want {
			return &vergeos.NotFoundError{Resource: "TenantSnapshot", ID: snap.Key.Int()}
		}
	}
	if name := knownString(data.Name); !name.IsNull() && snap.Name != name.ValueString() {
		return &vergeos.NotFoundError{Resource: "TenantSnapshot", ID: snap.Key.Int()}
	}
	return nil
}

func assignTenantSnapshot(data *TenantSnapshotResourceModel, snap *vergeos.TenantSnapshot) {
	data.Id = idString(snap.Key.Int())
	if snap.Tenant.Int() > 0 {
		data.TenantID = idString(snap.Tenant.Int())
	}
	data.Name = types.StringValue(snap.Name)
	data.Description = types.StringValue(snap.Description)
	typ := snap.Type
	if typ == "" {
		typ = vergeos.TenantSnapshotTypeFull
	}
	data.Type = types.StringValue(typ)
	data.Created = timestamp(snap.Created)
	if snap.Expires == 0 {
		data.Expires = types.Int64Null()
		data.NeverExpires = types.BoolValue(true)
		return
	}
	data.Expires = types.Int64Value(snap.Expires)
	data.NeverExpires = types.BoolValue(false)
}

func (a *API) deleteTenantSnapshot(ctx context.Context, data *TenantSnapshotResourceModel) error {
	id, err := parseID(data.Id, "tenant snapshot")
	if err != nil {
		return err
	}
	if err := a.sdk.TenantSnapshots.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant snapshot %d", id))
	return nil
}

func (a *API) readTenantSnapshots(ctx context.Context, data *TenantSnapshotsDataSourceModel) error {
	tenantID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return err
	}
	snaps, err := a.sdk.TenantSnapshots.ListByTenant(ctx, tenantID)
	if err != nil {
		return err
	}
	sort.Slice(snaps, func(i, j int) bool {
		if snaps[i].Name != snaps[j].Name {
			return snaps[i].Name < snaps[j].Name
		}
		return snaps[i].Key.Int() < snaps[j].Key.Int()
	})
	models := make([]*TenantSnapshotListModel, 0, len(snaps))
	for i := range snaps {
		models = append(models, tenantSnapshotListModel(&snaps[i]))
	}
	data.Snapshots = models
	tflog.Debug(ctx, fmt.Sprintf("read %d tenant snapshots for tenant %d", len(models), tenantID))
	return nil
}

func tenantSnapshotListModel(snap *vergeos.TenantSnapshot) *TenantSnapshotListModel {
	return &TenantSnapshotListModel{
		Id:      idString(snap.Key.Int()),
		Name:    types.StringValue(snap.Name),
		Type:    enumString(snap.Type),
		Created: timestamp(snap.Created),
		Expires: timestamp(snap.Expires),
	}
}

func boolKnownTrue(v types.Bool) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueBool()
}
