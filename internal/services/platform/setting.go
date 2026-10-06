// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const settingDescription = "One VergeOS system setting, identified by key. Login lockout, session timeout, and banner text are rows in this same table. The key is the name VergeOS stores. This resource does not create settings and does not delete them. Update sends {\"value\": ...} for this key only. Destroy writes that key's default_value back and leaves every other setting alone. Use value for text that can live in state. Use value_wo when the value is a secret. Terraform stores value_wo_version, not the secret."

var (
	_ resource.Resource                     = &settingResource{}
	_ resource.ResourceWithConfigure        = &settingResource{}
	_ resource.ResourceWithImportState      = &settingResource{}
	_ resource.ResourceWithIdentity         = &settingResource{}
	_ resource.ResourceWithConfigValidators = &settingResource{}
)

func NewSettingResource() resource.Resource {
	return &settingResource{}
}

type settingResource struct {
	api *API
}

type settingModel struct {
	ID             types.String `tfsdk:"id"`
	Key            types.String `tfsdk:"key"`
	Value          types.String `tfsdk:"value"`
	ValueWO        types.String `tfsdk:"value_wo"`
	ValueWOVersion types.Int64  `tfsdk:"value_wo_version"`
	DefaultValue   types.String `tfsdk:"default_value"`
	Description    types.String `tfsdk:"description"`
}

type loadedSetting struct {
	Key          string
	Value        string
	DefaultValue string
	Description  string
	RowID        string
}

type settingWire struct {
	DollarKey any    `json:"$key"`
	Key       string `json:"key"`
}

type settingValueBody struct {
	Value string `json:"value"`
}

func (r *settingResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_setting"
}

func (r *settingResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: settingDescription,
		Attributes: map[string]schema.Attribute{
			"id": idAttr("Setting key. The same string as key."),
			"key": schema.StringAttribute{
				MarkdownDescription: "Setting key, such as max_connections. Changing it restores the previous key to its default and manages the new key.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{stringvalidator.LengthAtLeast(1)},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "Value stored for this key. Set value, or set value_wo, not both. An empty string is a real value and is sent. When value_wo is in use, Terraform stores null here so the secret is not kept in state.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"value_wo": schema.StringAttribute{
				MarkdownDescription: "Secret value for this key. Sent on create and when value_wo_version changes. Terraform does not store it. Requires Terraform 1.11 or OpenTofu 1.11.",
				Optional:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("value_wo_version")),
				},
			},
			"value_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of value_wo. Increment it to send a new secret. Terraform stores this number, not the secret.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("value_wo")),
				},
			},
			"default_value": stableString("Default VergeOS uses when this resource is destroyed."),
			"description":   stableString("Description VergeOS stores for this key."),
		},
	}
}

func (r *settingResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Setting key, such as max_connections.")
}

func (r *settingResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{settingConfigValidator{}}
}

