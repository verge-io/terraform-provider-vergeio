// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const (
	apiKeyPrivateKey = "api_key"
	apiKeyTTLMin     = 60
	apiKeyTTLMax     = 86400
)

// Ensure the ephemeral resource satisfies the framework interfaces.
var (
	_ ephemeral.EphemeralResource              = &APIKeyEphemeralResource{}
	_ ephemeral.EphemeralResourceWithConfigure = &APIKeyEphemeralResource{}
	_ ephemeral.EphemeralResourceWithClose     = &APIKeyEphemeralResource{}
	_ ephemeral.EphemeralResourceWithRenew     = &APIKeyEphemeralResource{}
)

func NewAPIKeyEphemeralResource() ephemeral.EphemeralResource {
	return &APIKeyEphemeralResource{}
}

// APIKeyEphemeralResource mints a VergeOS user API key for one Terraform run.
// The token is returned to the configuration and is not stored in state.
// Close deletes the key. The key also expires so a run that skips Close does
// not leave a long-lived credential.
type APIKeyEphemeralResource struct {
	sdk *vergeos.Client
}

// apiKeyModel is the ephemeral resource data.
type apiKeyModel struct {
	UserID      types.Int32  `tfsdk:"user_id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	TTLSeconds  types.Int64  `tfsdk:"ttl_seconds"`
	IPAllowList types.String `tfsdk:"ip_allow_list"`
	IPDenyList  types.String `tfsdk:"ip_deny_list"`
	ID          types.String `tfsdk:"id"`
	Token       types.String `tfsdk:"token"`
	Expires     types.Int64  `tfsdk:"expires"`
}

// apiKeyPrivateState is passed to Renew and Close. It is not the Terraform state file.
type apiKeyPrivateState struct {
	ID         int   `json:"id"`
	TTLSeconds int64 `json:"ttl_seconds"`
}

func (r *APIKeyEphemeralResource) Metadata(ctx context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_key"
}

func (r *APIKeyEphemeralResource) Schema(ctx context.Context, req ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Mints a short-lived VergeOS user API key for this run. The token is not stored in Terraform state. Close deletes the key, and the key expires on its own if Close does not run. Use the token as the api_key of a second provider configuration. Requires Terraform 1.10 or OpenTofu 1.11. The name is exclusive to this ephemeral resource: Open deletes an existing key with the same name for that user before creating a new one.",
		Attributes: map[string]schema.Attribute{
			"user_id": schema.Int32Attribute{
				MarkdownDescription: "Key of the user that owns the API key. A vergeio_users id can be used directly.",
				Required:            true,
				Validators: []validator.Int32{
					int32validator.AtLeast(1),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "API key name. Open deletes an existing key with this name for user_id, then creates a new one. Do not reuse a name that belongs to a long-lived key.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Optional description stored on the key.",
				Optional:            true,
			},
			"ttl_seconds": schema.Int64Attribute{
				MarkdownDescription: "Seconds until the key expires, from 60 to 86400. Close deletes the key at the end of the run. Renew extends the expiry when a run lasts longer than the TTL.",
				Required:            true,
				Validators: []validator.Int64{
					int64validator.Between(apiKeyTTLMin, apiKeyTTLMax),
				},
			},
			"ip_allow_list": schema.StringAttribute{
				MarkdownDescription: "Comma-separated IP addresses or CIDRs allowed to use the key. Omit to leave the VergeOS default.",
				Optional:            true,
			},
			"ip_deny_list": schema.StringAttribute{
				MarkdownDescription: "Comma-separated IP addresses or CIDRs denied from using the key. Omit to leave the VergeOS default.",
				Optional:            true,
			},
			"id": schema.StringAttribute{
				MarkdownDescription: "Key id assigned by VergeOS. Available during this run only.",
				Computed:            true,
			},
			"token": schema.StringAttribute{
				MarkdownDescription: "Bearer token. VergeOS returns it only when the key is created. It is not stored in state. Pass it to a provider api_key argument.",
				Computed:            true,
				Sensitive:           true,
			},
			"expires": schema.Int64Attribute{
				MarkdownDescription: "Unix time when the key expires.",
				Computed:            true,
			},
		},
	}
}

func (r *APIKeyEphemeralResource) Configure(ctx context.Context, req ephemeral.ConfigureRequest, resp *ephemeral.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Ephemeral Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	sdk, err := client.NewVergeosClient()
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.sdk = sdk
}

func (r *APIKeyEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var data apiKeyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.sdk == nil || r.sdk.UserAPIKeys == nil {
		resp.Diagnostics.AddError("Error opening API key", "The ephemeral resource is not configured.")
		return
	}

	opened, private, renewAt, err := openAPIKey(ctx, r.sdk, data)
	if err != nil {
		resp.Diagnostics.AddError("Error opening API key", err.Error())
		return
	}
	payload, err := json.Marshal(private)
	if err != nil {
		_ = closeAPIKey(ctx, r.sdk, private)
		resp.Diagnostics.AddError("Error opening API key", err.Error())
		return
	}
	if resp.Private == nil {
		_ = closeAPIKey(ctx, r.sdk, private)
		resp.Diagnostics.AddError("Error opening API key", "Ephemeral private state is not available.")
		return
	}
	resp.Diagnostics.Append(resp.Private.SetKey(ctx, apiKeyPrivateKey, payload)...)
	if resp.Diagnostics.HasError() {
		_ = closeAPIKey(ctx, r.sdk, private)
		return
	}
	resp.RenewAt = renewAt
	resp.Diagnostics.Append(resp.Result.Set(ctx, &opened)...)
}

func (r *APIKeyEphemeralResource) Renew(ctx context.Context, req ephemeral.RenewRequest, resp *ephemeral.RenewResponse) {
	private, diags := readAPIKeyPrivate(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.sdk == nil || r.sdk.UserAPIKeys == nil {
		resp.Diagnostics.AddError("Error renewing API key", "The ephemeral resource is not configured.")
		return
	}
	renewAt, err := renewAPIKey(ctx, r.sdk, private)
	if err != nil {
		resp.Diagnostics.AddError("Error renewing API key", err.Error())
		return
	}
	resp.RenewAt = renewAt
}

func (r *APIKeyEphemeralResource) Close(ctx context.Context, req ephemeral.CloseRequest, resp *ephemeral.CloseResponse) {
	private, diags := readAPIKeyPrivate(ctx, req.Private)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if private.ID == 0 {
		return
	}
	if r.sdk == nil || r.sdk.UserAPIKeys == nil {
		resp.Diagnostics.AddError("Error closing API key", "The ephemeral resource is not configured.")
		return
	}
	if err := closeAPIKey(ctx, r.sdk, private); err != nil {
		resp.Diagnostics.AddError("Error closing API key", err.Error())
	}
}

func openAPIKey(ctx context.Context, sdk *vergeos.Client, data apiKeyModel) (apiKeyModel, apiKeyPrivateState, time.Time, error) {
	if sdk == nil || sdk.UserAPIKeys == nil {
		return data, apiKeyPrivateState{}, time.Time{}, fmt.Errorf("API key client is not configured")
	}
	userID := int(data.UserID.ValueInt32())
	name := data.Name.ValueString()
	ttl := data.TTLSeconds.ValueInt64()
	if userID <= 0 || name == "" || ttl <= 0 {
		return data, apiKeyPrivateState{}, time.Time{}, fmt.Errorf("user_id, name, and ttl_seconds are required")
	}

	// A previous run may have stopped before Close. The name is exclusive to
	// this ephemeral resource, so replace that key instead of failing create.
	existing, err := sdk.UserAPIKeys.GetByName(ctx, userID, name)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return data, apiKeyPrivateState{}, time.Time{}, err
	}
	if existing != nil && existing.Key.Int() > 0 {
		if err := sdk.UserAPIKeys.Delete(ctx, existing.Key.Int()); err != nil && !vergeos.IsNotFoundError(err) {
			return data, apiKeyPrivateState{}, time.Time{}, err
		}
		tflog.Debug(ctx, fmt.Sprintf("deleted existing API key %d named %q before minting a new one", existing.Key.Int(), name))
	}

	expires := time.Now().Add(time.Duration(ttl) * time.Second).Unix()
	created, token, err := sdk.UserAPIKeys.Create(ctx, &vergeos.UserAPIKeyCreateRequest{
		User:        userID,
		Name:        name,
		Description: stringValue(data.Description),
		IPAllowList: stringValue(data.IPAllowList),
		IPDenyList:  stringValue(data.IPDenyList),
		ExpiresType: vergeos.APIKeyExpiresDate,
		Expires:     &expires,
	})
	if err != nil {
		return data, apiKeyPrivateState{}, time.Time{}, err
	}
	if created == nil || created.Key.Int() <= 0 {
		return data, apiKeyPrivateState{}, time.Time{}, fmt.Errorf("VergeOS did not return an API key id")
	}
	if token == "" {
		_ = sdk.UserAPIKeys.Delete(ctx, created.Key.Int())
		return data, apiKeyPrivateState{}, time.Time{}, fmt.Errorf("VergeOS did not return an API key token")
	}

	private := apiKeyPrivateState{ID: created.Key.Int(), TTLSeconds: ttl}
	data.ID = types.StringValue(fmt.Sprintf("%d", private.ID))
	data.Token = types.StringValue(token)
	data.Expires = types.Int64Value(expires)
	tflog.Debug(ctx, fmt.Sprintf("opened API key %d for user %d", private.ID, userID))
	return data, private, renewAt(time.Now(), time.Duration(ttl)*time.Second), nil
}

func renewAPIKey(ctx context.Context, sdk *vergeos.Client, private apiKeyPrivateState) (time.Time, error) {
	if sdk == nil || sdk.UserAPIKeys == nil {
		return time.Time{}, fmt.Errorf("API key client is not configured")
	}
	if private.ID <= 0 || private.TTLSeconds <= 0 {
		return time.Time{}, fmt.Errorf("API key private state is missing")
	}
	ttl := time.Duration(private.TTLSeconds) * time.Second
	expires := time.Now().Add(ttl).Unix()
	expiresType := vergeos.APIKeyExpiresDate
	if _, err := sdk.UserAPIKeys.Update(ctx, private.ID, &vergeos.UserAPIKeyUpdateRequest{
		ExpiresType: &expiresType,
		Expires:     &expires,
	}); err != nil {
		return time.Time{}, err
	}
	tflog.Debug(ctx, fmt.Sprintf("renewed API key %d", private.ID))
	return renewAt(time.Now(), ttl), nil
}

func closeAPIKey(ctx context.Context, sdk *vergeos.Client, private apiKeyPrivateState) error {
	if private.ID <= 0 {
		return nil
	}
	if sdk == nil || sdk.UserAPIKeys == nil {
		return fmt.Errorf("API key client is not configured")
	}
	if err := sdk.UserAPIKeys.Delete(ctx, private.ID); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("closed API key %d", private.ID))
	return nil
}

type apiKeyPrivateReader interface {
	GetKey(context.Context, string) ([]byte, diag.Diagnostics)
}

func readAPIKeyPrivate(ctx context.Context, private apiKeyPrivateReader) (apiKeyPrivateState, diag.Diagnostics) {
	var diags diag.Diagnostics
	var state apiKeyPrivateState
	if private == nil {
		diags.AddError("Error reading API key", "Ephemeral private state is not available.")
		return state, diags
	}
	raw, keyDiags := private.GetKey(ctx, apiKeyPrivateKey)
	diags.Append(keyDiags...)
	if diags.HasError() {
		return state, diags
	}
	if len(raw) == 0 {
		return state, diags
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		diags.AddError("Error reading API key", err.Error())
	}
	return state, diags
}

func stringValue(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

// renewAt is when Terraform should call Renew, shortly before the key expires.
func renewAt(now time.Time, ttl time.Duration) time.Time {
	return now.Add(ttl - renewLead(ttl))
}

func renewLead(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return 0
	}
	lead := ttl / 10
	if lead < 15*time.Second {
		lead = 15 * time.Second
	}
	if lead > time.Minute {
		lead = time.Minute
	}
	if lead >= ttl {
		return ttl / 2
	}
	return lead
}
