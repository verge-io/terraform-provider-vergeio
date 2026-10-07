// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var (
	_ action.Action                   = &TenantCloneAction{}
	_ action.ActionWithConfigure      = &TenantCloneAction{}
	_ action.ActionWithValidateConfig = &TenantCloneAction{}
)

func NewTenantCloneAction() action.Action {
	return &TenantCloneAction{}
}

// TenantCloneAction copies a tenant. The copy is not Terraform state.
type TenantCloneAction struct {
	sdk *vergeos.Client
}

type tenantCloneActionModel struct {
	TenantID  types.String `tfsdk:"tenant_id"`
	Name      types.String `tfsdk:"name"`
	NoVNet    types.Bool   `tfsdk:"no_vnet"`
	NoStorage types.Bool   `tfsdk:"no_storage"`
	NoNodes   types.Bool   `tfsdk:"no_nodes"`
}

func (a *TenantCloneAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tenant_clone"
}

func (a *TenantCloneAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Copy a VergeOS tenant. name is the new tenant's name. no_vnet, no_storage, and no_nodes skip the network, the storage, and the nodes. When a flag is omitted, that part is copied. VergeOS accepts the clone before that copy finishes, so a new tenant can still list the source node. When no_nodes or no_storage is true, this action waits until those rows are gone and removes any the copy left behind. The copy is not stored in Terraform state. Import it with vergeio_tenant when you want Terraform to manage it. Invoke this action with `terraform apply -invoke`, or from a resource `lifecycle` `action_trigger`. Requires Terraform 1.14 or later. OpenTofu does not implement actions, and no resource behavior depends on this action.",
		Attributes: map[string]schema.Attribute{
			"tenant_id": schema.StringAttribute{
				MarkdownDescription: "Tenant id, the same value as vergeio_tenant.id.",
				Required:            true,
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the new tenant. VergeOS rejects a name that is already in use.",
				Required:            true,
			},
			"no_vnet": schema.BoolAttribute{
				MarkdownDescription: "Skip copying the tenant network. When omitted, the network is copied.",
				Optional:            true,
			},
			"no_storage": schema.BoolAttribute{
				MarkdownDescription: "Skip copying tenant storage. When omitted, storage is copied.",
				Optional:            true,
			},
			"no_nodes": schema.BoolAttribute{
				MarkdownDescription: "Skip copying tenant nodes. When omitted, nodes are copied.",
				Optional:            true,
			},
		},
	}
}

func (a *TenantCloneAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	sdk, diags := shared.ActionSDK(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.sdk = sdk
}

func (a *TenantCloneAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data tenantCloneActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.TenantID, "tenant_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("tenant_id"), "Invalid Tenant ID", err.Error())
	}
	if _, _, err := knownCloneName(data.Name); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid Clone Name", err.Error())
	}
}

func (a *TenantCloneAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var data tenantCloneActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.sdk == nil {
		resp.Diagnostics.AddError("VergeOS Client Missing", "The provider has not been configured.")
		return
	}
	tenantID, known, err := shared.KnownPositiveID(data.TenantID, "tenant_id")
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("tenant_id is required")
		}
		resp.Diagnostics.AddAttributeError(path.Root("tenant_id"), "Invalid Tenant ID", err.Error())
		return
	}
	name, known, err := knownCloneName(data.Name)
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("name is required")
		}
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid Clone Name", err.Error())
		return
	}
	noVNet, ok := cloneFlag(data.NoVNet)
	if !ok {
		resp.Diagnostics.AddAttributeError(path.Root("no_vnet"), "Invalid Clone Option", "no_vnet is unknown")
		return
	}
	noStorage, ok := cloneFlag(data.NoStorage)
	if !ok {
		resp.Diagnostics.AddAttributeError(path.Root("no_storage"), "Invalid Clone Option", "no_storage is unknown")
		return
	}
	noNodes, ok := cloneFlag(data.NoNodes)
	if !ok {
		resp.Diagnostics.AddAttributeError(path.Root("no_nodes"), "Invalid Clone Option", "no_nodes is unknown")
		return
	}
	shared.ReportProgress(resp, fmt.Sprintf("Cloning tenant %d as %s", tenantID, name))
	err = a.sdk.Tenants.Clone(ctx, tenantID, &vergeos.TenantCloneOptions{
		Name:      name,
		NoVNet:    noVNet,
		NoStorage: noStorage,
		NoNodes:   noNodes,
	})
	if err != nil {
		resp.Diagnostics.AddError("Error Cloning Tenant", err.Error())
		return
	}
	if noStorage || noNodes {
		shared.ReportProgress(resp, fmt.Sprintf("Waiting for clone %s to finish skipping nodes and storage", name))
		if err := a.finishCloneExclusions(ctx, name, noStorage, noNodes); err != nil {
			resp.Diagnostics.AddError("Error Cloning Tenant", err.Error())
			return
		}
	}
	tflog.Info(ctx, fmt.Sprintf("cloned tenant %d as %s", tenantID, name))
	shared.ReportProgress(resp, fmt.Sprintf("Cloned tenant %d as %s", tenantID, name))
}

// cloneExclusionPoll is how often a clone with no_nodes or no_storage is read.
// cloneExclusionSettle is how long those rows must stay before they are removed.
// cloneExclusionTimeout is the whole wait, including that removal.
var (
	cloneExclusionPoll    = 2 * time.Second
	cloneExclusionSettle  = 20 * time.Second
	cloneExclusionTimeout = 2 * time.Minute
)

