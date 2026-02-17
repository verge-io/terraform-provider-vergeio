package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// TagCategoryService handles tag category operations.
// Tag categories organize tags and define which resource types can be tagged.
type TagCategoryService struct {
	client *Client
}

// List returns all tag categories, with optional filtering and pagination.
func (s *TagCategoryService) List(ctx context.Context, opts ...ListOption) ([]TagCategory, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = tagCategoryListFields
	}

	params := options.toQueryParams()

	var categories []TagCategory
	if err := s.client.get(ctx, "/tag_categories", params, &categories); err != nil {
		return nil, err
	}

	return categories, nil
}

// Get returns a single tag category by ID.
func (s *TagCategoryService) Get(ctx context.Context, id int) (*TagCategory, error) {
	params := url.Values{}
	params.Set("fields", tagCategoryGetFields)

	var category TagCategory
	endpoint := fmt.Sprintf("/tag_categories/%d", id)
	if err := s.client.get(ctx, endpoint, params, &category); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TagCategory", ID: id}
		}
		return nil, err
	}

	return &category, nil
}

// GetByName returns a tag category by name.
func (s *TagCategoryService) GetByName(ctx context.Context, name string) (*TagCategory, error) {
	categories, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(categories) == 0 {
		return nil, &NotFoundError{Resource: "TagCategory", ID: name}
	}

	return &categories[0], nil
}

// Create creates a new tag category and returns the created category.
func (s *TagCategoryService) Create(ctx context.Context, req *TagCategoryCreateRequest) (*TagCategory, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/tag_categories", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created category's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created category
	return s.Get(ctx, id)
}

// Update updates a tag category and returns the updated category.
func (s *TagCategoryService) Update(ctx context.Context, id int, req *TagCategoryUpdateRequest) (*TagCategory, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/tag_categories/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TagCategory", ID: id}
		}
		return nil, err
	}

	// Read back the updated category
	return s.Get(ctx, id)
}

// Delete deletes a tag category.
// Note: Deleting a category also deletes all tags within it.
func (s *TagCategoryService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tag_categories/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "TagCategory", ID: id}
		}
		return err
	}
	return nil
}
