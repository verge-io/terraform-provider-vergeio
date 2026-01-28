// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	TagsEndpoint          = "api/v4/tags"
	TagMembersEndpoint    = "api/v4/tag_members"
	TagCategoriesEndpoint = "api/v4/tag_categories"
)

var _ vergeio.IClient = &TagsApi{}

func NewTagsApi(c *vergeio.Client) *TagsApi {
	return &TagsApi{
		name:   "Tags Api",
		client: c,
	}
}

type TagsApi struct {
	name   string
	client *vergeio.Client
}

func (ta *TagsApi) Name() string {
	return ta.name
}

type TagAPIModel struct {
	Key             int    `json:"$key"`
	Name            string `json:"name"`
	Category        int    `json:"category"`
	CategoryDisplay string `json:"category_display"`
}

type TagMemberAPIModel struct {
	Key    interface{} `json:"$key,omitempty"`
	Tag    int         `json:"tag"`
	Member string      `json:"member"`
}

// Read tags from the API.
func (ta *TagsApi) readTags(ctx context.Context, data *TagsDataSourceModel) error {
	tflog.Debug(ctx, "Reading tags data")

	// Prepare options for API call - request category fields
	options := &vergeio.Options{
		Fields: "$key,name,category,category#$display as category_display",
	}

	// Build filter conditions
	var filterParts []string

	// Add name filter if specified
	if !data.Filter.IsNull() && !data.Filter.IsUnknown() {
		filterName := data.Filter.ValueString()
		if filterName != "" {
			filterParts = append(filterParts, fmt.Sprintf("name eq '%s'", filterName))
		}
	}

	// Add category filter by ID if specified
	if !data.CategoryFilter.IsNull() && !data.CategoryFilter.IsUnknown() {
		categoryID := data.CategoryFilter.ValueInt32()
		filterParts = append(filterParts, fmt.Sprintf("category eq %d", categoryID))
	}

	// Handle category_name filter (requires lookup) - only if category_filter not set
	if data.CategoryFilter.IsNull() && !data.CategoryName.IsNull() && !data.CategoryName.IsUnknown() {
		categoryName := data.CategoryName.ValueString()
		if categoryName != "" {
			// Look up category ID by name
			categoryID, err := ta.getCategoryIDByName(ctx, categoryName)
			if err != nil {
				return fmt.Errorf("failed to look up category '%s': %w", categoryName, err)
			}
			filterParts = append(filterParts, fmt.Sprintf("category eq %d", categoryID))
		}
	}

	// Combine filter parts with AND
	if len(filterParts) > 0 {
		options.Filter = strings.Join(filterParts, " and ")
	}

	tflog.Debug(ctx, fmt.Sprintf("Tags API filter: %s", options.Filter))

	apiResp, err := ta.client.Get(TagsEndpoint, options)

	// Error checking with version-aware handling
	if err != nil {
		// Check if this is a 404 error from the client
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			return fmt.Errorf(vergeio.ErrEndpointV26, "tags")
		}
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// First check for 404 - version compatibility issue
	if apiResp.StatusCode == 404 {
		return fmt.Errorf(vergeio.ErrEndpointV26, "tags")
	}

	// Check for any other non-200 status code
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("API returned status code %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the tags resource %v", apiResp.StatusCode))

	// Read response body
	body, err := io.ReadAll(apiResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Response body: %s", string(body)))

	// Decode the API response
	var tagsAPIResp []TagAPIModel
	if err := json.Unmarshal(body, &tagsAPIResp); err != nil {
		return fmt.Errorf("invalid format received for tags: %w", err)
	}

	// Convert API response to Terraform types
	var tagsList []TagModel
	for _, tag := range tagsAPIResp {
		tagModel := TagModel{
			Key:          types.Int32Value(int32(tag.Key)),
			Name:         types.StringValue(tag.Name),
			Category:     types.Int32Value(int32(tag.Category)),
			CategoryName: types.StringValue(tag.CategoryDisplay),
		}
		tagsList = append(tagsList, tagModel)
	}

	// Set the tags in the data model
	data.Tags = tagsList

	tflog.Debug(ctx, fmt.Sprintf("Successfully converted %d tags to resource", len(tagsList)))

	return nil
}

