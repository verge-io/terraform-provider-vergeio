// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"errors"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &APIKeyResource{}
	_ resource.ResourceWithImportState = &APIKeyResource{}
)

func NewAPIKeyResource() resource.Resource {
	return &APIKeyResource{}
}

// APIKeyResource is the managed vergeio_api_key. It keeps a long-lived user
// API key. The bearer token is returned by VergeOS only on create and is not
// stored. ephemeral.vergeio_api_key mints a short-lived token for one run.
type APIKeyResource struct {
	api *APIKeyApi
}

// APIKeyResourceModel is the Terraform model for the managed vergeio_api_key.
// There is no token attribute. A token attribute would be written to state.
type APIKeyResourceModel struct {
	Id             types.String `tfsdk:"id"`
	UserID         types.Int32  `tfsdk:"user_id"`
	Name           types.String `tfsdk:"name"`
	Description    types.String `tfsdk:"description"`
	IPAllowList    types.String `tfsdk:"ip_allow_list"`
	IPDenyList     types.String `tfsdk:"ip_deny_list"`
	Expires        types.Int64  `tfsdk:"expires"`
	Created        types.Int64  `tfsdk:"created"`
	LastLoginStamp types.Int64  `tfsdk:"lastlogin_stamp"`
	LastLoginIP    types.String `tfsdk:"lastlogin_ip"`
}

func (r *APIKeyResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *APIKeyResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Long-lived VergeOS user API key. Sets the name, description, expiry, and IP allow and deny lists. VergeOS returns the bearer token only when the key is created, and this resource does not store it. Use ephemeral.vergeio_api_key when a run needs a token. Do not reuse a name across the two: the ephemeral resource deletes an existing key of that name for the user.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "API key id assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user_id": schema.Int32Attribute{
				MarkdownDescription: "Key of the user that owns the API key. A vergeio_user id can be used with tonumber. Changing the user replaces the key.",
				Required:            true,
				Validators: []validator.Int32{
					int32validator.AtLeast(1),
				},
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "API key name. Must be unique for the user if you also use ephemeral.vergeio_api_key, because that resource deletes a key of the same name.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "API key description. Omit to leave an existing description unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ip_allow_list": schema.StringAttribute{
				MarkdownDescription: "Comma-separated IP addresses or CIDRs allowed to use the key. Omit to leave the current list unchanged. Set to an empty string to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ip_deny_list": schema.StringAttribute{
				MarkdownDescription: "Comma-separated IP addresses or CIDRs denied from using the key. Omit to leave the current list unchanged. Set to an empty string to clear it.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"expires": schema.Int64Attribute{
				MarkdownDescription: "Unix time when the key expires. Leave it unset for a key that does not expire. Removing a saved expiry clears it. The configuration is authoritative, so an expiry set in the VergeOS UI is replaced on the next apply.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"created": schema.Int64Attribute{
				MarkdownDescription: "Unix time when VergeOS created the key.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"lastlogin_stamp": schema.Int64Attribute{
				MarkdownDescription: "Unix time of the last use. Zero means the key has not been used. Refresh reads a new value. The plan keeps the refreshed value.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"lastlogin_ip": schema.StringAttribute{
				MarkdownDescription: "IP address of the last use. Empty when the key has not been used. Refresh reads a new value. The plan keeps the refreshed value.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *APIKeyResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	api, err := NewAPIKeyApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.api = api
}

func (r *APIKeyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createAPIKey(ctx, &data); err != nil {
		if apiKeyIDSet(&data) {
			r.rememberAPIKey(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating API key", err.Error())
		return
	}
	r.rememberAPIKey(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readAPIKey(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading API key", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, apiKeyForState(&data))...)
}

func (r *APIKeyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readAPIKey(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) || errors.Is(err, errStaleIdentity) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading API key", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, apiKeyForState(&data))...)
}

func (r *APIKeyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state APIKeyResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateAPIKey(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating API key", err.Error())
		return
	}
	plan.Id = state.Id
	if err := r.api.readAPIKey(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading API key", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, apiKeyForState(&plan))...)
}

func (r *APIKeyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data APIKeyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteAPIKey(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting API key", err.Error())
		return
	}
	tflog.Debug(ctx, "API key deleted")
}

func (r *APIKeyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid API Key Import ID",
			"Import vergeio_api_key with the API key id.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *APIKeyResource) rememberAPIKey(ctx context.Context, resp *resource.CreateResponse, data *APIKeyResourceModel) {
	if !apiKeyIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, apiKeyForState(data))...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored API key %s in state before read", data.Id.ValueString()))
}

func apiKeyIDSet(data *APIKeyResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && data.Id.ValueString() != ""
}

func apiKeyForState(data *APIKeyResourceModel) APIKeyResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.UserID = knownInt32(data.UserID)
	stored.Name = knownString(data.Name)
	stored.Description = knownString(data.Description)
	stored.IPAllowList = knownString(data.IPAllowList)
	stored.IPDenyList = knownString(data.IPDenyList)
	stored.Expires = knownInt64(data.Expires)
	stored.Created = knownInt64(data.Created)
	stored.LastLoginStamp = knownInt64(data.LastLoginStamp)
	stored.LastLoginIP = knownString(data.LastLoginIP)
	return stored
}
