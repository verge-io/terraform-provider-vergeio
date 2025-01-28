// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package cluster

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
	ClusterEndpoint = vergeio.APIEndpoint + "/clusters"
)

// IClient interface.
var _ vergeio.IClient = &ClusterApi{}

func NewClusterApi(c *vergeio.Client) *ClusterApi {
	return &ClusterApi{
		name:   "Cluster Api",
		client: c,
	}
}

type ClusterApi struct {
	name   string
	client *vergeio.Client
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

	// Call the API
	apiResp, err := va.client.Get(ClusterEndpoint,
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
	var clusterAPIResp []ClusterAPIDataSourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&clusterAPIResp); err != nil {
		return errors.New("invalid format received for VM Item")
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
