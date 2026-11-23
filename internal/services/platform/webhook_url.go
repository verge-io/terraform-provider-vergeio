// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const webhookURLDescription = "A destination for VergeOS event notifications. Point it at a chat room, a ticket system, or an automation endpoint. name and url are required. authorization_value_wo is sent when the destination is created and when authorization_value_wo_version changes. Terraform stores the version, not the credential. timeout is seconds from 3 through 120. retries is 0 through 100. Removing this destination also removes its delivery log. An apply that is interrupted after VergeOS stores the row keeps that key in state. A later create adopts an existing destination with the same name when the configured fields match. A different destination is left in place, and the error includes its key and an import command."

var (
	_ resource.Resource                = &webhookURLResource{}
	_ resource.ResourceWithConfigure   = &webhookURLResource{}
	_ resource.ResourceWithImportState = &webhookURLResource{}
	_ resource.ResourceWithIdentity    = &webhookURLResource{}

	webhookAuthTypes = []string{
		vergeos.WebhookAuthNone,
		vergeos.WebhookAuthBasic,
		vergeos.WebhookAuthBearer,
		vergeos.WebhookAuthAPIKey,
	}
	webhookURLPattern = regexp.MustCompile(`^https?://\S+$`)
)

func NewWebhookURLResource() resource.Resource {
	return &webhookURLResource{}
}

type webhookURLResource struct {
	api *API
}

type webhookURLModel struct {
	ID                          types.String `tfsdk:"id"`
	Name                        types.String `tfsdk:"name"`
	URL                         types.String `tfsdk:"url"`
	Type                        types.String `tfsdk:"type"`
	Headers                     types.String `tfsdk:"headers"`
	AuthorizationType           types.String `tfsdk:"authorization_type"`
	AuthorizationValueWO        types.String `tfsdk:"authorization_value_wo"`
	AuthorizationValueWOVersion types.Int64  `tfsdk:"authorization_value_wo_version"`
	AllowInsecure               types.Bool   `tfsdk:"allow_insecure"`
	Timeout                     types.Int64  `tfsdk:"timeout"`
	Retries                     types.Int64  `tfsdk:"retries"`
}

func (r *webhookURLResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_url"
}

func (r *webhookURLResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: webhookURLDescription,
		Attributes: map[string]schema.Attribute{
			"id":   idAttr("Webhook destination key assigned by VergeOS."),
			"name": schema.StringAttribute{MarkdownDescription: "Destination name. Must be unique. 1 to 128 characters. VergeOS reuses keys, and this table has no readonly id. Refresh drops a row whose name differs, including a rename made outside Terraform. Destroy without a refresh leaves that row.", Required: true, Validators: []validator.String{stringvalidator.LengthBetween(1, 128)}},
			"url": schema.StringAttribute{
				MarkdownDescription: "HTTP or HTTPS endpoint that receives the POST.",
				Required:            true,
				Validators:          []validator.String{stringvalidator.RegexMatches(webhookURLPattern, "Use an http:// or https:// URL.")},
			},
			"type":               optString("Destination type. VergeOS uses custom. Omit to leave the current value unchanged.", stringvalidator.OneOf("custom")),
			"headers":            optString("HTTP headers, one Name:Value pair per line. Omit to leave the current headers unchanged."),
			"authorization_type": optString("How the destination authenticates: none, basic, bearer, or apikey. Omit to leave the current value unchanged.", stringvalidator.OneOf(webhookAuthTypes...)),
			"authorization_value_wo": schema.StringAttribute{
				MarkdownDescription: "Credential for authorization_type. For basic, use username:password. For bearer, the token only. For apikey, the key only. Sent when the destination is created and when authorization_value_wo_version changes. Terraform does not store it. Requires Terraform 1.11 or OpenTofu 1.11.",
				Optional:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
					stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("authorization_value_wo_version")),
				},
			},
			"authorization_value_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of authorization_value_wo. Increment it to send a new credential. Terraform stores this number, not the credential.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
					int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("authorization_value_wo")),
				},
			},
			"allow_insecure": optBool("Allow a TLS certificate VergeOS would otherwise reject. Omit to leave the current value unchanged."),
			"timeout":        optInt("Seconds to wait for a response, from 3 through 120. Omit to leave the current value unchanged.", int64validator.Between(3, 120)),
			"retries":        optInt("Delivery attempts after the first failure, from 0 through 100. Omit to leave the current value unchanged.", int64validator.Between(0, 100)),
		},
	}
}

