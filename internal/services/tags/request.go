// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import "encoding/json"

// apiRequestEmpty reports whether req would marshal to an empty JSON object.
// Update calls skip that body so an unchanged resource is not written back.
func apiRequestEmpty(req any) bool {
	raw, err := json.Marshal(req)
	if err != nil {
		return false
	}
	switch string(raw) {
	case "{}", "null":
		return true
	default:
		return false
	}
}
