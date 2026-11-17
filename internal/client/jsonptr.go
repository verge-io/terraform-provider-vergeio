// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import "github.com/hashicorp/terraform-plugin-framework/types"

// Pointer helpers for VergeOS request bodies.
//
// A plain Go bool, number, or string tagged omitempty is left out of JSON
// when it holds false, 0, or "". VergeOS then keeps the previous value or
// its platform default. Request fields are pointers so an explicit zero is
// sent and an unset attribute is omitted. That is the govergeos ADR-004
// pattern: set the pointer only when the plan value is known and not null.

// KnownBool returns a pointer when v is set, including false.
// Null and unknown values are omitted.
func KnownBool(v types.Bool) *bool {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	b := v.ValueBool()
	return &b
}

// KnownString returns a pointer when v is set, including "".
// Null and unknown values are omitted.
func KnownString(v types.String) *string {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	s := v.ValueString()
	return &s
}

// KnownInt32 returns a pointer when v is set, including 0.
// Null and unknown values are omitted.
func KnownInt32(v types.Int32) *int32 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := v.ValueInt32()
	return &n
}

// KnownInt64 returns a pointer when v is set, including 0.
// Null and unknown values are omitted.
func KnownInt64(v types.Int64) *int64 {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := v.ValueInt64()
	return &n
}

// ChangedBool returns a pointer when the plan value is set and differs from
// state. Unset plan values are omitted so an update does not send false for
// an attribute the configuration left alone.
func ChangedBool(plan, state types.Bool) *bool {
	if plan.IsNull() || plan.IsUnknown() {
		return nil
	}
	if !state.IsNull() && !state.IsUnknown() && state.ValueBool() == plan.ValueBool() {
		return nil
	}
	b := plan.ValueBool()
	return &b
}

// ChangedString returns a pointer when the plan value is set and differs
// from state, including a change to "".
func ChangedString(plan, state types.String) *string {
	if plan.IsNull() || plan.IsUnknown() {
		return nil
	}
	if !state.IsNull() && !state.IsUnknown() && state.ValueString() == plan.ValueString() {
		return nil
	}
	s := plan.ValueString()
	return &s
}

// ChangedInt32 returns a pointer when the plan value is set and differs
// from state, including a change to 0.
func ChangedInt32(plan, state types.Int32) *int32 {
	if plan.IsNull() || plan.IsUnknown() {
		return nil
	}
	if !state.IsNull() && !state.IsUnknown() && state.ValueInt32() == plan.ValueInt32() {
		return nil
	}
	n := plan.ValueInt32()
	return &n
}

// ChangedInt64 returns a pointer when the plan value is set and differs
// from state, including a change to 0.
func ChangedInt64(plan, state types.Int64) *int64 {
	if plan.IsNull() || plan.IsUnknown() {
		return nil
	}
	if !state.IsNull() && !state.IsUnknown() && state.ValueInt64() == plan.ValueInt64() {
		return nil
	}
	n := plan.ValueInt64()
	return &n
}

// BoolOr, StringOr, Int32Or, and Int64Or read a decoded pointer.
// A missing JSON field stays nil and yields the fallback, which is the zero
// value the previous plain fields produced.

func BoolOr(p *bool, fallback bool) bool {
	if p == nil {
		return fallback
	}
	return *p
}

func StringOr(p *string, fallback string) string {
	if p == nil {
		return fallback
	}
	return *p
}

func Int32Or(p *int32, fallback int32) int32 {
	if p == nil {
		return fallback
	}
	return *p
}

func Int64Or(p *int64, fallback int64) int64 {
	if p == nil {
		return fallback
	}
	return *p
}
