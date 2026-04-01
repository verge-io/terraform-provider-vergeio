// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package resourseGroups

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// API endpoint.

// IClient interface.
var _ vergeio.IClient = &ResourceGroupsApi{}

func NewResourceGroupsApi(c *vergeio.Client) *ResourceGroupsApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &ResourceGroupsApi{
		name:   "Resource Groups Api",
		client: c,
		sdk:    sdk,
	}
}

type ResourceGroupsApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
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

	// Call the SDK API
	var listOpts []vergeos.ListOption
	if opts.Filter != "" {
		listOpts = append(listOpts, vergeos.WithFilter(opts.Filter))
	}

	resourceGroups, err := va.sdk.ResourceGroups.List(ctx, listOpts...)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", resourceGroups))

	// Convert SDK resource groups to API model for existing field mapping logic
	var resourceGroupsAPIResp []ResourceGroupsAPIDataSourceModel
	for _, rg := range resourceGroups {
		resourceGroupsAPIResp = append(resourceGroupsAPIResp, ResourceGroupsAPIDataSourceModel{
			Id:          rg.ID,
			Name:        rg.Name,
			Description: rg.Description,
			Enabled:     rg.Enabled,
			Type:        rg.Type,
			Class:       "", // Class field not available in SDK
		})
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
