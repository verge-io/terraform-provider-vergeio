// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestApplyWriteOnlyString(t *testing.T) {
	secret := types.StringValue("secret")
	version := types.Int64Value(1)

	var created types.String
	ApplyWriteOnlyString(&created, version, types.Int64Null(), secret, false)
	if created.ValueString() != "secret" {
		t.Fatalf("create = %#v", created)
	}

	var same types.String
	ApplyWriteOnlyString(&same, version, version, secret, true)
	if !same.IsNull() {
		t.Fatalf("unchanged version copied the secret: %#v", same)
	}

	var rotated types.String
	ApplyWriteOnlyString(&rotated, types.Int64Value(2), version, secret, true)
	if rotated.ValueString() != "secret" {
		t.Fatalf("rotated = %#v", rotated)
	}

	var skipped types.String
	ApplyWriteOnlyString(&skipped, version, types.Int64Null(), types.StringNull(), false)
	if !skipped.IsNull() {
		t.Fatalf("null config copied: %#v", skipped)
	}
}

func TestWriteOnlyVersionSet(t *testing.T) {
	if WriteOnlyVersionSet(types.Int64Null()) || WriteOnlyVersionSet(types.Int64Unknown()) {
		t.Fatal("unset version was treated as set")
	}
	if !WriteOnlyVersionSet(types.Int64Value(0)) {
		t.Fatal("0 is a configured version")
	}
}
