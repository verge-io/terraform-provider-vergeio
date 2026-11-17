// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// KnownPositiveID parses a configured object id.
// A null value is an error. An unknown value is not known yet and is not an error.
// A known value must be a positive integer.
func KnownPositiveID(value types.String, what string) (int, bool, error) {
	if value.IsNull() {
		return 0, false, fmt.Errorf("%s is required", what)
	}
	if value.IsUnknown() {
		return 0, false, nil
	}
	text := strings.TrimSpace(value.ValueString())
	id, err := strconv.Atoi(text)
	if err != nil || id <= 0 {
		return 0, false, fmt.Errorf("%s must be a positive integer", what)
	}
	return id, true, nil
}
