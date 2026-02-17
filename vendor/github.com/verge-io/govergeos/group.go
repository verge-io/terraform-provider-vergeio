package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// GroupService handles group read operations.
type GroupService struct {
	client *Client
}

// List returns all groups, with optional filtering and pagination.
func (s *GroupService) List(ctx context.Context, opts ...ListOption) ([]Group, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = groupListFields
	}

	params := options.toQueryParams()

	var groups []Group
	if err := s.client.get(ctx, "/groups", params, &groups); err != nil {
		return nil, err
	}

	return groups, nil
}

// Get returns a single group by ID.
func (s *GroupService) Get(ctx context.Context, id int) (*Group, error) {
	params := url.Values{}
	params.Set("fields", groupListFields)

	var group Group
	endpoint := fmt.Sprintf("/groups/%d", id)
	if err := s.client.get(ctx, endpoint, params, &group); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Group", ID: id}
		}
		return nil, err
	}

	return &group, nil
}

// GetByName returns a group by name.
func (s *GroupService) GetByName(ctx context.Context, name string) (*Group, error) {
	groups, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(groups) == 0 {
		return nil, &NotFoundError{Resource: "Group", ID: name}
	}

	return &groups[0], nil
}

// Create creates a new group and returns it.
func (s *GroupService) Create(ctx context.Context, req *GroupCreateRequest) (*Group, error) {
	var result struct {
		Key FlexInt `json:"$key"`
	}
	if err := s.client.post(ctx, "/groups", req, &result); err != nil {
		return nil, err
	}

	return s.Get(ctx, int(result.Key))
}

// Update updates an existing group and returns the updated group.
func (s *GroupService) Update(ctx context.Context, id int, req *GroupUpdateRequest) (*Group, error) {
	endpoint := fmt.Sprintf("/groups/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a group by ID.
func (s *GroupService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/groups/%d", id)
	return s.client.delete(ctx, endpoint)
}
