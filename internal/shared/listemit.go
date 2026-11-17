// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"context"
	"iter"

	vergeio "terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

// Listed is one object returned by a list resource.
type Listed struct {
	DisplayName string
	ID          string
	Resource    any
}

// ListError is a result stream that contains only diagnostics.
func ListError(diags diag.Diagnostics) iter.Seq[list.ListResult] {
	return list.ListResultsStreamDiagnostics(diags)
}

// EmitListed writes identity, and resource data when the request asked for it.
// Limit stops the stream after that many successful results.
func EmitListed(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream, items []Listed) {
	stream.Results = func(push func(list.ListResult) bool) {
		var sent int64
		for _, item := range items {
			if req.Limit > 0 && sent >= req.Limit {
				return
			}
			result := req.NewListResult(ctx)
			result.DisplayName = item.DisplayName
			result.Diagnostics.Append(SetKeyIdentity(ctx, result.Identity, types.StringValue(item.ID))...)
			if req.IncludeResource {
				if item.Resource == nil {
					result.Diagnostics.AddError("Incomplete list result", "Resource data was not loaded for "+item.DisplayName+".")
				} else {
					result.Diagnostics.Append(result.Resource.Set(ctx, item.Resource)...)
				}
			}
			if !push(result) {
				return
			}
			if result.Diagnostics.HasError() {
				return
			}
			sent++
		}
	}
}

// ConfiguredClient returns the provider client passed to a list resource Configure.
func ConfiguredClient(data any, diags *diag.Diagnostics) *vergeio.Client {
	if data == nil {
		return nil
	}
	c, ok := data.(*vergeio.Client)
	if !ok {
		if diags != nil {
			diags.AddError(
				"Unexpected List Resource Configure Type",
				"Expected *client.Client. Please report this issue to the provider developers.",
			)
		}
		return nil
	}
	return c
}

// ObjectLister is the shared list resource for one VergeOS table.
type ObjectLister struct {
	Suffix          string
	Description     string
	Collection      string
	TenantSupported bool
	Client          *vergeio.Client
	ListFn          func(ctx context.Context, filter string, sel Selection, include bool) ([]Listed, diag.Diagnostics)
}

func (l *ObjectLister) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + l.Suffix
}

func (l *ObjectLister) ListResourceConfigSchema(ctx context.Context, req list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: l.Description,
		Attributes:          ListQueryAttributes(),
	}
}

func (l *ObjectLister) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	l.Client = ConfiguredClient(req.ProviderData, &resp.Diagnostics)
}

func (l *ObjectLister) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	if l == nil || l.ListFn == nil {
		stream.Results = ListError(diag.Diagnostics{diag.NewErrorDiagnostic(
			"Unable to Create VergeOS API Client",
			"List resource is not configured.",
		)})
		return
	}
	var query ListQuery
	diags := req.Config.Get(ctx, &query)
	if diags.HasError() {
		stream.Results = ListError(diags)
		return
	}
	sdk, diags := SDKClient(l.Client)
	if diags.HasError() {
		stream.Results = ListError(diags)
		return
	}
	filter, sel, diags := BuildSelection(ctx, sdk, query, l.Collection, l.TenantSupported)
	if diags.HasError() {
		stream.Results = ListError(diags)
		return
	}
	items, diags := l.ListFn(ctx, filter, sel, req.IncludeResource)
	if diags.HasError() {
		stream.Results = ListError(diags)
		return
	}
	EmitListed(ctx, req, stream, items)
}

// SDKClient returns the shared govergeos client for a list call.
func SDKClient(c *vergeio.Client) (*vergeos.Client, diag.Diagnostics) {
	var diags diag.Diagnostics
	if c == nil {
		diags.AddError("Unable to Create VergeOS API Client", "List resource is not configured.")
		return nil, diags
	}
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		diags.AddError("Unable to Create VergeOS API Client", err.Error())
		return nil, diags
	}
	return sdk, diags
}
