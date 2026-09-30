// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	vergeos "github.com/verge-io/govergeos"
)

var _ vergeio.IClient = &UsersApi{}

func NewUsersApi(c *vergeio.Client) *UsersApi {
	sdk, _ := vergeos.NewClient(c.SDKOptions()...)
	return &UsersApi{
		name: "Users Api",
		sdk:  sdk,
	}
}

// UsersApi is the govergeos UserService client for the vergeio_users data source.
type UsersApi struct {
	name string
	sdk  *vergeos.Client
}

func (api *UsersApi) Name() string {
	return api.name
}

func (api *UsersApi) readUsers(ctx context.Context, data *UsersDataSourceModel) error {
	tflog.Debug(ctx, "reading the users data source")

	var listOpts []vergeos.ListOption
	name := data.FilterName.ValueString()
	if name != "" {
		listOpts = append(listOpts, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(name))))
	}

	users, err := api.sdk.Users.List(ctx, listOpts...)
	if err != nil {
		return err
	}
	users = vergeio.KeepExact(users, name, func(user vergeos.User) string { return user.Name })

	listed := make([]*UserModel, 0, len(users))
	for _, user := range users {
		listed = append(listed, &UserModel{
			Id:          types.Int32Value(int32(user.Key.Int())),
			Name:        types.StringValue(user.Name),
			DisplayName: types.StringValue(user.DisplayName),
			Email:       types.StringValue(user.Email),
			Type:        types.StringValue(user.Type),
			Enabled:     types.BoolValue(user.Enabled),
		})
	}
	data.Users = listed
	tflog.Debug(ctx, fmt.Sprintf("read %d users", len(listed)))
	return nil
}
