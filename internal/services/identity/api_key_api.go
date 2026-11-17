// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"errors"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// errStaleIdentity means the stored key now points at a different object.
// VergeOS reuses numeric keys. Read removes the resource so the next apply
// creates another one instead of adopting the foreign row.
var errStaleIdentity = errors.New("stored key now points at a different object")

var _ vergeio.IClient = &APIKeyApi{}

func NewAPIKeyApi(c *vergeio.Client) (*APIKeyApi, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &APIKeyApi{
		name: "API Key Api",
		sdk:  sdk,
	}, nil
}

// APIKeyApi is the govergeos UserAPIKeyService client for the managed
// vergeio_api_key resource.
type APIKeyApi struct {
	name string
	sdk  *vergeos.Client
}

func (api *APIKeyApi) Name() string {
	return api.name
}

func apiKeyCreateRequest(data *APIKeyResourceModel) *vergeos.UserAPIKeyCreateRequest {
	req := &vergeos.UserAPIKeyCreateRequest{
		User: int(data.UserID.ValueInt32()),
		Name: data.Name.ValueString(),
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	if allow := vergeio.KnownString(data.IPAllowList); allow != nil {
		req.IPAllowList = *allow
	}
	if deny := vergeio.KnownString(data.IPDenyList); deny != nil {
		req.IPDenyList = *deny
	}
	expiresType, expires := apiKeyExpires(data.Expires, types.Int64Null(), false)
	if expiresType != nil {
		req.ExpiresType = *expiresType
	}
	req.Expires = expires
	return req
}

func apiKeyUpdateRequest(plan, state *APIKeyResourceModel) *vergeos.UserAPIKeyUpdateRequest {
	req := &vergeos.UserAPIKeyUpdateRequest{
		Name:        vergeio.ChangedString(plan.Name, state.Name),
		Description: vergeio.ChangedString(plan.Description, state.Description),
		IPAllowList: vergeio.ChangedString(plan.IPAllowList, state.IPAllowList),
		IPDenyList:  vergeio.ChangedString(plan.IPDenyList, state.IPDenyList),
	}
	expiresType, expires := apiKeyExpires(plan.Expires, state.Expires, true)
	req.ExpiresType = expiresType
	req.Expires = expires
	if req.Name == nil && req.Description == nil && req.IPAllowList == nil && req.IPDenyList == nil && req.ExpiresType == nil && req.Expires == nil {
		return nil
	}
	return req
}

// apiKeyExpires maps the optional expires timestamp to the VergeOS expiry
// pair. Null means the key does not expire. On update, a null plan after a
// stored timestamp sends expires_type never and expires 0 so the old date
// is cleared. An unchanged value is omitted.
func apiKeyExpires(plan, state types.Int64, hasState bool) (*string, *int64) {
	planSet := !plan.IsNull() && !plan.IsUnknown()
	if !hasState {
		if !planSet {
			never := vergeos.APIKeyExpiresNever
			return &never, nil
		}
		date := vergeos.APIKeyExpiresDate
		value := plan.ValueInt64()
		return &date, &value
	}
	stateSet := !state.IsNull() && !state.IsUnknown()
	if !planSet && !stateSet {
		return nil, nil
	}
	if !planSet && stateSet {
		never := vergeos.APIKeyExpiresNever
		zero := int64(0)
		return &never, &zero
	}
	if stateSet && plan.ValueInt64() == state.ValueInt64() {
		return nil, nil
	}
	date := vergeos.APIKeyExpiresDate
	value := plan.ValueInt64()
	return &date, &value
}

func applyAPIKey(data *APIKeyResourceModel, key *vergeos.UserAPIKey) {
	data.Id = idString(key.Key.Int())
	if key.User.Int() > 0 {
		data.UserID = types.Int32Value(int32(key.User.Int()))
	}
	data.Name = types.StringValue(key.Name)
	data.Description = types.StringValue(key.Description)
	data.IPAllowList = types.StringValue(key.IPAllowList)
	data.IPDenyList = types.StringValue(key.IPDenyList)
	if key.Expires > 0 {
		data.Expires = types.Int64Value(key.Expires)
	} else {
		data.Expires = types.Int64Null()
	}
	data.Created = types.Int64Value(key.Created)
	data.LastLoginStamp = types.Int64Value(key.LastLoginStamp)
	data.LastLoginIP = types.StringValue(key.LastLoginIP)
}

func apiKeyReused(data *APIKeyResourceModel, key *vergeos.UserAPIKey) bool {
	if data == nil || key == nil || data.UserID.IsNull() || data.UserID.IsUnknown() {
		return false
	}
	got := key.User.Int()
	if got <= 0 {
		return false
	}
	return int(data.UserID.ValueInt32()) != got
}

func (api *APIKeyApi) createAPIKey(ctx context.Context, data *APIKeyResourceModel) error {
	// The token is returned only here. It is discarded. The managed resource
	// does not store it. Ephemeral vergeio_api_key is the token used in a run.
	created, _, err := api.sdk.UserAPIKeys.Create(ctx, apiKeyCreateRequest(data))
	if err != nil {
		return err
	}
	if created == nil || created.Key.Int() <= 0 {
		return fmt.Errorf("VergeOS did not return an API key id")
	}
	applyAPIKey(data, created)
	tflog.Debug(ctx, fmt.Sprintf("created API key %s", data.Id.ValueString()))
	return nil
}

func (api *APIKeyApi) readAPIKey(ctx context.Context, data *APIKeyResourceModel) error {
	id, err := parseID(data.Id, "API key")
	if err != nil {
		return err
	}
	key, err := api.sdk.UserAPIKeys.Get(ctx, id)
	if err != nil {
		return err
	}
	if apiKeyReused(data, key) {
		return errStaleIdentity
	}
	applyAPIKey(data, key)
	tflog.Debug(ctx, fmt.Sprintf("read API key %d", id))
	return nil
}

func (api *APIKeyApi) updateAPIKey(ctx context.Context, plan, state *APIKeyResourceModel) error {
	id, err := parseID(state.Id, "API key")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	req := apiKeyUpdateRequest(plan, state)
	if req == nil {
		tflog.Debug(ctx, fmt.Sprintf("API key %d is unchanged", id))
		return nil
	}
	updated, err := api.sdk.UserAPIKeys.Update(ctx, id, req)
	if err != nil {
		return err
	}
	applyAPIKey(plan, updated)
	tflog.Debug(ctx, fmt.Sprintf("updated API key %d", id))
	return nil
}

func (api *APIKeyApi) deleteAPIKey(ctx context.Context, data *APIKeyResourceModel) error {
	id, err := parseID(data.Id, "API key")
	if err != nil {
		return err
	}
	if err := api.sdk.UserAPIKeys.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted API key %d", id))
	return nil
}
