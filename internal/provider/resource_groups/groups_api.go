// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package resourseGroups

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// API endpoint.
const (
	ResourceGroupsEndpoint = vergeio.APIEndpoint + "/resource_groups"
)

// IClient interface.
var _ vergeio.IClient = &ResourceGroupsApi{}

func NewResourceGroupsApi(c *vergeio.Client) *ResourceGroupsApi {
	return &ResourceGroupsApi{
		name:   "Resource Groups Api",
		client: c,
	}
}

type ResourceGroupsApi struct {
	name   string
	client *vergeio.Client
}

func (nc *ResourceGroupsApi) Name() string {
	return nc.name
}

// ResourceGroupsAPIDataSourceModel describes the data model received from the Verge API.
type ResourceGroupsAPIDataSourceModel struct {
	Id          string `json:"uuid,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled,omitempty"`
	Type        string `json:"type,omitempty"`
	Class       string `json:"class,omitempty"`
}

// Read groups from the API.
func (va *ResourceGroupsApi) readResourceGroups(ctx context.Context, data *ResourceGroupDataSourceModel) error {

	tflog.Debug(ctx, "Reading the resource groups data")

	// What fields do we want
	opts := vergeio.Options{Fields: "most"}

	// Build filter
	if fn := data.FilterName.ValueString(); fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn)
	}

	// Call the API
	apiResp, err := va.client.Get(ResourceGroupsEndpoint,
		&opts)

	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", apiResp.Body))

	// Decode the API response
	var resourceGroupsAPIResp []ResourceGroupsAPIDataSourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&resourceGroupsAPIResp); err != nil {
		return errors.New("invalid format received for resource group Item")
	}

	// Convert the API response to a resource
	for _, nwAPIResp := range resourceGroupsAPIResp {
		data.ResourceGroups = append(data.ResourceGroups, &ResourceGroupModel{
			Id:          types.StringValue(nwAPIResp.Id),
			Name:        types.StringValue(nwAPIResp.Name),
			Description: types.StringValue(nwAPIResp.Description),
			Enabled:     types.BoolValue(nwAPIResp.Enabled),
			Type:        types.StringValue(nwAPIResp.Type),
			Class:       types.StringValue(nwAPIResp.Class),
		})

	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
