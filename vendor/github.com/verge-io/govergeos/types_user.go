package vergeos

import "strings"

// User represents a VergeOS user.
type User struct {
	// Key is the row key (unique identifier).
	Key FlexInt `json:"$key,omitempty"`
	// ID is the 40-character unique user identifier string.
	ID string `json:"id,omitempty"`
	// AuthSource is the authentication source ID.
	AuthSource int `json:"auth_source,omitempty"`
	// Name is the username.
	Name string `json:"name"`
	// RemoteName is the remote username (for external auth).
	RemoteName string `json:"remote_name,omitempty"`
	// Enabled indicates whether the user is enabled.
	Enabled bool `json:"enabled"`
	// DisplayName is the user's display name.
	DisplayName string `json:"displayname,omitempty"`
	// Email is the user's email address.
	Email string `json:"email,omitempty"`
	// Type is the user type (normal, api, vdi, site_sync, site_user).
	Type string `json:"type,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Creator is the username that created this user.
	Creator string `json:"creator,omitempty"`

	// Password and authentication
	// ChangePassword indicates whether the user must change their password.
	ChangePassword bool `json:"change_password,omitempty"`
	// FailedAttempts is the number of failed login attempts.
	FailedAttempts int `json:"failed_attempts,omitempty"`
	// AccountLocked is the timestamp when the account was locked (0 = not locked).
	AccountLocked int64 `json:"account_locked,omitempty"`
	// LastLogin is the timestamp of the last successful login.
	LastLogin int64 `json:"last_login,omitempty"`

	// Two-factor authentication
	// TwoFactorAuthentication indicates whether 2FA is enabled.
	TwoFactorAuthentication bool `json:"two_factor_authentication,omitempty"`
	// TwoFactorType is the 2FA method (email, authenticator).
	TwoFactorType string `json:"two_factor_type,omitempty"`

	// Advanced access
	// PhysicalAccess indicates whether console/SSH access is enabled.
	PhysicalAccess bool `json:"physical_access,omitempty"`
	// SSHKeys contains the user's SSH public keys (newline separated).
	SSHKeys string `json:"ssh_keys,omitempty"`

	// UI customization
	// Theme is the theme ID override (string hash).
	Theme string `json:"theme,omitempty"`
}

// UserCreateRequest is the request body for creating a user.
type UserCreateRequest struct {
	// AuthSource is the authentication source ID.
	AuthSource *int `json:"auth_source,omitempty"`
	// Name is the username (required).
	Name string `json:"name"`
	// RemoteName is the remote username (for external auth).
	RemoteName string `json:"remote_name,omitempty"`
	// Enabled indicates whether the user is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// DisplayName is the user's display name.
	DisplayName string `json:"displayname,omitempty"`
	// Email is the user's email address.
	Email string `json:"email,omitempty"`
	// Type is the user type (normal, api, vdi).
	Type string `json:"type,omitempty"`

	// Password is the user's password.
	Password string `json:"password,omitempty"`
	// ChangePassword indicates whether the user must change their password.
	ChangePassword *bool `json:"change_password,omitempty"`

	// Two-factor authentication
	// TwoFactorAuthentication indicates whether 2FA is enabled.
	TwoFactorAuthentication *bool `json:"two_factor_authentication,omitempty"`
	// TwoFactorType is the 2FA method (email, authenticator).
	TwoFactorType string `json:"two_factor_type,omitempty"`
	// TwoFactorSetupNextLogin requires 2FA setup on next login.
	TwoFactorSetupNextLogin *bool `json:"two_factor_setup_next_login,omitempty"`

	// Advanced access
	// PhysicalAccess indicates whether console/SSH access is enabled.
	PhysicalAccess *bool `json:"physical_access,omitempty"`
	// SSHKeys contains the user's SSH public keys.
	SSHKeys string `json:"ssh_keys,omitempty"`

	// UI customization
	// Theme is the theme ID override (string hash).
	Theme string `json:"theme,omitempty"`
}

// UserUpdateRequest is the request body for updating a user.
// Note: auth_source and type are readonly after creation.
type UserUpdateRequest struct {
	// Name is the username.
	Name *string `json:"name,omitempty"`
	// RemoteName is the remote username.
	RemoteName *string `json:"remote_name,omitempty"`
	// Enabled indicates whether the user is enabled.
	Enabled *bool `json:"enabled,omitempty"`
	// DisplayName is the user's display name.
	DisplayName *string `json:"displayname,omitempty"`
	// Email is the user's email address.
	Email *string `json:"email,omitempty"`

	// Password management
	// Password is the user's password (argument field).
	Password *string `json:"password,omitempty"`
	// ChangePassword indicates whether the user must change their password.
	ChangePassword *bool `json:"change_password,omitempty"`

	// Two-factor authentication
	// TwoFactorAuthentication indicates whether 2FA is enabled.
	TwoFactorAuthentication *bool `json:"two_factor_authentication,omitempty"`
	// TwoFactorType is the 2FA method (email, authenticator).
	TwoFactorType *string `json:"two_factor_type,omitempty"`
	// TwoFactorSetupNextLogin requires 2FA setup on next login.
	TwoFactorSetupNextLogin *bool `json:"two_factor_setup_next_login,omitempty"`

	// Advanced access
	// PhysicalAccess indicates whether console/SSH access is enabled.
	PhysicalAccess *bool `json:"physical_access,omitempty"`
	// SSHKeys contains the user's SSH public keys.
	SSHKeys *string `json:"ssh_keys,omitempty"`

	// UI customization
	// Theme is the theme ID override (string hash).
	Theme *string `json:"theme,omitempty"`
}

// userListFields are the fields to request when listing users.
const userListFields = "$key,id,auth_source,name,remote_name,enabled,displayname,email,type,created,creator,change_password,failed_attempts,account_locked,last_login,two_factor_authentication,two_factor_type,physical_access,ssh_keys,theme"

// GetSSHKeys returns the SSH keys as a slice of public key strings.
// SSH keys are stored as newline-separated values.
func (u *User) GetSSHKeys() []string {
	if u.SSHKeys == "" {
		return nil
	}
	var keys []string
	for _, line := range strings.Split(u.SSHKeys, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			keys = append(keys, line)
		}
	}
	return keys
}
