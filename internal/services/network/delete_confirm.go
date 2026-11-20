// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"fmt"
	"strings"

	"github.com/verge-io/govergeos"
)

// confirmDeleted decides whether a delete that returned deleteErr removed the
// row. govergeos turns every 404 on delete into NotFoundError, including a
// trigger that refused the delete while the row is still there. A following
// read that also returns NotFoundError means the row is gone. A read that
// finds the row is an error, so Terraform keeps the resource in state.
// Any other deleteErr is returned unchanged.
func confirmDeleted(deleteErr error, row string, get func() error) error {
	if deleteErr == nil || !vergeos.IsNotFoundError(deleteErr) {
		return deleteErr
	}
	row = strings.TrimSpace(row)
	if row == "" {
		row = "the row"
	}
	getErr := get()
	if vergeos.IsNotFoundError(getErr) {
		return nil
	}
	if getErr != nil {
		return fmt.Errorf("VergeOS refused the delete of %s, probably because the parent network was deleted outside Terraform, and reading it again failed: %w", row, getErr)
	}
	return fmt.Errorf("VergeOS refused the delete of %s, probably because the parent network was deleted outside Terraform", row)
}

func namedRow(kind string, id int, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Sprintf("%s %d", kind, id)
	}
	return fmt.Sprintf("%s %d (%s)", kind, id, name)
}
