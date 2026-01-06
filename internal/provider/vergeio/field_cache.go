// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// FieldCache provides session-based lazy loading cache for API field values
type FieldCache struct {
	cache  map[string][]string // endpoint:field -> values
	mutex  sync.RWMutex
	client *Client
}

// NewFieldCache creates a new field cache instance
func NewFieldCache(client *Client) *FieldCache {
	return &FieldCache{
		cache:  make(map[string][]string),
		client: client,
	}
}

// TableSchemaField represents a field in the table schema response
type TableSchemaField struct {
	Type string            `json:"type"`
	List map[string]string `json:"list,omitempty"` // Map of value -> description
}

// TableSchemaResponse represents the response from any $table endpoint
type TableSchemaResponse struct {
	Fields map[string]TableSchemaField `json:"fields"` // Map of field name -> field info
}

// GetFieldValues fetches field values from API with session-based caching
// endpoint: e.g. "api/v4/vms", "api/v4/machine_drives"
// fieldName: e.g. "machine_type", "interface"
func (fc *FieldCache) GetFieldValues(ctx context.Context, endpoint, fieldName string) ([]string, error) {
	key := endpoint + ":" + fieldName

	// Check if already cached
	fc.mutex.RLock()
	if values, exists := fc.cache[key]; exists {
		fc.mutex.RUnlock()
		tflog.Debug(ctx, fmt.Sprintf("Retrieved %s values from cache (%d items)", fieldName, len(values)))
		return values, nil
	}
	fc.mutex.RUnlock()

	tflog.Debug(ctx, fmt.Sprintf("Cache miss for %s, fetching from API: %s/$table", fieldName, endpoint))

	// Lazy load from API
	values, err := fc.fetchFromAPI(ctx, endpoint, fieldName)
	if err != nil {
		return nil, err
	}

	// Cache the results for session duration
	fc.mutex.Lock()
	fc.cache[key] = values
	fc.mutex.Unlock()

	tflog.Debug(ctx, fmt.Sprintf("Cached %s values for session (%d items)", fieldName, len(values)))
	return values, nil
}

// fetchFromAPI retrieves field values from the VergeOS API
func (fc *FieldCache) fetchFromAPI(ctx context.Context, endpoint, fieldName string) ([]string, error) {
	// Call the $table endpoint to get schema
	tableEndpoint := endpoint + "/$table"
	apiResp, err := fc.client.Get(tableEndpoint, nil)
	if err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to fetch %s values from %s: %v", fieldName, tableEndpoint, err))
		return nil, fmt.Errorf("failed to fetch %s values from API: %w", fieldName, err)
	}
	defer apiResp.Body.Close()

	// Parse the table schema response
	var schema TableSchemaResponse
	decoder := json.NewDecoder(apiResp.Body)
	if err := decoder.Decode(&schema); err != nil {
		tflog.Error(ctx, fmt.Sprintf("Failed to decode table schema from %s: %v", tableEndpoint, err))
		return nil, fmt.Errorf("failed to decode table schema: %w", err)
	}

	// Find the specified field and extract its valid values
	field, exists := schema.Fields[fieldName]
	if !exists {
		return nil, fmt.Errorf("%s field not found in table schema from %s", fieldName, tableEndpoint)
	}

	if field.List == nil || len(field.List) == 0 {
		return nil, fmt.Errorf("%s field has no list of valid values in schema from %s", fieldName, tableEndpoint)
	}

	// Extract the keys (valid values) from the map
	values := make([]string, 0, len(field.List))
	for value := range field.List {
		values = append(values, value)
	}

	tflog.Debug(ctx, fmt.Sprintf("Successfully fetched %d %s values from API", len(values), fieldName))
	return values, nil
}

// GetMachineTypes convenience method for machine types
func (fc *FieldCache) GetMachineTypes(ctx context.Context) ([]string, error) {
	return fc.GetFieldValues(ctx, "api/v4/vms", "machine_type")
}

// GetDiskInterfaces convenience method for disk interfaces
func (fc *FieldCache) GetDiskInterfaces(ctx context.Context) ([]string, error) {
	return fc.GetFieldValues(ctx, "api/v4/machine_drives", "interface")
}
