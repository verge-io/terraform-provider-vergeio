// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// A staged DHCP change on VergeOS 26.1 cleared need_restart about four
// seconds after reset. Poll a little longer than that. The first read is
// immediate. Tests shorten these so a stuck flag fails without waiting.
var (
	networkRestartAttempts = 15
	networkRestartInterval = time.Second
)

// pendingRestartNotice is the apply warning when a staged change was left
// in place. An empty Detail means there is nothing to warn about.
type pendingRestartNotice struct {
	Summary string
	Detail  string
}

// networkIsRunning reports whether the router machine is up.
// The powerstate column is not maintained and can read false while the
// network is running, so the machine status is what decides a restart.
func networkIsRunning(network *vergeos.Network) bool {
	if network == nil {
		return false
	}
	if network.Running {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(network.Status), "running")
}

// restartOnChangeEnabled is true unless the configuration explicitly opts out.
// A null or unknown value uses the schema default of true.
func restartOnChangeEnabled(v types.Bool) bool {
	if v.IsNull() || v.IsUnknown() {
		return true
	}
	return v.ValueBool()
}

// stagedRestartAction is what to do with need_restart after an update.
type stagedRestartAction int

const (
	stagedRestartNone stagedRestartAction = iota
	stagedRestartNow
	stagedRestartSkipped
)

// stagedRestartDecision restarts only a running network when the operator
// has not asked to wait for a maintenance window.
func stagedRestartDecision(needRestart, running, restartOnChange bool) stagedRestartAction {
	if !needRestart {
		return stagedRestartNone
	}
	if running && restartOnChange {
		return stagedRestartNow
	}
	return stagedRestartSkipped
}

func pendingRestartNoticeFor(data *NetworkResourceModel, running, restartOnChange bool) pendingRestartNotice {
	label := data.Id.ValueString()
	if !data.Name.IsNull() && !data.Name.IsUnknown() && data.Name.ValueString() != "" {
		label = data.Name.ValueString()
		if !data.Id.IsNull() && !data.Id.IsUnknown() && data.Id.ValueString() != "" {
			label = fmt.Sprintf("%s (id %s)", data.Name.ValueString(), data.Id.ValueString())
		}
	}

	if !restartOnChange {
		return pendingRestartNotice{
			Summary: "Network change is not live yet",
			Detail: fmt.Sprintf(
				"Network %s has need_restart set. restart_on_change is false, so Terraform did not restart it. The staged change is not live until the network is restarted, and VergeOS may restart it later on its own.",
				label,
			),
		}
	}
	if !running {
		return pendingRestartNotice{
			Summary: "Network change is not live yet",
			Detail: fmt.Sprintf(
				"Network %s has need_restart set and is not running, so Terraform did not restart it. The staged change takes effect when the network starts.",
				label,
			),
		}
	}
	return pendingRestartNotice{
		Summary: "Network change is not live yet",
		Detail: fmt.Sprintf(
			"Network %s has need_restart set. The staged change is not live until the network is restarted.",
			label,
		),
	}
}

// reconcileStagedRestart reads need_restart after an update. A running
// network is reset when restart_on_change is true, and the reset is retried
// until the flag clears. data.NeedRestart is set from the network after that
// wait, or from the post-update read when the restart is skipped.
func (nc *NetworkApi) reconcileStagedRestart(ctx context.Context, data *NetworkResourceModel) (pendingRestartNotice, error) {
	if data.Id.IsNull() || data.Id.IsUnknown() || data.Id.ValueString() == "" {
		return pendingRestartNotice{}, fmt.Errorf("missing network id")
	}
	networkID, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return pendingRestartNotice{}, fmt.Errorf("invalid network ID format: %v", err)
	}

	network, err := nc.sdk.Networks.Get(ctx, networkID)
	if err != nil {
		return pendingRestartNotice{}, err
	}

	restartOnChange := restartOnChangeEnabled(data.RestartOnChange)
	running := networkIsRunning(network)
	switch stagedRestartDecision(network.NeedRestart, running, restartOnChange) {
	case stagedRestartNow:
		tflog.Debug(ctx, fmt.Sprintf("Network %d has need_restart set and is running; resetting", networkID))
		// apply=false restarts the router without also applying staged
		// firewall rules. Firewall apply is a separate action.
		if err := nc.sdk.Networks.Reset(ctx, networkID, false); err != nil {
			return pendingRestartNotice{}, err
		}
		cleared, err := nc.waitForNeedRestartClear(ctx, networkID)
		if err != nil {
			return pendingRestartNotice{}, err
		}
		data.NeedRestart = types.BoolValue(cleared.NeedRestart)
		return pendingRestartNotice{}, nil
	case stagedRestartSkipped:
		tflog.Warn(ctx, fmt.Sprintf("Network %d has need_restart set; leaving it for a later restart (running=%v restart_on_change=%v)", networkID, running, restartOnChange))
		data.NeedRestart = types.BoolValue(true)
		return pendingRestartNoticeFor(data, running, restartOnChange), nil
	default:
		data.NeedRestart = types.BoolValue(false)
		return pendingRestartNotice{}, nil
	}
}

// waitForNeedRestartClear polls until reset has cleared need_restart.
// A read error is retried because the network can be briefly unreachable
// while it restarts. The flag clearing is the signal that the staged
// change is live; on VergeOS 26.1 that happened about four seconds after reset.
func (nc *NetworkApi) waitForNeedRestartClear(ctx context.Context, networkID int) (*vergeos.Network, error) {
	var last *vergeos.Network
	var lastErr error
	for attempt := 0; attempt < networkRestartAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, networkRestartInterval); err != nil {
				return last, err
			}
		}
		network, err := nc.sdk.Networks.Get(ctx, networkID)
		if err != nil {
			lastErr = err
			tflog.Debug(ctx, fmt.Sprintf("Reading network %d after reset failed: %v", networkID, err))
			continue
		}
		last = network
		lastErr = nil
		if !network.NeedRestart {
			return network, nil
		}
		tflog.Debug(ctx, fmt.Sprintf("Network %d still has need_restart set after reset", networkID))
	}
	if lastErr != nil && last == nil {
		return nil, fmt.Errorf("network %d could not be read after reset: %w", networkID, lastErr)
	}
	return last, fmt.Errorf("network %d still has need_restart set after reset", networkID)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