func (r *webhookURLResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Webhook destination key, a positive integer.")
}

func (r *webhookURLResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureAPI(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *webhookURLResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config webhookURLModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createWebhookURL(ctx, &plan, secret(config.AuthorizationValueWO)); err != nil {
		// The row exists once its key is known. Keep that key so the next
		// apply reads it instead of posting the same name again.
		r.keepWebhookURL(ctx, resp, &plan)
		resp.Diagnostics.AddError("Error creating webhook destination", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(shared.WritableContext(ctx), webhookURLForState(&plan))...)
	shared.RememberIdentity(shared.WritableContext(ctx), &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *webhookURLResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data webhookURLModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readWebhookURL(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading webhook destination", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, webhookURLForState(&data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *webhookURLResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config webhookURLModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	credential := ""
	if versionChanged(plan.AuthorizationValueWOVersion, state.AuthorizationValueWOVersion) {
		credential = secret(config.AuthorizationValueWO)
	}
	if err := r.api.updateWebhookURL(ctx, &plan, &state, credential); err != nil {
		resp.Diagnostics.AddError("Error updating webhook destination", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, webhookURLForState(&plan))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *webhookURLResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data webhookURLModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteWebhookURL(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting webhook destination", err.Error())
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted webhook destination %s", data.ID.ValueString()))
}

func (r *webhookURLResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importPositive(ctx, req, resp, "Invalid webhook destination import id", "Import vergeio_webhook_url with the destination key, a positive integer.")
}

func webhookURLForState(data *webhookURLModel) webhookURLModel {
	stored := *data
	stored.AuthorizationValueWO = types.StringNull()
	return stored
}

func (a *API) createWebhookURL(ctx context.Context, data *webhookURLModel, credential string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if err := validateWebhookAuth(data.AuthorizationType, credential, true); err != nil {
		return err
	}
	timeout, err := knownIntPtr(data.Timeout)
	if err != nil {
		return err
	}
	retries, err := knownIntPtr(data.Retries)
	if err != nil {
		return err
	}
	req := &vergeos.WebhookURLCreateRequest{
		Name:               data.Name.ValueString(),
		URL:                data.URL.ValueString(),
		Type:               stringValue(data.Type),
		Headers:            stringValue(data.Headers),
		AuthorizationType:  stringValue(data.AuthorizationType),
		AuthorizationValue: credential,
		AllowInsecure:      boolPtr(data.AllowInsecure),
		Timeout:            timeout,
		Retries:            retries,
	}
	created, err := a.sdk.WebhookURLs.Create(shared.CommitContext(ctx), req)
	if err != nil {
		if shared.NameConflict(err) {
			return a.adoptWebhookURL(ctx, data)
		}
		return err
	}
	applyWebhookURL(data, created, *data)
	return nil
}

// adoptWebhookURL stores an existing destination after VergeOS rejected
// the name. The configured fields have to match. A different destination
// is left where it is, and the error names the key so it can be imported.
func (a *API) adoptWebhookURL(ctx context.Context, data *webhookURLModel) error {
	if data == nil || !knownString(data.Name) {
		return fmt.Errorf("webhook destination name is empty")
	}
	name := data.Name.ValueString()
	got, err := a.sdk.WebhookURLs.GetByName(ctx, name)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return fmt.Errorf("webhook destination %q is already in use, but no destination with that name was found. Delete the conflicting row in VergeOS, or import it with `terraform import vergeio_webhook_url.<name> <id>`", name)
		}
		return fmt.Errorf("webhook destination %q is already in use, and looking it up failed: %w. Delete it in VergeOS, or import it with `terraform import vergeio_webhook_url.<name> <id>`", name, err)
	}
	if ok, reason := webhookURLMatches(data, got); !ok {
		return fmt.Errorf("webhook destination %q already exists with id %d and was not stored because %s. Import it with `terraform import vergeio_webhook_url.<name> %d`, or delete it in VergeOS and apply again", name, got.Key.Int(), reason, got.Key.Int())
	}
	applyWebhookURL(data, got, *data)
	return nil
}

func webhookURLMatches(plan *webhookURLModel, got *vergeos.WebhookURL) (bool, string) {
	if plan == nil || got == nil {
		return false, "the destination was not found"
	}
	var diffs []string
	diffWebhookString("name", plan.Name, got.Name, &diffs)
	diffWebhookString("url", plan.URL, got.URL, &diffs)
	diffWebhookString("type", plan.Type, got.Type, &diffs)
	diffWebhookString("headers", plan.Headers, got.Headers, &diffs)
	diffWebhookString("authorization_type", plan.AuthorizationType, got.AuthorizationType, &diffs)
	if !plan.AllowInsecure.IsNull() && !plan.AllowInsecure.IsUnknown() && plan.AllowInsecure.ValueBool() != got.AllowInsecure {
		diffs = append(diffs, fmt.Sprintf("allow_insecure is %t, configuration is %t", got.AllowInsecure, plan.AllowInsecure.ValueBool()))
	}
	if !plan.Timeout.IsNull() && !plan.Timeout.IsUnknown() && plan.Timeout.ValueInt64() != int64(got.Timeout) {
		diffs = append(diffs, fmt.Sprintf("timeout is %d, configuration is %d", got.Timeout, plan.Timeout.ValueInt64()))
	}
	if !plan.Retries.IsNull() && !plan.Retries.IsUnknown() && plan.Retries.ValueInt64() != int64(got.Retries) {
		diffs = append(diffs, fmt.Sprintf("retries is %d, configuration is %d", got.Retries, plan.Retries.ValueInt64()))
	}
	if len(diffs) == 0 {
		return true, ""
	}
	return false, strings.Join(diffs, "; ")
}

func diffWebhookString(attr string, planned types.String, got string, diffs *[]string) {
	if planned.IsNull() || planned.IsUnknown() {
		return
	}
	if planned.ValueString() == got {
		return
	}
	*diffs = append(*diffs, fmt.Sprintf("%s is %q, configuration is %q", attr, got, planned.ValueString()))
}

func (r *webhookURLResource) keepWebhookURL(ctx context.Context, resp *resource.CreateResponse, data *webhookURLModel) {
	if resp == nil || data == nil || !knownString(data.ID) {
		return
	}
	ctx = shared.WritableContext(ctx)
	resp.Diagnostics.Append(resp.State.Set(ctx, webhookURLForState(data))...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (a *API) readWebhookURL(ctx context.Context, data *webhookURLModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseID(data.ID, "webhook destination")
	if err != nil {
		return err
	}
	got, err := a.sdk.WebhookURLs.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureWebhookURLOwned(data, got); err != nil {
		return err
	}
	applyWebhookURL(data, got, *data)
	return nil
}

// ensureWebhookURLOwned returns NotFound when the stored key now points at
// another destination. webhook_urls has no readonly id on VergeOS 26.2, so
// the unique name is the token. Skip the check when name is unset (import).
func ensureWebhookURLOwned(data *webhookURLModel, got *vergeos.WebhookURL) error {
	if data == nil || got == nil {
		return fmt.Errorf("webhook destination read returned no row")
	}
	if !knownString(data.Name) || data.Name.ValueString() == "" {
		return nil
	}
	if data.Name.ValueString() != got.Name {
		return &vergeos.NotFoundError{Resource: "WebhookURL", ID: got.Key.Int()}
	}
	return nil
}

func (a *API) updateWebhookURL(ctx context.Context, plan, state *webhookURLModel, credential string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if err := validateWebhookAuth(plan.AuthorizationType, credential, false); err != nil {
		return err
	}
	id, err := parseID(state.ID, "webhook destination")
	if err != nil {
		return err
	}
	current, err := a.sdk.WebhookURLs.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureWebhookURLOwned(state, current); err != nil {
		return err
	}
	timeout, err := changedIntPtr(plan.Timeout, state.Timeout)
	if err != nil {
		return err
	}
	retries, err := changedIntPtr(plan.Retries, state.Retries)
	if err != nil {
		return err
	}
	req := &vergeos.WebhookURLUpdateRequest{
		Name:              changedString(plan.Name, state.Name),
		URL:               changedString(plan.URL, state.URL),
		Type:              changedString(plan.Type, state.Type),
		Headers:           changedString(plan.Headers, state.Headers),
		AuthorizationType: changedString(plan.AuthorizationType, state.AuthorizationType),
		AllowInsecure:     changedBool(plan.AllowInsecure, state.AllowInsecure),
		Timeout:           timeout,
		Retries:           retries,
	}
	if credential != "" {
		req.AuthorizationValue = &credential
	}
	if !webhookURLUpdateEmpty(req) {
		if _, err := a.sdk.WebhookURLs.Update(ctx, id, req); err != nil {
			return err
		}
	}
	plan.ID = state.ID
	got, err := a.sdk.WebhookURLs.Get(ctx, id)
	if err != nil {
		return err
	}
	applyWebhookURL(plan, got, *plan)
	return nil
}

func (a *API) deleteWebhookURL(ctx context.Context, data *webhookURLModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseID(data.ID, "webhook destination")
	if err != nil {
		return err
	}
	current, getErr := a.sdk.WebhookURLs.Get(ctx, id)
	foreign, err := shared.ForeignRow(getErr, ensureWebhookURLOwned(data, current))
	if err != nil {
		return err
	}
	if foreign {
		return nil
	}
	if err := a.deleteWebhookDeliveries(ctx, id); err != nil {
		return err
	}
	return ignoreMissing(a.sdk.WebhookURLs.Delete(ctx, id))
}

func (a *API) deleteWebhookDeliveries(ctx context.Context, urlID int) error {
	rows, err := a.sdk.Webhooks.ListByWebhookURL(ctx, urlID)
	if listMissing(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list deliveries for webhook destination %d: %w", urlID, err)
	}
	for _, row := range rows {
		if err := ignoreMissing(a.sdk.Webhooks.Delete(ctx, row.Key.Int())); err != nil {
			return fmt.Errorf("delete webhook delivery %d: %w", row.Key.Int(), err)
		}
	}
	return nil
}

func validateWebhookAuth(authType types.String, credential string, requireCredential bool) error {
	kind := ""
	if knownString(authType) {
		kind = authType.ValueString()
	}
	if kind == "" || kind == vergeos.WebhookAuthNone {
		return nil
	}
	if requireCredential && credential == "" {
		return fmt.Errorf("authorization_type %s requires authorization_value_wo and authorization_value_wo_version", kind)
	}
	return nil
}

func applyWebhookURL(data *webhookURLModel, got *vergeos.WebhookURL, prior webhookURLModel) {
	if got == nil {
		return
	}
	data.ID = idString(got.Key.Int())
	data.Name = stringFromAPI(got.Name, prior.Name)
	data.URL = stringFromAPI(got.URL, prior.URL)
	data.Type = normalizeChoice(got.Type, prior.Type, "custom")
	data.Headers = stringFromAPI(got.Headers, prior.Headers)
	data.AuthorizationType = normalizeChoice(got.AuthorizationType, prior.AuthorizationType, vergeos.WebhookAuthNone)
	data.AllowInsecure = types.BoolValue(got.AllowInsecure)
	data.Timeout = types.Int64Value(int64(got.Timeout))
	data.Retries = types.Int64Value(int64(got.Retries))
	data.AuthorizationValueWOVersion = prior.AuthorizationValueWOVersion
}

func normalizeChoice(api string, prior types.String, fallback string) types.String {
	if api != "" {
		return types.StringValue(api)
	}
	if knownString(prior) {
		return prior
	}
	if fallback == "" {
		return types.StringNull()
	}
	return types.StringValue(fallback)
}

func webhookURLUpdateEmpty(req *vergeos.WebhookURLUpdateRequest) bool {
	if req == nil {
		return true
	}
	return req.Name == nil &&
		req.URL == nil &&
		req.Type == nil &&
		req.Headers == nil &&
		req.AuthorizationType == nil &&
		req.AuthorizationValue == nil &&
		req.AllowInsecure == nil &&
		req.Timeout == nil &&
		req.Retries == nil
}
