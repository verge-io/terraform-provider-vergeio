package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// SnapshotProfileService handles snapshot profile operations.
type SnapshotProfileService struct {
	client *Client
}

// List returns all snapshot profiles, with optional filtering and pagination.
func (s *SnapshotProfileService) List(ctx context.Context, opts ...ListOption) ([]SnapshotProfile, error) {
	options := applyListOptions(opts)

	// Use profile-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = snapshotProfileListFields
	}

	params := options.toQueryParams()

	var profiles []SnapshotProfile
	if err := s.client.get(ctx, "/snapshot_profiles", params, &profiles); err != nil {
		return nil, err
	}

	return profiles, nil
}

// Get returns a single snapshot profile by ID.
func (s *SnapshotProfileService) Get(ctx context.Context, id int) (*SnapshotProfile, error) {
	params := url.Values{}
	params.Set("fields", snapshotProfileGetFields)

	var profile SnapshotProfile
	endpoint := fmt.Sprintf("/snapshot_profiles/%d", id)
	if err := s.client.get(ctx, endpoint, params, &profile); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SnapshotProfile", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &profile, nil
}

// GetByName returns a snapshot profile by name.
func (s *SnapshotProfileService) GetByName(ctx context.Context, name string) (*SnapshotProfile, error) {
	profiles, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}
	if len(profiles) == 0 {
		return nil, &NotFoundError{Resource: "SnapshotProfile", ID: name}
	}
	// Get full details
	return s.Get(ctx, int(profiles[0].Key))
}

// Create creates a new snapshot profile and returns the created profile.
func (s *SnapshotProfileService) Create(ctx context.Context, req *SnapshotProfileCreateRequest) (*SnapshotProfile, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/snapshot_profiles", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created profile's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created profile
	return s.Get(ctx, id)
}

// Update updates a snapshot profile and returns the updated profile.
func (s *SnapshotProfileService) Update(ctx context.Context, id int, req *SnapshotProfileUpdateRequest) (*SnapshotProfile, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/snapshot_profiles/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SnapshotProfile", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	// Read back the updated profile
	return s.Get(ctx, id)
}

// Delete deletes a snapshot profile.
func (s *SnapshotProfileService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/snapshot_profiles/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "SnapshotProfile", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// SnapshotProfilePeriodService handles snapshot profile period operations.
type SnapshotProfilePeriodService struct {
	client *Client
}

// List returns all snapshot profile periods, with optional filtering and pagination.
func (s *SnapshotProfilePeriodService) List(ctx context.Context, opts ...ListOption) ([]SnapshotProfilePeriod, error) {
	options := applyListOptions(opts)

	// Use period-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = snapshotProfilePeriodListFields
	}

	params := options.toQueryParams()

	var periods []SnapshotProfilePeriod
	if err := s.client.get(ctx, "/snapshot_profile_periods", params, &periods); err != nil {
		return nil, err
	}

	return periods, nil
}

// ListByProfile returns all periods for a specific snapshot profile.
func (s *SnapshotProfilePeriodService) ListByProfile(ctx context.Context, profileID int, opts ...ListOption) ([]SnapshotProfilePeriod, error) {
	// Prepend profile filter to any existing filters
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("profile eq %d", profileID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single snapshot profile period by ID.
func (s *SnapshotProfilePeriodService) Get(ctx context.Context, id int) (*SnapshotProfilePeriod, error) {
	params := url.Values{}
	params.Set("fields", snapshotProfilePeriodGetFields)

	var period SnapshotProfilePeriod
	endpoint := fmt.Sprintf("/snapshot_profile_periods/%d", id)
	if err := s.client.get(ctx, endpoint, params, &period); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SnapshotProfilePeriod", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &period, nil
}

// GetByName returns a snapshot profile period by name within a specific profile.
func (s *SnapshotProfilePeriodService) GetByName(ctx context.Context, profileID int, name string) (*SnapshotProfilePeriod, error) {
	periods, err := s.List(ctx, WithFilter(fmt.Sprintf("profile eq %d and name eq '%s'", profileID, name)))
	if err != nil {
		return nil, err
	}
	if len(periods) == 0 {
		return nil, &NotFoundError{Resource: "SnapshotProfilePeriod", ID: name}
	}
	// Get full details
	return s.Get(ctx, int(periods[0].Key))
}

// Create creates a new snapshot profile period and returns the created period.
func (s *SnapshotProfilePeriodService) Create(ctx context.Context, req *SnapshotProfilePeriodCreateRequest) (*SnapshotProfilePeriod, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Profile <= 0 {
		return nil, &ValidationError{Field: "profile", Message: "profile is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.Retention <= 0 {
		return nil, &ValidationError{Field: "retention", Message: "retention is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/snapshot_profile_periods", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created period's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created period
	return s.Get(ctx, id)
}

// Update updates a snapshot profile period and returns the updated period.
func (s *SnapshotProfilePeriodService) Update(ctx context.Context, id int, req *SnapshotProfilePeriodUpdateRequest) (*SnapshotProfilePeriod, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/snapshot_profile_periods/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SnapshotProfilePeriod", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	// Read back the updated period
	return s.Get(ctx, id)
}

// Delete deletes a snapshot profile period.
func (s *SnapshotProfilePeriodService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/snapshot_profile_periods/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "SnapshotProfilePeriod", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}
