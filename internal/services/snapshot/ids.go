// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

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
	return parseIDText(id.ValueString(), what)
}

func parseIDText(text, what string) (int, error) {
	text = strings.TrimSpace(text)
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s id %q is not a positive integer", what, text)
	}
	return n, nil
}
