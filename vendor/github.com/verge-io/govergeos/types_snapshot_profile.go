package vergeos

// SnapshotProfile represents a VergeOS snapshot profile.
// Snapshot profiles define automated backup schedules for VMs, volumes, and cloud snapshots.
type SnapshotProfile struct {
	// Key is the unique identifier for the snapshot profile.
	Key FlexInt `json:"$key,omitempty"`
	// Name is the profile name (unique, required).
	Name string `json:"name"`
	// Description is an optional description of the profile.
	Description string `json:"description,omitempty"`
	// IgnoreWarnings suppresses warnings about snapshot count estimates.
	IgnoreWarnings bool `json:"ignore_warnings,omitempty"`
}

// SnapshotProfileCreateRequest is the request body for creating a snapshot profile.
type SnapshotProfileCreateRequest struct {
	// Name is the profile name (required, unique).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// IgnoreWarnings suppresses warnings about snapshot count estimates.
	IgnoreWarnings *bool `json:"ignore_warnings,omitempty"`
}

// SnapshotProfileUpdateRequest is the request body for updating a snapshot profile.
type SnapshotProfileUpdateRequest struct {
	// Name is the profile name.
	Name *string `json:"name,omitempty"`
	// Description is the profile description.
	Description *string `json:"description,omitempty"`
	// IgnoreWarnings suppresses warnings about snapshot count estimates.
	IgnoreWarnings *bool `json:"ignore_warnings,omitempty"`
}

// snapshotProfileListFields are the fields to request when listing snapshot profiles.
const snapshotProfileListFields = "$key,name,description,ignore_warnings"

// snapshotProfileGetFields are the fields to request when getting a single snapshot profile.
const snapshotProfileGetFields = snapshotProfileListFields

// SnapshotProfilePeriod represents a scheduling period within a snapshot profile.
// Each period defines when snapshots are taken and how long they are retained.
type SnapshotProfilePeriod struct {
	// Key is the unique identifier for the period.
	Key FlexInt `json:"$key,omitempty"`
	// Profile is the parent snapshot profile ID (required).
	Profile FlexInt `json:"profile,omitempty"`
	// Name is the period name (required, unique within profile).
	Name string `json:"name"`
	// Frequency is the snapshot frequency (custom, hourly, daily, weekly, monthly, yearly).
	Frequency string `json:"frequency,omitempty"`
	// Minute is the minute of the hour (0-59).
	Minute int `json:"minute,omitempty"`
	// Hour is the hour of the day (0-23). Set to 0 for hourly frequency.
	Hour int `json:"hour,omitempty"`
	// DayOfWeek is the day of the week (sun, mon, tue, wed, thu, fri, sat, any).
	// Default is "any".
	DayOfWeek string `json:"day_of_week,omitempty"`
	// DayOfMonth is the day of the month (1-31, 0 for any).
	DayOfMonth int `json:"day_of_month,omitempty"`
	// Month is the month of the year (1-12, 0 for any).
	Month int `json:"month,omitempty"`
	// Retention is how long to keep snapshots in seconds (required).
	Retention int `json:"retention,omitempty"`
	// SkipMissed skips taking a snapshot if the schedule was missed.
	SkipMissed bool `json:"skip_missed,omitempty"`
	// MaxTier is the maximum storage tier for storing snapshots (1-5, default "1").
	// Tiers higher than max will be demoted to this tier.
	MaxTier string `json:"max_tier,omitempty"`
	// Quiesce temporarily freezes disk activity while taking the snapshot.
	// Requires guest agent support. Applies to VMs and volumes, not system snapshots.
	Quiesce bool `json:"quiesce,omitempty"`
	// MinSnapshots is the minimum number of snapshots to retain (default 1).
	// Helps prevent all snapshots from expiring during a prolonged outage.
	MinSnapshots int `json:"min_snapshots,omitempty"`
	// Immutable makes snapshots locked and read-only until unlocked.
	// Applies to system snapshots only.
	Immutable bool `json:"immutable,omitempty"`
	// EstimatedSnapshotCount is the estimated number of snapshots (readonly).
	EstimatedSnapshotCount int `json:"estimated_snapshot_count,omitempty"`
}

