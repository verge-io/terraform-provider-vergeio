// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tags

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)


var _ vergeio.IClient = &TagsApi{}

func NewTagsApi(c *vergeio.Client) *TagsApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &TagsApi{
		name:   "Tags Api",
		client: c,
		sdk:    sdk,
	}
}

type TagsApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
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

	// Build filter conditions
	var listOpts []vergeos.ListOption
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
			// Look up category ID by name using SDK
			categoryID, err := ta.getCategoryIDByNameSDK(ctx, categoryName)
			if err != nil {
				return fmt.Errorf("failed to look up category '%s': %w", categoryName, err)
			}
			filterParts = append(filterParts, fmt.Sprintf("category eq %d", categoryID))
		}
	}

	// Combine filter parts with AND
	if len(filterParts) > 0 {
		listOpts = append(listOpts, vergeos.WithFilter(strings.Join(filterParts, " and ")))
	}

	tflog.Debug(ctx, fmt.Sprintf("Tags SDK filter: %s", strings.Join(filterParts, " and ")))

	// Call the SDK API
	tags, err := ta.sdk.Tags.List(ctx, listOpts...)
	if err != nil {
		// Check if this is a version compatibility issue
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return fmt.Errorf(vergeio.ErrEndpointV26, "tags")
		}
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the tags resource %v", len(tags)))

	// Convert SDK tags to API model for existing field mapping logic
	var tagsAPIResp []TagAPIModel
	for _, tag := range tags {
		tagsAPIResp = append(tagsAPIResp, TagAPIModel{
			Key:             int(tag.Key.Int()),
			Name:            tag.Name,
			Category:        int(tag.Category),
			CategoryDisplay: tag.CategoryDisplay,
		})
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

// getCategoryIDByNameSDK looks up a category ID by its name using SDK.
func (ta *TagsApi) getCategoryIDByNameSDK(ctx context.Context, name string) (int32, error) {
	tflog.Debug(ctx, fmt.Sprintf("Looking up category ID for name: %s", name))

	// Call the SDK API with filter
	listOpts := []vergeos.ListOption{
		vergeos.WithFilter(fmt.Sprintf("name eq '%s'", name)),
	}

	categories, err := ta.sdk.TagCategories.List(ctx, listOpts...)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return 0, fmt.Errorf(vergeio.ErrEndpointV26, "tag_categories")
		}
		return 0, err
	}

	if len(categories) == 0 {
		return 0, fmt.Errorf("category '%s' not found", name)
	}

	categoryID := int32(categories[0].Key.Int())
	tflog.Debug(ctx, fmt.Sprintf("Found category '%s' with ID %d", name, categoryID))
	return categoryID, nil
}

// getCategoryIDByName looks up a category ID by its name (legacy method kept for compatibility).
func (ta *TagsApi) getCategoryIDByName(ctx context.Context, name string) (int32, error) {
	return ta.getCategoryIDByNameSDK(ctx, name)
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

	// Prepare payload
	payload := TagMemberAPIModel{
		Tag:    int(data.TagId.ValueInt32()),
		Member: data.Member.ValueString(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(payload); err != nil {
		return fmt.Errorf("failed to marshal tag member payload: %w", err)
	}

	// Convert to SDK request format
	var req vergeos.TagMemberCreateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Call the SDK API
	tagMember, err := ta.sdk.TagMembers.Create(ctx, &req)
	if err != nil {
		// Check if this is a version compatibility issue
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return fmt.Errorf(vergeio.ErrEndpointV26, "tag_members")
		}
		return err
	}

	// Set the ID from the response
	data.Id = types.StringValue(fmt.Sprintf("%d", tagMember.Key.Int()))

	tflog.Debug(ctx, fmt.Sprintf("Successfully created tag member with key %d", tagMember.Key.Int()))

	return nil
}

// Read tag member from the API.
func (ta *TagsApi) readTagMember(ctx context.Context, data *TagMemberResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Reading tag member with ID %s", data.Id.ValueString()))

	// Call the SDK API
	id, _ := strconv.Atoi(data.Id.ValueString())
	tagMember, err := ta.sdk.TagMembers.Get(ctx, id)
	if err != nil {
		// Check if this is a version compatibility issue
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("tag member not found")
		}
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the tag member resource %v", tagMember))

	// Convert SDK tag member to API model for existing field mapping logic
	tagMemberAPIResp := TagMemberAPIModel{
		Key:    fmt.Sprintf("%d", tagMember.Key.Int()), // Keep as interface{} since original expects it
		Tag:    int(tagMember.Tag.Int()),
		Member: tagMember.Member,
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

	// Prepare payload
	payload := TagMemberAPIModel{
		Tag:    int(data.TagId.ValueInt32()),
		Member: data.Member.ValueString(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(payload); err != nil {
		return fmt.Errorf("failed to marshal tag member payload: %w", err)
	}

	// Convert to SDK request format
	var req vergeos.TagMemberUpdateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Call the SDK API
	id, _ := strconv.Atoi(data.Id.ValueString())
	_, err := ta.sdk.TagMembers.Update(ctx, id, &req)
	if err != nil {
		// Check if this is a version compatibility issue or resource not found
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("tag member not found")
		}
		return err
	}

	tflog.Debug(ctx, "Successfully updated tag member")

	return nil
}

// Delete tag member.
func (ta *TagsApi) deleteTagMember(ctx context.Context, data *TagMemberResourceModel) error {
	tflog.Debug(ctx, fmt.Sprintf("Deleting tag member with ID %s", data.Id.ValueString()))

	// Call the SDK API
	id, _ := strconv.Atoi(data.Id.ValueString())
	err := ta.sdk.TagMembers.Delete(ctx, id)
	if err != nil {
		// Handle not found during deletion - treat as success since it's gone
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "not found") {
			tflog.Debug(ctx, "Tag member not found during deletion (may already be deleted)")
			return nil // Treat as success since resource is gone
		}
		return err
	}

	tflog.Debug(ctx, "Successfully deleted tag member")

	return nil
}
