package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// SchemaService handles schema discovery operations.
type SchemaService struct {
	client *Client
}

// GetTableSchema returns the schema for a resource type.
// This can be used to discover valid values for enum fields.
//
// Example:
//
//	schema, err := client.Schema.GetTableSchema(ctx, "vms")
//	machineTypes := schema.Fields["machine_type"].List // map of valid machine types
func (s *SchemaService) GetTableSchema(ctx context.Context, resource string) (*TableSchema, error) {
	params := url.Values{}

	var schema TableSchema
	endpoint := fmt.Sprintf("/%s/$table", resource)
	if err := s.client.get(ctx, endpoint, params, &schema); err != nil {
		return nil, err
	}

	return &schema, nil
}

// GetValidValues returns the valid values for a field in a resource type.
// Returns a map where keys are valid values and values are descriptions.
//
// Example:
//
//	machineTypes, err := client.Schema.GetValidValues(ctx, "vms", "machine_type")
//	for value, description := range machineTypes {
//	    fmt.Printf("%s: %s\n", value, description)
//	}
func (s *SchemaService) GetValidValues(ctx context.Context, resource, field string) (map[string]string, error) {
	schema, err := s.GetTableSchema(ctx, resource)
	if err != nil {
		return nil, err
	}

	fieldSchema, ok := schema.Fields[field]
	if !ok {
		return nil, &NotFoundError{Resource: "field", ID: field}
	}

	if fieldSchema.List == nil {
		return make(map[string]string), nil
	}

	return fieldSchema.List, nil
}

// GetVMMachineTypes returns valid machine types for VMs.
func (s *SchemaService) GetVMMachineTypes(ctx context.Context) (map[string]string, error) {
	return s.GetValidValues(ctx, "vms", "machine_type")
}

// GetVMOSFamilies returns valid OS families for VMs.
func (s *SchemaService) GetVMOSFamilies(ctx context.Context) (map[string]string, error) {
	return s.GetValidValues(ctx, "vms", "os_family")
}
