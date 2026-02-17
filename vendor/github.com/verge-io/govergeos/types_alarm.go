package vergeos

// Alarm represents an active alarm in VergeOS.
// Alarms are raised by the system to indicate issues requiring attention.
type Alarm struct {
	// Key is the unique identifier for the alarm.
	Key FlexInt `json:"$key,omitempty"`
	// Owner is the resource path that owns this alarm (e.g., "vms/123", "nodes/1").
	Owner string `json:"owner,omitempty"`
	// OwnerType is the type of owner resource.
	// Valid values: vms, vnets, tenant_nodes, nodes, users, system, cloud_snapshots
	OwnerType string `json:"owner_type,omitempty"`
	// SubOwner is an optional sub-resource path (e.g., "machine_drives/456").
	SubOwner string `json:"sub_owner,omitempty"`
	// SubOwnerType is the type of sub-owner resource.
	// Valid values: machine_drives, machine_nics, machine_devices, smtp_settings
	SubOwnerType string `json:"sub_owner_type,omitempty"`
	// AlarmType is the alarm type key (foreign key to alarm_types).
	// Note: This is a string key like "vm_cpu_high", not an integer.
	AlarmType string `json:"alarm_type,omitempty"`
	// Level is the severity level of the alarm.
	// Valid values: audit, message, warning, error, critical, summary, debug
	Level string `json:"level,omitempty"`
	// Status is the current status message of the alarm.
	Status string `json:"status,omitempty"`
	// AlarmID is a unique 8-character identifier for the alarm.
	AlarmID string `json:"alarm_id,omitempty"`
	// Resolvable indicates whether the alarm can be manually resolved.
	Resolvable bool `json:"resolvable,omitempty"`
	// ResolveText is the text displayed for the resolve action.
	ResolveText string `json:"resolve_text,omitempty"`
	// ResolveAction is the action to execute when resolving.
	ResolveAction string `json:"resolve_action,omitempty"`
	// SnoozeThreshold is the threshold value for snoozing.
	SnoozeThreshold float64 `json:"snooze_threshold,omitempty"`
	// Snooze is the timestamp until which the alarm is snoozed (0 if not snoozed).
	Snooze int64 `json:"snooze,omitempty"`
	// SnoozedBy is the username who snoozed the alarm.
	SnoozedBy string `json:"snoozed_by,omitempty"`
	// Expires is the timestamp when the alarm expires (0 if never).
	Expires int64 `json:"expires,omitempty"`
	// Created is the timestamp when the alarm was created.
	Created int64 `json:"created,omitempty"`
	// Modified is the timestamp when the alarm was last modified.
	Modified int64 `json:"modified,omitempty"`

	// Expanded fields from views
	// OwnerName is the display name of the owner resource.
	OwnerName string `json:"owner_name,omitempty"`
	// OwnerTypeDisplay is the human-readable owner type.
	OwnerTypeDisplay string `json:"owner_type_display,omitempty"`
	// LevelDisplay is the human-readable level.
	LevelDisplay string `json:"level_display,omitempty"`
}

// AlarmUpdateRequest is the request body for updating an alarm.
// Note: Most alarm fields are readonly. Only snooze can be updated.
type AlarmUpdateRequest struct {
	// Snooze is the timestamp until which to snooze the alarm.
	// Set to 0 to unsnooze. Set to a future timestamp to snooze.
	Snooze *int64 `json:"snooze,omitempty"`
}

// alarmListFields are the fields to request when listing alarms.
const alarmListFields = "$key,owner,owner_type,sub_owner,sub_owner_type,alarm_type,level,status,alarm_id,resolvable,resolve_text,snooze_threshold,snooze,snoozed_by,expires,created,modified"

// alarmGetFields are the fields to request when getting a single alarm.
const alarmGetFields = alarmListFields

// AlarmLevel constants for alarm severity.
const (
	AlarmLevelAudit    = "audit"
	AlarmLevelMessage  = "message"
	AlarmLevelWarning  = "warning"
	AlarmLevelError    = "error"
	AlarmLevelCritical = "critical"
	AlarmLevelSummary  = "summary"
	AlarmLevelDebug    = "debug"
)

// AlarmOwnerType constants for alarm owner types.
const (
	AlarmOwnerTypeVM            = "vms"
	AlarmOwnerTypeNetwork       = "vnets"
	AlarmOwnerTypeTenantNode    = "tenant_nodes"
	AlarmOwnerTypeNode          = "nodes"
	AlarmOwnerTypeUser          = "users"
	AlarmOwnerTypeSystem        = "system"
	AlarmOwnerTypeCloudSnapshot = "cloud_snapshots"
)

// AlarmSubOwnerType constants for alarm sub-owner types.
const (
	AlarmSubOwnerTypeDrive        = "machine_drives"
	AlarmSubOwnerTypeNIC          = "machine_nics"
	AlarmSubOwnerTypeDevice       = "machine_devices"
	AlarmSubOwnerTypeSMTPSettings = "smtp_settings"
)

// AlarmType represents an alarm type definition in VergeOS.
// Alarm types define the categories and behavior of alarms.
// Note: Alarm types use string keys, not integer IDs.
type AlarmType struct {
	// Key is the unique string identifier for the alarm type.
	Key string `json:"$key,omitempty"`
	// Name is the display name of the alarm type.
	Name string `json:"name,omitempty"`
	// Description is a detailed description of the alarm type.
	Description string `json:"description,omitempty"`
	// Level is the default severity level for alarms of this type.
	Level string `json:"level,omitempty"`
	// DisableLogging prevents logging for this alarm type when true.
	DisableLogging bool `json:"disable_logging,omitempty"`
	// AllowDelete permits deletion of alarms of this type.
	AllowDelete bool `json:"allow_delete,omitempty"`
	// Threshold is the threshold value for triggering this alarm type.
	Threshold float64 `json:"threshold,omitempty"`
	// MaxSnoozeThreshold is the maximum threshold for snoozing (-1 for unlimited).
	MaxSnoozeThreshold float64 `json:"max_snooze_threshold,omitempty"`
	// MaxSnoozeSeconds is the maximum snooze duration in seconds (-1 for unlimited).
	MaxSnoozeSeconds int `json:"max_snooze_seconds,omitempty"`
	// DefaultSnoozeSeconds is the default snooze duration in seconds.
	DefaultSnoozeSeconds int `json:"default_snooze_seconds,omitempty"`
}

// alarmTypeListFields are the fields to request when listing alarm types.
const alarmTypeListFields = "$key,name,description,level,disable_logging,allow_delete,threshold,max_snooze_threshold,max_snooze_seconds,default_snooze_seconds"

// alarmTypeGetFields are the fields to request when getting a single alarm type.
const alarmTypeGetFields = alarmTypeListFields