// getCategoryIDByName looks up a category ID by its name.
func (ta *TagsApi) getCategoryIDByName(ctx context.Context, name string) (int32, error) {
	tflog.Debug(ctx, fmt.Sprintf("Looking up category ID for name: %s", name))

	options := &vergeio.Options{
		Fields: "$key,name",
		Filter: fmt.Sprintf("name eq '%s'", name),
	}

	apiResp, err := ta.client.Get(TagCategoriesEndpoint, options)
	if err != nil {
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			return 0, fmt.Errorf(vergeio.ErrEndpointV26, "tag_categories")
		}
		return 0, err
	}
	if apiResp == nil {
		return 0, errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	if apiResp.StatusCode == 404 {
		return 0, fmt.Errorf(vergeio.ErrEndpointV26, "tag_categories")
	}

	if apiResp.StatusCode != 200 {
		return 0, fmt.Errorf("API returned status code %d", apiResp.StatusCode)
	}

	body, err := io.ReadAll(apiResp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %w", err)
	}

	var categories []struct {
		Key int `json:"$key"`
	}
	if err := json.Unmarshal(body, &categories); err != nil {
		return 0, fmt.Errorf("invalid format received for tag categories: %w", err)
	}

	if len(categories) == 0 {
		return 0, fmt.Errorf("category '%s' not found", name)
	}

	tflog.Debug(ctx, fmt.Sprintf("Found category '%s' with ID %d", name, categories[0].Key))
	return int32(categories[0].Key), nil
}

// checkEndpointAvailability tests if an endpoint is available (for version compatibility)
func (ta *TagsApi) checkEndpointAvailability(ctx context.Context, endpoint string) error {
	tflog.Debug(ctx, fmt.Sprintf("Checking availability of endpoint: %s", endpoint))

	// Make a simple GET request to check if endpoint exists
	// We expect either 200 (success) or 40x (endpoint exists but other error)
	// We only care about catching endpoint not found (version issue)
	options := &vergeio.Options{
		Limit: "1", // Minimal response
	}

	apiResp, err := ta.client.Get(endpoint, options)

	// If we get an error from the client, check if it's endpoint-related
	if err != nil {
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			// Check if the error message indicates endpoint doesn't exist
			if strings.Contains(apiError.VergeError, "not found") && strings.Contains(apiError.Endpoint, endpoint) {
				return fmt.Errorf(vergeio.ErrEndpointV26, strings.TrimPrefix(endpoint, "api/v4/"))
			}
		}
		// Other errors are not version-related, endpoint might still exist
		return nil
	}

	// If we got a response (even non-200), the endpoint exists
	if apiResp != nil {
		apiResp.Body.Close()
	}

	tflog.Debug(ctx, fmt.Sprintf("Endpoint %s is available", endpoint))
	return nil
}