// finishCloneExclusions waits until a clone that asked to skip nodes or storage
// no longer has those rows. TenantService.Clone posts params.no_nodes and
// params.no_storage and returns when VergeOS accepts the action. The new
// tenant is visible first, still carrying the source node, and the copy
// drops that node later. A row that is still there after cloneExclusionSettle
// is deleted so the skip is the state the action reports.
func (a *TenantCloneAction) finishCloneExclusions(ctx context.Context, name string, noStorage, noNodes bool) error {
	deadline := time.Now().Add(cloneExclusionTimeout)
	var pendingSince time.Time
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		tenant, err := a.sdk.Tenants.GetByName(ctx, name)
		if err != nil {
			if !vergeos.IsNotFoundError(err) || !time.Now().Before(deadline) {
				return fmt.Errorf("tenant %q: %w", name, err)
			}
			if err := sleepCloneExclusion(ctx); err != nil {
				return err
			}
			continue
		}
		tenantID := tenant.Key.Int()
		nodes, err := cloneTenantNodes(ctx, a.sdk, tenantID)
		if err != nil {
			return err
		}
		storage, err := cloneTenantStorage(ctx, a.sdk, tenantID)
		if err != nil {
			return err
		}
		pendingNodes := noNodes && len(nodes) > 0
		pendingStorage := noStorage && len(storage) > 0
		if !pendingNodes && !pendingStorage {
			return nil
		}
		if pendingSince.IsZero() {
			pendingSince = time.Now()
		}
		if time.Since(pendingSince) >= cloneExclusionSettle {
			if pendingNodes {
				if err := deleteCloneNodes(ctx, a.sdk, tenantID, nodes); err != nil {
					return err
				}
			}
			if pendingStorage {
				if err := deleteCloneStorage(ctx, a.sdk, tenantID, storage); err != nil {
					return err
				}
			}
			pendingSince = time.Time{}
			continue
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("cloned tenant %q has %d nodes and %d storage rows; no_nodes=%t no_storage=%t", name, len(nodes), len(storage), noNodes, noStorage)
		}
		if err := sleepCloneExclusion(ctx); err != nil {
			return err
		}
	}
}

func cloneTenantNodes(ctx context.Context, sdk *vergeos.Client, tenantID int) ([]vergeos.TenantNode, error) {
	nodes, err := sdk.TenantNodes.ListByTenant(ctx, tenantID)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return nil, err
	}
	return nodes, nil
}

func cloneTenantStorage(ctx context.Context, sdk *vergeos.Client, tenantID int) ([]vergeos.TenantStorage, error) {
	storage, err := sdk.TenantStorage.ListByTenant(ctx, tenantID)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return nil, err
	}
	return storage, nil
}

func deleteCloneNodes(ctx context.Context, sdk *vergeos.Client, tenantID int, nodes []vergeos.TenantNode) error {
	for _, node := range nodes {
		if owner := node.Tenant.Int(); owner != 0 && owner != tenantID {
			return fmt.Errorf("tenant node %d belongs to tenant %d, not clone %d", node.Key.Int(), owner, tenantID)
		}
		nodeID := node.Key.Int()
		if err := sdk.TenantNodes.Kill(ctx, nodeID); err != nil && !vergeos.IsNotFoundError(err) {
			if err := sdk.TenantNodes.Delete(ctx, nodeID); err != nil && !vergeos.IsNotFoundError(err) {
				return fmt.Errorf("delete tenant node %d: %w", nodeID, err)
			}
			continue
		}
		if err := sdk.TenantNodes.Delete(ctx, nodeID); err != nil && !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("delete tenant node %d: %w", nodeID, err)
		}
	}
	return nil
}

func deleteCloneStorage(ctx context.Context, sdk *vergeos.Client, tenantID int, storage []vergeos.TenantStorage) error {
	for _, row := range storage {
		if owner := row.Tenant.Int(); owner != 0 && owner != tenantID {
			return fmt.Errorf("tenant storage %d belongs to tenant %d, not clone %d", row.Key.Int(), owner, tenantID)
		}
		rowID := row.Key.Int()
		if err := sdk.TenantStorage.Delete(ctx, rowID); err != nil && !vergeos.IsNotFoundError(err) {
			return fmt.Errorf("delete tenant storage %d: %w", rowID, err)
		}
	}
	return nil
}

func sleepCloneExclusion(ctx context.Context) error {
	timer := time.NewTimer(cloneExclusionPoll)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// knownCloneName returns the trimmed clone name.
// A null value is an error. An unknown value is not known yet and is not an error.
func knownCloneName(value types.String) (string, bool, error) {
	if value.IsNull() {
		return "", false, fmt.Errorf("name is required")
	}
	if value.IsUnknown() {
		return "", false, nil
	}
	name := strings.TrimSpace(value.ValueString())
	if name == "" {
		return "", false, fmt.Errorf("name is required")
	}
	return name, true, nil
}

// cloneFlag reads an optional skip flag. Null means copy that part. Unknown is not known yet.
func cloneFlag(value types.Bool) (bool, bool) {
	if value.IsNull() {
		return false, true
	}
	if value.IsUnknown() {
		return false, false
	}
	return value.ValueBool(), true
}
