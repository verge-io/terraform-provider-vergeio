// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// WireGuard apply returns before the running router has the disabled
// interface. Deleting in that window is rejected. These match the wait
// that has cleared the same rejection on a running network.
var (
	wireGuardDeleteAttempts = 16
	wireGuardDeletePoll     = time.Second
)

// vpnDeleteOrder is the teardown VergeOS does not do on its own.
// A phase 1 delete while a phase 2 row remains fails, and that failure sticks.
// A network delete leaves the IPsec rows in place.
const vpnDeleteOrder = "Delete phase 2, then phase 1, then the IPsec configuration, and delete WireGuard peers before the WireGuard interface, before deleting the network."

// DeleteNetworkVPNRows removes VPN rows for a network in the order VergeOS
// requires. Sweep uses it. The resources do not cascade: each one refuses
// to delete while a child row remains, and Terraform references supply the
// same order.
func DeleteNetworkVPNRows(ctx context.Context, sdk *vergeos.Client, networkID int) error {
	if sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	if networkID <= 0 {
		return fmt.Errorf("network id must be a positive integer")
	}
	var errs []error
	if err := deleteWireGuardRows(ctx, sdk, networkID); err != nil {
		errs = append(errs, err)
	}
	if err := deleteIPSecRows(ctx, sdk, networkID); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func deleteWireGuardRows(ctx context.Context, sdk *vergeos.Client, networkID int) error {
	interfaces, err := sdk.VNetWireGuards.ListByNetwork(ctx, networkID)
	if err != nil {
		return fmt.Errorf("list WireGuard interfaces for network %d: %w", networkID, err)
	}
	var errs []error
	for _, iface := range interfaces {
		id := iface.Key.Int()
		if err := deleteWireGuardPeerRows(ctx, sdk, id); err != nil {
			errs = append(errs, err)
			continue
		}
		if err := deleteWireGuardInterface(ctx, sdk, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteWireGuardInterface disables the interface, applies that change on
// the interface's own network when that network is running, and deletes
// the interface once VergeOS accepts it. The apply returns before the
// router has the disabled interface, so a delete in that window is
// rejected and is retried. A row that already reads as disabled is
// disabled again, because the running configuration can still have it
// enabled. A stopped network rejects the apply, so the interface is
// disabled and deleted without it. Destroy, sweep, and rollback of a
// failed create all use this path.
func deleteWireGuardInterface(ctx context.Context, sdk *vergeos.Client, id int) error {
	row, err := sdk.VNetWireGuards.Get(ctx, id)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("read WireGuard interface %d before delete: %w", id, err)
	}
	networkID, err := disableWireGuardInterface(ctx, sdk, id, row.VNet.Int())
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	if networkID <= 0 {
		return fmt.Errorf("WireGuard interface %d has no network, so it was not deleted", id)
	}
	network, err := sdk.Networks.Get(ctx, networkID)
	if err != nil {
		return fmt.Errorf("read network %d before deleting WireGuard interface %d: %w", networkID, id, err)
	}
	if !networkIsRunning(network) {
		tflog.Warn(ctx, fmt.Sprintf("Network %d is not running; deleting WireGuard interface %d without applying rules", networkID, id))
		return deleteWireGuardOnce(ctx, sdk, id)
	}
	tflog.Debug(ctx, fmt.Sprintf("Disabling WireGuard interface %d and applying network %d before delete", id, networkID))
	return deleteAppliedWireGuard(ctx, sdk, id, networkID)
}

// disableWireGuardInterface sets enabled to false. networkID is the
// interface network from the read that preceded this call. The update
// read replaces it when that read names a network.
func disableWireGuardInterface(ctx context.Context, sdk *vergeos.Client, id, networkID int) (int, error) {
	disabled := false
	updated, err := sdk.VNetWireGuards.Update(ctx, id, &vergeos.VNetWireGuardUpdateRequest{Enabled: &disabled})
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return networkID, err
		}
		return 0, fmt.Errorf("disable WireGuard interface %d before delete: %w", id, err)
	}
	if updated != nil && updated.VNet.Int() > 0 {
		networkID = updated.VNet.Int()
	}
	return networkID, nil
}

// deleteAppliedWireGuard applies networkID, then deletes the interface.
// A 422 that says the interface must be disabled and applied means the
// refresh has not landed. The interface is disabled again, the same
// network is applied again, and the delete is retried.
func deleteAppliedWireGuard(ctx context.Context, sdk *vergeos.Client, id, networkID int) error {
	if err := sdk.Networks.ApplyRules(ctx, networkID); err != nil {
		return fmt.Errorf("apply network %d before deleting WireGuard interface %d: %w", networkID, id, err)
	}
	var last error
	for attempt := 0; attempt < wireGuardDeleteAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, wireGuardDeletePoll); err != nil {
				return err
			}
			if _, err := disableWireGuardInterface(ctx, sdk, id, networkID); err != nil {
				if vergeos.IsNotFoundError(err) {
					return nil
				}
				return err
			}
			if err := sdk.Networks.ApplyRules(ctx, networkID); err != nil {
				return fmt.Errorf("apply network %d before deleting WireGuard interface %d: %w", networkID, id, err)
			}
		}
		last = sdk.VNetWireGuards.Delete(ctx, id)
		if last == nil || vergeos.IsNotFoundError(last) {
			return nil
		}
		if !wireGuardDeletePending(last) {
			break
		}
		tflog.Debug(ctx, fmt.Sprintf("WireGuard interface %d on network %d is not applied yet; retrying delete", id, networkID))
	}
	return fmt.Errorf("delete WireGuard interface %d: %w", id, last)
}

