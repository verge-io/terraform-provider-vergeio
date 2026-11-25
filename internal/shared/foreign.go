// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import "github.com/verge-io/govergeos"

// ForeignRow reports that the row at the stored key is a different object.
// getErr is the error from reading that key. owned is the identity check
// Read already uses, and is ignored when the read failed. A missing row is
// not foreign. A not-found identity check is foreign, so Delete must not
// call DELETE. Any other error is returned.
func ForeignRow(getErr, owned error) (bool, error) {
	if getErr != nil {
		if vergeos.IsNotFoundError(getErr) {
			return false, nil
		}
		return false, getErr
	}
	if vergeos.IsNotFoundError(owned) {
		return true, nil
	}
	if owned != nil {
		return false, owned
	}
	return false, nil
}
