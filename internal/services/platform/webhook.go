// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"fmt"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const webhookDescription = "One message sent to a vergeio_webhook_url. Create queues the message with WebhookURLs.Send. VergeOS does not edit a delivery, so changing message or webhook_url_id sends a new message and deletes the previous delivery row. Destroy deletes that delivery row and leaves the destination in place. A delivery expires on the system after about 70 days. A missing row is dropped from state, and the next apply sends the message again."

var (
	_ resource.Resource                = &webhookResource{}
	_ resource.ResourceWithConfigure   = &webhookResource{}
	_ resource.ResourceWithImportState = &webhookResource{}
	_ resource.ResourceWithIdentity    = &webhookResource{}
)

func NewWebhookResource() resource.Resource {
	return &webhookResource{}
}

type webhookResource struct {
	api *API
}

type webhookModel struct {
	ID           types.String `tfsdk:"id"`
	WebhookURLID types.String `tfsdk:"webhook_url_id"`
	Message      types.String `tfsdk:"message"`
	Status       types.String `tfsdk:"status"`
	StatusInfo   types.String `tfsdk:"status_info"`
	LastAttempt  types.Int64  `tfsdk:"last_attempt"`
	Created      types.Int64  `tfsdk:"created"`
}

func (r *webhookResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook"
}

func (r *webhookResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: webhookDescription,
		Attributes: map[string]schema.Attribute{
			"id": idAttr("Delivery key assigned by VergeOS."),
			"webhook_url_id": schema.StringAttribute{
				MarkdownDescription: "vergeio_webhook_url.id. Changing it sends a new message and deletes this delivery.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"message": schema.StringAttribute{
				MarkdownDescription: "JSON payload sent to the destination. Changing it sends a new message and deletes this delivery.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"status":       volatileString("Delivery status: queued, running, sent, or error."),
			"status_info":  volatileString("Detail for status, including an error from the destination."),
			"last_attempt": timestampAttr("Time of the last delivery attempt, as seconds since the epoch."),
			"created":      timestampAttr("Time the delivery was queued, as seconds since the epoch."),
		},
	}
}

func (r *webhookResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("Webhook delivery key, a positive integer.")
}

func (r *webhookResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configureAPI(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createWebhook(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error sending webhook", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readWebhook(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading webhook", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readWebhook(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading webhook", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := parseID(data.ID, "webhook")
	if err != nil {
		resp.Diagnostics.AddError("Error deleting webhook", err.Error())
		return
	}
	if err := r.api.deleteWebhook(ctx, id); err != nil {
		resp.Diagnostics.AddError("Error deleting webhook", err.Error())
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted webhook delivery %d", id))
}

func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importPositive(ctx, req, resp, "Invalid webhook import id", "Import vergeio_webhook with the delivery key, a positive integer.")
}

func (a *API) createWebhook(ctx context.Context, data *webhookModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	urlID, err := parseID(data.WebhookURLID, "webhook destination")
	if err != nil {
		return err
	}
	if !knownString(data.Message) || strings.TrimSpace(data.Message.ValueString()) == "" {
		return fmt.Errorf("message is required")
	}
	message := data.Message.ValueString()
	if err := a.sdk.WebhookURLs.Send(ctx, urlID, message); err != nil {
		return err
	}
	got, err := a.waitForWebhook(ctx, urlID, message)
	if err != nil {
		return err
	}
	applyWebhook(data, got)
	return nil
}

func (a *API) readWebhook(ctx context.Context, data *webhookModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseID(data.ID, "webhook")
	if err != nil {
		return err
	}
	got, err := a.sdk.Webhooks.Get(ctx, id)
	if err != nil {
		return err
	}
	applyWebhook(data, got)
	return nil
}

func (a *API) deleteWebhook(ctx context.Context, id int) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	return ignoreMissing(a.sdk.Webhooks.Delete(ctx, id))
}

func (a *API) waitForWebhook(ctx context.Context, urlID int, message string) (*vergeos.Webhook, error) {
	tries := a.webhookTries
	if tries <= 0 {
		tries = 1
	}
	wait := a.webhookWait
	var last error
	for attempt := 0; attempt < tries; attempt++ {
		if attempt > 0 && wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		rows, err := a.sdk.Webhooks.ListByWebhookURL(ctx, urlID)
		if err != nil {
			last = err
			continue
		}
		var best *vergeos.Webhook
		for i := range rows {
			row := &rows[i]
			if strings.TrimSpace(row.Message) != strings.TrimSpace(message) {
				continue
			}
			if best == nil || row.Key.Int() > best.Key.Int() {
				best = row
			}
		}
		if best != nil {
			return a.sdk.Webhooks.Get(ctx, best.Key.Int())
		}
	}
	shown := message
	if len(shown) > 200 {
		shown = shown[:200]
	}
	if last != nil {
		return nil, fmt.Errorf("webhook message was queued for destination %d but did not appear: %w", urlID, last)
	}
	return nil, fmt.Errorf("webhook message was queued for destination %d but did not appear: %s", urlID, shown)
}

func applyWebhook(data *webhookModel, got *vergeos.Webhook) {
	if got == nil {
		return
	}
	data.ID = idString(got.Key.Int())
	if got.WebhookURL.Int() > 0 {
		reported := idString(got.WebhookURL.Int())
		if !knownString(data.WebhookURLID) || data.WebhookURLID.ValueString() != reported.ValueString() {
			data.WebhookURLID = reported
		}
	}
	if got.Message != "" {
		if !knownString(data.Message) || strings.TrimSpace(data.Message.ValueString()) != strings.TrimSpace(got.Message) {
			data.Message = types.StringValue(got.Message)
		}
	}
	data.Status = stringFromAPI(got.Status, data.Status)
	data.StatusInfo = stringFromAPI(got.StatusInfo, data.StatusInfo)
	data.LastAttempt = timestampFromAPI(got.LastAttempt)
	data.Created = timestampFromAPI(got.Created)
}
