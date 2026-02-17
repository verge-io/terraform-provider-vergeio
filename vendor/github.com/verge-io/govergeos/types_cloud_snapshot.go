package vergeos

// CloudSnapshot represents a system-wide snapshot in VergeOS.
// Cloud snapshots capture the entire system state including VMs and tenants.
type CloudSnapshot struct {
	Key                  FlexInt `json:"$key,omitempty"`
	Name                 string  `json:"name,omitempty"`
	Description          string  `json:"description,omitempty"`
	SnapshotProfile      FlexInt `json:"snapshot_profile,omitempty"`       // FK to snapshot_profiles
	SnapshotPeriod       FlexInt `json:"snapshot_period,omitempty"`        // FK to snapshot_profile_periods
	ScheduleTask         FlexInt `json:"schedule_task,omitempty"`          // FK to schedule_tasks
	Task                 FlexInt `json:"task,omitempty"`                   // FK to tasks (active task)
	Expires              int64   `json:"expires,omitempty"`                // Expiration timestamp (0 = never)
	Provider             bool    `json:"provider,omitempty"`               // From provider tenant
	Private              bool    `json:"private,omitempty"`                // Hidden from tenants
	RemoteSync           bool    `json:"remote_sync,omitempty"`            // Synced from remote system
	IncomingSync         FlexInt `json:"incoming_sync,omitempty"`          // FK to site_syncs_incoming
	Immutable            bool    `json:"immutable,omitempty"`              // Locked, read-only, cannot be deleted
	ImmutableStatus      string  `json:"immutable_status,omitempty"`       // unlocked, unlocking, locked
	ImmutableLockExpires int64   `json:"immutable_lock_expires,omitempty"` // When immutable lock expires
	Status               string  `json:"status,omitempty"`                 // normal, held
	StatusInfo           string  `json:"status_info,omitempty"`
	Created              int64   `json:"created,omitempty"`
}

// CloudSnapshotCreateRequest represents the request body for creating a cloud snapshot.
// Note: Cloud snapshots are typically created via table_actions/create, which uses different field names.
type CloudSnapshotCreateRequest struct {
	Name         string `json:"name"` // Required: snapshot name (supports date format like "Snapshot_%Y%m%d_%H%M")
	Description  string `json:"description,omitempty"`
	Retention    *int   `json:"retention,omitempty"`     // Seconds until expiration (default 259200 = 3 days)
	MinSnapshots *int   `json:"min_snapshots,omitempty"` // Minimum snapshots to retain (default 1)
	Immutable    *bool  `json:"immutable,omitempty"`     // Lock snapshot
	Private      *bool  `json:"private,omitempty"`       // Hide from tenants
}

// CloudSnapshotUpdateRequest represents the request body for updating a cloud snapshot.
type CloudSnapshotUpdateRequest struct {
	Description          *string `json:"description,omitempty"`
	Expires              *int64  `json:"expires,omitempty"` // 0 = never expire
	Private              *bool   `json:"private,omitempty"`
	Immutable            *bool   `json:"immutable,omitempty"`
	ImmutableLockExpires *int64  `json:"immutable_lock_expires,omitempty"`
}

// cloudSnapshotAction represents an action request for a cloud snapshot.
type cloudSnapshotAction struct {
	CloudSnapshot int                    `json:"cloud_snapshot"`
	Action        string                 `json:"action"`
	Params        map[string]interface{} `json:"params,omitempty"`
}

// CloudSnapshotCloneOptions specifies options for cloning a cloud snapshot.
type CloudSnapshotCloneOptions struct {
	Name string `json:"name"` // Required: name for the cloned snapshot
}

// Cloud snapshot status values
const (
	CloudSnapshotStatusNormal string = "normal"
	CloudSnapshotStatusHeld   string = "held"
)

// Cloud snapshot immutable status values
const (
	CloudSnapshotImmutableUnlocked  string = "unlocked"
	CloudSnapshotImmutableUnlocking string = "unlocking"
	CloudSnapshotImmutableLocked    string = "locked"
)

// cloudSnapshotListFields defines the default fields for listing cloud snapshots.
const cloudSnapshotListFields = "$key,name,description,snapshot_profile,expires,provider,private,remote_sync,immutable,immutable_status,immutable_lock_expires,status,status_info,created"

// cloudSnapshotGetFields defines the default fields for getting a single cloud snapshot.
const cloudSnapshotGetFields = "$key,name,description,snapshot_profile,snapshot_period,schedule_task,task,expires,provider,private,remote_sync,incoming_sync,immutable,immutable_status,immutable_lock_expires,status,status_info,created"

// CloudSnapshotVM represents a VM within a cloud snapshot.
type CloudSnapshotVM struct {
	Key           FlexInt `json:"$key,omitempty"`
	CloudSnapshot FlexInt `json:"cloud_snapshot,omitempty"`
	VM            FlexInt `json:"vm,omitempty"`
	Name          string  `json:"name,omitempty"`
	Description   string  `json:"description,omitempty"`
	Machine       FlexInt `json:"machine,omitempty"`
	CPUCores      int     `json:"cpu_cores,omitempty"`
	RAM           int     `json:"ram,omitempty"`
	OSFamily      string  `json:"os_family,omitempty"`
}

// cloudSnapshotVMListFields defines the default fields for listing cloud snapshot VMs.
const cloudSnapshotVMListFields = "$key,cloud_snapshot,vm,name,description,machine,cpu_cores,ram,os_family"

// CloudSnapshotTenant represents a tenant within a cloud snapshot.
type CloudSnapshotTenant struct {
	Key           FlexInt `json:"$key,omitempty"`
	CloudSnapshot FlexInt `json:"cloud_snapshot,omitempty"`
	Tenant        FlexInt `json:"tenant,omitempty"`
	Name          string  `json:"name,omitempty"`
	Description   string  `json:"description,omitempty"`
}

// cloudSnapshotTenantListFields defines the default fields for listing cloud snapshot tenants.
const cloudSnapshotTenantListFields = "$key,cloud_snapshot,tenant,name,description"
