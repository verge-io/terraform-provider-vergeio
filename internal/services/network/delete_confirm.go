// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"errors"
	"fmt"
	"strings"

	"github.com/verge-io/govergeos"
)

// confirmDeleted decides whether a delete that returned deleteErr removed the
// row. A NotFoundError means the follow-up read was also 404. A 404 APIError
// means that read found the row. Either way, read the row again. A missing
// row is success. A row that is still there is an error, so Terraform keeps
// the resource in state. Any other deleteErr is returned unchanged.
func confirmDeleted(deleteErr error, row string, get func() error) error {
	if deleteErr == nil || !deleteReportedMissing(deleteErr) {
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

// deleteReportedMissing reports a delete whose status is 404.
// NotFoundError means a follow-up read was also 404. An APIError means
// that read found the row.
func deleteReportedMissing(err error) bool {
	if vergeos.IsNotFoundError(err) {
		return true
	}
	var apiErr *vergeos.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}

func namedRow(kind string, id int, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Sprintf("%s %d", kind, id)
	}
	return fmt.Sprintf("%s %d (%s)", kind, id, name)
}
