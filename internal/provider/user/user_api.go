// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// User API endpoint - DEPRECATED: SDK handles endpoints internally
// Keeping temporarily for reference during transition

// IClient interface.
var _ vergeio.IClient = &UserApi{}

func NewUserApi(c *vergeio.Client) *UserApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &UserApi{
		name:   "User Api",
		client: c,
		sdk:    sdk,
	}
}

type UserApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *UserApi) Name() string {
	return nc.name
}

// UserAPIResourceModel is the user create/update body.
// Pointer fields keep false, 0, and "" in the JSON. A nil pointer is omitted.
type UserAPIResourceModel struct {
	Id             *string `json:"id,omitempty"`
	AuthSource     *int32  `json:"auth_source,omitempty"`
	Name           *string `json:"name,omitempty"`
	RemoteName     *string `json:"remote_name,omitempty"`
	Enabled        *bool   `json:"enabled,omitempty"`
	DisplayName    *string `json:"displayname,omitempty"`
	Email          *string `json:"email,omitempty"`
	Type           *string `json:"type,omitempty"`
	Password       *string `json:"password,omitempty"`
	ChangePassword *bool   `json:"change_password,omitempty"`
}

// userCreateModel is the provider create body.
func userCreateModel(data *UserResourceModel) UserAPIResourceModel {
	return UserAPIResourceModel{
		AuthSource:     vergeio.KnownInt32(data.AuthSource),
		Name:           vergeio.KnownString(data.Name),
		RemoteName:     vergeio.KnownString(data.RemoteName),
		DisplayName:    vergeio.KnownString(data.DisplayName),
		Email:          vergeio.KnownString(data.Email),
		Enabled:        vergeio.KnownBool(data.Enabled),
		Type:           vergeio.KnownString(data.Type),
		Password:       vergeio.KnownString(data.Password),
		ChangePassword: vergeio.KnownBool(data.ChangePassword),
	}
}

// userCreateRequest builds the SDK create body from known plan values.
func userCreateRequest(data *UserResourceModel) (*vergeos.UserCreateRequest, error) {
	return decodeUserRequest[vergeos.UserCreateRequest](userCreateModel(data))
}

// userUpdateRequest builds the SDK update body from attributes that differ
// from state. Auth source and type are readonly. Password and change_password
// stay off the update body, matching the previous request.
func userUpdateRequest(planData *UserResourceModel, stateData *UserResourceModel) (*vergeos.UserUpdateRequest, error) {
	apiData := UserAPIResourceModel{
		Name:        vergeio.ChangedString(planData.Name, stateData.Name),
		Enabled:     vergeio.ChangedBool(planData.Enabled, stateData.Enabled),
		DisplayName: vergeio.ChangedString(planData.DisplayName, stateData.DisplayName),
		Email:       vergeio.ChangedString(planData.Email, stateData.Email),
		RemoteName:  vergeio.ChangedString(planData.RemoteName, stateData.RemoteName),
	}
	return decodeUserRequest[vergeos.UserUpdateRequest](apiData)
}

func decodeUserRequest[T any](apiData UserAPIResourceModel) (*T, error) {
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return nil, errors.New("invalid format received for User Item")
	}
	var req T
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return nil, fmt.Errorf("failed to convert API data: %v", err)
	}
	return &req, nil
}

// Create the user in the API.
func (nc *UserApi) createUser(ctx context.Context, data *UserResourceModel) error {

	req, err := userCreateRequest(data)
	if err != nil {
		return err
	}

	user, err := nc.sdk.Users.Create(ctx, req)
	if err != nil {
		return err
	}

	// Fill the data.id with the new user key from the API
	data.Id = types.StringValue(fmt.Sprintf("%d", user.Key.Int()))
	tflog.Debug(ctx, fmt.Sprintf("Created a user with Id %v", data.Id.ValueString()))

	return nil
}

// Update the user in the API.
func (nc *UserApi) updateUser(ctx context.Context, planData *UserResourceModel, stateData *UserResourceModel) error {

	req, err := userUpdateRequest(planData, stateData)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Update user request %v", req))

	userIDInt, err := strconv.Atoi(stateData.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid user ID format: %v", err)
	}

	// Call SDK API
	_, err = nc.sdk.Users.Update(ctx, userIDInt, req)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", req))

	return nil
}

// Read the User from the API.
func (nc *UserApi) readUser(ctx context.Context, data *UserResourceModel) error {

	tflog.Debug(ctx, "Reading the user data")

	// Parse user ID
	userIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid user ID format: %v", err)
	}

	// Call SDK API to get specific user
	user, err := nc.sdk.Users.Get(ctx, userIDInt)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the user %v", user))

	data.Id = types.StringValue(fmt.Sprintf("%d", user.Key.Int()))
	data.Name = types.StringValue(user.Name)
	data.Enabled = types.BoolValue(user.Enabled)
	data.DisplayName = types.StringValue(user.DisplayName)
	data.Email = types.StringValue(user.Email)
	data.AuthSource = types.Int32Value(int32(user.AuthSource))
	data.RemoteName = types.StringValue(user.RemoteName)
	data.Type = types.StringValue(user.Type)
	// Password not returned for security reasons - preserve planned value if it exists
	if data.Password.IsNull() || data.Password.IsUnknown() {
		// For new resources, password is not readable from API
		data.Password = types.StringNull()
	}
	// ChangePassword defaults to false after creation since API doesn't return this field
	data.ChangePassword = types.BoolValue(false)

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// Delete the User from the API.
func (nc *UserApi) deleteUser(ctx context.Context, data *UserResourceModel) error {

	tflog.Debug(ctx, "Deleting the user data")

	// Parse user ID
	userIDInt, err := strconv.Atoi(data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid user ID format: %v", err)
	}

	// Call SDK API to delete user
	err = nc.sdk.Users.Delete(ctx, userIDInt)
	if err != nil {
		return fmt.Errorf("error deleting the user: %w", err)
	}

	tflog.Debug(ctx, "User was successfully deleted")

	return nil
}
