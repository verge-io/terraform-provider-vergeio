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
	TagsEndpoint       = "api/v4/tags"
	TagMembersEndpoint = "api/v4/tag_members"
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
	Key  int    `json:"$key"`
	Name string `json:"name"`
}

type TagMemberAPIModel struct {
	Key    interface{} `json:"$key,omitempty"`
	Tag    int         `json:"tag"`
	Member string      `json:"member"`
}

// Read tags from the API.
func (ta *TagsApi) readTags(ctx context.Context, data *TagsDataSourceModel) error {
	tflog.Debug(ctx, "Reading tags data")

	// Prepare options for API call
	options := &vergeio.Options{
		Fields: "most",
	}

	// Add name filter if specified
	if !data.Filter.IsNull() && !data.Filter.IsUnknown() {
		filterName := data.Filter.ValueString()
		if filterName != "" {
			options.Filter = fmt.Sprintf("name eq '%s'", filterName)
		}
	}

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
			Key:  types.Int32Value(int32(tag.Key)),
			Name: types.StringValue(tag.Name),
		}
		tagsList = append(tagsList, tagModel)
	}

	// Set the tags in the data model
	data.Tags = tagsList

	tflog.Debug(ctx, fmt.Sprintf("Successfully converted %d tags to resource", len(tagsList)))

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

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal tag member payload: %w", err)
	}

	apiResp, err := ta.client.Post(TagMembersEndpoint, bytes.NewBuffer(payloadBytes))

	// Error checking with version-aware handling
	if err != nil {
		// Check if this is a 404 error from the client
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			return fmt.Errorf(vergeio.ErrEndpointV26, "tag_members")
		}
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// First check for 404 - version compatibility issue
	if apiResp.StatusCode == 404 {
		return fmt.Errorf(vergeio.ErrEndpointV26, "tag_members")
	}

	// Check for any other non-200 status code
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

	endpoint := fmt.Sprintf("%s/%s", TagMembersEndpoint, data.Id.ValueString())
	apiResp, err := ta.client.Get(endpoint, nil)

	// Error checking with version-aware handling
	if err != nil {
		// Check if this is a 404 error from the client
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			// Could be endpoint not available or resource not found
			if strings.Contains(apiError.VergeError, "not found") && strings.Contains(apiError.Endpoint, "tag_members") {
				return fmt.Errorf(vergeio.ErrEndpointV26, "tag_members")
			}
			// Resource not found - return specific error for state management
			return fmt.Errorf("tag member not found")
		}
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// First check for 404 - version compatibility or resource not found
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

	// Error checking with version-aware handling
	if err != nil {
		// Check if this is a 404 error from the client
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			return fmt.Errorf(vergeio.ErrEndpointV26, "tag_members")
		}
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// First check for 404 - version compatibility issue
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

	endpoint := fmt.Sprintf("%s/%s", TagMembersEndpoint, data.Id.ValueString())
	apiResp, err := ta.client.Delete(endpoint)

	// Error checking with version-aware handling
	if err != nil {
		// Check if this is a 404 error from the client
		if apiError, ok := err.(vergeio.Error); ok && apiError.StatusCode == 404 {
			// Could be endpoint not available or resource already deleted
			tflog.Debug(ctx, "Tag member not found during deletion (may already be deleted)")
			return nil // Treat as success since resource is gone
		}
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	defer apiResp.Body.Close()

	// First check for 404 - resource already deleted
	if apiResp.StatusCode == 404 {
		tflog.Debug(ctx, "Tag member not found during deletion (may already be deleted)")
		return nil // Treat as success since resource is gone
	}

	// Check for any other non-200 status code
	if apiResp.StatusCode != 200 && apiResp.StatusCode != 204 {
		return fmt.Errorf("API returned status code %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, "Successfully deleted tag member")

	return nil
}
