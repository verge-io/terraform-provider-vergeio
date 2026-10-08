// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package tenant

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

// API is the govergeos client for tenant objects.
type API struct {
	name string
	sdk  *vergeos.Client
}

func NewAPI(c *vergeio.Client) (*API, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &API{
		name: "Tenant Api",
		sdk:  sdk,
	}, nil
}

func (a *API) Name() string {
	return a.name
}

// RemoveTenant powers the tenant off, waits for its network to stop, and
// deletes it. A missing tenant is already gone. This is the vergeio_tenant
// destroy sequence.
func RemoveTenant(ctx context.Context, sdk *vergeos.Client, id int) error {
	if sdk == nil || id <= 0 {
		return nil
	}
	return (&API{sdk: sdk}).deleteTenant(ctx, &TenantResourceModel{Id: idString(id)})
}

// StartTenantOnce powers the tenant on, waits until it is running, powers
// it off, and waits until its network has stopped. This is the vergeio_tenant
// power-on and power-off sequence. A tenant with no node cannot reach
// running, so the caller creates a node first.
func StartTenantOnce(ctx context.Context, sdk *vergeos.Client, id int) error {
	if sdk == nil || id <= 0 {
		return fmt.Errorf("tenant id is empty")
	}
	api := &API{sdk: sdk}
	if err := api.reconcilePower(ctx, id, types.BoolValue(true), types.Int32Null()); err != nil {
		return err
	}
	return api.ensurePoweredOff(ctx, id)
}

// RemoveTenantNode stops the node when it is running, then deletes it. A
// missing node is already gone. This is the vergeio_tenant_node destroy
// sequence.
func RemoveTenantNode(ctx context.Context, sdk *vergeos.Client, id int) error {
	if sdk == nil || id <= 0 {
		return nil
	}
	return (&API{sdk: sdk}).deleteTenantNode(ctx, &TenantNodeResourceModel{Id: idString(id)})
}

func idString(id int) types.String {
	return types.StringValue(strconv.Itoa(id))
}

func parseID(id types.String, what string) (int, error) {
	if id.IsNull() || id.IsUnknown() {
		return 0, fmt.Errorf("%s id is empty", what)
	}
	text := strings.TrimSpace(id.ValueString())
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s id %q is not a positive integer", what, text)
	}
	return n, nil
}

func knownInt(v types.Int32) *int {
	p := vergeio.KnownInt32(v)
	if p == nil {
		return nil
	}
	n := int(*p)
	return &n
}

func changedInt(plan, state types.Int32) *int {
	p := vergeio.ChangedInt32(plan, state)
	if p == nil {
		return nil
	}
	n := int(*p)
	return &n
}

func flexID(v vergeos.FlexInt) types.Int32 {
	if v.Int() == 0 {
		return types.Int32Null()
	}
	return types.Int32Value(int32(v.Int()))
}

func flexPtr(v *vergeos.FlexInt) types.Int32 {
	if v == nil {
		return types.Int32Null()
	}
	return types.Int32Value(int32(v.Int()))
}

// enumString stores a constrained API string. Blank becomes null so an
// omitted argument stays valid for OneOf validators.
func enumString(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

func timestamp(v int64) types.Int64 {
	if v == 0 {
		return types.Int64Null()
	}
	return types.Int64Value(v)
}
