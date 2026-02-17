package vergeos

// CloudInitFile represents a cloud-init file in VergeOS.
type CloudInitFile struct {
	// ID is the unique identifier for the file.
	ID FlexInt `json:"$key,omitempty"`
	// Owner is the owner of the file.
	Owner string `json:"owner,omitempty"`
	// Name is the file name.
	Name string `json:"name"`
	// FileSize is the file size in bytes.
	FileSize int64 `json:"filesize,omitempty"`
	// AllocatedBytes is the storage space allocated for the file.
	AllocatedBytes int64 `json:"allocated_bytes,omitempty"`
	// UsedBytes is the storage space used by the file.
	UsedBytes int64 `json:"used_bytes,omitempty"`
	// Modified is the last modified timestamp.
	Modified int64 `json:"modified,omitempty"`
	// Contents is the file contents (max 65536 bytes).
	Contents string `json:"contents,omitempty"`
	// ContainsVariables indicates whether the file contains variables.
	// This is automatically set based on the Render field.
	ContainsVariables bool `json:"contains_variables,omitempty"`
	// Render specifies how the file should be rendered.
	// Valid values: "no" (default), "variables", "jinja2"
	Render string `json:"render,omitempty"`
	// Creator is the username who created the file.
	Creator string `json:"creator,omitempty"`
}

// CloudInitFileCreateRequest is the request body for creating a cloud-init file.
type CloudInitFileCreateRequest struct {
	// Name is the file name (required).
	Name string `json:"name"`
	// Contents is the file contents (max 65536 bytes).
	Contents string `json:"contents,omitempty"`
	// Render specifies how the file should be rendered.
	// Valid values: "no" (default), "variables", "jinja2"
	// Note: Setting to "variables" automatically sets ContainsVariables to true.
	Render *string `json:"render,omitempty"`
}

// CloudInitFileUpdateRequest is the request body for updating a cloud-init file.
type CloudInitFileUpdateRequest struct {
	// Name is the file name.
	Name *string `json:"name,omitempty"`
	// Contents is the file contents (max 65536 bytes).
	Contents *string `json:"contents,omitempty"`
	// Render specifies how the file should be rendered.
	// Valid values: "no", "variables", "jinja2"
	// Note: Setting to "variables" automatically sets ContainsVariables to true.
	Render *string `json:"render,omitempty"`
}

// cloudInitListFields are the fields to request when listing cloud-init files.
const cloudInitListFields = "$key,owner,name,filesize,allocated_bytes,used_bytes,modified,contents,contains_variables,render,creator"
