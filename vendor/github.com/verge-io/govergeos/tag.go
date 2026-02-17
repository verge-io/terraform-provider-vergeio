package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// TagService handles tag read operations.
// Tags are used to categorize and organize resources in VergeOS.
type TagService struct {
	client *Client
}

// List returns all tags, with optional filtering and pagination.
func (s *TagService) List(ctx context.Context, opts ...ListOption) ([]Tag, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = tagListFields
	}

	params := options.toQueryParams()

	var tags []Tag
	if err := s.client.get(ctx, "/tags", params, &tags); err != nil {
		return nil, err
	}

	return tags, nil
}

// Get returns a single tag by ID.
func (s *TagService) Get(ctx context.Context, id int) (*Tag, error) {
	params := url.Values{}
	params.Set("fields", tagListFields)

	var tag Tag
	endpoint := fmt.Sprintf("/tags/%d", id)
	if err := s.client.get(ctx, endpoint, params, &tag); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Tag", ID: id}
		}
		return nil, err
	}

	return &tag, nil
}

// GetByName returns a tag by name.
func (s *TagService) GetByName(ctx context.Context, name string) (*Tag, error) {
	tags, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(tags) == 0 {
		return nil, &NotFoundError{Resource: "Tag", ID: name}
	}

	return &tags[0], nil
}

// ListByCategory returns all tags in a specific category.
func (s *TagService) ListByCategory(ctx context.Context, categoryID int) ([]Tag, error) {
	return s.List(ctx, WithFilter(fmt.Sprintf("category eq %d", categoryID)))
}

// Create creates a new tag and returns the created tag.
func (s *TagService) Create(ctx context.Context, req *TagCreateRequest) (*Tag, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Category <= 0 {
		return nil, &ValidationError{Field: "category", Message: "category is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/tags", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created tag's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created tag
	return s.Get(ctx, id)
}

// Update updates a tag and returns the updated tag.
func (s *TagService) Update(ctx context.Context, id int, req *TagUpdateRequest) (*Tag, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/tags/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Tag", ID: id}
		}
		return nil, err
	}

	// Read back the updated tag
	return s.Get(ctx, id)
}

// Delete deletes a tag.
// Note: Deleting a tag also removes all tag member associations.
func (s *TagService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tags/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Tag", ID: id}
		}
		return err
	}
	return nil
}
