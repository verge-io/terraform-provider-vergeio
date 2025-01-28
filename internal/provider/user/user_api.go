// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// User API endpoint.
const (
	UserEndpoint = vergeio.APIEndpoint + "/users"
)

// IClient interface.
var _ vergeio.IClient = &UserApi{}

func NewUserApi(c *vergeio.Client) *UserApi {
	return &UserApi{
		name:   "User Api",
		client: c,
	}
}

type UserApi struct {
	name   string
	client *vergeio.Client
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

	// Time to call the API
	apiResp, err := nc.client.Post(UserEndpoint, encodedBuffer)
	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 201 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	// Decode the API response
	var userAPIResp vergeio.VergeResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&userAPIResp); err != nil {
		return errors.New("invalid format received for Item")
	}

	// Fill the data.id with the new user id from the API
	data.Id = types.StringValue(userAPIResp.Key)
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
		return errors.New("invalid format received for VM Item")
	}

	tflog.Debug(ctx, fmt.Sprintf("Encoded buffer for update user %v", encodedBuffer.String()))

	// Time to call the API
	apiResp, err := nc.client.Put(fmt.Sprintf("%s/%s",
		UserEndpoint,
		url.PathEscape(stateData.Id.ValueString()),
	), encodedBuffer)
	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", apiData))

	defer apiResp.Body.Close()

	return nil
}

// Read the User from the API.
func (nc *UserApi) readUser(ctx context.Context, data *UserResourceModel) error {

	tflog.Debug(ctx, "Reading the user data")

	// Call the Get API with the user id and get the fields we need
	// most fields are not returned by default
	apiResp, err := nc.client.Get(fmt.Sprintf("%s/%s",
		UserEndpoint,
		url.PathEscape(data.Id.ValueString()),
	), &vergeio.Options{Fields: "$key,auth_source,name,remote_name,enabled,displayname,email,type,change_password"})

	// error checking
	if err != nil {
		return err
	}
	if apiResp == nil {
		return errors.New("missing response from the API")
	}
	if apiResp.StatusCode != 200 {
		return fmt.Errorf("missing response from API %d", apiResp.StatusCode)
	}

	tflog.Debug(ctx, fmt.Sprintf("read the resource %v", apiResp.Body))

	// Decode the API response
	var userAPIResp UserAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&userAPIResp); err != nil {
		return errors.New("invalid format received for Item")
	}

	// save into the resource model
	data.Name = types.StringValue(userAPIResp.Name)
	data.Enabled = types.BoolValue(userAPIResp.Enabled)
	data.DisplayName = types.StringValue(userAPIResp.DisplayName)
	data.Email = types.StringValue(userAPIResp.Email)
	data.AuthSource = types.Int32Value(userAPIResp.AuthSource)
	data.RemoteName = types.StringValue(userAPIResp.RemoteName)
	data.Type = types.StringValue(userAPIResp.Type)
	// data.Password = types.StringValue(userAPIResp.Password)
	// data.ChangePassword = types.BoolValue(userAPIResp.ChangePassword)

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// Delete the User from the API.
func (nc *UserApi) deleteUser(ctx context.Context, data *UserResourceModel) error {

	tflog.Debug(ctx, "Deleting the user data")

	// Call the Get API with the user id and Proceed with user deletion
	_, err := nc.client.Delete(fmt.Sprintf("%s/%s",
		UserEndpoint,
		url.PathEscape(data.Id.ValueString())))

	// error checking
	if err != nil {
		return errors.New("Error deleting the user: " + err.Error())
	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
