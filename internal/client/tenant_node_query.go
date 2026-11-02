// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

const (
	tenantNodeEndpoint      = APIEndpoint + "/tenant_nodes"
	tenantNodeQueryEndpoint = APIEndpoint + "/tenant_node_queries"
)

// tenantNodeTCPQuery is the POST body for a tcp_connect diagnostic.
// The platform runs it inside the tenant node with lxc-attach.
// govergeos has no tenant_node_queries method.
type tenantNodeTCPQuery struct {
	TenantNode int               `json:"tenant_node"`
	Query      string            `json:"query"`
	Params     map[string]string `json:"params"`
}

// TenantNodeDBVersion reads tenant_nodes.db_version for one node.
// An empty string means the node has not reported a version yet.
func (c *Client) TenantNodeDBVersion(ctx context.Context, id int) (string, error) {
	if c == nil {
		return "", fmt.Errorf("vergeio client is nil")
	}
	if id <= 0 {
		return "", fmt.Errorf("tenant node id %d is not a positive integer", id)
	}
	resp, err := c.Get(ctx, ObjectPath(tenantNodeEndpoint, strconv.Itoa(id)), &Options{
		Fields: "$key,db_version",
	})
	if err != nil {
		return "", err
	}
	body, err := readResponse(resp)
	if err != nil {
		return "", err
	}
	var row struct {
		DBVersion string `json:"db_version"`
	}
	if err := json.Unmarshal(body, &row); err != nil {
		return "", fmt.Errorf("tenant node %d: %w", id, err)
	}
	return row.DBVersion, nil
}

// CreateTenantNodeTCPQuery posts tenant_node_queries tcp_connect to
// 127.0.0.1 port 443 and returns the new query id.
func (c *Client) CreateTenantNodeTCPQuery(ctx context.Context, nodeID int) (string, error) {
	if nodeID <= 0 {
		return "", fmt.Errorf("tenant node id %d is not a positive integer", nodeID)
	}
	return c.postKey(ctx, tenantNodeQueryEndpoint, tenantNodeTCPQuery{
		TenantNode: nodeID,
		Query:      "tcp_connect",
		Params: map[string]string{
			"host": "127.0.0.1",
			"port": "443",
		},
	})
}

// GetTenantNodeQuery reads one tenant_node_queries row.
// status is "running" while the query is still going.
// result is the text the query returned.
func (c *Client) GetTenantNodeQuery(ctx context.Context, id string) (string, string, error) {
	if c == nil {
		return "", "", fmt.Errorf("vergeio client is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", fmt.Errorf("tenant node query id is empty")
	}
	resp, err := c.Get(ctx, ObjectPath(tenantNodeQueryEndpoint, id), &Options{
		Fields: "$key,status,result",
	})
	if err != nil {
		return "", "", err
	}
	body, err := readResponse(resp)
	if err != nil {
		return "", "", err
	}
	var row struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(body, &row); err != nil {
		return "", "", fmt.Errorf("tenant node query %s: %w", id, err)
	}
	return row.Status, strings.Trim(string(row.Result), `"`), nil
}

// DeleteTenantNodeQuery removes one tenant_node_queries row.
// A missing row is success.
func (c *Client) DeleteTenantNodeQuery(ctx context.Context, id string) error {
	return c.deleteHex(ctx, tenantNodeQueryEndpoint, id)
}
