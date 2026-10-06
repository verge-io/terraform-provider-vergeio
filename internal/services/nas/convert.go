// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func missing(err error) bool {
	return vergeos.IsNotFoundError(err)
}

func parsePositiveID(v types.String, kind string) (int, error) {
	text := strings.TrimSpace(stringOrEmpty(v))
	if text == "" {
		return 0, fmt.Errorf("%s id is empty", kind)
	}
	id, err := strconv.Atoi(text)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%s id %q is not a positive integer", kind, text)
	}
	return id, nil
}

func parseObjectID(v types.String, kind string) (string, error) {
	text := strings.TrimSpace(stringOrEmpty(v))
	if text == "" {
		return "", fmt.Errorf("%s id is empty", kind)
	}
	return text, nil
}

func stringOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

func knownInt(v types.Int64) *int {
	n := vergeio.KnownInt64(v)
	if n == nil {
		return nil
	}
	i := int(*n)
	return &i
}

func changedInt(plan, state types.Int64) *int {
	n := vergeio.ChangedInt64(plan, state)
	if n == nil {
		return nil
	}
	i := int(*n)
	return &i
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func rowID(key, id string) string {
	return firstNonEmpty(id, key)
}

// abandon explains a row that was created and then removed, or left in place
// when that cleanup failed.
func abandon(cause error, kind, id string, cleanupErr error) error {
	if cleanupErr != nil {
		return fmt.Errorf("%s %s was created but not stored in state: %w (it was left in place: %v)", kind, id, cause, cleanupErr)
	}
	return fmt.Errorf("%s %s was created but not stored in state, and it was removed: %w", kind, id, cause)
}
