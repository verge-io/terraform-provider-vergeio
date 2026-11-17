// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestProvisionedAligned(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		bytes int64
		want  bool
	}{
		{name: "1 GiB", bytes: 1073741824, want: true},
		{name: "2 GiB", bytes: 2147483648, want: true},
		{name: "100 GiB", bytes: 107374182400, want: true},
		{name: "1.5 GiB", bytes: 1610612736, want: false},
		{name: "2.5 GiB", bytes: 2684354560, want: false},
		{name: "1 MiB", bytes: 1048576, want: false},
		{name: "1 byte over GiB", bytes: 1073741825, want: false},
		{name: "zero", bytes: 0, want: false},
		{name: "negative", bytes: -1073741824, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := provisionedAligned(tc.bytes); got != tc.want {
				t.Fatalf("provisionedAligned(%d) = %v, want %v", tc.bytes, got, tc.want)
			}
		})
	}
}

func TestProvisionedMultipleOfGiBValidator(t *testing.T) {
	t.Parallel()
	v := provisionedMultipleOfGiB{}

	t.Run("aligned accepted", func(t *testing.T) {
		t.Parallel()
		var resp validator.Int64Response
		v.ValidateInt64(context.Background(), validator.Int64Request{
			Path:        path.Root("provisioned"),
			ConfigValue: types.Int64Value(1073741824),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
	})

	t.Run("non-aligned rejected", func(t *testing.T) {
		t.Parallel()
		var resp validator.Int64Response
		v.ValidateInt64(context.Background(), validator.Int64Request{
			Path:        path.Root("provisioned"),
			ConfigValue: types.Int64Value(1610612736),
		}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("expected error for non-aligned provisioned")
		}
		summary := resp.Diagnostics[0].Summary()
		if summary != "Invalid provisioned value" {
			t.Fatalf("summary = %q", summary)
		}
	})

	t.Run("unknown skipped", func(t *testing.T) {
		t.Parallel()
		var resp validator.Int64Response
		v.ValidateInt64(context.Background(), validator.Int64Request{
			Path:        path.Root("provisioned"),
			ConfigValue: types.Int64Unknown(),
		}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
		}
	})
}
