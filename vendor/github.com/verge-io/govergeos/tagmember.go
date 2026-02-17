package vergeos

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// TagMemberService handles tag member (tag assignment) operations.
// TagMembers link tags to objects like VMs, networks, etc.
type TagMemberService struct {
	client *Client
}

// List returns all tag members, with optional filtering and pagination.
func (s *TagMemberService) List(ctx context.Context, opts ...ListOption) ([]TagMember, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = tagMemberListFields
	}

	params := options.toQueryParams()

	var members []TagMember
	if err := s.client.get(ctx, "/tag_members", params, &members); err != nil {
		return nil, err
	}

	return members, nil
}

// ListByTag returns all tag members for a specific tag.
func (s *TagMemberService) ListByTag(ctx context.Context, tagID int) ([]TagMember, error) {
	return s.List(ctx, WithFilter(fmt.Sprintf("tag eq %d", tagID)))
}

// ListByMember returns all tag members for a specific object.
// member should be in format "object_type/object_id" (e.g., "vms/123").
func (s *TagMemberService) ListByMember(ctx context.Context, member string) ([]TagMember, error) {
	return s.List(ctx, WithFilter(fmt.Sprintf("member eq '%s'", member)))
}

// Get returns a single tag member by ID.
func (s *TagMemberService) Get(ctx context.Context, id int) (*TagMember, error) {
	params := url.Values{}
	params.Set("fields", tagMemberListFields)

	var member TagMember
	endpoint := fmt.Sprintf("/tag_members/%d", id)
	if err := s.client.get(ctx, endpoint, params, &member); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TagMember", ID: id}
		}
		return nil, err
	}

	return &member, nil
}

// Create creates a new tag member and returns the created tag member.
func (s *TagMemberService) Create(ctx context.Context, req *TagMemberCreateRequest) (*TagMember, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Tag <= 0 {
		return nil, &ValidationError{Field: "tag", Message: "tag is required"}
	}
	if req.Member == "" {
		return nil, &ValidationError{Field: "member", Message: "member is required"}
	}
	if !strings.Contains(req.Member, "/") {
		return nil, &ValidationError{Field: "member", Message: "member must be in format 'object_type/object_id' (e.g., 'vms/123')"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/tag_members", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created tag member's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created tag member
	return s.Get(ctx, id)
}

// Update updates a tag member and returns the updated tag member.
func (s *TagMemberService) Update(ctx context.Context, id int, req *TagMemberUpdateRequest) (*TagMember, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}
	if req.Member != nil && !strings.Contains(*req.Member, "/") {
		return nil, &ValidationError{Field: "member", Message: "member must be in format 'object_type/object_id' (e.g., 'vms/123')"}
	}

	endpoint := fmt.Sprintf("/tag_members/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "TagMember", ID: id}
		}
		return nil, err
	}

	// Read back the updated tag member
	return s.Get(ctx, id)
}

// Delete deletes a tag member.
func (s *TagMemberService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tag_members/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}
	return nil
}

// Assign is a convenience method to assign a tag to an object.
// member should be in format "object_type/object_id" (e.g., "vms/123").
func (s *TagMemberService) Assign(ctx context.Context, tagID int, member string) (*TagMember, error) {
	return s.Create(ctx, &TagMemberCreateRequest{
		Tag:    tagID,
		Member: member,
	})
}

// Unassign is a convenience method to remove a tag from an object.
// member should be in format "object_type/object_id" (e.g., "vms/123").
func (s *TagMemberService) Unassign(ctx context.Context, tagID int, member string) error {
	// Find the tag member
	members, err := s.List(ctx,
		WithFilter(fmt.Sprintf("tag eq %d and member eq '%s'", tagID, member)),
	)
	if err != nil {
		return err
	}

	if len(members) == 0 {
		return nil // Already unassigned
	}

	return s.Delete(ctx, members[0].Key.Int())
}
