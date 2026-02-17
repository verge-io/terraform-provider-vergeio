package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// TaskService handles scheduled task operations.
type TaskService struct {
	client *Client
}

// List returns all tasks, with optional filtering and pagination.
func (s *TaskService) List(ctx context.Context, opts ...ListOption) ([]Task, error) {
	options := applyListOptions(opts)

	// Use task-specific fields if not specified
	if options.Fields == "most" {
		options.Fields = taskListFields
	}

	params := options.toQueryParams()

	var tasks []Task
	if err := s.client.get(ctx, "/tasks", params, &tasks); err != nil {
		return nil, err
	}

	return tasks, nil
}

// ListRunning returns all currently running tasks.
func (s *TaskService) ListRunning(ctx context.Context, opts ...ListOption) ([]Task, error) {
	filterOpts := []ListOption{WithFilter("status eq 'running'")}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByOwner returns all tasks for a specific owner resource.
// owner should be a resource path like "vms/123".
func (s *TaskService) ListByOwner(ctx context.Context, owner string, opts ...ListOption) ([]Task, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("owner eq '%s'", owner))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListEnabled returns all enabled tasks.
func (s *TaskService) ListEnabled(ctx context.Context, opts ...ListOption) ([]Task, error) {
	filterOpts := []ListOption{WithFilter("enabled eq true")}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single task by ID.
func (s *TaskService) Get(ctx context.Context, id int) (*Task, error) {
	params := url.Values{}
	params.Set("fields", taskGetFields)

	var task Task
	endpoint := fmt.Sprintf("/tasks/%d", id)
	if err := s.client.get(ctx, endpoint, params, &task); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Task", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &task, nil
}

// GetByID returns a task by its 40-character SHA1 ID.
func (s *TaskService) GetByID(ctx context.Context, taskID string) (*Task, error) {
	tasks, err := s.List(ctx, WithFilter(fmt.Sprintf("id eq '%s'", taskID)))
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, &NotFoundError{Resource: "Task", ID: taskID}
	}
	// Get full details
	return s.Get(ctx, int(tasks[0].Key))
}

// GetByName returns a task by name for a specific owner.
func (s *TaskService) GetByName(ctx context.Context, owner, name string) (*Task, error) {
	tasks, err := s.List(ctx, WithFilter(fmt.Sprintf("owner eq '%s' and name eq '%s'", owner, name)))
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return nil, &NotFoundError{Resource: "Task", ID: name}
	}
	// Get full details
	return s.Get(ctx, int(tasks[0].Key))
}

// Create creates a new task and returns the created task.
func (s *TaskService) Create(ctx context.Context, req *TaskCreateRequest) (*Task, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Owner == "" {
		return nil, &ValidationError{Field: "owner", Message: "owner is required"}
	}
	if req.Action == "" {
		return nil, &ValidationError{Field: "action", Message: "action is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/tasks", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created task's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created task
	return s.Get(ctx, id)
}

// Update updates a task and returns the updated task.
func (s *TaskService) Update(ctx context.Context, id int, req *TaskUpdateRequest) (*Task, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/tasks/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Task", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	// Read back the updated task
	return s.Get(ctx, id)
}

// Delete deletes a task.
func (s *TaskService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/tasks/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Task", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// Execute triggers immediate execution of a task.
func (s *TaskService) Execute(ctx context.Context, id int, opts *TaskExecuteOptions) error {
	endpoint := "/task_actions"
	body := map[string]interface{}{
		"task":   id,
		"action": "execute",
	}
	if opts != nil && opts.Params != nil {
		body["params"] = opts.Params
	}
	return s.client.post(ctx, endpoint, body, nil)
}

// Enable enables a task.
func (s *TaskService) Enable(ctx context.Context, id int) error {
	enabled := true
	_, err := s.Update(ctx, id, &TaskUpdateRequest{Enabled: &enabled})
	return err
}

// Disable disables a task.
func (s *TaskService) Disable(ctx context.Context, id int) error {
	enabled := false
	_, err := s.Update(ctx, id, &TaskUpdateRequest{Enabled: &enabled})
	return err
}
