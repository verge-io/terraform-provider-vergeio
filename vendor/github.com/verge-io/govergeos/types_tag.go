package vergeos

// Tag represents a tag in VergeOS.
// Tags are used to categorize and organize resources.
type Tag struct {
	// Key is the unique identifier for the tag.
	Key FlexInt `json:"$key,omitempty"`
	// Name is the tag name.
	Name string `json:"name"`
	// Description is the tag description.
	Description string `json:"description,omitempty"`
	// Category is the tag category ID.
	Category FlexInt `json:"category,omitempty"`
	// CategoryDisplay is the display name of the category (from join).
	CategoryDisplay string `json:"category_display,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`
}

// TagCreateRequest is the request body for creating a tag.
type TagCreateRequest struct {
	// Category is the tag category ID (required).
	Category int `json:"category"`
	// Name is the tag name (required).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
}

// TagUpdateRequest is the request body for updating a tag.
type TagUpdateRequest struct {
	// Name is the tag name.
	Name *string `json:"name,omitempty"`
	// Description is the tag description.
	Description *string `json:"description,omitempty"`
}

// TagMember represents a tag assignment to a resource in VergeOS.
// TagMembers link tags to objects like VMs, networks, etc.
type TagMember struct {
	// Key is the unique identifier for the tag member.
	Key FlexInt `json:"$key,omitempty"`
	// Tag is the tag ID this member belongs to.
	Tag FlexInt `json:"tag,omitempty"`
	// Member is the object reference in format "object_type/object_id" (e.g., "vms/123").
	Member string `json:"member,omitempty"`
}

// TagMemberCreateRequest is the request body for creating a tag member.
type TagMemberCreateRequest struct {
	// Tag is the tag ID to assign (required).
	Tag int `json:"tag"`
	// Member is the object reference in format "object_type/object_id" (required).
	// Examples: "vms/123", "vnets/456", "users/789"
	Member string `json:"member"`
}

// TagMemberUpdateRequest is the request body for updating a tag member.
// Note: Both tag and member fields are readonly after creation per API schema.
// Updates are supported by the SDK but may have no effect.
type TagMemberUpdateRequest struct {
	// Tag is the tag ID (readonly after creation).
	Tag *int `json:"tag,omitempty"`
	// Member is the object reference (readonly after creation).
	Member *string `json:"member,omitempty"`
}

// Field list constants for tag resources.
const (
	tagListFields       = "$key,name,description,category,category#$display as category_display,created,modified"
	tagMemberListFields = "$key,tag,member"
)
