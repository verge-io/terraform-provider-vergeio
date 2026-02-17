// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package cloudinitFile

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


// IClient interface.
var _ vergeio.IClient = &CloudinitFileApi{}

func NewCloudinitFileApi(c *vergeio.Client) *CloudinitFileApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &CloudinitFileApi{
		name:   "CloudinitFile Api",
		client: c,
		sdk:    sdk,
	}
}

type CloudinitFileApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *CloudinitFileApi) Name() string {
	return nc.name
}

// CloudinitFileAPIResourceModel describes the data model received from the Verge API.
type CloudinitFileAPIResourceModel struct {
	Id                string `json:"id,omitempty"`
	Owner             string `json:"owner,omitempty"`
	Name              string `json:"name,omitempty"`
	Filesize          int64  `json:"filesize,omitempty"`
	Contents          string `json:"contents,omitempty"`
	ContainsVariables bool   `json:"contains_variables,omitempty"`
}

// createCloudinitFile creates a new cloudinitFile.
func (nc *CloudinitFileApi) createCloudinitFile(ctx context.Context, data *CloudinitFileResourceModel) error {

	apiData := CloudinitFileAPIResourceModel{
		Name:              data.Name.ValueString(),
		Contents:          data.Contents.ValueString(),
		ContainsVariables: data.ContainsVariables.ValueBool(),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for cloudinitFile Item")
	}

	// Convert to SDK request format
	var req vergeos.CloudInitFileCreateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Call the SDK API
	cloudinitFile, err := nc.sdk.CloudInitFiles.Create(ctx, &req)
	if err != nil {
		return err
	}

	// save into the Terraform state.
	data.Id = types.StringValue(fmt.Sprintf("%d", cloudinitFile.ID.Int()))
	tflog.Debug(ctx, fmt.Sprintf("Created a cloudinitFile with Id %v", data.Id.ValueString()))

	return nil
}

// updateCloudinitFile updates an existing cloudinitFile.
func (nc *CloudinitFileApi) updateCloudinitFile(ctx context.Context, planData *CloudinitFileResourceModel, stateData *CloudinitFileResourceModel) error {

	// Prepare the API data packet from the plan
	apiData := CloudinitFileAPIResourceModel{

		Id:                vergeio.StringToNil(planData.Id, stateData.Id, ""),
		Name:              vergeio.StringToNil(planData.Name, stateData.Name, ""),
		Contents:          vergeio.StringToNil(planData.Contents, stateData.Contents, ""),
		ContainsVariables: vergeio.BoolToNil(planData.ContainsVariables, stateData.ContainsVariables, false),
		Filesize:          vergeio.Int64ToNil(planData.Filesize, stateData.Filesize, 0),
	}

	// Encode the API data
	encodedBuffer := new(bytes.Buffer)
	if err := json.NewEncoder(encodedBuffer).Encode(apiData); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	// Convert to SDK request format
	var req vergeos.CloudInitFileUpdateRequest
	if err := json.Unmarshal(encodedBuffer.Bytes(), &req); err != nil {
		return fmt.Errorf("failed to convert API data: %v", err)
	}

	// Call the SDK API
	id, _ := strconv.Atoi(planData.Id.ValueString())
	_, err := nc.sdk.CloudInitFiles.Update(ctx, id, &req)
	if err != nil {
		return err
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Updated a resource %v", apiData))

	return nil
}

// deleteCloudinitFile deletes a cloudinitFile.
func (nc *CloudinitFileApi) deleteCloudinitFile(ctx context.Context, data *CloudinitFileResourceModel) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill CloudinitFile API for CloudinitFile %v", data.Id.ValueString()))

	// Call the SDK API
	id, _ := strconv.Atoi(data.Id.ValueString())
	err := nc.sdk.CloudInitFiles.Delete(ctx, id)
	if err != nil {
		return err
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("Deleted the cloudinitFile with the ID %v", data.Id))

	return nil
}

// Read the CloudinitFile (Vnet) from the API.
func (nc *CloudinitFileApi) readCloudinitFile(ctx context.Context, data *CloudinitFileResourceModel) error {

	tflog.Debug(ctx, "Reading the cloudinitFile data")

	// Call the SDK API
	id, _ := strconv.Atoi(data.Id.ValueString())
	cloudinitFile, err := nc.sdk.CloudInitFiles.Get(ctx, id)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the cloudinitFile resource %v", cloudinitFile))

	// Convert SDK cloudinitFile to API model for existing field mapping logic
	cloudinitFileAPIResp := CloudinitFileAPIResourceModel{
		Id:                fmt.Sprintf("%d", cloudinitFile.ID.Int()),
		Name:              cloudinitFile.Name,
		Contents:          cloudinitFile.Contents,
		ContainsVariables: cloudinitFile.ContainsVariables,
		Filesize:          cloudinitFile.FileSize,
	}

	// save into the resource model
	data.Name = types.StringValue(cloudinitFileAPIResp.Name)
	data.Contents = types.StringValue(cloudinitFileAPIResp.Contents)
	data.ContainsVariables = types.BoolValue(cloudinitFileAPIResp.ContainsVariables)
	data.Filesize = types.Int64Value(cloudinitFileAPIResp.Filesize)

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}

// Read the CloudinitFiles from the API for data source.
func (va *CloudinitFileApi) readCloudinitFiles(ctx context.Context, data *CloudinitFileDataSourceModel) error {

	tflog.Debug(ctx, "Reading the cloudinitFile data")

	// Build filter
	var listOpts []vergeos.ListOption
	if fn := data.FilterName.ValueString(); fn != "" {
		listOpts = append(listOpts, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", fn)))
	}

	// Call the SDK API
	cloudinitFiles, err := va.sdk.CloudInitFiles.List(ctx, listOpts...)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", cloudinitFiles))

	// Convert SDK cloudinitFiles to API model for existing field mapping logic
	var cloudinitFileAPIResp []CloudinitFileAPIResourceModel
	for _, file := range cloudinitFiles {
		cloudinitFileAPIResp = append(cloudinitFileAPIResp, CloudinitFileAPIResourceModel{
			Id:                fmt.Sprintf("%d", file.ID.Int()),
			Name:              file.Name,
			Filesize:          file.FileSize,
			Contents:          file.Contents,
			ContainsVariables: file.ContainsVariables,
		})
	}

	// save into the resource model
	for _, nwAPIResp := range cloudinitFileAPIResp {
		data.CloudinitFiles = append(data.CloudinitFiles, &CloudinitFileModel{
			Id:                types.StringValue(nwAPIResp.Id),
			Name:              types.StringValue(nwAPIResp.Name),
			Filesize:          types.Int64Value(nwAPIResp.Filesize),
			Contents:          types.StringValue(nwAPIResp.Contents),
			ContainsVariables: types.BoolValue(nwAPIResp.ContainsVariables),
		})

	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
