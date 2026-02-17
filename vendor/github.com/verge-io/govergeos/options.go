package vergeos

import (
	"fmt"
	"net/url"
)

// ListOptions contains options for list operations.
type ListOptions struct {
	// Filter is a filter expression (e.g., "name eq 'my-vm'").
	Filter string
	// Fields specifies which fields to return (e.g., "most", "all", "name,enabled").
	Fields string
	// Sort specifies the sort order (e.g., "name", "-created" for descending).
	Sort string
	// Limit is the maximum number of results to return.
	Limit int
	// Offset is the number of results to skip.
	Offset int
}

// ListOption is a function that modifies ListOptions.
type ListOption func(*ListOptions)

// WithFilter sets the filter expression for a list operation.
//
// Example filters:
//   - "name eq 'my-vm'"
//   - "enabled eq true"
//   - "name eq 'vm1' and enabled eq true"
func WithFilter(filter string) ListOption {
	return func(opts *ListOptions) {
		opts.Filter = filter
	}
}

// WithFields sets which fields to return in a list operation.
//
// Common values:
//   - "most" - returns most common fields (default)
//   - "all" - returns all fields
//   - "name,enabled,ram" - returns specific fields
func WithFields(fields string) ListOption {
	return func(opts *ListOptions) {
		opts.Fields = fields
	}
}

// WithSort sets the sort order for a list operation.
//
// Examples:
//   - "name" - sort by name ascending
//   - "-created" - sort by created descending
func WithSort(sort string) ListOption {
	return func(opts *ListOptions) {
		opts.Sort = sort
	}
}

// WithLimit sets the maximum number of results to return.
func WithLimit(limit int) ListOption {
	return func(opts *ListOptions) {
		opts.Limit = limit
	}
}

// WithOffset sets the number of results to skip (for pagination).
func WithOffset(offset int) ListOption {
	return func(opts *ListOptions) {
		opts.Offset = offset
	}
}

// applyListOptions applies the given options to a ListOptions struct.
func applyListOptions(opts []ListOption) *ListOptions {
	options := &ListOptions{
		Fields: "most", // Default field selection
	}
	for _, opt := range opts {
		opt(options)
	}
	return options
}

// toQueryParams converts ListOptions to URL query parameters.
func (opts *ListOptions) toQueryParams() url.Values {
	params := url.Values{}

	if opts.Fields != "" {
		params.Set("fields", opts.Fields)
	}
	if opts.Filter != "" {
		params.Set("filter", opts.Filter)
	}
	if opts.Sort != "" {
		params.Set("sort", opts.Sort)
	}
	if opts.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	if opts.Offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", opts.Offset))
	}

	return params
}
