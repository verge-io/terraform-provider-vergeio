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
	// tenantStartedTimeout bounds the wait for tenant_status.started after
	// the tenant is already online. Nested VergeOS sets that timestamp when
	// its own boot finishes, which is later than status online.
	tenantStartedTimeout = 5 * time.Minute
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
// Stopping still has a running node, so it is not off yet. The tenant vnet
// may still be Running after this returns true; ensurePoweredOff waits for
// that separately (#205).
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

// reconcilePowerOnCreate is create and update's power path when the desired
// powerstate is known. powerstate=true before any vergeio_tenant_node exists
// cannot reach terminal online: the tenant goes starting then offline while
// its network starts, then waitPower times out and leaves an orphan running
// vnet (#207 create, #219 update). Defer power-on until a later apply once
// nodes exist. powerstate=false still powers off as usual.
func (a *API) reconcilePowerOnCreate(ctx context.Context, id int, desired types.Bool, preferred types.Int32) (deferred bool, err error) {
	if desired.IsNull() || desired.IsUnknown() {
		return false, nil
	}
	if !desired.ValueBool() {
		return false, a.ensurePoweredOff(ctx, id)
	}
	nodes, err := a.sdk.TenantNodes.ListByTenant(ctx, id)
	if err != nil {
		return false, err
	}
	if len(nodes) == 0 {
		return true, nil
	}
	return false, a.reconcilePower(ctx, id, desired, preferred)
}

// ensurePoweredOff powers the tenant off and waits until it is terminal
// offline and its tenant vnet is not Running. Used by reconcilePower(false)
// and deleteTenant so destroy does not hit "Tenant node cannot be deleted
// while running" (#195) or "Tenant network must be powered off to delete
// tenant" (#205). deleteTenantNode stops only the target node (#206).
func (a *API) ensurePoweredOff(ctx context.Context, id int) error {
	status, err := a.tenantStatus(ctx, id)
	if err != nil {
		return err
	}
	if !tenantPoweredOff(status) {
		// VergeOS: "Tenant must be in running state to poweroff"
		if tenantIsStarting(status) {
			if err := a.waitPower(ctx, id, true); err != nil {
				return err
			}
			status, err = a.tenantStatus(ctx, id)
			if err != nil {
				return err
			}
		}
		if !tenantPoweredOff(status) {
			if !tenantIsStopping(status) {
				if err := a.sdk.Tenants.PowerOff(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
					return err
				}
			}
			if err := a.waitPower(ctx, id, false); err != nil {
				return err
			}
		}
	}
	// Status offline is not enough: the tenant vnet can stay Running for
	// several seconds, and Tenants.Delete returns 405 until it stops (#205).
	return a.ensureTenantVNetStopped(ctx, id)
}

// ensureTenantVNetStopped waits until the tenant's auto-created vnet reports
// Running=false. When it is still running, Kill once (same pattern as
// network stop-before-delete) and poll until it stops or the power timeout
// elapses. A missing tenant or vnet is already gone.
func (a *API) ensureTenantVNetStopped(ctx context.Context, tenantID int) error {
	tenant, err := a.sdk.Tenants.Get(ctx, tenantID)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	vnetID := tenant.VNet.Int()
	if vnetID <= 0 {
		return nil
	}
	return a.stopTenantVNet(ctx, vnetID)
}

func (a *API) stopTenantVNet(ctx context.Context, vnetID int) error {
	deadline := time.Now().Add(tenantPowerTimeout)
	killed := false
	for {
		network, err := a.sdk.Networks.Get(ctx, vnetID)
		if err != nil {
			if vergeos.IsNotFoundError(err) {
				return nil
			}
			return err
		}
		if !network.Running {
			return nil
		}
		if !killed {
			if err := a.sdk.Networks.Kill(ctx, vnetID); err != nil && !vergeos.IsNotFoundError(err) {
				return fmt.Errorf("kill tenant vnet %d before delete: %w", vnetID, err)
			}
			killed = true
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timed out waiting for tenant vnet %d to stop; the tenant still exists and can be imported", vnetID)
		}
		if err := sleepPower(ctx); err != nil {
			return err
		}
	}
}

// waitTenantStarted polls tenant_status.started until VergeOS records that
// the tenant has been started. Status online is a different column and can
// be true while started is still 0. The timeout error includes the last
// value of that field. The tenant is left running.
func (a *API) waitTenantStarted(ctx context.Context, id int) (int64, error) {
	deadline := time.Now().Add(tenantStartedTimeout)
	var started int64
	var statusName string
	var running bool
	for {
		status, err := a.tenantStatus(ctx, id)
		if err != nil {
			return 0, err
		}
		started = 0
		statusName = ""
		running = false
		if status != nil {
			started = status.Started
			statusName = status.Status
			running = status.Running
		}
		if started > 0 {
			return started, nil
		}
		if !time.Now().Before(deadline) {
			return 0, fmt.Errorf("timed out waiting for tenant %d tenant_status.started to be set; started=%d status=%q running=%t", id, started, statusName, running)
		}
		if err := sleepPower(ctx); err != nil {
			return 0, err
		}
	}
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
