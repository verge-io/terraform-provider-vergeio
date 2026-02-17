// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package node

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)


var _ vergeio.IClient = &NodeApi{}

func NewNodeApi(c *vergeio.Client) *NodeApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &NodeApi{
		name:   "Node Api",
		client: c,
		sdk:    sdk,
	}
}

type NodeApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *NodeApi) Name() string {
	return nc.name
}

type NodeAPIDataSourceModel struct {
	Id          int32  `json:"$key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Read the VM from the API.
func (va *NodeApi) readNodes(ctx context.Context, data *NodeDataSourceModel) error {

	tflog.Debug(ctx, "Reading the node data")

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

	nodes, err := va.sdk.Nodes.List(ctx, listOpts...)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", nodes))

	// Convert SDK nodes to API model for existing field mapping logic
	var nodeAPIResp []NodeAPIDataSourceModel
	for _, node := range nodes {
		nodeAPIResp = append(nodeAPIResp, NodeAPIDataSourceModel{
			Id:          int32(node.ID),
			Name:        node.Name,
			Description: node.Description,
		})
	}

	for _, nwAPIResp := range nodeAPIResp {

		data.Nodes = append(data.Nodes, &NodeModel{
			Id:          types.Int32Value(nwAPIResp.Id),
			Name:        types.StringValue(nwAPIResp.Name),
			Description: types.StringValue(nwAPIResp.Description),
		})

	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
