// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func mergeBool(config types.Bool, api, trustAPI, fillOmitted bool) types.Bool {
	if !trustAPI {
		if config.IsNull() || config.IsUnknown() {
			return types.BoolNull()
		}
		return config
	}
	if config.IsNull() && !fillOmitted {
		return types.BoolNull()
	}
	return types.BoolValue(api)
}

func mergeString(config types.String, api string, fillOmitted bool) types.String {
	omitted := config.IsNull() || config.IsUnknown()
	if omitted && !fillOmitted {
		return types.StringNull()
	}
	if strings.TrimSpace(api) != "" {
		return types.StringValue(api)
	}
	if omitted || strings.TrimSpace(config.ValueString()) == "" {
		return types.StringNull()
	}
	return config
}

func (data *importModel) clearUnknownVMOptions() {
	data.PreserveMACs = clearUnknownBool(data.PreserveMACs)
	data.PreserveDriveFormat = clearUnknownBool(data.PreserveDriveFormat)
	data.PreserveBIOSSerial = clearUnknownBool(data.PreserveBIOSSerial)
	data.GuestAgentPreinstalled = clearUnknownBool(data.GuestAgentPreinstalled)
	data.ReplicateTags = clearUnknownBool(data.ReplicateTags)
	data.NoOpticalDrives = clearUnknownBool(data.NoOpticalDrives)
	data.OverrideDriveInterface = clearUnknownString(data.OverrideDriveInterface)
	data.OverrideNICInterface = clearUnknownString(data.OverrideNICInterface)
	if data.PreferredTier.IsUnknown() {
		data.PreferredTier = types.StringNull()
	}
}

func clearUnknownBool(v types.Bool) types.Bool {
	if v.IsUnknown() {
		return types.BoolNull()
	}
	return v
}

func clearUnknownString(v types.String) types.String {
	if v.IsUnknown() {
		return types.StringNull()
	}
	return v
}
