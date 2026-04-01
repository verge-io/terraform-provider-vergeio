// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package member

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

// API endpoint.

// IClient interface.
var _ vergeio.IClient = &MemberApi{}

func NewMemberApi(c *vergeio.Client) *MemberApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &MemberApi{
		name:   "Member Api",
		client: c,
		sdk:    sdk,
	}
}

type MemberApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
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

	// Convert to SDK request format
	var req vergeos.MemberCreateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Call the SDK API
	member, err := nc.sdk.Members.Create(ctx, &req)
	if err != nil {
		return err
	}

	// Extract the id and put it in the state
	data.Id = types.StringValue(fmt.Sprintf("%d", member.ID.Int()))

	tflog.Debug(ctx, fmt.Sprintf("Created a member with Id %v", data.Id))

	return nil
}

// updateMember updates an existing member.
func (nc *MemberApi) updateMember(ctx context.Context, planData *MemberResourceModel, stateData *MemberResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := MemberAPIResourceModel{
		Id:     planData.Id.ValueString(),
		Group:  vergeio.Int32ToNil(planData.Group, stateData.Group, 0),
		Member: vergeio.StringToNil(planData.Member, stateData.Member, ""),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Convert to SDK request format
	var req vergeos.MemberUpdateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Call the SDK API
	id, _ := strconv.Atoi(planData.Id.ValueString())
	_, err := nc.sdk.Members.Update(ctx, id, &req)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", apiData))

	// Read data into the model to get all the attributes
	if readDataError := nc.readMember(ctx, planData); readDataError != nil {
		return fmt.Errorf("error Fetching Data %v", readDataError)
	}

	return nil
}

// Read the Member (Vnet) from the API.
func (nc *MemberApi) readMember(ctx context.Context, data *MemberResourceModel) error {

	tflog.Debug(ctx, "Reading the member data")

	// Call the SDK API
	id, _ := strconv.Atoi(data.Id.ValueString())
	member, err := nc.sdk.Members.Get(ctx, id)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the member resource %v", member))

	// Convert SDK member to API model for existing field mapping logic
	memberAPIResp := MemberAPIResourceModel{
		Id:     fmt.Sprintf("%d", member.ID.Int()),
		Group:  int32(member.Group.Int()),
		Member: member.Member,
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

	// Call the SDK API
	id, _ := strconv.Atoi(data.Id.ValueString())
	err := nc.sdk.Members.Delete(ctx, id)

	// error checking
	if err != nil {
		return errors.New("Error deleting the member: " + err.Error())
	}

	tflog.Debug(ctx, "Member was successfully deleted")

	return nil
}
