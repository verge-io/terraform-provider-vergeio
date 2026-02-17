package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// WebhookURLService handles webhook URL configuration operations.
type WebhookURLService struct {
	client *Client
}

// List returns all webhook URLs, with optional filtering and pagination.
func (s *WebhookURLService) List(ctx context.Context, opts ...ListOption) ([]WebhookURL, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = webhookURLListFields
	}

	params := options.toQueryParams()

	var webhooks []WebhookURL
	if err := s.client.get(ctx, "/webhook_urls", params, &webhooks); err != nil {
		return nil, err
	}

	return webhooks, nil
}

// Get returns a single webhook URL by ID.
func (s *WebhookURLService) Get(ctx context.Context, id int) (*WebhookURL, error) {
	params := url.Values{}
	params.Set("fields", webhookURLGetFields)

	var webhook WebhookURL
	endpoint := fmt.Sprintf("/webhook_urls/%d", id)
	if err := s.client.get(ctx, endpoint, params, &webhook); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "WebhookURL", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &webhook, nil
}

// GetByName returns a webhook URL by name.
func (s *WebhookURLService) GetByName(ctx context.Context, name string) (*WebhookURL, error) {
	webhooks, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}
	if len(webhooks) == 0 {
		return nil, &NotFoundError{Resource: "WebhookURL", ID: name}
	}
	return s.Get(ctx, int(webhooks[0].Key))
}

// Create creates a new webhook URL and returns the created webhook URL.
func (s *WebhookURLService) Create(ctx context.Context, req *WebhookURLCreateRequest) (*WebhookURL, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.URL == "" {
		return nil, &ValidationError{Field: "url", Message: "url is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/webhook_urls", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a webhook URL and returns the updated webhook URL.
func (s *WebhookURLService) Update(ctx context.Context, id int, req *WebhookURLUpdateRequest) (*WebhookURL, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/webhook_urls/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "WebhookURL", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a webhook URL.
func (s *WebhookURLService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/webhook_urls/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "WebhookURL", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// webhookURLAction represents a webhook URL action request.
type webhookURLAction struct {
	WebhookURL int    `json:"webhook_url"`
	Action     string `json:"action"`
	Message    string `json:"message,omitempty"`
}

// Send sends a test message to the webhook URL.
func (s *WebhookURLService) Send(ctx context.Context, id int, message string) error {
	action := webhookURLAction{
		WebhookURL: id,
		Action:     "send",
		Message:    message,
	}

	if err := s.client.post(ctx, "/webhook_url_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to send test webhook %d: %w", id, err)
	}
	return nil
}

// WebhookService handles webhook message log operations.
// Webhooks are the actual messages sent/queued for delivery.
type WebhookService struct {
	client *Client
}

// List returns all webhook messages, with optional filtering and pagination.
func (s *WebhookService) List(ctx context.Context, opts ...ListOption) ([]Webhook, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = webhookListFields
	}

	params := options.toQueryParams()

	var webhooks []Webhook
	if err := s.client.get(ctx, "/webhooks", params, &webhooks); err != nil {
		return nil, err
	}

	return webhooks, nil
}

// ListByWebhookURL returns all webhook messages for a specific webhook URL.
func (s *WebhookService) ListByWebhookURL(ctx context.Context, webhookURLID int, opts ...ListOption) ([]Webhook, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("webhook_url eq %d", webhookURLID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByStatus returns all webhook messages with a specific status.
func (s *WebhookService) ListByStatus(ctx context.Context, status string, opts ...ListOption) ([]Webhook, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("status eq '%s'", status))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListPending returns all webhook messages that are queued or running.
func (s *WebhookService) ListPending(ctx context.Context, opts ...ListOption) ([]Webhook, error) {
	filterOpts := []ListOption{WithFilter("status eq 'queued' or status eq 'running'")}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListFailed returns all webhook messages that failed to deliver.
func (s *WebhookService) ListFailed(ctx context.Context, opts ...ListOption) ([]Webhook, error) {
	return s.ListByStatus(ctx, WebhookStatusError, opts...)
}

// Get returns a single webhook message by ID.
func (s *WebhookService) Get(ctx context.Context, id int) (*Webhook, error) {
	params := url.Values{}
	params.Set("fields", webhookGetFields)

	var webhook Webhook
	endpoint := fmt.Sprintf("/webhooks/%d", id)
	if err := s.client.get(ctx, endpoint, params, &webhook); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Webhook", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &webhook, nil
}

// Delete deletes a webhook message from the queue/log.
func (s *WebhookService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/webhooks/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Webhook", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}
