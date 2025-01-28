// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package member

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

// API endpoint.
const (
	MemberEndpoint = vergeio.APIEndpoint + "/members"
)

// IClient interface.
var _ vergeio.IClient = &MemberApi{}

func NewMemberApi(c *vergeio.Client) *MemberApi {
	return &MemberApi{
		name:   "Member Api",
		client: c,
	}
}

type MemberApi struct {
	name   string
	client *vergeio.Client
}

func (nc *MemberApi) Name() string {
	return nc.name
}

// MemberAPIResourceModel describes the data model received from the Verge API.
type MemberAPIResourceModel struct {
	Id     string `json:"id,omitempty"`
	Group  int32  `json:"parent_group,omitempty"`
	Member string `json:"member,omitempty"`
}

// createMember creates a new member.
func (nc *MemberApi) createMember(ctx context.Context, data *MemberResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := MemberAPIResourceModel{
		Group:  data.Group.ValueInt32(),
		Member: data.Member.ValueString(),
	}

	tflog.Debug(ctx, fmt.Sprintf("Creating a member with data %v", apiData))

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	tflog.Debug(ctx, fmt.Sprintf("Encoded buffer %v", encodedBuffer.String()))

	// Call the API
	apiResp, err := nc.client.Post(MemberEndpoint, encodedBuffer)
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

	// Decode the API response and put it in the response object
	var memberAPIResp vergeio.VergeResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&memberAPIResp); err != nil {
		return errors.New("invalid format received for Item")
	}

	// Extract the id and put it in the state
	data.Id = types.StringValue(memberAPIResp.Key)

	tflog.Debug(ctx, fmt.Sprintf("Created a member with Id %v", data.Id))

	return nil
}

// updateMember updates an existing member.
func (nc *MemberApi) updateMember(ctx context.Context, planData *MemberResourceModel, stateData *MemberResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := MemberAPIResourceModel{
		Group:  vergeio.Int32ToNil(planData.Group, stateData.Group, 0),
		Member: vergeio.StringToNil(planData.Member, stateData.Member, ""),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Call the API
	apiResp, err := nc.client.Put(fmt.Sprintf("%s/%s",
		MemberEndpoint,
		url.PathEscape(apiData.Id),
	), encodedBuffer)
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

	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", apiData))

	defer apiResp.Body.Close()

	// Read data into the model to get all the attributes
	if readDataError := nc.readMember(ctx, planData); readDataError != nil {
		return fmt.Errorf("error Fetching Data %v", readDataError)
	}

	return nil
}

// Read the Member (Vnet) from the API.
func (nc *MemberApi) readMember(ctx context.Context, data *MemberResourceModel) error {

	tflog.Debug(ctx, "Reading the member data")

	// Call the Get API with the member id and get the fields we need
	// most fields are not returned by default
	tflog.Debug(ctx, fmt.Sprintf("Member endpoint %v", fmt.Sprintf("%s/%s",
		MemberEndpoint,
		url.PathEscape(data.Id.ValueString()))))

	apiResp, err := nc.client.Get(fmt.Sprintf("%s/%s",
		MemberEndpoint,
		url.PathEscape(data.Id.ValueString()),
	), nil)

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

	tflog.Debug(ctx, fmt.Sprintf("Member API resonse body %v", apiResp.Body))

	// Decode the API response
	var memberAPIResp MemberAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&memberAPIResp); err != nil {
		return fmt.Errorf("invalid format received for Item %v", err)
	}

	// save into the resource model
	data.Group = types.Int32Value(memberAPIResp.Group)
	data.Member = types.StringValue(memberAPIResp.Member)

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// Read the Member (Vnet) from the API.
func (nc *MemberApi) deleteMember(ctx context.Context, data *MemberResourceModel) error {

	tflog.Debug(ctx, "Deleting the member data")

	// Call the Get API with the member id and Proceed with member deletion
	_, err := nc.client.Delete(fmt.Sprintf("%s/%s",
		MemberEndpoint,
		url.PathEscape(data.Id.ValueString())))

	// error checking
	if err != nil {
		return errors.New("Error deleting the member: " + err.Error())
	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
