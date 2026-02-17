package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

const (
	// User action constants
	userActionEnable  = "enable"
	userActionDisable = "disable"
)

// userAction represents a user action request.
type userAction struct {
	User   int         `json:"user"`
	Action string      `json:"action"`
	Params interface{} `json:"params"`
}

// UserService handles user operations.
type UserService struct {
	client *Client
}

// List returns all users, with optional filtering and pagination.
func (s *UserService) List(ctx context.Context, opts ...ListOption) ([]User, error) {
	options := applyListOptions(opts)

	// Use user-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = userListFields
	}

	params := options.toQueryParams()

	var users []User
	if err := s.client.get(ctx, "/users", params, &users); err != nil {
		return nil, err
	}

	return users, nil
}

// Get returns a single user by ID.
func (s *UserService) Get(ctx context.Context, id int) (*User, error) {
	params := url.Values{}
	params.Set("fields", userListFields)

	var user User
	endpoint := fmt.Sprintf("/users/%d", id)
	if err := s.client.get(ctx, endpoint, params, &user); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "User", ID: id}
		}
		return nil, err
	}

	return &user, nil
}

// GetByName returns a user by username.
func (s *UserService) GetByName(ctx context.Context, name string) (*User, error) {
	users, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(users) == 0 {
		return nil, &NotFoundError{Resource: "User", ID: name}
	}

	return &users[0], nil
}

// Create creates a new user and returns the created user.
func (s *UserService) Create(ctx context.Context, req *UserCreateRequest) (*User, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/users", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created user's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created user
	return s.Get(ctx, id)
}

// Update updates a user and returns the updated user.
func (s *UserService) Update(ctx context.Context, id int, req *UserUpdateRequest) (*User, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/users/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "User", ID: id}
		}
		return nil, err
	}

	// Read back the updated user
	return s.Get(ctx, id)
}

// Delete deletes a user.
func (s *UserService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/users/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}
	return nil
}

// Enable enables a user account.
func (s *UserService) Enable(ctx context.Context, id int) error {
	action := userAction{
		User:   id,
		Action: userActionEnable,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/user_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to enable user %d: %w", id, err)
	}

	return nil
}

// Disable disables a user account.
func (s *UserService) Disable(ctx context.Context, id int) error {
	action := userAction{
		User:   id,
		Action: userActionDisable,
		Params: struct{}{},
	}

	if err := s.client.post(ctx, "/user_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to disable user %d: %w", id, err)
	}

	return nil
}
