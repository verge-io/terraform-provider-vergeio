package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// LogService handles system log operations.
// Logs are read-only and provide audit, error, and operational information.
type LogService struct {
	client *Client
}

// List returns system logs, with optional filtering and pagination.
// Note: Logs are sorted by timestamp descending by default (newest first).
func (s *LogService) List(ctx context.Context, opts ...ListOption) ([]Log, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = logListFields
	}
	// Default sort by timestamp descending (newest first)
	if options.Sort == "" {
		options.Sort = "-timestamp"
	}

	params := options.toQueryParams()

	var logs []Log
	if err := s.client.get(ctx, "/logs", params, &logs); err != nil {
		return nil, err
	}

	return logs, nil
}

// ListByLevel returns logs filtered by level.
func (s *LogService) ListByLevel(ctx context.Context, level string, opts ...ListOption) ([]Log, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("level eq '%s'", level)))
	return s.List(ctx, opts...)
}

// ListByObjectType returns logs filtered by object type.
func (s *LogService) ListByObjectType(ctx context.Context, objectType string, opts ...ListOption) ([]Log, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("object_type eq '%s'", objectType)))
	return s.List(ctx, opts...)
}

// ListErrors returns logs with error or critical level.
func (s *LogService) ListErrors(ctx context.Context, opts ...ListOption) ([]Log, error) {
	opts = append(opts, WithFilter("(level eq 'error') or (level eq 'critical')"))
	return s.List(ctx, opts...)
}

// ListAudit returns audit logs.
func (s *LogService) ListAudit(ctx context.Context, opts ...ListOption) ([]Log, error) {
	return s.ListByLevel(ctx, LogLevelAudit, opts...)
}

// ListWarnings returns warning logs.
func (s *LogService) ListWarnings(ctx context.Context, opts ...ListOption) ([]Log, error) {
	return s.ListByLevel(ctx, LogLevelWarning, opts...)
}

// ListByUser returns logs filtered by username.
func (s *LogService) ListByUser(ctx context.Context, username string, opts ...ListOption) ([]Log, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("user eq '%s'", username)))
	return s.List(ctx, opts...)
}

// ListSince returns logs since the specified timestamp (microseconds since epoch).
func (s *LogService) ListSince(ctx context.Context, timestampMicros int64, opts ...ListOption) ([]Log, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("timestamp ge %d", timestampMicros)))
	return s.List(ctx, opts...)
}

// Get returns a single log entry by ID (row number).
func (s *LogService) Get(ctx context.Context, id int) (*Log, error) {
	params := url.Values{}
	params.Set("fields", logGetFields)

	var log Log
	endpoint := fmt.Sprintf("/logs/%d", id)
	if err := s.client.get(ctx, endpoint, params, &log); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Log", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &log, nil
}

// GetRecent returns the most recent n log entries.
func (s *LogService) GetRecent(ctx context.Context, count int) ([]Log, error) {
	return s.List(ctx, WithLimit(count))
}

// GetRecentErrors returns the most recent n error/critical logs.
func (s *LogService) GetRecentErrors(ctx context.Context, count int) ([]Log, error) {
	return s.ListErrors(ctx, WithLimit(count))
}

// Search searches logs by text pattern.
// Uses the API's text contains filter.
func (s *LogService) Search(ctx context.Context, pattern string, opts ...ListOption) ([]Log, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("text ct '%s'", pattern)))
	return s.List(ctx, opts...)
}
