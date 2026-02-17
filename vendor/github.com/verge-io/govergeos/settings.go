package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// SettingsService handles system settings read operations.
type SettingsService struct {
	client *Client
}

// List returns all settings, with optional filtering and pagination.
func (s *SettingsService) List(ctx context.Context, opts ...ListOption) ([]Setting, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = settingListFields
	}

	params := options.toQueryParams()

	var settings []Setting
	if err := s.client.get(ctx, "/settings", params, &settings); err != nil {
		return nil, err
	}

	return settings, nil
}

// Get returns a single setting by ID.
func (s *SettingsService) Get(ctx context.Context, id int) (*Setting, error) {
	params := url.Values{}
	params.Set("fields", settingListFields)

	var setting Setting
	endpoint := fmt.Sprintf("/settings/%d", id)
	if err := s.client.get(ctx, endpoint, params, &setting); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Setting", ID: id}
		}
		return nil, err
	}

	return &setting, nil
}

// GetByKey returns a setting by key name.
func (s *SettingsService) GetByKey(ctx context.Context, key string) (*Setting, error) {
	settings, err := s.List(ctx, WithFilter(fmt.Sprintf("key eq '%s'", key)))
	if err != nil {
		return nil, err
	}

	if len(settings) == 0 {
		return nil, &NotFoundError{Resource: "Setting", ID: key}
	}

	return &settings[0], nil
}

// GetValue returns the value of a setting by key name.
func (s *SettingsService) GetValue(ctx context.Context, key string) (string, error) {
	setting, err := s.GetByKey(ctx, key)
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}

// GetCloudName returns the cloud_name setting value.
func (s *SettingsService) GetCloudName(ctx context.Context) (string, error) {
	return s.GetValue(ctx, "cloud_name")
}
