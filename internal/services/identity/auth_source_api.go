// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var _ vergeio.IClient = &AuthSourceApi{}

func NewAuthSourceApi(c *vergeio.Client) (*AuthSourceApi, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &AuthSourceApi{
		name: "Auth Source Api",
		sdk:  sdk,
	}, nil
}

// AuthSourceApi is the govergeos AuthSourceService client for vergeio_auth_source.
type AuthSourceApi struct {
	name string
	sdk  *vergeos.Client
}

func (api *AuthSourceApi) Name() string {
	return api.name
}

func authSourceCreateRequest(data *AuthSourceResourceModel, secret string) (*vergeos.AuthSourceCreateRequest, error) {
	req := &vergeos.AuthSourceCreateRequest{
		Name:   data.Name.ValueString(),
		Driver: data.Driver.ValueString(),
	}
	doc, err := settingsDocument(data.Settings, secret)
	if err != nil {
		return nil, err
	}
	req.Settings = doc
	if menu := vergeio.KnownBool(data.Menu); menu != nil {
		req.Menu = *menu
	}
	if debug := vergeio.KnownBool(data.Debug); debug != nil {
		req.Debug = *debug
	}
	if color := vergeio.KnownString(data.ButtonBackgroundColor); color != nil {
		req.ButtonBackgroundColor = *color
	}
	if color := vergeio.KnownString(data.ButtonColor); color != nil {
		req.ButtonColor = *color
	}
	if icon := vergeio.KnownString(data.ButtonFAIcon); icon != nil {
		req.ButtonFAIcon = *icon
	}
	if color := vergeio.KnownString(data.IconColor); color != nil {
		req.IconColor = *color
	}
	return req, nil
}

// authSourceUpdateRequest sends changed columns. Settings is the configured
// non-secret document, plus client_secret when the write-only version
// changed. govergeos reads the stored document and merges this on top before
// the PUT, because the API replaces settings and a partial object would
// delete client_secret and every omitted key. A key removed from settings
// is therefore kept on the auth source. Replace the auth source to drop it.
func authSourceUpdateRequest(plan, state *AuthSourceResourceModel, secret string) (*vergeos.AuthSourceUpdateRequest, error) {
	req := &vergeos.AuthSourceUpdateRequest{
		Name:                  vergeio.ChangedString(plan.Name, state.Name),
		Menu:                  vergeio.ChangedBool(plan.Menu, state.Menu),
		Debug:                 vergeio.ChangedBool(plan.Debug, state.Debug),
		ButtonBackgroundColor: vergeio.ChangedString(plan.ButtonBackgroundColor, state.ButtonBackgroundColor),
		ButtonColor:           vergeio.ChangedString(plan.ButtonColor, state.ButtonColor),
		ButtonFAIcon:          vergeio.ChangedString(plan.ButtonFAIcon, state.ButtonFAIcon),
		IconColor:             vergeio.ChangedString(plan.IconColor, state.IconColor),
	}
	if secret != "" || !plan.Settings.Equal(state.Settings) {
		doc, err := settingsDocument(plan.Settings, secret)
		if err != nil {
			return nil, err
		}
		req.Settings = doc
	}
	if req.Name == nil && req.Menu == nil && req.Debug == nil &&
		req.ButtonBackgroundColor == nil && req.ButtonColor == nil &&
		req.ButtonFAIcon == nil && req.IconColor == nil && req.Settings == nil {
		return nil, nil
	}
	return req, nil
}

func applyAuthSource(data *AuthSourceResourceModel, source *vergeos.AuthSource) error {
	data.Id = idString(source.Key.Int())
	data.Name = types.StringValue(source.Name)
	data.Driver = types.StringValue(source.Driver)
	data.Menu = types.BoolValue(source.Menu)
	data.Debug = types.BoolValue(source.Debug)
	data.ButtonBackgroundColor = types.StringValue(source.ButtonBackgroundColor)
	data.ButtonColor = types.StringValue(source.ButtonColor)
	data.ButtonFAIcon = types.StringValue(source.ButtonFAIcon)
	data.IconColor = types.StringValue(source.IconColor)
	settings, err := reconcileSettings(data.Settings, source.Settings)
	if err != nil {
		return err
	}
	data.Settings = settings
	return nil
}

func authSourceReused(data *AuthSourceResourceModel, source *vergeos.AuthSource) bool {
	if data == nil || source == nil || data.Driver.IsNull() || data.Driver.IsUnknown() {
		return false
	}
	if source.Driver == "" {
		return false
	}
	return data.Driver.ValueString() != source.Driver
}

func (api *AuthSourceApi) createAuthSource(ctx context.Context, data *AuthSourceResourceModel, secret string) error {
	req, err := authSourceCreateRequest(data, secret)
	if err != nil {
		return err
	}
	created, err := api.sdk.AuthSources.Create(ctx, req)
	if err != nil {
		return err
	}
	if created == nil || created.Key.Int() <= 0 {
		return fmt.Errorf("VergeOS did not return an auth source id")
	}
	if err := applyAuthSource(data, created); err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("created auth source %s", data.Id.ValueString()))
	return nil
}

func (api *AuthSourceApi) readAuthSource(ctx context.Context, data *AuthSourceResourceModel) error {
	id, err := parseID(data.Id, "auth source")
	if err != nil {
		return err
	}
	source, err := api.sdk.AuthSources.Get(ctx, id)
	if err != nil {
		return err
	}
	if authSourceReused(data, source) {
		return errStaleIdentity
	}
	if err := applyAuthSource(data, source); err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("read auth source %d", id))
	return nil
}

func (api *AuthSourceApi) updateAuthSource(ctx context.Context, plan, state *AuthSourceResourceModel, secret string) error {
	id, err := parseID(state.Id, "auth source")
	if err != nil {
		return err
	}
	plan.Id = state.Id
	req, err := authSourceUpdateRequest(plan, state, secret)
	if err != nil {
		return err
	}
	if req == nil {
		tflog.Debug(ctx, fmt.Sprintf("auth source %d is unchanged", id))
		return nil
	}
	updated, err := api.sdk.AuthSources.Update(ctx, id, req)
	if err != nil {
		return err
	}
	if err := applyAuthSource(plan, updated); err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("updated auth source %d", id))
	return nil
}

func (api *AuthSourceApi) deleteAuthSource(ctx context.Context, data *AuthSourceResourceModel) error {
	id, err := parseID(data.Id, "auth source")
	if err != nil {
		return err
	}
	if err := api.sdk.AuthSources.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted auth source %d", id))
	return nil
}