// SnapshotProfilePeriodCreateRequest is the request body for creating a snapshot profile period.
type SnapshotProfilePeriodCreateRequest struct {
	// Profile is the parent snapshot profile ID (required).
	Profile int `json:"profile"`
	// Name is the period name (required, unique within profile).
	Name string `json:"name"`
	// Frequency is the snapshot frequency (custom, hourly, daily, weekly, monthly, yearly).
	Frequency string `json:"frequency,omitempty"`
	// Minute is the minute of the hour (0-59).
	Minute *int `json:"minute,omitempty"`
	// Hour is the hour of the day (0-23).
	Hour *int `json:"hour,omitempty"`
	// DayOfWeek is the day of the week (sun, mon, tue, wed, thu, fri, sat, any).
	DayOfWeek *string `json:"day_of_week,omitempty"`
	// DayOfMonth is the day of the month (1-31, 0 for any).
	DayOfMonth *int `json:"day_of_month,omitempty"`
	// Month is the month of the year (1-12, 0 for any).
	Month *int `json:"month,omitempty"`
	// Retention is how long to keep snapshots in seconds (required).
	Retention int `json:"retention"`
	// SkipMissed skips taking a snapshot if the schedule was missed.
	SkipMissed *bool `json:"skip_missed,omitempty"`
	// MaxTier is the maximum storage tier (1-5).
	MaxTier *string `json:"max_tier,omitempty"`
	// Quiesce temporarily freezes disk activity while taking the snapshot.
	Quiesce *bool `json:"quiesce,omitempty"`
	// MinSnapshots is the minimum number of snapshots to retain.
	MinSnapshots *int `json:"min_snapshots,omitempty"`
	// Immutable makes snapshots locked and read-only.
	Immutable *bool `json:"immutable,omitempty"`
}

// SnapshotProfilePeriodUpdateRequest is the request body for updating a snapshot profile period.
type SnapshotProfilePeriodUpdateRequest struct {
	// Name is the period name.
	Name *string `json:"name,omitempty"`
	// Frequency is the snapshot frequency.
	Frequency *string `json:"frequency,omitempty"`
	// Minute is the minute of the hour (0-59).
	Minute *int `json:"minute,omitempty"`
	// Hour is the hour of the day (0-23).
	Hour *int `json:"hour,omitempty"`
	// DayOfWeek is the day of the week.
	DayOfWeek *string `json:"day_of_week,omitempty"`
	// DayOfMonth is the day of the month (1-31, 0 for any).
	DayOfMonth *int `json:"day_of_month,omitempty"`
	// Month is the month of the year (1-12, 0 for any).
	Month *int `json:"month,omitempty"`
	// Retention is how long to keep snapshots in seconds.
	Retention *int `json:"retention,omitempty"`
	// SkipMissed skips taking a snapshot if the schedule was missed.
	SkipMissed *bool `json:"skip_missed,omitempty"`
	// MaxTier is the maximum storage tier (1-5).
	MaxTier *string `json:"max_tier,omitempty"`
	// Quiesce temporarily freezes disk activity while taking the snapshot.
	Quiesce *bool `json:"quiesce,omitempty"`
	// MinSnapshots is the minimum number of snapshots to retain.
	MinSnapshots *int `json:"min_snapshots,omitempty"`
	// Immutable makes snapshots locked and read-only.
	Immutable *bool `json:"immutable,omitempty"`
}

// snapshotProfilePeriodListFields are the fields to request when listing periods.
const snapshotProfilePeriodListFields = "$key,profile,name,frequency,minute,hour,day_of_week,day_of_month,month,retention,skip_missed,max_tier,quiesce,min_snapshots,immutable,estimated_snapshot_count"

// snapshotProfilePeriodGetFields are the fields to request when getting a single period.
const snapshotProfilePeriodGetFields = snapshotProfilePeriodListFields

// Frequency constants for snapshot profile periods.
const (
	// FrequencyCustom allows custom scheduling with all time fields.
	FrequencyCustom = "custom"
	// FrequencyHourly takes snapshots every hour.
	FrequencyHourly = "hourly"
	// FrequencyDaily takes snapshots once per day.
	FrequencyDaily = "daily"
	// FrequencyWeekly takes snapshots once per week.
	FrequencyWeekly = "weekly"
	// FrequencyMonthly takes snapshots once per month.
	FrequencyMonthly = "monthly"
	// FrequencyYearly takes snapshots once per year.
	FrequencyYearly = "yearly"
)

// DayOfWeek constants for snapshot profile periods.
const (
	DayOfWeekSunday    = "sun"
	DayOfWeekMonday    = "mon"
	DayOfWeekTuesday   = "tue"
	DayOfWeekWednesday = "wed"
	DayOfWeekThursday  = "thu"
	DayOfWeekFriday    = "fri"
	DayOfWeekSaturday  = "sat"
	DayOfWeekAny       = "any"
)

// MaxTier constants for snapshot storage tier restrictions.
const (
	MaxTier1 = "1" // No restrictions (highest tier)
	MaxTier2 = "2"
	MaxTier3 = "3"
	MaxTier4 = "4"
	MaxTier5 = "5" // Lowest tier
)
