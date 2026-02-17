package vergeos

import (
	"errors"
	"fmt"
)

// APIError represents an error returned by the VergeOS API.
type APIError struct {
	// StatusCode is the HTTP status code returned by the API.
	StatusCode int
	// Endpoint is the API endpoint that was called.
	Endpoint string
	// Message is the error message from the API.
	Message string
}

// Error implements the error interface.
func (e *APIError) Error() string {
	return fmt.Sprintf("vergeos: API error %d at %s: %s", e.StatusCode, e.Endpoint, e.Message)
}

// NotFoundError is returned when a resource is not found.
type NotFoundError struct {
	Resource string
	ID       interface{}
}

// Error implements the error interface.
func (e *NotFoundError) Error() string {
	return fmt.Sprintf("vergeos: %s with ID %v not found", e.Resource, e.ID)
}

// AuthError is returned when authentication fails.
type AuthError struct {
	Message string
}

// Error implements the error interface.
func (e *AuthError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("vergeos: authentication failed: %s", e.Message)
	}
	return "vergeos: authentication failed"
}

// ValidationError is returned when request validation fails.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("vergeos: validation error on field %s: %s", e.Field, e.Message)
	}
	return fmt.Sprintf("vergeos: validation error: %s", e.Message)
}

// IsNotFoundError returns true if the error is a NotFoundError or a 404 API error.
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}

// IsAuthError returns true if the error is an AuthError or a 401/403 API error.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}
	var authErr *AuthError
	if errors.As(err, &authErr) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
	}
	return false
}

// IsValidationError returns true if the error is a ValidationError or a 400 API error.
func IsValidationError(err error) bool {
	if err == nil {
		return false
	}
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 400
	}
	return false
}
