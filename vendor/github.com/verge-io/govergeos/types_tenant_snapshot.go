package vergeos

// TenantSnapshot represents a VergeOS tenant snapshot.
// Tenant snapshots capture the state of a tenant at a point in time.
type TenantSnapshot struct {
	// Key is the unique identifier for the snapshot.
	Key FlexInt `json:"$key,omitempty"`
	// Tenant is the parent tenant ID.
	Tenant FlexInt `json:"tenant,omitempty"`
	// Name is the snapshot name (readonly, auto-generated from profile).
	Name string `json:"name,omitempty"`
	// Description is an optional description of the snapshot.
	Description string `json:"description,omitempty"`
	// Profile is the snapshot profile name used to create this snapshot.
	Profile string `json:"profile,omitempty"`
	// Period is the profile period name.
	Period string `json:"period,omitempty"`
	// MinSnapshots is the minimum number of snapshots to retain.
	MinSnapshots int `json:"min_snapshots,omitempty"`
	// Created is the creation timestamp (Unix epoch, readonly).
	Created int64 `json:"created,omitempty"`
	// Expires is the expiration timestamp (Unix epoch, 0 = never).
	Expires int64 `json:"expires,omitempty"`
}

// TenantSnapshotUpdateRequest is the request body for updating a tenant snapshot.
type TenantSnapshotUpdateRequest struct {
	// Description is the snapshot description.
	Description *string `json:"description,omitempty"`
	// Expires is the expiration timestamp (0 = never expires).
	Expires *int64 `json:"expires,omitempty"`
}

// tenantSnapshotListFields are the fields to request when listing tenant snapshots.
const tenantSnapshotListFields = "$key,tenant,name,description,profile,period,min_snapshots,created,expires"

// tenantSnapshotGetFields are the fields to request when getting a single tenant snapshot.
const tenantSnapshotGetFields = tenantSnapshotListFields
