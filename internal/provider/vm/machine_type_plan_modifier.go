// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vm

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
)

// machineTypeSemanticEquality handles the case where the API expands
// short-form machine types to full versions. This prevents Terraform
// from detecting drift when:
//   - User configures "q35" and API returns "pc-q35-X.X"
//   - User configures "pc" and API returns "pc-i440fx-X.X"
type machineTypeSemanticEquality struct{}

// MachineTypeSemanticEquality returns a plan modifier that treats
// short-form machine types as equivalent to their API-expanded forms.
func MachineTypeSemanticEquality() planmodifier.String {
	return machineTypeSemanticEquality{}
}

func (m machineTypeSemanticEquality) Description(_ context.Context) string {
	return "Treats short-form machine types as equivalent to their API-expanded forms"
}

func (m machineTypeSemanticEquality) MarkdownDescription(_ context.Context) string {
	return "Treats short-form machine types (e.g., `q35`) as equivalent to their API-expanded forms (e.g., `pc-q35-10.0`)"
}

func (m machineTypeSemanticEquality) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	// If there's no state value, nothing to compare
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() {
		return
	}

	// If plan is null/unknown, nothing to do
	if req.PlanValue.IsNull() || req.PlanValue.IsUnknown() {
		return
	}

	planVal := req.PlanValue.ValueString()
	stateVal := req.StateValue.ValueString()

	// If they're already equal, nothing to do
	if planVal == stateVal {
		return
	}

	// Check if state value is an expanded form of the plan value
	if machineTypesAreEquivalent(planVal, stateVal) {
		// Use the state value to suppress drift
		resp.PlanValue = req.StateValue
	}
}

// machineTypesAreEquivalent checks if two machine types are semantically equivalent.
// This handles the API v26 behavior where short-form aliases are expanded:
//   - "q35" is equivalent to any "pc-q35-*" value
//   - "pc" is equivalent to any "pc-i440fx-*" value
func machineTypesAreEquivalent(configured, fromAPI string) bool {
	if configured == fromAPI {
		return true
	}

	// Handle "q35" -> "pc-q35-X.X" expansion
	if configured == "q35" && strings.HasPrefix(fromAPI, "pc-q35-") {
		return true
	}

	// Handle "pc" -> "pc-i440fx-X.X" expansion
	if configured == "pc" && strings.HasPrefix(fromAPI, "pc-i440fx-") {
		return true
	}

	return false
}
