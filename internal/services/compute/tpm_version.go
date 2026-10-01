// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

var (
	_ basetypes.StringTypable                    = TPMVersionType{}
	_ basetypes.StringValuableWithSemanticEquals = TPMVersion{}
)

// TPMVersionType is the schema type for a TPM version. "2.0" and "2" are the
// same version, as are "1.2" and "1". The plan keeps the configured text.
// SemanticEquals is what lets Terraform accept the stored key after apply.
type TPMVersionType struct {
	basetypes.StringType
}

func (t TPMVersionType) Equal(o attr.Type) bool {
	other, ok := o.(TPMVersionType)
	if !ok {
		return false
	}
	return t.StringType.Equal(other.StringType)
}

func (t TPMVersionType) String() string {
	return "TPMVersionType"
}

func (t TPMVersionType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return TPMVersion{StringValue: in}, nil
}

func (t TPMVersionType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	attrValue, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	stringValue, ok := attrValue.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type of %T", attrValue)
	}
	stringValuable, diags := t.ValueFromString(ctx, stringValue)
	if diags.HasError() {
		return nil, fmt.Errorf("unexpected error converting StringValue to StringValuable: %v", diags)
	}
	return stringValuable, nil
}

func (t TPMVersionType) ValueType(context.Context) attr.Value {
	return TPMVersion{}
}

// TPMVersion is the value type for TPMVersionType.
type TPMVersion struct {
	basetypes.StringValue
}

func NewTPMVersionNull() TPMVersion {
	return TPMVersion{StringValue: basetypes.NewStringNull()}
}

func NewTPMVersionValue(value string) TPMVersion {
	return TPMVersion{StringValue: basetypes.NewStringValue(value)}
}

func (v TPMVersion) Equal(o attr.Value) bool {
	other, ok := o.(TPMVersion)
	if !ok {
		return false
	}
	return v.StringValue.Equal(other.StringValue)
}

func (v TPMVersion) Type(context.Context) attr.Type {
	return TPMVersionType{}
}

// StringSemanticEquals reports whether two TPM versions are the same VergeOS
// value. "2.0" equals "2" and "1.2" equals "1". The comparison does not
// rewrite either value.
func (v TPMVersion) StringSemanticEquals(_ context.Context, newValuable basetypes.StringValuable) (bool, diag.Diagnostics) {
	var diags diag.Diagnostics
	newValue, ok := newValuable.(TPMVersion)
	if !ok {
		diags.AddError(
			"TPM Version Semantic Equality Error",
			"An unexpected value type was received while comparing TPM versions. "+
				"Please report this to the provider developers.\n\n"+
				fmt.Sprintf("Expected Value Type: %T\n", v)+
				fmt.Sprintf("Got Value Type: %T", newValuable),
		)
		return false, diags
	}
	return normalizeTPMVersion(v.ValueString()) == normalizeTPMVersion(newValue.ValueString()), diags
}
