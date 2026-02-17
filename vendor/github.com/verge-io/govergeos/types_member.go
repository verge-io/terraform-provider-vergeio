package vergeos

// Member represents a group membership in VergeOS.
type Member struct {
	// ID is the unique identifier for the membership.
	ID FlexInt `json:"$key,omitempty"`
	// Group is the parent group ID.
	Group FlexInt `json:"parent_group,omitempty"`
	// Member is the member value (user or group reference).
	Member string `json:"member,omitempty"`
	// System indicates whether this is a system-managed membership.
	System bool `json:"system,omitempty"`
	// Creator is the username that created this membership.
	Creator string `json:"creator,omitempty"`
}

// MemberCreateRequest is the request body for creating a membership.
type MemberCreateRequest struct {
	// Group is the parent group ID (required).
	Group int `json:"parent_group"`
	// Member is the member value - user or group reference (required).
	Member string `json:"member"`
}

// MemberUpdateRequest is the request body for updating a membership.
// Note: both parent_group and member are readonly after creation per schema.
type MemberUpdateRequest struct {
	// Group is the parent group ID (readonly after creation).
	Group *int `json:"parent_group,omitempty"`
	// Member is the member value (readonly after creation).
	Member *string `json:"member,omitempty"`
}

// memberListFields are the fields to request when listing members.
const memberListFields = "$key,parent_group,member,system,creator"
