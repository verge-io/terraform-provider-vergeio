// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type keyIdentity struct {
	ID types.String `tfsdk:"id"`
}

// KeyIdentitySchema is the identity for a resource whose import id is the
// string already stored in id. OpenTofu import blocks use that same string.
func KeyIdentitySchema(description string) identityschema.Schema {
	return identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"id": identityschema.StringAttribute{
				Description:       description,
				RequiredForImport: true,
			},
		},
	}
}

// RememberIdentity writes id into identity when the framework supplied one.
func RememberIdentity(ctx context.Context, diags *diag.Diagnostics, identity *tfsdk.ResourceIdentity, id types.String) {
	if diags == nil {
		return
	}
	diags.Append(SetKeyIdentity(ctx, identity, id)...)
}

// SetKeyIdentity writes id into resource identity. A nil identity means the
// caller is a unit test that did not go through the framework server.
func SetKeyIdentity(ctx context.Context, identity *tfsdk.ResourceIdentity, id types.String) diag.Diagnostics {
	if identity == nil {
		return nil
	}
	text := ""
	if !id.IsNull() && !id.IsUnknown() {
		text = strings.TrimSpace(id.ValueString())
	}
	if text == "" {
		return diag.Diagnostics{diag.NewErrorDiagnostic(
			"Missing resource identity",
			"The resource id is empty.",
		)}
	}
	return identity.Set(ctx, &keyIdentity{ID: types.StringValue(text)})
}

// ImportByID accepts the existing import id or an identity id attribute.
// An empty import with no identity returns summary and detail.
func ImportByID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary, detail string) {
	if strings.TrimSpace(req.ID) == "" && req.Identity == nil {
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	if strings.TrimSpace(req.ID) != "" {
		resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
		return
	}
	resource.ImportStatePassthroughWithIdentity(ctx, path.Root("id"), path.Root("id"), req, resp)
}
