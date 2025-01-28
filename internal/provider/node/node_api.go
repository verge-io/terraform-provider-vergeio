// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	NodeEndpoint = vergeio.APIEndpoint + "/nodes"
)

var _ vergeio.IClient = &NodeApi{}

func NewNodeApi(c *vergeio.Client) *NodeApi {
	return &NodeApi{
		name:   "Node Api",
		client: c,
	}
}

type NodeApi struct {
	name   string
	client *vergeio.Client
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

	apiResp, err := va.client.Get(NodeEndpoint,
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
	var nodeAPIResp []NodeAPIDataSourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&nodeAPIResp); err != nil {
		return errors.New("invalid format received for VM Item")
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
