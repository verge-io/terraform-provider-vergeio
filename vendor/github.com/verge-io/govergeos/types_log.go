package vergeos

// Log represents a VergeOS system log entry.
// Logs are read-only and provide audit, error, and operational information.
type Log struct {
	// Key is the unique identifier for the log entry (row number).
	Key FlexInt `json:"$key,omitempty"`
	// Level is the log level (audit, message, warning, error, critical, summary, debug).
	Level string `json:"level,omitempty"`
	// Text is the log message text.
	Text string `json:"text,omitempty"`
	// Timestamp is the creation timestamp in microseconds (readonly).
	Timestamp int64 `json:"timestamp,omitempty"`
	// User is the username associated with the log entry.
	User string `json:"user,omitempty"`
	// ObjectType is the type of object this log relates to.
	ObjectType string `json:"object_type,omitempty"`
	// ObjectName is the name of the object this log relates to.
	ObjectName string `json:"object_name,omitempty"`
}

// Log level constants
const (
	LogLevelAudit    string = "audit"
	LogLevelMessage  string = "message"
	LogLevelWarning  string = "warning"
	LogLevelError    string = "error"
	LogLevelCritical string = "critical"
	LogLevelSummary  string = "summary"
	LogLevelDebug    string = "debug"
)

// Log object type constants
const (
	LogObjectTypeCatalogRepository string = "catalog_repository"
	LogObjectTypeCloudSnapshots    string = "cloud_snapshots"
	LogObjectTypeCluster           string = "cluster"
	LogObjectTypeFile              string = "file"
	LogObjectTypeGroup             string = "group"
	LogObjectTypeNode              string = "node"
	LogObjectTypeOIDCApplication   string = "oidc_application"
	LogObjectTypeOther             string = "other"
	LogObjectTypePermission        string = "permission"
	LogObjectTypeServiceContainer  string = "service_container"
	LogObjectTypeSMTP              string = "smtp"
	LogObjectTypeTenant            string = "tenant"
	LogObjectTypeUpdates           string = "updates"
	LogObjectTypeUser              string = "user"
	LogObjectTypeVM                string = "vm"
	LogObjectTypeVMService         string = "vm_service" // NAS Service
	LogObjectTypeVMImport          string = "vm_import"
	LogObjectTypeVMwareContainer   string = "vmware_container"
	LogObjectTypeVNet              string = "vnet"
	LogObjectTypeSite              string = "site"
	LogObjectTypeSystem            string = "system"
	LogObjectTypeSnapshotProfile   string = "snapshot_profile"
	LogObjectTypeImportExport      string = "import_export"
	LogObjectTypeTask              string = "task"
)

// logListFields defines the default fields for listing logs.
const logListFields = "$key,level,text,timestamp,user,object_type,object_name"

// logGetFields defines the default fields for getting a single log entry.
const logGetFields = logListFields
