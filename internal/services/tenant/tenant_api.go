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

var _ vergeio.IClient = &API{}

func tenantCreateRequest(data *TenantResourceModel) *vergeos.TenantCreateRequest {
	req := &vergeos.TenantCreateRequest{
		Name: data.Name.ValueString(),
	}
	if password := vergeio.KnownString(data.Password); password != nil {
		req.Password = *password
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	if url := vergeio.KnownString(data.URL); url != nil {
		req.URL = *url
	}
	req.OIDCApplication = knownInt(data.OIDCApplication)
	req.ExposeCloudSnapshots = vergeio.KnownBool(data.ExposeCloudSnapshots)
	req.AllowBranding = vergeio.KnownBool(data.AllowBranding)
	req.ChangePassword = vergeio.KnownBool(data.ChangePassword)
	req.ThemeAccess = vergeio.KnownString(data.ThemeAccess)
	req.HelpURL = vergeio.KnownString(data.HelpURL)
	req.Note = vergeio.KnownString(data.Note)
	return req
}

func tenantUpdateRequest(plan, state *TenantResourceModel) *vergeos.TenantUpdateRequest {
	req := &vergeos.TenantUpdateRequest{
		Name:                 vergeio.ChangedString(plan.Name, state.Name),
		Password:             vergeio.ChangedString(plan.Password, state.Password),
		Description:          vergeio.ChangedString(plan.Description, state.Description),
		URL:                  vergeio.ChangedString(plan.URL, state.URL),
		OIDCApplication:      changedInt(plan.OIDCApplication, state.OIDCApplication),
		ExposeCloudSnapshots: vergeio.ChangedBool(plan.ExposeCloudSnapshots, state.ExposeCloudSnapshots),
		AllowBranding:        vergeio.ChangedBool(plan.AllowBranding, state.AllowBranding),
		ThemeAccess:          vergeio.ChangedString(plan.ThemeAccess, state.ThemeAccess),
		HelpURL:              vergeio.ChangedString(plan.HelpURL, state.HelpURL),
		Note:                 vergeio.ChangedString(plan.Note, state.Note),
	}
	if tenantUpdateEmpty(req) {
		return nil
	}
	return req
}

func tenantUpdateEmpty(req *vergeos.TenantUpdateRequest) bool {
	if req == nil {
		return true
	}
	return req.Name == nil &&
		req.Password == nil &&
		req.Description == nil &&
		req.URL == nil &&
		req.OIDCApplication == nil &&
		req.ExposeCloudSnapshots == nil &&
		req.AllowBranding == nil &&
		req.ThemeAccess == nil &&
		req.HelpURL == nil &&
		req.Note == nil
}

func (a *API) createTenant(ctx context.Context, data *TenantResourceModel) error {
	created, err := a.sdk.Tenants.Create(ctx, tenantCreateRequest(data))
	if err != nil {
		return err
	}
	id := created.Key.Int()
	data.Id = idString(id)
	tflog.Debug(ctx, fmt.Sprintf("created tenant %d", id))
	return nil
}

func (a *API) updateTenant(ctx context.Context, plan, state *TenantResourceModel) error {
	id, err := parseID(state.Id, "tenant")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	if req := tenantUpdateRequest(plan, state); req != nil {
		if _, err := a.sdk.Tenants.Update(ctx, id, req); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("updated tenant %d", id))
	}
	return a.reconcilePower(ctx, id, plan.PowerState, plan.PreferredNode)
}

func (a *API) readTenant(ctx context.Context, data *TenantResourceModel) error {
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		return err
	}
	tenant, err := a.sdk.Tenants.Get(ctx, id)
	if err != nil {
		return err
	}
	assignTenantResource(data, tenant)
	return a.readTenantRuntime(ctx, data, tenant)
}

func assignTenantResource(data *TenantResourceModel, tenant *vergeos.Tenant) {
	data.Id = idString(tenant.Key.Int())
	data.Name = types.StringValue(tenant.Name)
	data.Description = types.StringValue(tenant.Description)
	data.URL = types.StringValue(tenant.URL)
	data.OIDCApplication = flexPtr(tenant.OIDCApplication)
	data.ExposeCloudSnapshots = types.BoolValue(tenant.ExposeCloudSnapshots)
	data.AllowBranding = types.BoolValue(tenant.AllowBranding)
	// change_password is create-only. VergeOS clears it after first login.
	// Refreshing that false would disagree with a configured true and replace the tenant.
	data.ThemeAccess = enumString(tenant.ThemeAccess)
	data.HelpURL = types.StringValue(tenant.HelpURL)
	data.Note = types.StringValue(tenant.Note)
	data.UUID = types.StringValue(tenant.UUID)
	data.VNet = flexID(tenant.VNet)
	data.Isolate = types.BoolValue(tenant.Isolate)
	data.IsSnapshot = types.BoolValue(tenant.IsSnapshot)
	data.Creator = types.StringValue(tenant.Creator)
	data.Created = timestamp(tenant.Created)
}

func (a *API) readTenantRuntime(ctx context.Context, data *TenantResourceModel, tenant *vergeos.Tenant) error {
	facts, err := a.tenantFacts(ctx, tenant.Key.Int(), tenant.UIAddress.Int())
	if err != nil {
		return err
	}
	data.UIAddressID = facts.UIAddressID
	data.UIAddress = facts.UIAddress
	data.PowerState = facts.PowerState
	data.Status = facts.Status
	data.State = facts.State
	return nil
}

type tenantFacts struct {
	UIAddressID types.Int32
	UIAddress   types.String
	PowerState  types.Bool
	Status      types.String
	State       types.String
}

func (a *API) tenantFacts(ctx context.Context, tenantID, uiAddressID int) (tenantFacts, error) {
	facts := tenantFacts{
		UIAddressID: types.Int32Null(),
		UIAddress:   types.StringNull(),
		PowerState:  types.BoolValue(false),
		Status:      types.StringNull(),
		State:       types.StringNull(),
	}
	if uiAddressID > 0 {
		facts.UIAddressID = types.Int32Value(int32(uiAddressID))
		address, err := a.sdk.VNetAddresses.Get(ctx, uiAddressID)
		if err != nil && !vergeos.IsNotFoundError(err) {
			return facts, err
		}
		if err == nil && address.IP != "" {
			facts.UIAddress = types.StringValue(address.IP)
		}
	}
	status, err := a.sdk.TenantStatus.Get(ctx, tenantID)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return facts, nil
		}
		return facts, err
	}
	facts.PowerState = types.BoolValue(tenantPoweredOn(status))
	facts.Status = enumString(status.Status)
	facts.State = enumString(status.State)
	return facts, nil
}

// deleteTenant powers the tenant off when it is not yet terminal offline,
// waits for the tenant vnet to stop, then deletes it. VergeOS rejects
// deletion of a running tenant and of a tenant whose network is still
// running (#205). A missing tenant is already gone. ensurePoweredOff waits
// through starting/stopping so destroy does not race a transitional status
// (#196).
func (a *API) deleteTenant(ctx context.Context, data *TenantResourceModel) error {
	id, err := parseID(data.Id, "tenant")
	if err != nil {
		return err
	}
	if err := a.ensurePoweredOff(ctx, id); err != nil {
		return err
	}
	if err := a.sdk.Tenants.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted tenant %d", id))
	return nil
}
