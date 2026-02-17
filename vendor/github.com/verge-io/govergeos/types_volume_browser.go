package vergeos

import "encoding/json"

// VolumeBrowserJob represents a volume browser job.
// The volume browser API is asynchronous - you create a job and poll for results.
type VolumeBrowserJob struct {
	// Key is the unique SHA1 hash job ID.
	Key string `json:"$key,omitempty"`
	// ID is the unique SHA1 hash identifier (same as Key).
	ID string `json:"id,omitempty"`
	// Volume is the volume SHA1 key being browsed.
	Volume string `json:"volume,omitempty"`
	// Query is the operation type: get-dir, rename, delete, paste.
	Query string `json:"query,omitempty"`
	// Params contains the query parameters.
	Params json.RawMessage `json:"params,omitempty"`
	// Status is the job status: running, complete, error.
	Status string `json:"status,omitempty"`
	// Result contains the operation result (must be explicitly requested with fields param).
	Result json.RawMessage `json:"result,omitempty"`
	// Command is the command used to execute the query (read-only).
	Command string `json:"command,omitempty"`
	// Created is the creation timestamp in microseconds.
	Created int64 `json:"created,omitempty"`
	// Modified is the last modified timestamp.
	Modified int64 `json:"modified,omitempty"`
	// Expires is when the job expires.
	Expires int64 `json:"expires,omitempty"`
}

// VolumeBrowserJob status values
const (
	VolumeBrowserStatusRunning  = "running"
	VolumeBrowserStatusComplete = "complete"
	VolumeBrowserStatusError    = "error"
)

// VolumeBrowserQuery types
const (
	VolumeBrowserQueryGetDir = "get-dir"
	VolumeBrowserQueryRename = "rename"
	VolumeBrowserQueryDelete = "delete"
	VolumeBrowserQueryPaste  = "paste"
)

// VolumeBrowserEntry represents a file or directory entry in browse results.
type VolumeBrowserEntry struct {
	// Name is the file or directory name.
	Name string `json:"name"`
	// NName is the normalized name (lowercase).
	NName string `json:"n_name,omitempty"`
	// Size is the size in bytes.
	Size int64 `json:"size"`
	// Date is the modification time (Unix timestamp).
	Date int64 `json:"date"`
	// Type is "file" or "directory".
	Type string `json:"type"`
}

// VolumeBrowserRequest is the request body for creating a browse job.
type VolumeBrowserRequest struct {
	// Volume is the volume SHA1 key to browse (required).
	Volume string `json:"volume"`
	// Query is the operation type (required): get-dir, rename, delete, paste.
	Query string `json:"query"`
	// Params contains the query-specific parameters (required).
	Params *VolumeBrowserParams `json:"params"`
}

// VolumeBrowserParams contains parameters for browse operations.
type VolumeBrowserParams struct {
	// Dir is the directory path to browse. Use "" for root, NOT "/".
	Dir string `json:"dir"`
	// Limit is the maximum number of entries to return.
	Limit int `json:"limit"`
	// Offset is the pagination offset (nil for first page).
	Offset *int `json:"offset"`
	// Filter contains filter options.
	Filter *VolumeBrowserFilter `json:"filter"`
	// Volume is the volume SHA1 key (must match top-level Volume).
	Volume string `json:"volume"`
	// Sort is the sort field (empty for default).
	Sort string `json:"sort"`

	// Additional params for rename/delete/paste operations
	// Name is the new name (for rename).
	Name string `json:"name,omitempty"`
	// Items is the list of items to operate on (for delete/paste).
	Items []string `json:"items,omitempty"`
	// DestDir is the destination directory (for paste).
	DestDir string `json:"dest_dir,omitempty"`
	// Mode is the paste mode: copy, move.
	Mode string `json:"mode,omitempty"`
}

// VolumeBrowserFilter contains filter options for browse operations.
type VolumeBrowserFilter struct {
	// Extensions filters by file extensions (empty for all).
	Extensions string `json:"extensions"`
}

// Field list constants for volume browser
const volumeBrowserListFields = "$key,id,volume,query,status,command,created,modified,expires"
const volumeBrowserGetFields = "$key,id,volume,query,status,result,command,created,modified,expires"
