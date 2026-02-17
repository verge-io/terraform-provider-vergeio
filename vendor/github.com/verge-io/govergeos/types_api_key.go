package vergeos

// UserAPIKey represents a VergeOS user API key.
// API keys allow programmatic access to the API without using username/password.
type UserAPIKey struct {
	// Key is the unique identifier.
	Key FlexInt `json:"$key,omitempty"`
	// User is the user ID this API key belongs to.
	User FlexInt `json:"user,omitempty"`
	// UserName is the name of the user (joined field).
	UserName string `json:"user_name,omitempty"`
	// Name is the API key name.
	Name string `json:"name,omitempty"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// IPAllowList is a comma-separated list of allowed IP addresses or CIDRs.
	IPAllowList string `json:"ip_allow_list,omitempty"`
	// IPDenyList is a comma-separated list of denied IP addresses or CIDRs.
	IPDenyList string `json:"ip_deny_list,omitempty"`
	// LastLoginStamp is the timestamp of the last API key usage (Unix epoch).
	LastLoginStamp int64 `json:"lastlogin_stamp,omitempty"`
	// LastLoginIP is the IP address of the last API key usage.
	LastLoginIP string `json:"lastlogin_ip,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// ExpiresType is the expiration type: never, date.
	ExpiresType string `json:"expires_type,omitempty"`
	// Expires is the expiration timestamp (Unix epoch). 0 means never expires.
	Expires int64 `json:"expires,omitempty"`
}

// UserAPIKeyCreateRequest is the request body for creating an API key.
type UserAPIKeyCreateRequest struct {
	// User is the user ID this API key belongs to (required).
	User int `json:"user"`
	// Name is the API key name (required).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// IPAllowList is a comma-separated list of allowed IP addresses or CIDRs.
	IPAllowList string `json:"ip_allow_list,omitempty"`
	// IPDenyList is a comma-separated list of denied IP addresses or CIDRs.
	IPDenyList string `json:"ip_deny_list,omitempty"`
	// ExpiresType is the expiration type: never, date.
	ExpiresType string `json:"expires_type,omitempty"`
	// Expires is the expiration timestamp (Unix epoch).
	Expires *int64 `json:"expires,omitempty"`
}

// UserAPIKeyUpdateRequest is the request body for updating an API key.
type UserAPIKeyUpdateRequest struct {
	// Name is the API key name.
	Name *string `json:"name,omitempty"`
	// Description is the description.
	Description *string `json:"description,omitempty"`
	// IPAllowList is a comma-separated list of allowed IP addresses or CIDRs.
	IPAllowList *string `json:"ip_allow_list,omitempty"`
	// IPDenyList is a comma-separated list of denied IP addresses or CIDRs.
	IPDenyList *string `json:"ip_deny_list,omitempty"`
	// ExpiresType is the expiration type: never, date.
	ExpiresType *string `json:"expires_type,omitempty"`
	// Expires is the expiration timestamp (Unix epoch).
	Expires *int64 `json:"expires,omitempty"`
}

// UserAPIKeyCreateResponse is the response from creating an API key.
// This includes the token which is only returned on creation.
type UserAPIKeyCreateResponse struct {
	// Key is the unique identifier.
	Key FlexInt `json:"$key,omitempty"`
	// Token is the API key token. This is only returned on creation and cannot be retrieved later.
	Token string `json:"token,omitempty"`
}

// API key expiration type constants.
const (
	// APIKeyExpiresNever indicates the API key never expires.
	APIKeyExpiresNever = "never"
	// APIKeyExpiresDate indicates the API key expires at a specific date.
	APIKeyExpiresDate = "date"
)

// userAPIKeyListFields are the fields to request when listing API keys.
const userAPIKeyListFields = "$key,user,user#name as user_name,name,description,lastlogin_stamp,lastlogin_ip,created,expires,ip_allow_list,ip_deny_list,expires_type"

// userAPIKeyGetFields are the fields to request when getting a single API key.
const userAPIKeyGetFields = userAPIKeyListFields