func (r *settingResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureAPI(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *settingResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config settingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createSetting(ctx, &plan, secret(config.ValueWO)); err != nil {
		resp.Diagnostics.AddError("Error updating setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, settingForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *settingResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data settingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readSetting(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, settingForState(&data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *settingResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config settingModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	secretValue := ""
	if versionChanged(plan.ValueWOVersion, state.ValueWOVersion) {
		secretValue = secret(config.ValueWO)
	}
	if err := r.api.updateSetting(ctx, &plan, &state, secretValue); err != nil {
		resp.Diagnostics.AddError("Error updating setting", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, settingForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *settingResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data settingModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	key, err := settingLookupKey(&data)
	if err != nil {
		resp.Diagnostics.AddError("Error restoring setting", err.Error())
		return
	}
	if err := r.api.deleteSetting(ctx, key); err != nil {
		resp.Diagnostics.AddError("Error restoring setting", err.Error())
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("restored setting %s to its default", key))
}

func (r *settingResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importSetting(ctx, req, resp)
}

func settingForState(data *settingModel) settingModel {
	stored := *data
	stored.ValueWO = types.StringNull()
	return stored
}

type settingConfigValidator struct{}

func (settingConfigValidator) Description(context.Context) string {
	return "Requires value or value_wo, and a setting key VergeOS can address."
}

func (v settingConfigValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (settingConfigValidator) ValidateResource(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data settingModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.Value.IsUnknown() || data.ValueWO.IsUnknown() || data.Key.IsUnknown() {
		return
	}
	if knownString(data.Key) {
		if err := validSettingKey(data.Key.ValueString()); err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("key"), "Invalid setting key", err.Error())
		}
	}
	hasValue := knownString(data.Value)
	hasSecret := knownString(data.ValueWO)
	if hasValue == hasSecret {
		resp.Diagnostics.AddError("Invalid setting", "Set value or value_wo, and only one of them.")
	}
}

func importSetting(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if id == "" && req.Identity != nil {
		var got types.String
		resp.Diagnostics.Append(req.Identity.GetAttribute(ctx, path.Root("id"), &got)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if knownString(got) {
			id = strings.TrimSpace(got.ValueString())
		}
	}
	if err := validSettingKey(id); err != nil {
		resp.Diagnostics.AddError("Invalid setting import id", "Import vergeio_setting with the setting key, such as max_connections.")
		return
	}
	shared.ImportByID(ctx, req, resp, "Invalid setting import id", "Import vergeio_setting with the setting key, such as max_connections.")
}

func (a *API) createSetting(ctx context.Context, data *settingModel, secretValue string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	key, err := settingLookupKey(data)
	if err != nil {
		return err
	}
	loaded, err := a.loadSetting(ctx, key)
	if vergeos.IsNotFoundError(err) {
		return fmt.Errorf("setting %q does not exist. vergeio_setting changes a setting VergeOS already has. It does not create settings", key)
	}
	if err != nil {
		return err
	}
	writeOnly := secretValue != "" || writeOnlyActive(data.ValueWOVersion)
	desired := loaded.Value
	send := false
	if writeOnly {
		desired = secretValue
		send = true
	} else if knownString(data.Value) {
		desired = data.Value.ValueString()
		send = desired != loaded.Value
	}
	if send {
		if err := a.putSettingValue(ctx, loaded.RowID, desired); err != nil {
			return err
		}
		loaded, err = a.loadSetting(ctx, key)
		if err != nil {
			return err
		}
	}
	applySetting(data, loaded, writeOnly)
	return nil
}

func (a *API) readSetting(ctx context.Context, data *settingModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	key, err := settingLookupKey(data)
	if err != nil {
		return err
	}
	loaded, err := a.loadSetting(ctx, key)
	if err != nil {
		return err
	}
	applySetting(data, loaded, writeOnlyActive(data.ValueWOVersion))
	return nil
}

func (a *API) updateSetting(ctx context.Context, plan, state *settingModel, secretValue string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	key, err := settingLookupKey(plan)
	if err != nil {
		return err
	}
	loaded, err := a.loadSetting(ctx, key)
	if vergeos.IsNotFoundError(err) {
		return fmt.Errorf("setting %q does not exist. vergeio_setting changes a setting VergeOS already has. It does not create settings", key)
	}
	if err != nil {
		return err
	}
	writeOnly := writeOnlyActive(plan.ValueWOVersion)
	send := false
	desired := loaded.Value
	if writeOnly {
		if versionChanged(plan.ValueWOVersion, state.ValueWOVersion) {
			send = true
			desired = secretValue
		}
	} else if knownString(plan.Value) && (!knownString(state.Value) || state.Value.ValueString() != plan.Value.ValueString()) {
		send = true
		desired = plan.Value.ValueString()
	}
	if send {
		if err := a.putSettingValue(ctx, loaded.RowID, desired); err != nil {
			return err
		}
		loaded, err = a.loadSetting(ctx, key)
		if err != nil {
			return err
		}
	}
	applySetting(plan, loaded, writeOnly)
	return nil
}

func (a *API) deleteSetting(ctx context.Context, key string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	loaded, err := a.loadSetting(ctx, key)
	if vergeos.IsNotFoundError(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if loaded.Value == loaded.DefaultValue {
		return nil
	}
	return a.putSettingValue(ctx, loaded.RowID, loaded.DefaultValue)
}

func (a *API) loadSetting(ctx context.Context, key string) (*loadedSetting, error) {
	got, err := a.sdk.Settings.GetByKey(ctx, key)
	if err != nil {
		return nil, err
	}
	loaded := &loadedSetting{
		Key:          key,
		Value:        got.Value,
		DefaultValue: got.DefaultValue,
		Description:  got.Description,
		RowID:        key,
	}
	if got.Key != "" {
		loaded.Key = got.Key
	}
	var rows []settingWire
	qerr := a.getJSON(ctx, settingsCollection, &vergeio.Options{
		Fields: "$key,key,value,default_value,description",
		Filter: fmt.Sprintf("key eq '%s'", escapeFilterValue(key)),
	}, &rows)
	if qerr != nil {
		return loaded, nil
	}
	for _, item := range rows {
		if item.Key != "" && item.Key != key {
			continue
		}
		loaded.RowID = settingRowID(item.DollarKey, loaded.Key)
		break
	}
	return loaded, nil
}

func (a *API) putSettingValue(ctx context.Context, rowID, value string) error {
	if strings.TrimSpace(rowID) == "" {
		return fmt.Errorf("setting row id is empty")
	}
	return a.putJSON(ctx, vergeio.ObjectPath(settingsCollection, rowID), settingValueBody{Value: value})
}

func applySetting(data *settingModel, loaded *loadedSetting, writeOnly bool) {
	if loaded == nil {
		return
	}
	data.ID = types.StringValue(loaded.Key)
	data.Key = types.StringValue(loaded.Key)
	if writeOnly {
		data.Value = types.StringNull()
	} else {
		data.Value = types.StringValue(loaded.Value)
	}
	data.DefaultValue = types.StringValue(loaded.DefaultValue)
	data.Description = types.StringValue(loaded.Description)
}

func settingLookupKey(data *settingModel) (string, error) {
	raw := ""
	if data != nil && knownString(data.Key) {
		raw = data.Key.ValueString()
	} else if data != nil && knownString(data.ID) {
		raw = data.ID.ValueString()
	}
	raw = strings.TrimSpace(raw)
	if err := validSettingKey(raw); err != nil {
		return "", err
	}
	return raw, nil
}

func settingRowID(v any, key string) string {
	switch n := v.(type) {
	case float64:
		if n > 0 && n == math.Trunc(n) && n < float64(math.MaxInt) {
			return strconv.FormatInt(int64(n), 10)
		}
	case string:
		s := strings.TrimSpace(n)
		if s != "" {
			return s
		}
	}
	return key
}
