// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package cloudinitFile

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

// CloudinitFile endpoints.
const (
	CloudinitFileEndpoint = vergeio.APIEndpoint + "/cloudinit_files"
)

// IClient interface.
var _ vergeio.IClient = &CloudinitFileApi{}

func NewCloudinitFileApi(c *vergeio.Client) *CloudinitFileApi {
	return &CloudinitFileApi{
		name:   "CloudinitFile Api",
		client: c,
	}
}

type CloudinitFileApi struct {
	name   string
	client *vergeio.Client
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

	// Time to call the API
	apiResp, err := nc.client.Post(CloudinitFileEndpoint, encodedBuffer)
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
	var cloudinitFileAPIResp vergeio.VergeResponse
	if err := json.NewDecoder(apiResp.Body).Decode(&cloudinitFileAPIResp); err != nil {
		return errors.New("invalid format received for Item")
	}

	// save into the Terraform state.
	data.Id = types.StringValue(cloudinitFileAPIResp.Key)
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

	// Time to call the API
	apiResp, err := nc.client.Put(fmt.Sprintf("%s/%s",
		CloudinitFileEndpoint,
		url.PathEscape(apiData.Id),
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

// deleteCloudinitFile deletes a cloudinitFile.
func (nc *CloudinitFileApi) deleteCloudinitFile(ctx context.Context, data *CloudinitFileResourceModel) error {

	tflog.Debug(ctx, fmt.Sprintf("Calling the Kill CloudinitFile API for CloudinitFile %v", data.Id.ValueString()))

	// call the API
	apiResp, err := nc.client.Delete(fmt.Sprintf("%s/%s",
		CloudinitFileEndpoint,
		url.PathEscape(data.Id.ValueString()),
	))
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
	tflog.Debug(ctx, fmt.Sprintf("Deleted the cloudinitFile with the ID %v", data.Id))

	defer apiResp.Body.Close()

	return nil
}

// Read the CloudinitFile (Vnet) from the API.
func (nc *CloudinitFileApi) readCloudinitFile(ctx context.Context, data *CloudinitFileResourceModel) error {

	tflog.Debug(ctx, "Reading the cloudinitFile data")

	// Call the Get API with the cloudinitFile id and get the fields we need
	// most fields are not returned by default
	apiResp, err := nc.client.Get(fmt.Sprintf("%s/%s",
		CloudinitFileEndpoint,
		url.PathEscape(data.Id.ValueString()),
	), &vergeio.Options{Fields: "$key,name,filesize,contents,containsVariables"})

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
	var cloudinitFileAPIResp CloudinitFileAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&cloudinitFileAPIResp); err != nil {
		return errors.New("invalid format received for Item")
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

	// What fields do we want
	opts := vergeio.Options{Fields: "$key,name,filesize,contents,containsVariables"}

	// Build filter
	if fn := data.FilterName.ValueString(); fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn)
	}

	// Call the API
	apiResp, err := va.client.Get(CloudinitFileEndpoint,
		&opts)

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

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", apiResp.Body))

	// Decode the API response
	var cloudinitFileAPIResp []CloudinitFileAPIResourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&cloudinitFileAPIResp); err != nil {
		return errors.New("invalid format received for VM Item")
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
