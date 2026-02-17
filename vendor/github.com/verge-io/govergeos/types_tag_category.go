package vergeos

// TagCategory represents a tag category in VergeOS.
// Tag categories organize tags and define which resource types can be tagged.
type TagCategory struct {
	// Key is the unique identifier for the category.
	Key FlexInt `json:"$key,omitempty"`
	// Name is the category name.
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// SingleTagSelection indicates whether only one tag from this category can be selected.
	SingleTagSelection bool `json:"single_tag_selection,omitempty"`
	// Created is the creation timestamp (Unix epoch).
	Created int64 `json:"created,omitempty"`
	// Modified is the last modification timestamp (Unix epoch).
	Modified int64 `json:"modified,omitempty"`

	// Taggable resource types - which resources can use tags from this category
	// TaggableVolumes indicates whether volumes can be tagged.
	TaggableVolumes bool `json:"taggable_volumes,omitempty"`
	// TaggableVNets indicates whether networks can be tagged.
	TaggableVNets bool `json:"taggable_vnets,omitempty"`
	// TaggableVNetRules indicates whether network rules can be tagged.
	TaggableVNetRules bool `json:"taggable_vnet_rules,omitempty"`
	// TaggableVMwareContainers indicates whether VMware containers can be tagged.
	TaggableVMwareContainers bool `json:"taggable_vmware_containers,omitempty"`
	// TaggableVMs indicates whether VMs can be tagged.
	TaggableVMs bool `json:"taggable_vms,omitempty"`
	// TaggableUsers indicates whether users can be tagged.
	TaggableUsers bool `json:"taggable_users,omitempty"`
	// TaggableTenantNodes indicates whether tenant nodes can be tagged.
	TaggableTenantNodes bool `json:"taggable_tenant_nodes,omitempty"`
	// TaggableSites indicates whether sites can be tagged.
	TaggableSites bool `json:"taggable_sites,omitempty"`
	// TaggableNodes indicates whether nodes can be tagged.
	TaggableNodes bool `json:"taggable_nodes,omitempty"`
	// TaggableGroups indicates whether groups can be tagged.
	TaggableGroups bool `json:"taggable_groups,omitempty"`
	// TaggableClusters indicates whether clusters can be tagged.
	TaggableClusters bool `json:"taggable_clusters,omitempty"`
	// TaggableTenants indicates whether tenants can be tagged.
	TaggableTenants bool `json:"taggable_tenants,omitempty"`
}

// TagCategoryCreateRequest is the request body for creating a tag category.
type TagCategoryCreateRequest struct {
	// Name is the category name (required).
	Name string `json:"name"`
	// Description is an optional description.
	Description string `json:"description,omitempty"`
	// SingleTagSelection indicates whether only one tag from this category can be selected.
	SingleTagSelection *bool `json:"single_tag_selection,omitempty"`

	// Taggable resource types
	// TaggableVolumes indicates whether volumes can be tagged.
	TaggableVolumes *bool `json:"taggable_volumes,omitempty"`
	// TaggableVNets indicates whether networks can be tagged.
	TaggableVNets *bool `json:"taggable_vnets,omitempty"`
	// TaggableVNetRules indicates whether network rules can be tagged.
	TaggableVNetRules *bool `json:"taggable_vnet_rules,omitempty"`
	// TaggableVMwareContainers indicates whether VMware containers can be tagged.
	TaggableVMwareContainers *bool `json:"taggable_vmware_containers,omitempty"`
	// TaggableVMs indicates whether VMs can be tagged.
	TaggableVMs *bool `json:"taggable_vms,omitempty"`
	// TaggableUsers indicates whether users can be tagged.
	TaggableUsers *bool `json:"taggable_users,omitempty"`
	// TaggableTenantNodes indicates whether tenant nodes can be tagged.
	TaggableTenantNodes *bool `json:"taggable_tenant_nodes,omitempty"`
	// TaggableSites indicates whether sites can be tagged.
	TaggableSites *bool `json:"taggable_sites,omitempty"`
	// TaggableNodes indicates whether nodes can be tagged.
	TaggableNodes *bool `json:"taggable_nodes,omitempty"`
	// TaggableGroups indicates whether groups can be tagged.
	TaggableGroups *bool `json:"taggable_groups,omitempty"`
	// TaggableClusters indicates whether clusters can be tagged.
	TaggableClusters *bool `json:"taggable_clusters,omitempty"`
	// TaggableTenants indicates whether tenants can be tagged.
	TaggableTenants *bool `json:"taggable_tenants,omitempty"`
}

// TagCategoryUpdateRequest is the request body for updating a tag category.
type TagCategoryUpdateRequest struct {
	// Name is the category name.
	Name *string `json:"name,omitempty"`
	// Description is the category description.
	Description *string `json:"description,omitempty"`
	// SingleTagSelection indicates whether only one tag from this category can be selected.
	SingleTagSelection *bool `json:"single_tag_selection,omitempty"`

	// Taggable resource types
	// TaggableVolumes indicates whether volumes can be tagged.
	TaggableVolumes *bool `json:"taggable_volumes,omitempty"`
	// TaggableVNets indicates whether networks can be tagged.
	TaggableVNets *bool `json:"taggable_vnets,omitempty"`
	// TaggableVNetRules indicates whether network rules can be tagged.
	TaggableVNetRules *bool `json:"taggable_vnet_rules,omitempty"`
	// TaggableVMwareContainers indicates whether VMware containers can be tagged.
	TaggableVMwareContainers *bool `json:"taggable_vmware_containers,omitempty"`
	// TaggableVMs indicates whether VMs can be tagged.
	TaggableVMs *bool `json:"taggable_vms,omitempty"`
	// TaggableUsers indicates whether users can be tagged.
	TaggableUsers *bool `json:"taggable_users,omitempty"`
	// TaggableTenantNodes indicates whether tenant nodes can be tagged.
	TaggableTenantNodes *bool `json:"taggable_tenant_nodes,omitempty"`
	// TaggableSites indicates whether sites can be tagged.
	TaggableSites *bool `json:"taggable_sites,omitempty"`
	// TaggableNodes indicates whether nodes can be tagged.
	TaggableNodes *bool `json:"taggable_nodes,omitempty"`
	// TaggableGroups indicates whether groups can be tagged.
	TaggableGroups *bool `json:"taggable_groups,omitempty"`
	// TaggableClusters indicates whether clusters can be tagged.
	TaggableClusters *bool `json:"taggable_clusters,omitempty"`
	// TaggableTenants indicates whether tenants can be tagged.
	TaggableTenants *bool `json:"taggable_tenants,omitempty"`
}

// Field list constants for tag category resources.
const (
	tagCategoryListFields = "$key,name,description,single_tag_selection,created,modified,taggable_volumes,taggable_vnets,taggable_vnet_rules,taggable_vmware_containers,taggable_vms,taggable_users,taggable_tenant_nodes,taggable_sites,taggable_nodes,taggable_groups,taggable_clusters,taggable_tenants"
	tagCategoryGetFields  = tagCategoryListFields
)
