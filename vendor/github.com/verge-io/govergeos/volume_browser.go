package vergeos

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// VolumeBrowserService handles volume file browsing operations.
// The volume browser API is asynchronous: create a job, then poll for results.
// Note: The NAS service VM must be running to browse volumes.
type VolumeBrowserService struct {
	client *Client
}

// Browse browses a directory in a volume and returns the entries.
// This is a convenience method that handles the async job creation and polling.
// Use "" for dir to browse the root directory, NOT "/".
func (s *VolumeBrowserService) Browse(ctx context.Context, volumeID, dir string, limit int) ([]VolumeBrowserEntry, error) {
	return s.BrowseWithOptions(ctx, volumeID, dir, limit, nil, "")
}

// BrowseWithOptions browses a directory with additional options.
func (s *VolumeBrowserService) BrowseWithOptions(ctx context.Context, volumeID, dir string, limit int, offset *int, extensions string) ([]VolumeBrowserEntry, error) {
	// Create the browse job
	job, err := s.CreateJob(ctx, &VolumeBrowserRequest{
		Volume: volumeID,
		Query:  VolumeBrowserQueryGetDir,
		Params: &VolumeBrowserParams{
			Dir:    dir,
			Limit:  limit,
			Offset: offset,
			Filter: &VolumeBrowserFilter{
				Extensions: extensions,
			},
			Volume: volumeID,
			Sort:   "",
		},
	})
	if err != nil {
		return nil, err
	}

	// Poll for results
	result, err := s.WaitForResult(ctx, job.ID, 30*time.Second)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// CreateJob creates a new browse job.
// Use GetJob or WaitForResult to poll for the job result.
func (s *VolumeBrowserService) CreateJob(ctx context.Context, req *VolumeBrowserRequest) (*VolumeBrowserJob, error) {
	if req == nil {
		return nil, &ValidationError{Message: "request is required"}
	}
	if req.Volume == "" {
		return nil, &ValidationError{Field: "volume", Message: "volume is required"}
	}
	if req.Query == "" {
		return nil, &ValidationError{Field: "query", Message: "query is required"}
	}
	if req.Params == nil {
		return nil, &ValidationError{Field: "params", Message: "params is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/volume_browser", req, &resp); err != nil {
		return nil, err
	}

	// Extract the job ID
	id, err := getStringKey(resp)
	if err != nil {
		return nil, err
	}

	return &VolumeBrowserJob{
		ID:     id,
		Key:    id,
		Volume: req.Volume,
		Query:  req.Query,
		Status: VolumeBrowserStatusRunning,
	}, nil
}

// GetJob returns a browse job by ID.
// IMPORTANT: You must request the "result" field explicitly to get the actual results.
func (s *VolumeBrowserService) GetJob(ctx context.Context, id string) (*VolumeBrowserJob, error) {
	params := url.Values{}
	// CRITICAL: The result field is NOT returned by default
	params.Set("fields", volumeBrowserGetFields)

	var job VolumeBrowserJob
	endpoint := fmt.Sprintf("/volume_browser/%s", id)
	if err := s.client.get(ctx, endpoint, params, &job); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VolumeBrowserJob", ID: id}
		}
		return nil, err
	}

	return &job, nil
}

// WaitForResult polls a browse job until it completes or times out.
// Returns the parsed file/directory entries.
func (s *VolumeBrowserService) WaitForResult(ctx context.Context, jobID string, timeout time.Duration) ([]VolumeBrowserEntry, error) {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		job, err := s.GetJob(ctx, jobID)
		if err != nil {
			return nil, err
		}

		switch job.Status {
		case VolumeBrowserStatusComplete:
			// Parse the result
			return s.parseResult(job.Result)
		case VolumeBrowserStatusError:
			// Extract error message from result
			errMsg := "browse operation failed"
			if len(job.Result) > 0 {
				var errResult string
				if json.Unmarshal(job.Result, &errResult) == nil {
					errMsg = errResult
				}
			}
			return nil, fmt.Errorf("vergeos: %s", errMsg)
		case VolumeBrowserStatusRunning:
			// Wait before polling again
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
				continue
			}
		default:
			return nil, fmt.Errorf("vergeos: unexpected job status: %s", job.Status)
		}
	}

	return nil, fmt.Errorf("vergeos: browse operation timed out after %v", timeout)
}

// parseResult parses the browse result JSON into entries.
func (s *VolumeBrowserService) parseResult(result json.RawMessage) ([]VolumeBrowserEntry, error) {
	// Empty directories return null
	if len(result) == 0 || string(result) == "null" {
		return []VolumeBrowserEntry{}, nil
	}

	var entries []VolumeBrowserEntry
	if err := json.Unmarshal(result, &entries); err != nil {
		return nil, fmt.Errorf("vergeos: failed to parse browse result: %w", err)
	}

	return entries, nil
}

// List returns all browse jobs, with optional filtering.
func (s *VolumeBrowserService) List(ctx context.Context, opts ...ListOption) ([]VolumeBrowserJob, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = volumeBrowserListFields
	}

	params := options.toQueryParams()

	var jobs []VolumeBrowserJob
	if err := s.client.get(ctx, "/volume_browser", params, &jobs); err != nil {
		return nil, err
	}

	return jobs, nil
}

// ListByVolume returns all browse jobs for a specific volume.
func (s *VolumeBrowserService) ListByVolume(ctx context.Context, volumeID string, opts ...ListOption) ([]VolumeBrowserJob, error) {
	return s.List(ctx, append(opts, WithFilter(fmt.Sprintf("volume eq '%s'", volumeID)))...)
}
