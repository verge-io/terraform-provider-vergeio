// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"errors"
	"fmt"
	"testing"

	"github.com/verge-io/govergeos"
)

func TestForeignRow(t *testing.T) {
	missing := &vergeos.NotFoundError{Resource: "Tag", ID: 3}
	other := errors.New("read failed")
	wrapped := fmt.Errorf("tag category: %w", missing)

	cases := []struct {
		name    string
		getErr  error
		owned   error
		foreign bool
		wantErr error
	}{
		{name: "match", foreign: false},
		{name: "missing row", getErr: missing, foreign: false},
		{name: "wrapped missing row", getErr: wrapped, foreign: false},
		{name: "read error", getErr: other, wantErr: other},
		{name: "foreign identity", owned: missing, foreign: true},
		{name: "identity error", owned: other, wantErr: other},
		{name: "missing row ignores identity error", getErr: missing, owned: other, foreign: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			foreign, err := ForeignRow(tc.getErr, tc.owned)
			if foreign != tc.foreign {
				t.Fatalf("foreign = %t, want %t", foreign, tc.foreign)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
