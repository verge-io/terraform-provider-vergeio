// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

// tenantPowerTimeout is how long create, update, and delete wait for a
// power action to show up in tenant status. Tests shorten it.
var (
	tenantPowerTimeout  = 2 * time.Minute
	tenantPowerInterval = time.Second
)

// tenantPoweredOn reports whether the tenant should be treated as on.
// Stopping and offline are off. A missing status record is off.
// starting, running, and the in-progress status strings are on, so a
// tenant that has accepted power on is not powered on a second time.
func tenantPoweredOn(status *vergeos.TenantStatus) bool {
	if status == nil || status.Stopping {
		return false
	}
	if status.Running || status.Starting {
		return true
	}
	switch status.Status {
	case "online", "starting", "migrating", "restarting", "reduced", "provisioning":
		return true
	default:
		return false
	}
}

// reconcilePower issues power on or power off when the planned powerstate
// is known and differs from the current status. A null or unknown
// powerstate leaves the tenant alone. preferred_node is sent only when
// powering on.
func (a *API) reconcilePower(ctx context.Context, id int, desired types.Bool, preferred types.Int32) error {
	if desired.IsNull() || desired.IsUnknown() {
		return nil
	}
	status, err := a.sdk.TenantStatus.Get(ctx, id)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	if vergeos.IsNotFoundError(err) {
		status = nil
	}
	wantOn := desired.ValueBool()
	if tenantPoweredOn(status) == wantOn {
		return nil
	}
	if wantOn {
		node := 0
		if n := knownInt(preferred); n != nil && *n > 0 {
			node = *n
		}
		if err := a.sdk.Tenants.PowerOnWithNode(ctx, id, node); err != nil {
			return err
		}
	} else if err := a.sdk.Tenants.PowerOff(ctx, id); err != nil {
		return err
	}
	return a.waitPower(ctx, id, wantOn)
}

func (a *API) waitPower(ctx context.Context, id int, wantOn bool) error {
	deadline := time.Now().Add(tenantPowerTimeout)
	for {
		status, err := a.sdk.TenantStatus.Get(ctx, id)
		if err != nil && !vergeos.IsNotFoundError(err) {
			return err
		}
		if vergeos.IsNotFoundError(err) {
			status = nil
		}
		if tenantPoweredOn(status) == wantOn {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timed out waiting for tenant %d to reach powerstate %t; the tenant still exists and can be imported", id, wantOn)
		}
		if err := sleepPower(ctx); err != nil {
			return err
		}
	}
}

func sleepPower(ctx context.Context) error {
	if tenantPowerInterval <= 0 {
		return nil
	}
	timer := time.NewTimer(tenantPowerInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
