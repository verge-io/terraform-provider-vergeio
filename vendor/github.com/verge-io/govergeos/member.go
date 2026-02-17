package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// MemberService handles group membership operations.
type MemberService struct {
	client *Client
}

// List returns all members, with optional filtering and pagination.
func (s *MemberService) List(ctx context.Context, opts ...ListOption) ([]Member, error) {
	options := applyListOptions(opts)

	// Use member-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = memberListFields
	}

	params := options.toQueryParams()

	var members []Member
	if err := s.client.get(ctx, "/members", params, &members); err != nil {
		return nil, err
	}

	return members, nil
}

// ListByGroup returns all members of a specific group.
func (s *MemberService) ListByGroup(ctx context.Context, groupID int) ([]Member, error) {
	return s.List(ctx, WithFilter(fmt.Sprintf("parent_group eq %d", groupID)))
}

// Get returns a single member by ID.
func (s *MemberService) Get(ctx context.Context, id int) (*Member, error) {
	params := url.Values{}
	params.Set("fields", memberListFields)

	var member Member
	endpoint := fmt.Sprintf("/members/%d", id)
	if err := s.client.get(ctx, endpoint, params, &member); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Member", ID: id}
		}
		return nil, err
	}

	return &member, nil
}

// Create creates a new membership and returns the created member.
func (s *MemberService) Create(ctx context.Context, req *MemberCreateRequest) (*Member, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Group <= 0 {
		return nil, &ValidationError{Field: "parent_group", Message: "parent_group is required"}
	}
	if req.Member == "" {
		return nil, &ValidationError{Field: "member", Message: "member is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/members", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created member's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created member
	return s.Get(ctx, id)
}

// Update updates a membership and returns the updated member.
func (s *MemberService) Update(ctx context.Context, id int, req *MemberUpdateRequest) (*Member, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/members/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Member", ID: id}
		}
		return nil, err
	}

	// Read back the updated member
	return s.Get(ctx, id)
}

// Delete deletes a membership.
func (s *MemberService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/members/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}
	return nil
}

// Add is a convenience method to add a member to a group.
func (s *MemberService) Add(ctx context.Context, groupID int, member string) (*Member, error) {
	return s.Create(ctx, &MemberCreateRequest{
		Group:  groupID,
		Member: member,
	})
}

// Remove is a convenience method to remove a member from a group.
func (s *MemberService) Remove(ctx context.Context, groupID int, member string) error {
	// Find the membership
	members, err := s.List(ctx,
		WithFilter(fmt.Sprintf("parent_group eq %d and member eq '%s'", groupID, member)),
	)
	if err != nil {
		return err
	}

	if len(members) == 0 {
		return nil // Already removed
	}

	return s.Delete(ctx, members[0].ID.Int())
}
