// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package cluster

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)


// IClient interface.
var _ vergeio.IClient = &ClusterApi{}

func NewClusterApi(c *vergeio.Client) *ClusterApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &ClusterApi{
		name:   "Cluster Api",
		client: c,
		sdk:    sdk,
	}
}

type ClusterApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *ClusterApi) Name() string {
	return nc.name
}

// ClusterAPIDataSourceModel represents the data model received from the API.
type ClusterAPIDataSourceModel struct {
	Id          int32  `json:"$key,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// Read the Clusters from the API.
func (va *ClusterApi) readClusters(ctx context.Context, data *ClusterDataSourceModel) error {

	tflog.Debug(ctx, "Reading the cluster data")

	// What fields do we want
	opts := vergeio.Options{Fields: "description,name,$key"}

	// Build filter
	if fn := data.FilterName.ValueString(); fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn)
	}

	// Call the SDK API
	var listOpts []vergeos.ListOption
	if opts.Filter != "" {
		listOpts = append(listOpts, vergeos.WithFilter(opts.Filter))
	}

	clusters, err := va.sdk.Clusters.List(ctx, listOpts...)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", clusters))

	// Convert SDK clusters to API model for existing field mapping logic
	var clusterAPIResp []ClusterAPIDataSourceModel
	for _, cluster := range clusters {
		clusterAPIResp = append(clusterAPIResp, ClusterAPIDataSourceModel{
			Id:          int32(cluster.Key.Int()),
			Name:        cluster.Name,
			Description: cluster.Description,
		})
	}

	// save into the resource model
	for _, nwAPIResp := range clusterAPIResp {
		data.Clusters = append(data.Clusters, &ClusterModel{
			Id:          types.Int32Value(nwAPIResp.Id),
			Name:        types.StringValue(nwAPIResp.Name),
			Description: types.StringValue(nwAPIResp.Description),
		})

	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
