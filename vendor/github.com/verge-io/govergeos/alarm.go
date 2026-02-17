package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// AlarmService handles alarm operations.
type AlarmService struct {
	client *Client
}

// List returns all alarms, with optional filtering and pagination.
func (s *AlarmService) List(ctx context.Context, opts ...ListOption) ([]Alarm, error) {
	options := applyListOptions(opts)

	// Use alarm-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = alarmListFields
	}

	params := options.toQueryParams()

	var alarms []Alarm
	if err := s.client.get(ctx, "/alarms", params, &alarms); err != nil {
		return nil, err
	}

	return alarms, nil
}

// ListActive returns all active (non-snoozed) alarms.
func (s *AlarmService) ListActive(ctx context.Context, opts ...ListOption) ([]Alarm, error) {
	// Prepend active filter to any existing filters
	filterOpts := []ListOption{WithFilter("snooze eq 0 or snooze le {$now}")}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByOwner returns all alarms for a specific owner resource.
// owner should be a resource path like "vms/123" or "nodes/1".
func (s *AlarmService) ListByOwner(ctx context.Context, owner string, opts ...ListOption) ([]Alarm, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("owner eq '%s'", owner))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByLevel returns all alarms with a specific severity level.
func (s *AlarmService) ListByLevel(ctx context.Context, level string, opts ...ListOption) ([]Alarm, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("level eq '%s'", level))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByAlarmType returns all alarms of a specific alarm type.
func (s *AlarmService) ListByAlarmType(ctx context.Context, alarmTypeKey string, opts ...ListOption) ([]Alarm, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("alarm_type eq '%s'", alarmTypeKey))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single alarm by ID.
func (s *AlarmService) Get(ctx context.Context, id int) (*Alarm, error) {
	params := url.Values{}
	params.Set("fields", alarmGetFields)

	var alarm Alarm
	endpoint := fmt.Sprintf("/alarms/%d", id)
	if err := s.client.get(ctx, endpoint, params, &alarm); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Alarm", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &alarm, nil
}

// Update updates an alarm (primarily for snoozing).
func (s *AlarmService) Update(ctx context.Context, id int, req *AlarmUpdateRequest) (*Alarm, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/alarms/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Alarm", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	// Read back the updated alarm
	return s.Get(ctx, id)
}

// Snooze snoozes an alarm until the specified timestamp.
func (s *AlarmService) Snooze(ctx context.Context, id int, until int64) error {
	_, err := s.Update(ctx, id, &AlarmUpdateRequest{Snooze: &until})
	return err
}

// Unsnooze removes the snooze from an alarm.
func (s *AlarmService) Unsnooze(ctx context.Context, id int) error {
	zero := int64(0)
	_, err := s.Update(ctx, id, &AlarmUpdateRequest{Snooze: &zero})
	return err
}

// Resolve resolves an alarm if it is resolvable.
func (s *AlarmService) Resolve(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/alarm_actions")
	body := map[string]interface{}{
		"alarm":  id,
		"action": "resolve",
	}
	return s.client.post(ctx, endpoint, body, nil)
}

// Delete deletes an alarm.
func (s *AlarmService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/alarms/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Alarm", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// AlarmTypeService handles alarm type operations.
// Alarm types are read-only reference data.
type AlarmTypeService struct {
	client *Client
}

// List returns all alarm types, with optional filtering and pagination.
func (s *AlarmTypeService) List(ctx context.Context, opts ...ListOption) ([]AlarmType, error) {
	options := applyListOptions(opts)

	// Use alarm type-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = alarmTypeListFields
	}

	params := options.toQueryParams()

	var types []AlarmType
	if err := s.client.get(ctx, "/alarm_types", params, &types); err != nil {
		return nil, err
	}

	return types, nil
}

// Get returns a single alarm type by key.
// Note: Alarm types use string keys, not integer IDs.
func (s *AlarmTypeService) Get(ctx context.Context, key string) (*AlarmType, error) {
	params := url.Values{}
	params.Set("fields", alarmTypeGetFields)

	var alarmType AlarmType
	endpoint := fmt.Sprintf("/alarm_types/%s", key)
	if err := s.client.get(ctx, endpoint, params, &alarmType); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "AlarmType", ID: key}
		}
		return nil, err
	}

	return &alarmType, nil
}

// ListByLevel returns all alarm types with a specific default severity level.
func (s *AlarmTypeService) ListByLevel(ctx context.Context, level string, opts ...ListOption) ([]AlarmType, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("level eq '%s'", level))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}