func deleteWireGuardOnce(ctx context.Context, sdk *vergeos.Client, id int) error {
	if err := sdk.VNetWireGuards.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return fmt.Errorf("delete WireGuard interface %d: %w", id, err)
	}
	return nil
}

// wireGuardDeletePending reports the 422 VergeOS returns when a running
// network still has the interface enabled or the disable has not been applied.
func wireGuardDeletePending(err error) bool {
	var apiErr *vergeos.APIError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return false
	}
	if apiErr.StatusCode != 422 {
		return false
	}
	msg := strings.ToLower(apiErr.Message)
	return strings.Contains(msg, "disabled") && strings.Contains(msg, "applied")
}

func deleteWireGuardPeerRows(ctx context.Context, sdk *vergeos.Client, wireguardID int) error {
	peers, err := sdk.VNetWireGuardPeers.ListByWireGuard(ctx, wireguardID)
	if err != nil {
		return fmt.Errorf("list WireGuard peers for interface %d: %w", wireguardID, err)
	}
	var errs []error
	for _, peer := range peers {
		id := peer.Key.Int()
		if err := sdk.VNetWireGuardPeers.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
			errs = append(errs, fmt.Errorf("delete WireGuard peer %d: %w", id, err))
		}
	}
	return errors.Join(errs...)
}

func deleteIPSecRows(ctx context.Context, sdk *vergeos.Client, networkID int) error {
	ipsec, err := sdk.VNetIPSecs.GetByNetwork(ctx, networkID)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("read IPsec configuration for network %d: %w", networkID, err)
	}
	ipsecID := ipsec.Key.Int()
	phase1s, err := sdk.VNetIPSecPhase1s.ListByIPSec(ctx, ipsecID)
	if err != nil {
		return fmt.Errorf("list IPsec phase 1 for configuration %d: %w", ipsecID, err)
	}
	var errs []error
	for _, phase1 := range phase1s {
		if err := deletePhase1AfterPhase2(ctx, sdk, phase1.Key.Int()); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	remaining, err := sdk.VNetIPSecPhase1s.ListByIPSec(ctx, ipsecID)
	if err != nil {
		return fmt.Errorf("list IPsec phase 1 for configuration %d: %w", ipsecID, err)
	}
	if len(remaining) > 0 {
		return fmt.Errorf("IPsec configuration %d still has phase 1 rows, so it was not deleted. %s", ipsecID, vpnDeleteOrder)
	}
	if err := sdk.VNetIPSecs.Delete(ctx, ipsecID); err != nil && !vergeos.IsNotFoundError(err) {
		return fmt.Errorf("delete IPsec configuration %d: %w", ipsecID, err)
	}
	return nil
}

