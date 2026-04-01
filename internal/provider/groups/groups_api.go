// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package groups

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	vergeos "github.com/verge-io/govergeos"
)

// IClient interface.
var _ vergeio.IClient = &GroupsApi{}

func NewGroupsApi(c *vergeio.Client) *GroupsApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &GroupsApi{
		name:   "Group Api",
		client: c,
		sdk:    sdk,
	}
}

type GroupsApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
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

	// Call the SDK API
	var listOpts []vergeos.ListOption
	if opts.Filter != "" {
		listOpts = append(listOpts, vergeos.WithFilter(opts.Filter))
	}

	groups, err := va.sdk.Groups.List(ctx, listOpts...)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", groups))

	// Convert SDK groups to API model for existing field mapping logic
	var groupsAPIResp []GroupsAPIDataSourceModel
	for _, group := range groups {
		groupsAPIResp = append(groupsAPIResp, GroupsAPIDataSourceModel{
			Id:          int32(group.ID.Int()),
			Name:        group.Name,
			Description: group.Description,
			Enabled:     group.Enabled,
		})
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
