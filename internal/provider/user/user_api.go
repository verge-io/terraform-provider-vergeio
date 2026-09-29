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

// UserAPIResourceModel describes the data model received from the Verge API.
type UserAPIResourceModel struct {
	Id             string `json:"id,omitempty"`
	AuthSource     int32  `json:"auth_source,omitempty"`
	Name           string `json:"name,omitempty"`
	RemoteName     string `json:"remote_name,omitempty"`
	Enabled        bool   `json:"enabled,omitempty"`
	DisplayName    string `json:"displayname,omitempty"`
	Email          string `json:"email,omitempty"`
	Type           string `json:"type,omitempty"`
	Password       string `json:"password,omitempty"`
	ChangePassword bool   `json:"change_password,omitempty"`
}

// Create the user in the API.
func (nc *UserApi) createUser(ctx context.Context, data *UserResourceModel) error {

	apiData := UserAPIResourceModel{
		Id:             data.Id.ValueString(),
		AuthSource:     data.AuthSource.ValueInt32(),
		Name:           data.Name.ValueString(),
		RemoteName:     data.RemoteName.ValueString(),
		DisplayName:    data.DisplayName.ValueString(),
		Email:          data.Email.ValueString(),
		Enabled:        data.Enabled.ValueBool(),
		Type:           data.Type.ValueString(),
		Password:       data.Password.ValueString(),
		ChangePassword: data.ChangePassword.ValueBool(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Time to call the SDK API - convert to SDK request
	var req vergeos.UserCreateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	user, err := nc.sdk.Users.Create(ctx, &req)
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

	// Prepare the API data packet from the plan
	apiData := UserAPIResourceModel{
		Name:        vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Enabled:     vergeio.BoolToNil(planData.Enabled, stateData.Enabled, false),
		DisplayName: vergeio.StringToNil(planData.DisplayName, stateData.DisplayName, ""),
		Email:       vergeio.StringToNil(planData.Email, stateData.Email, ""),
		RemoteName:  vergeio.StringToNil(planData.RemoteName, stateData.RemoteName, ""),
		// AuthSource is read only
		// Type is read only
		// Password is not read only but we should not change it after creation
		// ChangePassword is not read only but we should not chagne it after creation
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for User Item")
	}

	tflog.Debug(ctx, fmt.Sprintf("Encoded buffer for update user %v", encodedBuffer.String()))

	// Convert to SDK request
	var req vergeos.UserUpdateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Parse user ID
	userIDInt, err := strconv.Atoi(stateData.Id.ValueString())
	if err != nil {
		return fmt.Errorf("invalid user ID format: %v", err)
	}

	// Call SDK API
	_, err = nc.sdk.Users.Update(ctx, userIDInt, &req)
	if err != nil {
		return err
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", apiData))

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

	// Convert SDK response to API model for field mapping consistency
	userAPIResp := UserAPIResourceModel{
		Name:           user.Name,
		Enabled:        user.Enabled,
		DisplayName:    user.DisplayName,
		Email:          user.Email,
		AuthSource:     int32(user.AuthSource),
		RemoteName:     user.RemoteName,
		Type:           user.Type,
		ChangePassword: user.ChangePassword,
	}

	// save into the resource model
	data.Id = types.StringValue(fmt.Sprintf("%d", user.Key.Int()))
	data.Name = types.StringValue(userAPIResp.Name)
	data.Enabled = types.BoolValue(userAPIResp.Enabled)
	data.DisplayName = types.StringValue(userAPIResp.DisplayName)
	data.Email = types.StringValue(userAPIResp.Email)
	data.AuthSource = types.Int32Value(userAPIResp.AuthSource)
	data.RemoteName = types.StringValue(userAPIResp.RemoteName)
	data.Type = types.StringValue(userAPIResp.Type)
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