// deletePhase1AfterPhase2 deletes every phase 2 under phase1ID, then the
// phase 1. A phase 2 delete error returns before the phase 1 delete. That
// delete is what leaves the phase 1 trigger failing on later attempts.
func deletePhase1AfterPhase2(ctx context.Context, sdk *vergeos.Client, phase1ID int) error {
	phase2s, err := sdk.VNetIPSecPhase2s.ListByPhase1(ctx, phase1ID)
	if err != nil {
		return fmt.Errorf("list IPsec phase 2 for phase 1 %d failed, so phase 1 was not deleted: %w", phase1ID, err)
	}
	for _, phase2 := range phase2s {
		id := phase2.Key.Int()
		if err := sdk.VNetIPSecPhase2s.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("IPsec phase 2 %d was not deleted, so phase 1 %d was left in place: %w", id, phase1ID, err)
		}
	}
	if err := sdk.VNetIPSecPhase1s.Delete(ctx, phase1ID); err != nil && !vergeos.IsNotFoundError(err) {
		return fmt.Errorf("delete IPsec phase 1 %d: %w", phase1ID, err)
	}
	return nil
}

func networkVPNBlockers(ctx context.Context, sdk *vergeos.Client, networkID int) ([]string, error) {
	if sdk == nil {
		return nil, fmt.Errorf("vergeos client is nil")
	}
	var blockers []string

	interfaces, err := sdk.VNetWireGuards.ListByNetwork(ctx, networkID)
	if err != nil {
		return nil, fmt.Errorf("list WireGuard interfaces for network %d: %w", networkID, err)
	}
	for _, iface := range interfaces {
		peers, err := sdk.VNetWireGuardPeers.ListByWireGuard(ctx, iface.Key.Int())
		if err != nil {
			return nil, fmt.Errorf("list WireGuard peers for interface %d: %w", iface.Key.Int(), err)
		}
		for _, peer := range peers {
			blockers = append(blockers, fmt.Sprintf("WireGuard peer %d (%s)", peer.Key.Int(), peer.Name))
		}
		blockers = append(blockers, fmt.Sprintf("WireGuard interface %d (%s)", iface.Key.Int(), iface.Name))
	}

	ipsec, err := sdk.VNetIPSecs.GetByNetwork(ctx, networkID)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return blockers, nil
		}
		return nil, fmt.Errorf("read IPsec configuration for network %d: %w", networkID, err)
	}
	phase1s, err := sdk.VNetIPSecPhase1s.ListByIPSec(ctx, ipsec.Key.Int())
	if err != nil {
		return nil, fmt.Errorf("list IPsec phase 1 for configuration %d: %w", ipsec.Key.Int(), err)
	}
	for _, phase1 := range phase1s {
		phase2s, err := sdk.VNetIPSecPhase2s.ListByPhase1(ctx, phase1.Key.Int())
		if err != nil {
			return nil, fmt.Errorf("list IPsec phase 2 for phase 1 %d: %w", phase1.Key.Int(), err)
		}
		for _, phase2 := range phase2s {
			blockers = append(blockers, fmt.Sprintf("IPsec phase 2 %d (%s)", phase2.Key.Int(), phase2.Name))
		}
		blockers = append(blockers, fmt.Sprintf("IPsec phase 1 %d (%s)", phase1.Key.Int(), phase1.Name))
	}
	blockers = append(blockers, fmt.Sprintf("IPsec configuration %d", ipsec.Key.Int()))
	return blockers, nil
}

func refuseNetworkDelete(ctx context.Context, sdk *vergeos.Client, networkID int) error {
	blockers, err := networkVPNBlockers(ctx, sdk, networkID)
	if err != nil {
		return err
	}
	if len(blockers) == 0 {
		return nil
	}
	return fmt.Errorf("network %d still has VPN configuration VergeOS will not delete with the network: %s. %s", networkID, strings.Join(blockers, "; "), vpnDeleteOrder)
}
