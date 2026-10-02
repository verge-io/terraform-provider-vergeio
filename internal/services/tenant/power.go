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
// power action to reach a terminal online or offline status. Tests shorten it.
var (
	tenantPowerTimeout  = 2 * time.Minute
	tenantPowerInterval = time.Second
)

// tenantPoweredOn reports whether Terraform should store powerstate as true.
// Only a terminal on state counts. Starting and stopping are transitional:
// waitPower must not treat them as done, or create/update returns too early
// and destroy can race a still-running node (#196).
func tenantPoweredOn(status *vergeos.TenantStatus) bool {
	if status == nil || status.Starting || status.Stopping {
		return false
	}
	if status.Running {
		return true
	}
	switch status.Status {
	case "online", "migrating", "restarting", "reduced":
		return true
	default:
		return false
	}
}

// tenantPoweredOff reports whether the tenant has reached a terminal off state.
// Stopping still has a running node, so it is not off yet.
func tenantPoweredOff(status *vergeos.TenantStatus) bool {
	if status == nil {
		return true
	}
	if status.Running || status.Starting || status.Stopping {
		return false
	}
	switch status.Status {
	case "starting", "stopping", "provisioning", "online", "migrating", "restarting", "reduced":
		return false
	default:
		return true
	}
}

func tenantIsStarting(status *vergeos.TenantStatus) bool {
	if status == nil {
		return false
	}
	if status.Starting {
		return true
	}
	return status.Status == "starting" || status.Status == "provisioning"
}

func tenantIsStopping(status *vergeos.TenantStatus) bool {
	if status == nil {
		return false
	}
	if status.Stopping {
		return true
	}
	return status.Status == "stopping"
}

func (a *API) tenantStatus(ctx context.Context, id int) (*vergeos.TenantStatus, error) {
	status, err := a.sdk.TenantStatus.Get(ctx, id)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return nil, err
	}
	if vergeos.IsNotFoundError(err) {
		return nil, nil
	}
	return status, nil
}

// reconcilePower issues power on or power off when the planned powerstate
// is known and differs from the current terminal status. A null or unknown
// powerstate leaves the tenant alone. preferred_node is sent only when
// powering on. Power off while starting waits for online first: VergeOS
// rejects poweroff until the tenant is running (#196).
func (a *API) reconcilePower(ctx context.Context, id int, desired types.Bool, preferred types.Int32) error {
	if desired.IsNull() || desired.IsUnknown() {
		return nil
	}
	wantOn := desired.ValueBool()
	if !wantOn {
		return a.ensurePoweredOff(ctx, id)
	}
	status, err := a.tenantStatus(ctx, id)
	if err != nil {
		return err
	}
	if tenantPoweredOn(status) {
		return nil
	}
	if !tenantIsStarting(status) {
		node := 0
		if n := knownInt(preferred); n != nil && *n > 0 {
			node = *n
		}
		if err := a.sdk.Tenants.PowerOnWithNode(ctx, id, node); err != nil {
			return err
		}
	}
	return a.waitPower(ctx, id, true)
}

// ensurePoweredOff powers the tenant off and waits until it is terminal
// offline. Used by reconcilePower(false), deleteTenant, and deleteTenantNode
// so a destroy does not hit "Tenant node cannot be deleted while running" (#195).
func (a *API) ensurePoweredOff(ctx context.Context, id int) error {
	status, err := a.tenantStatus(ctx, id)
	if err != nil {
		return err
	}
	if tenantPoweredOff(status) {
		return nil
	}
	// VergeOS: "Tenant must be in running state to poweroff"
	if tenantIsStarting(status) {
		if err := a.waitPower(ctx, id, true); err != nil {
			return err
		}
		status, err = a.tenantStatus(ctx, id)
		if err != nil {
			return err
		}
		if tenantPoweredOff(status) {
			return nil
		}
	}
	if !tenantIsStopping(status) {
		if err := a.sdk.Tenants.PowerOff(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
			return err
		}
	}
	return a.waitPower(ctx, id, false)
}

func (a *API) waitPower(ctx context.Context, id int, wantOn bool) error {
	deadline := time.Now().Add(tenantPowerTimeout)
	for {
		status, err := a.tenantStatus(ctx, id)
		if err != nil {
			return err
		}
		if wantOn {
			if tenantPoweredOn(status) {
				return nil
			}
		} else if tenantPoweredOff(status) {
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