// Create a tag member assignment.
func (ta *TagsApi) createTagMember(ctx context.Context, data *TagMemberResourceModel) error {
	tflog.Debug(ctx, "Creating tag member")

	// First check if the tag_members endpoint is available (version check)
	if err := ta.checkEndpointAvailability(ctx, TagMembersEndpoint); err != nil {
		return err
	}

	// Prepare payload
	payload := TagMemberAPIModel{
		Tag:    int(data.TagId.ValueInt32()),
		Member: data.Member.ValueString(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal tag member payload: %w", err)
	}

	apiResp, err := ta.client.Post(TagMembersEndpoint, bytes.NewBuffer(payloadBytes))

	// Error checking - endpoint is available, so 404s are resource-specific
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// Check for non-success status codes
	if apiResp.StatusCode != 200 && apiResp.StatusCode != 201 {
		return fmt.Errorf("API returned status code %d", apiResp.StatusCode)
	}

	// Read response body to get the created resource
	body, err := io.ReadAll(apiResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Create response body: %s", string(body)))

	// Parse response to get the key
	var createdTagMember TagMemberAPIModel
	if err := json.Unmarshal(body, &createdTagMember); err != nil {
		return fmt.Errorf("invalid format received for created tag member: %w", err)
	}

	// Set the ID from the response
	var keyStr string
	switch v := createdTagMember.Key.(type) {
	case int:
		keyStr = fmt.Sprintf("%d", v)
	case string:
		keyStr = v
	case float64:
		keyStr = fmt.Sprintf("%.0f", v)
	default:
		return fmt.Errorf("unexpected key type: %T", v)
	}
	data.Id = types.StringValue(keyStr)

	tflog.Debug(ctx, fmt.Sprintf("Successfully created tag member with key %s", keyStr))

	return nil
}

// Read tag member from the API.
func (ta *TagsApi) readTagMember(ctx context.Context, data *TagMemberResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Reading tag member with ID %s", data.Id.ValueString()))

	// First check if the tag_members endpoint is available (version check)
	if err := ta.checkEndpointAvailability(ctx, TagMembersEndpoint); err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/%s", TagMembersEndpoint, data.Id.ValueString())
	apiResp, err := ta.client.Get(endpoint, nil)

	// Error checking - endpoint is available, so 404s are resource-specific
	if err != nil {
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			// Resource-specific not found
			return fmt.Errorf("tag member not found")
		}
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// Handle 404 from response (resource not found, endpoint exists)
	if apiResp.StatusCode == 404 {
		return fmt.Errorf("tag member not found")
	}

	// Check for any other non-200 status code
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("API returned status code %d", apiResp.StatusCode)
	}

	// Read response body
	body, err := io.ReadAll(apiResp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	tflog.Debug(ctx, fmt.Sprintf("Response body: %s", string(body)))

	// Decode the API response
	var tagMemberAPIResp TagMemberAPIModel
	if err := json.Unmarshal(body, &tagMemberAPIResp); err != nil {
		return fmt.Errorf("invalid format received for tag member: %w", err)
	}

	// Update the model with API data
	var keyStr string
	switch v := tagMemberAPIResp.Key.(type) {
	case int:
		keyStr = fmt.Sprintf("%d", v)
	case string:
		keyStr = v
	case float64:
		keyStr = fmt.Sprintf("%.0f", v)
	default:
		return fmt.Errorf("unexpected key type: %T", v)
	}
	data.Id = types.StringValue(keyStr)
	data.TagId = types.Int32Value(int32(tagMemberAPIResp.Tag))
	data.Member = types.StringValue(tagMemberAPIResp.Member)

	tflog.Debug(ctx, "Successfully read tag member from API")

	return nil
}

// Update tag member.
func (ta *TagsApi) updateTagMember(ctx context.Context, data *TagMemberResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Updating tag member with ID %s", data.Id.ValueString()))

	// First check if the tag_members endpoint is available (version check)
	if err := ta.checkEndpointAvailability(ctx, TagMembersEndpoint); err != nil {
		return err
	}

	// Prepare payload
	payload := TagMemberAPIModel{
		Tag:    int(data.TagId.ValueInt32()),
		Member: data.Member.ValueString(),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal tag member payload: %w", err)
	}

	endpoint := fmt.Sprintf("%s/%s", TagMembersEndpoint, data.Id.ValueString())
	apiResp, err := ta.client.Put(endpoint, bytes.NewBuffer(payloadBytes))

	// Error checking - endpoint is available, so 404s are resource-specific
	if err != nil {
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			return fmt.Errorf("tag member not found")
		}
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// Handle 404 from response (resource not found, endpoint exists)
	if apiResp.StatusCode == 404 {
		return fmt.Errorf("tag member not found")
	}

	// Check for any other non-200 status code
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("API returned status code %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, "Successfully updated tag member")

	return nil
}

// Delete tag member.
func (ta *TagsApi) deleteTagMember(ctx context.Context, data *TagMemberResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Deleting tag member with ID %s", data.Id.ValueString()))

	// First check if the tag_members endpoint is available (version check)
	if err := ta.checkEndpointAvailability(ctx, TagMembersEndpoint); err != nil {
		return err
	}

	endpoint := fmt.Sprintf("%s/%s", TagMembersEndpoint, data.Id.ValueString())
	apiResp, err := ta.client.Delete(endpoint)

	// Error checking - endpoint is available, so 404s are resource-specific
	if err != nil {
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			// Resource not found during deletion - treat as success since it's gone
			tflog.Debug(ctx, "Tag member not found during deletion (may already be deleted)")
			return nil
		}
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// Handle 404 from response (resource already deleted, endpoint exists)
	if apiResp.StatusCode == 404 {
		tflog.Debug(ctx, "Tag member not found during deletion (may already be deleted)")
		return nil // Treat as success since resource is gone
	}

	// Check for any other non-success status code
	if apiResp.StatusCode != 200 && apiResp.StatusCode != 204 {
		return fmt.Errorf("API returned status code %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, "Successfully deleted tag member")

	return nil
}
