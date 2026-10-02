// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func tenantNodeCreateRequest(data *TenantNodeResourceModel) (*vergeos.TenantNodeCreateRequest, error) {
	tenantID, err := parseID(data.TenantID, "tenant")
	if err != nil {
		return nil, err
	}
	req := &vergeos.TenantNodeCreateRequest{
		Tenant:          tenantID,
		CPUCores:        int(data.CPUCores.ValueInt32()),
		RAM:             int(data.RAM.ValueInt32()),
		Enabled:         vergeio.KnownBool(data.Enabled),
		Cluster:         knownInt(data.Cluster),
		ClusterFailover: knownInt(data.ClusterFailover),
		PreferredNode:   knownInt(data.PreferredNode),
		HAGroup:         vergeio.KnownString(data.HAGroup),
		OnPowerLoss:     vergeio.KnownString(data.OnPowerLoss),
	}
	if name := vergeio.KnownString(data.Name); name != nil {
		req.Name = *name
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	return req, nil
}

func tenantNodeUpdateRequest(plan, state *TenantNodeResourceModel) *vergeos.TenantNodeUpdateRequest {
	req := &vergeos.TenantNodeUpdateRequest{
		Name:            vergeio.ChangedString(plan.Name, state.Name),
		Description:     vergeio.ChangedString(plan.Description, state.Description),
		Enabled:         vergeio.ChangedBool(plan.Enabled, state.Enabled),
		CPUCores:        changedInt(plan.CPUCores, state.CPUCores),
		RAM:             changedInt(plan.RAM, state.RAM),
		Cluster:         changedInt(plan.Cluster, state.Cluster),
		ClusterFailover: changedInt(plan.ClusterFailover, state.ClusterFailover),
		PreferredNode:   changedInt(plan.PreferredNode, state.PreferredNode),
		HAGroup:         vergeio.ChangedString(plan.HAGroup, state.HAGroup),
		OnPowerLoss:     vergeio.ChangedString(plan.OnPowerLoss, state.OnPowerLoss),
	}
	if tenantNodeUpdateEmpty(req) {
		return nil
	}
	return req
}

func tenantNodeUpdateEmpty(req *vergeos.TenantNodeUpdateRequest) bool {
	if req == nil {
		return true
	}
	return req.Name == nil &&
		req.Description == nil &&
		req.Enabled == nil &&
		req.CPUCores == nil &&
		req.RAM == nil &&
		req.Cluster == nil &&
		req.ClusterFailover == nil &&
		req.PreferredNode == nil &&
		req.HAGroup == nil &&
		req.OnPowerLoss == nil
}

func (a *API) createTenantNode(ctx context.Context, data *TenantNodeResourceModel) error {
	req, err := tenantNodeCreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.TenantNodes.Create(ctx, req)
	if err != nil {
		return err
	}
	data.Id = idString(created.Key.Int())
	tflog.Debug(ctx, fmt.Sprintf("created tenant node %d", created.Key.Int()))
	return nil
}

func (a *API) updateTenantNode(ctx context.Context, plan, state *TenantNodeResourceModel) error {
	id, err := parseID(state.Id, "tenant node")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	plan.TenantID = state.TenantID
	req := tenantNodeUpdateRequest(plan, state)
	if req == nil {
		return nil
	}
	if _, err := a.sdk.TenantNodes.Update(ctx, id, req); err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("updated tenant node %d", id))
	return nil
}

func (a *API) readTenantNode(ctx context.Context, data *TenantNodeResourceModel) error {
	id, err := parseID(data.Id, "tenant node")
	if err != nil {
		return err
	}
	node, err := a.sdk.TenantNodes.Get(ctx, id)
	if err != nil {
		return err
	}
	assignTenantNode(data, node)
	return nil
}

func assignTenantNode(data *TenantNodeResourceModel, node *vergeos.TenantNode) {
	data.Id = idString(node.Key.Int())
	if node.Tenant.Int() > 0 {
		data.TenantID = idString(node.Tenant.Int())
	}
	data.Name = types.StringValue(node.Name)
	data.Description = types.StringValue(node.Description)
	data.Enabled = types.BoolValue(node.Enabled)
	data.CPUCores = types.Int32Value(int32(node.CPUCores))
	data.RAM = types.Int32Value(int32(node.RAM))
	data.Cluster = flexPtr(node.Cluster)
	data.ClusterFailover = flexPtr(node.ClusterFailover)
	data.PreferredNode = flexPtr(node.PreferredNode)
	data.HAGroup = types.StringValue(node.HAGroup)
	data.OnPowerLoss = enumString(node.OnPowerLoss)
	if node.NodeID > 0 {
		data.NodeID = types.Int32Value(int32(node.NodeID))
	} else {
		data.NodeID = types.Int32Null()
	}
	data.Machine = flexID(node.Machine)
	data.IsSnapshot = types.BoolValue(node.IsSnapshot)
	data.Creator = types.StringValue(node.Creator)
	data.Created = timestamp(node.Created)
	data.Modified = timestamp(node.Modified)
}

// deleteTenantNode powers the parent tenant off first. Terraform destroys
// vergeio_tenant_node before vergeio_tenant because of the tenant_id
// reference, and VergeOS rejects deleting a node while the tenant runs (#195).
func (a *API) deleteTenantNode(ctx context.Context, data *TenantNodeResourceModel) error {
	id, err := parseID(data.Id, "tenant node")
	if err != nil {
		return err
	}
	if tenantID, err := parseID(data.TenantID, "tenant"); err == nil && tenantID > 0 {
		if err := a.ensurePoweredOff(ctx, tenantID); err != nil {
			return fmt.Errorf("power off tenant %d before deleting node %d: %w", tenantID, id, err)
		}
	}
	if err := a.sdk.TenantNodes.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant node %d", id))
	return nil
}
