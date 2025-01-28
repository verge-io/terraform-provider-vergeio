// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package groups

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
	GroupsEndpoint = vergeio.APIEndpoint + "/groups"
)

// IClient interface.
var _ vergeio.IClient = &GroupsApi{}

func NewGroupsApi(c *vergeio.Client) *GroupsApi {
	return &GroupsApi{
		name:   "Group Api",
		client: c,
	}
}

type GroupsApi struct {
	name   string
	client *vergeio.Client
}

func (nc *GroupsApi) Name() string {
	return nc.name
}

// GroupsAPIDataSourceModel describes the data model received from the Verge API.
type GroupsAPIDataSourceModel struct {
	Id          int32  `json:"$key,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Enabled     bool   `json:"enabled,omitempty"`
}

// Read groups from the API.
func (va *GroupsApi) readGroups(ctx context.Context, data *GroupDataSourceModel) error {

	tflog.Debug(ctx, "Reading the groups data")

	// What fields do we want
	opts := vergeio.Options{Fields: "$key,name,description,enabled"}

	// Build filter
	if fn := data.FilterName.ValueString(); fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn)
	}

	// Call the API
	apiResp, err := va.client.Get(GroupsEndpoint,
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
	var groupsAPIResp []GroupsAPIDataSourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&groupsAPIResp); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Convert the API response to a resource
	for _, nwAPIResp := range groupsAPIResp {
		data.Groups = append(data.Groups, &GroupModel{
			Id:          types.Int32Value(nwAPIResp.Id),
			Name:        types.StringValue(nwAPIResp.Name),
			Description: types.StringValue(nwAPIResp.Description),
			Enabled:     types.BoolValue(nwAPIResp.Enabled),
		})

	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
