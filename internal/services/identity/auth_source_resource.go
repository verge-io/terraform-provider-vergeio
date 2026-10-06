// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"errors"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                = &AuthSourceResource{}
	_ resource.ResourceWithImportState = &AuthSourceResource{}
)

func NewAuthSourceResource() resource.Resource {
	return &AuthSourceResource{}
}

// authSourceDrivers are the VergeOS auth source driver values. The driver is
// chosen at creation and cannot be changed.
var authSourceDrivers = []string{
	vergeos.AuthSourceDriverAzure,
	vergeos.AuthSourceDriverGoogle,
	vergeos.AuthSourceDriverGitLab,
	vergeos.AuthSourceDriverOkta,
	vergeos.AuthSourceDriverOpenID,
	vergeos.AuthSourceDriverOpenIDWellKnown,
	vergeos.AuthSourceDriverOAuth2,
	vergeos.AuthSourceDriverVergeIO,
}

// AuthSourceResource is vergeio_auth_source.
type AuthSourceResource struct {
	api *AuthSourceApi
}

// AuthSourceResourceModel is the Terraform model for vergeio_auth_source.
// client_secret_wo is write-only. settings never includes client_secret.
type AuthSourceResourceModel struct {
	Id                    types.String `tfsdk:"id"`
	Name                  types.String `tfsdk:"name"`
	Driver                types.String `tfsdk:"driver"`
	Settings              types.String `tfsdk:"settings"`
	ClientSecretWO        types.String `tfsdk:"client_secret_wo"`
	ClientSecretWOVersion types.Int64  `tfsdk:"client_secret_wo_version"`
	Menu                  types.Bool   `tfsdk:"menu"`
	Debug                 types.Bool   `tfsdk:"debug"`
	ButtonBackgroundColor types.String `tfsdk:"button_background_color"`
	ButtonColor           types.String `tfsdk:"button_color"`
	ButtonFAIcon          types.String `tfsdk:"button_fa_icon"`
	IconColor             types.String `tfsdk:"icon_color"`
}

func (r *AuthSourceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_auth_source"
}

func (r *AuthSourceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "VergeOS external identity provider, such as OIDC or OAuth2. driver is fixed at creation. settings is the non-secret JSON document. client_secret_wo is sent on create and when client_secret_wo_version changes, and it is not stored. An update merges settings into the stored document so a partial change does not wipe client_secret. A key removed from settings stays on the auth source. Replace the auth source to drop it. VergeOS rejects delete while users or OIDC applications still reference the source.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Auth source key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name shown on the login button. Must be unique.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"driver": schema.StringAttribute{
				MarkdownDescription: "Identity provider type: azure, google, gitlab, okta, openid, openid-well-known, oauth2, or verge.io. Changing it replaces the auth source. VergeOS does not change the driver of an existing source.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(authSourceDrivers...),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"settings": schema.StringAttribute{
				MarkdownDescription: "Driver settings as one JSON object, without client_secret. Use jsonencode. Omit to leave the stored document unchanged. Terraform stores this document so a change to a tracked key is planned. client_secret belongs in client_secret_wo. The server-managed debug key inside settings is not tracked.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					settingsCanonicalModifier{},
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"client_secret_wo": schema.StringAttribute{
				MarkdownDescription: "OAuth client secret sent on create, and again when client_secret_wo_version changes. Terraform does not store it. Do not put client_secret in settings. Requires Terraform 1.11 or OpenTofu 1.11.",
				Optional:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.AlsoRequires(path.MatchRoot("client_secret_wo_version")),
				},
			},
			"client_secret_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of client_secret_wo. Increment it to send a new client secret. Terraform stores this number, not the secret. Changing client_secret_wo without changing this version does not update the auth source.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AlsoRequires(path.MatchRoot("client_secret_wo")),
				},
			},
			"menu": schema.BoolAttribute{
				MarkdownDescription: "Show the source in the login dropdown instead of as a button. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"debug": schema.BoolAttribute{
				MarkdownDescription: "Enable provider debug logging. VergeOS turns debug off on its own after about an hour, and the next apply turns it back on when this is true. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"button_background_color": schema.StringAttribute{
				MarkdownDescription: "Login button background color, as a CSS color. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"button_color": schema.StringAttribute{
				MarkdownDescription: "Login button text color, as a CSS color. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"button_fa_icon": schema.StringAttribute{
				MarkdownDescription: "Login button icon class, for example bi-google. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"icon_color": schema.StringAttribute{
				MarkdownDescription: "Login button icon color, as a CSS color. Omit to leave the current value unchanged.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *AuthSourceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	api, err := NewAuthSourceApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.api = api
}

func (r *AuthSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data, config AuthSourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	secret := writeOnlySecret(data.ClientSecretWOVersion, types.Int64Null(), config.ClientSecretWO, false)
	if err := r.api.createAuthSource(ctx, &data, secret); err != nil {
		if authSourceIDSet(&data) {
			r.rememberAuthSource(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating auth source", err.Error())
		return
	}
	r.rememberAuthSource(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readAuthSource(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading auth source", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, authSourceForState(&data))...)
}

func (r *AuthSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data AuthSourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readAuthSource(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) || errors.Is(err, errStaleIdentity) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading auth source", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, authSourceForState(&data))...)
}

func (r *AuthSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config AuthSourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	secret := writeOnlySecret(plan.ClientSecretWOVersion, state.ClientSecretWOVersion, config.ClientSecretWO, true)
	if err := r.api.updateAuthSource(ctx, &plan, &state, secret); err != nil {
		resp.Diagnostics.AddError("Error updating auth source", err.Error())
		return
	}
	plan.Id = state.Id
	if err := r.api.readAuthSource(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading auth source", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, authSourceForState(&plan))...)
}

func (r *AuthSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data AuthSourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteAuthSource(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting auth source", err.Error())
		return
	}
	tflog.Debug(ctx, "auth source deleted")
}

func (r *AuthSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid Auth Source Import ID",
			"Import vergeio_auth_source with the auth source key.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *AuthSourceResource) rememberAuthSource(ctx context.Context, resp *resource.CreateResponse, data *AuthSourceResourceModel) {
	if !authSourceIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, authSourceForState(data))...)
	if resp.Diagnostics.HasError() {
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("stored auth source %s in state before read", data.Id.ValueString()))
}

func authSourceIDSet(data *AuthSourceResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && data.Id.ValueString() != ""
}

func authSourceForState(data *AuthSourceResourceModel) AuthSourceResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.Name = knownString(data.Name)
	stored.Driver = knownString(data.Driver)
	stored.Settings = knownString(data.Settings)
	stored.ClientSecretWO = types.StringNull()
	stored.ClientSecretWOVersion = knownInt64(data.ClientSecretWOVersion)
	stored.Menu = knownBool(data.Menu)
	stored.Debug = knownBool(data.Debug)
	stored.ButtonBackgroundColor = knownString(data.ButtonBackgroundColor)
	stored.ButtonColor = knownString(data.ButtonColor)
	stored.ButtonFAIcon = knownString(data.ButtonFAIcon)
	stored.IconColor = knownString(data.IconColor)
	return stored
}
