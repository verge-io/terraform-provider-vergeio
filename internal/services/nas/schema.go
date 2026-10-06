// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

func idAttr(description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

func optString(description string, validators ...validator.String) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
		Validators: validators,
	}
}

func optBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifierUseState(),
		},
	}
}

func optInt(description string, validators ...validator.Int64) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: description,
		Optional:            true,
		Computed:            true,
		PlanModifiers: []planmodifier.Int64{
			int64planmodifierUseState(),
		},
		Validators: validators,
	}
}

// timestampAttr is a VergeOS time such as created or modified.
// UseStateForUnknown would keep the prior time in the plan, and a new time
// from VergeOS would fail apply. Leave it unknown so the read can store it.
func timestampAttr(description string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: description,
		Computed:            true,
	}
}

func computedString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Computed:            true,
	}
}

func stableString(description string) schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: description,
		Computed:            true,
		PlanModifiers: []planmodifier.String{
			stringplanmodifier.UseStateForUnknown(),
		},
	}
}

func stableInt(description string) schema.Int64Attribute {
	return schema.Int64Attribute{
		MarkdownDescription: description,
		Computed:            true,
		PlanModifiers: []planmodifier.Int64{
			int64planmodifier.UseStateForUnknown(),
		},
	}
}

func stableBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Computed:            true,
		PlanModifiers: []planmodifier.Bool{
			boolplanmodifier.UseStateForUnknown(),
		},
	}
}

// createdAttr is the creation time. It is a VergeOS timestamp, so it stays
// unknown for the same reason as modified.
func createdAttr(description string) schema.Int64Attribute {
	return timestampAttr(description)
}
