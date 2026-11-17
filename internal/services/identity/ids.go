// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func idString(id int) types.String {
	return types.StringValue(strconv.Itoa(id))
}

func parseID(id types.String, what string) (int, error) {
	if id.IsNull() || id.IsUnknown() {
		return 0, fmt.Errorf("%s id is empty", what)
	}
	text := strings.TrimSpace(id.ValueString())
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s id %q is not a positive integer", what, text)
	}
	return n, nil
}

func positiveID(id types.String) (int, bool) {
	n, err := parseID(id, "id")
	if err != nil {
		return 0, false
	}
	return n, true
}
