// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// networkPowerRequested reports whether configuration set powerstate.
// A null or unknown value leaves the current power alone. on is meaningful
// only when set is true.
func networkPowerRequested(v types.Bool) (set bool, on bool) {
	if v.IsNull() || v.IsUnknown() {
		return false, false
	}
	return true, v.ValueBool()
}

// applyPlannedPowerState posts poweron or kill when the planned powerstate
// differs from the router machine's running flag. VergeOS accepts a
// powerstate field on PUT vnets and leaves the machine unchanged, so the
// change goes through vnet_actions. A null or unknown powerstate is left
// alone. The live running flag is what is compared, so a powerstate column
// that disagrees with the machine does not skip the action.
func (nc *NetworkApi) applyPlannedPowerState(ctx context.Context, data *NetworkResourceModel) error {
	if data == nil {
		return fmt.Errorf("missing network")
	}
	set, wantOn := networkPowerRequested(data.PowerState)
	if !set {
		tflog.Debug(ctx, "Planned powerstate is unset; leaving network power unchanged")
		return nil
	}
	networkID, err := parseID(data.Id, "network")
	if err != nil {
		return err
	}

	network, err := nc.sdk.Networks.Get(ctx, networkID)
	if err != nil {
		return err
	}
	if network.Running == wantOn {
		return nil
	}
	if wantOn {
		tflog.Debug(ctx, fmt.Sprintf("Planned powerstate is on and network %d is stopped; powering on", networkID))
		return nc.sdk.Networks.PowerOn(ctx, networkID)
	}

	tflog.Debug(ctx, fmt.Sprintf("Planned powerstate is off and network %d is running; killing it", networkID))
	return nc.stopNetworkBeforeDelete(ctx, data)
}
