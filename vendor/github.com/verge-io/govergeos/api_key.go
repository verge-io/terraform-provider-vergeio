package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// UserAPIKeyService handles user API key operations.
type UserAPIKeyService struct {
	client *Client
}

// List returns all user API keys, with optional filtering and pagination.
func (s *UserAPIKeyService) List(ctx context.Context, opts ...ListOption) ([]UserAPIKey, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = userAPIKeyListFields
	}

	params := options.toQueryParams()

	var keys []UserAPIKey
	if err := s.client.get(ctx, "/user_api_keys", params, &keys); err != nil {
		return nil, err
	}

	return keys, nil
}

// ListByUser returns all API keys for a specific user.
func (s *UserAPIKeyService) ListByUser(ctx context.Context, userID int, opts ...ListOption) ([]UserAPIKey, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("user eq %d", userID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single API key by ID.
func (s *UserAPIKeyService) Get(ctx context.Context, id int) (*UserAPIKey, error) {
	params := url.Values{}
	params.Set("fields", userAPIKeyGetFields)

	var key UserAPIKey
	endpoint := fmt.Sprintf("/user_api_keys/%d", id)
	if err := s.client.get(ctx, endpoint, params, &key); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "UserAPIKey", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &key, nil
}

// GetByName returns an API key by name within a user's keys.
func (s *UserAPIKeyService) GetByName(ctx context.Context, userID int, name string) (*UserAPIKey, error) {
	keys, err := s.List(ctx,
		WithFilter(fmt.Sprintf("user eq %d", userID)),
		WithFilter(fmt.Sprintf("name eq '%s'", name)),
	)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, &NotFoundError{Resource: "UserAPIKey", ID: name}
	}
	return s.Get(ctx, int(keys[0].Key))
}

// Create creates a new API key and returns the created key with its token.
// IMPORTANT: The token is only returned on creation and cannot be retrieved later.
func (s *UserAPIKeyService) Create(ctx context.Context, req *UserAPIKeyCreateRequest) (*UserAPIKey, string, error) {
	if req == nil {
		return nil, "", &ValidationError{Message: "create request is required"}
	}
	if req.User == 0 {
		return nil, "", &ValidationError{Field: "user", Message: "user is required"}
	}
	if req.Name == "" {
		return nil, "", &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/user_api_keys", req, &resp); err != nil {
		return nil, "", err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, "", err
	}

	// Extract token from response if available
	var token string
	if respMap, ok := resp.Response.(map[string]interface{}); ok {
		if t, ok := respMap["token"].(string); ok {
			token = t
		}
	}

	key, err := s.Get(ctx, id)
	if err != nil {
		return nil, token, err
	}

	return key, token, nil
}

// Update updates an API key and returns the updated key.
func (s *UserAPIKeyService) Update(ctx context.Context, id int, req *UserAPIKeyUpdateRequest) (*UserAPIKey, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/user_api_keys/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "UserAPIKey", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes an API key.
func (s *UserAPIKeyService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/user_api_keys/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "UserAPIKey", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// ListExpired returns all expired API keys.
func (s *UserAPIKeyService) ListExpired(ctx context.Context, opts ...ListOption) ([]UserAPIKey, error) {
	// Note: The API auto-deletes expired keys, so this may return empty
	filterOpts := []ListOption{WithFilter("expires gt 0")}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}
