package vergeos

// Task represents a scheduled task in VergeOS.
// Tasks are automated jobs that can be triggered on schedule or by events.
type Task struct {
	// Key is the unique identifier for the task.
	Key FlexInt `json:"$key,omitempty"`
	// ID is a 40-character SHA1 hash identifier for the task.
	ID string `json:"id,omitempty"`
	// Owner is the resource path that owns this task (e.g., "vms/123").
	Owner string `json:"owner,omitempty"`
	// Table is the API table/resource type for the action.
	Table string `json:"table,omitempty"`
	// Action is the action to perform (e.g., "poweron", "snapshot").
	Action string `json:"action,omitempty"`
	// ActionDisplay is the human-readable action name (readonly).
	ActionDisplay string `json:"action_display,omitempty"`
	// Name is the task name (required, 1-64 characters).
	Name string `json:"name"`
	// Description is an optional description of the task.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the task is active.
	Enabled bool `json:"enabled,omitempty"`
	// LastRun is the timestamp of the last execution.
	LastRun string `json:"last_run,omitempty"`
	// DeleteAfterRun deletes the task after successful execution.
	DeleteAfterRun bool `json:"delete_after_run,omitempty"`
	// Status is the current task status.
	// Valid values: idle, running
	Status string `json:"status,omitempty"`
	// SystemCreated indicates whether this task was created by the system.
	SystemCreated bool `json:"system_created,omitempty"`
	// Creator is the user who created the task.
	Creator string `json:"creator,omitempty"`

	// Expanded fields from views
	// OwnerDisplay is the display name of the owner resource.
	OwnerDisplay string `json:"owner_display,omitempty"`
	// StatusDisplay is the human-readable status.
	StatusDisplay string `json:"status_display,omitempty"`
	// TriggersCount is the number of schedule triggers.
	TriggersCount int `json:"triggers_count,omitempty"`
	// EventsCount is the number of event triggers.
	EventsCount int `json:"events_count,omitempty"`
}

// TaskCreateRequest is the request body for creating a task.
type TaskCreateRequest struct {
	// Owner is the resource path that owns this task (required).
	Owner string `json:"owner"`
	// Table is the API table/resource type for the action.
	Table string `json:"table,omitempty"`
	// Action is the action to perform (required).
	Action string `json:"action"`
	// Name is the task name (required, 1-64 characters).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// Enabled indicates whether the task is active.
	Enabled *bool `json:"enabled,omitempty"`
	// DeleteAfterRun deletes the task after successful execution.
	DeleteAfterRun *bool `json:"delete_after_run,omitempty"`
}

// TaskUpdateRequest is the request body for updating a task.
type TaskUpdateRequest struct {
	// Name is the task name.
	Name *string `json:"name,omitempty"`
	// Description is the task description.
	Description *string `json:"description,omitempty"`
	// Enabled indicates whether the task is active.
	Enabled *bool `json:"enabled,omitempty"`
	// DeleteAfterRun deletes the task after successful execution.
	DeleteAfterRun *bool `json:"delete_after_run,omitempty"`
}

// TaskExecuteOptions are options for executing a task.
type TaskExecuteOptions struct {
	// Params are optional parameters to pass to the task action.
	Params map[string]interface{} `json:"params,omitempty"`
}

// taskListFields are the fields to request when listing tasks.
const taskListFields = "$key,id,owner,table,action,action_display,name,description,enabled,last_run,delete_after_run,status,system_created,creator"

// taskGetFields are the fields to request when getting a single task.
const taskGetFields = taskListFields

// TaskStatus constants for task status.
const (
	TaskStatusIdle    = "idle"
	TaskStatusRunning = "running"
)
