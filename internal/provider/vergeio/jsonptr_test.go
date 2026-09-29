// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestKnownBoolKeepsFalseAndOmitsUnset(t *testing.T) {
	if got := KnownBool(types.BoolValue(false)); got == nil || *got {
		t.Fatalf("explicit false = %v, want false", got)
	}
	if got := KnownBool(types.BoolValue(true)); got == nil || !*got {
		t.Fatalf("explicit true = %v, want true", got)
	}
	if got := KnownBool(types.BoolNull()); got != nil {
		t.Fatalf("null bool was set: %v", *got)
	}
	if got := KnownBool(types.BoolUnknown()); got != nil {
		t.Fatalf("unknown bool was set: %v", *got)
	}
}

func TestKnownStringKeepsEmpty(t *testing.T) {
	got := KnownString(types.StringValue(""))
	if got == nil || *got != "" {
		t.Fatalf("explicit empty string = %v, want \"\"", got)
	}
	if got := KnownString(types.StringNull()); got != nil {
		t.Fatalf("null string was set: %q", *got)
	}
	if got := KnownString(types.StringUnknown()); got != nil {
		t.Fatalf("unknown string was set: %q", *got)
	}
}

func TestKnownInt32KeepsZero(t *testing.T) {
	got := KnownInt32(types.Int32Value(0))
	if got == nil || *got != 0 {
		t.Fatalf("explicit zero = %v, want 0", got)
	}
	if got := KnownInt32(types.Int32Null()); got != nil {
		t.Fatalf("null int was set: %d", *got)
	}
	if got := KnownInt32(types.Int32Unknown()); got != nil {
		t.Fatalf("unknown int was set: %d", *got)
	}
}

func TestChangedBoolSendsOnlyDifferences(t *testing.T) {
	if got := ChangedBool(types.BoolValue(false), types.BoolValue(true)); got == nil || *got {
		t.Fatalf("false replacing true = %v, want false", got)
	}
	if got := ChangedBool(types.BoolValue(false), types.BoolValue(false)); got != nil {
		t.Fatalf("unchanged false was sent: %v", *got)
	}
	if got := ChangedBool(types.BoolNull(), types.BoolValue(true)); got != nil {
		t.Fatalf("unset plan bool was sent: %v", *got)
	}
	if got := ChangedBool(types.BoolValue(false), types.BoolNull()); got == nil || *got {
		t.Fatalf("false with unknown state = %v, want false", got)
	}
}

func TestChangedStringSendsEmpty(t *testing.T) {
	got := ChangedString(types.StringValue(""), types.StringValue("before"))
	if got == nil || *got != "" {
		t.Fatalf("clearing a string = %v, want \"\"", got)
	}
	if got := ChangedString(types.StringValue(""), types.StringValue("")); got != nil {
		t.Fatalf("unchanged empty string was sent: %q", *got)
	}
	if got := ChangedString(types.StringNull(), types.StringValue("before")); got != nil {
		t.Fatalf("unset plan string was sent: %q", *got)
	}
}

func TestChangedInt32SendsZero(t *testing.T) {
	got := ChangedInt32(types.Int32Value(0), types.Int32Value(5))
	if got == nil || *got != 0 {
		t.Fatalf("zero replacing 5 = %v, want 0", got)
	}
	if got := ChangedInt32(types.Int32Value(0), types.Int32Value(0)); got != nil {
		t.Fatalf("unchanged zero was sent: %d", *got)
	}
	if got := ChangedInt32(types.Int32Null(), types.Int32Value(5)); got != nil {
		t.Fatalf("unset plan int was sent: %d", *got)
	}
}
