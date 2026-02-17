package vergeos

// WebhookURL represents a VergeOS webhook endpoint configuration.
// This is where you configure where webhooks are sent.
type WebhookURL struct {
	// Key is the unique identifier.
	Key FlexInt `json:"$key,omitempty"`
	// Name is the webhook name (unique).
	Name string `json:"name,omitempty"`
	// Type is the webhook type (default: custom).
	Type string `json:"type,omitempty"`
	// URL is the destination URL for webhook payloads.
	URL string `json:"url,omitempty"`
	// Headers are custom HTTP headers in "Header:Value" format, newline-separated.
	Headers string `json:"headers,omitempty"`
	// AuthorizationType is the authorization method: none, basic, bearer, apikey.
	AuthorizationType string `json:"authorization_type,omitempty"`
	// AuthorizationValue is the authorization credential (hidden in responses).
	AuthorizationValue string `json:"authorization_value,omitempty"`
	// AllowInsecure allows connections to servers with invalid SSL certificates.
	AllowInsecure bool `json:"allow_insecure,omitempty"`
	// Timeout is the request timeout in seconds (default: 5, min: 3, max: 120).
	Timeout int `json:"timeout,omitempty"`
	// Retries is the number of retry attempts (default: 3, min: 0, max: 100).
	Retries int `json:"retries,omitempty"`
}

// WebhookURLCreateRequest is the request body for creating a webhook URL.
type WebhookURLCreateRequest struct {
	// Name is the webhook name (required, unique).
	Name string `json:"name"`
	// URL is the destination URL (required).
	URL string `json:"url"`
	// Type is the webhook type (optional, default: custom).
	Type string `json:"type,omitempty"`
	// Headers are custom HTTP headers in "Header:Value" format, newline-separated.
	Headers string `json:"headers,omitempty"`
	// AuthorizationType is the authorization method: none, basic, bearer, apikey.
	AuthorizationType string `json:"authorization_type,omitempty"`
	// AuthorizationValue is the authorization credential.
	AuthorizationValue string `json:"authorization_value,omitempty"`
	// AllowInsecure allows connections to servers with invalid SSL certificates.
	AllowInsecure *bool `json:"allow_insecure,omitempty"`
	// Timeout is the request timeout in seconds (default: 5).
	Timeout *int `json:"timeout,omitempty"`
	// Retries is the number of retry attempts (default: 3).
	Retries *int `json:"retries,omitempty"`
}

// WebhookURLUpdateRequest is the request body for updating a webhook URL.
type WebhookURLUpdateRequest struct {
	// Name is the webhook name.
	Name *string `json:"name,omitempty"`
	// URL is the destination URL.
	URL *string `json:"url,omitempty"`
	// Type is the webhook type.
	Type *string `json:"type,omitempty"`
	// Headers are custom HTTP headers.
	Headers *string `json:"headers,omitempty"`
	// AuthorizationType is the authorization method.
	AuthorizationType *string `json:"authorization_type,omitempty"`
	// AuthorizationValue is the authorization credential.
	AuthorizationValue *string `json:"authorization_value,omitempty"`
	// AllowInsecure allows connections to servers with invalid SSL certificates.
	AllowInsecure *bool `json:"allow_insecure,omitempty"`
	// Timeout is the request timeout in seconds.
	Timeout *int `json:"timeout,omitempty"`
	// Retries is the number of retry attempts.
	Retries *int `json:"retries,omitempty"`
}

// WebhookSendRequest is the request body for sending a test webhook.
type WebhookSendRequest struct {
	// Message is the JSON payload to send.
	Message string `json:"message"`
}

// Webhook represents a webhook message in the delivery queue/log.
// This is a read-only view of webhook messages that have been queued or sent.
type Webhook struct {
	// Key is the unique identifier.
	Key FlexInt `json:"$key,omitempty"`
	// WebhookURL is the webhook URL this message was sent to.
	WebhookURL FlexInt `json:"webhook_url,omitempty"`
	// Message is the JSON payload that was sent.
	Message string `json:"message,omitempty"`
	// Status is the delivery status: queued, running, sent, error.
	Status string `json:"status,omitempty"`
	// StatusInfo contains additional status details or error messages.
	StatusInfo string `json:"status_info,omitempty"`
	// LastAttempt is the timestamp of the last delivery attempt (Unix epoch).
	LastAttempt int64 `json:"last_attempt,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
}

// Webhook status constants.
const (
	// WebhookStatusQueued indicates the webhook is queued for delivery.
	WebhookStatusQueued = "queued"
	// WebhookStatusRunning indicates the webhook is currently being delivered.
	WebhookStatusRunning = "running"
	// WebhookStatusSent indicates the webhook was successfully delivered.
	WebhookStatusSent = "sent"
	// WebhookStatusError indicates the webhook delivery failed.
	WebhookStatusError = "error"
)

// WebhookURL authorization type constants.
const (
	// WebhookAuthNone indicates no authorization.
	WebhookAuthNone = "none"
	// WebhookAuthBasic indicates HTTP Basic authentication.
	WebhookAuthBasic = "basic"
	// WebhookAuthBearer indicates Bearer token authentication.
	WebhookAuthBearer = "bearer"
	// WebhookAuthAPIKey indicates API key authentication.
	WebhookAuthAPIKey = "apikey"
)

// webhookURLListFields are the fields to request when listing webhook URLs.
const webhookURLListFields = "$key,name,type,url,headers,authorization_type,allow_insecure,timeout,retries"

// webhookURLGetFields are the fields to request when getting a single webhook URL.
const webhookURLGetFields = webhookURLListFields

// webhookListFields are the fields to request when listing webhook messages.
const webhookListFields = "$key,webhook_url,message,status,status_info,last_attempt,created"

// webhookGetFields are the fields to request when getting a single webhook message.
const webhookGetFields = webhookListFields
