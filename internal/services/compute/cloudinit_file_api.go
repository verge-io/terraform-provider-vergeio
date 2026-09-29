// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// IClient interface.
var _ vergeio.IClient = &CloudinitFileApi{}

func NewCloudinitFileApi(c *vergeio.Client) *CloudinitFileApi {
	sdk, _ := vergeos.NewClient(c.SDKOptions()...)
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

// Read the CloudinitFiles from the API for data source.
func (va *CloudinitFileApi) readCloudinitFiles(ctx context.Context, data *CloudinitFileDataSourceModel) error {

	tflog.Debug(ctx, "Reading the cloudinitFile data")

	// Build filter
	var listOpts []vergeos.ListOption
	fn := data.FilterName.ValueString()
	if fn != "" {
		listOpts = append(listOpts, vergeos.WithFilter(fmt.Sprintf("name eq '%s'", vergeio.EscapeFilterValue(fn))))
	}

	// Call the SDK API
	cloudinitFiles, err := va.sdk.CloudInitFiles.List(ctx, listOpts...)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", cloudinitFiles))

	cloudinitFiles = vergeio.KeepExact(cloudinitFiles, fn, func(file vergeos.CloudInitFile) string { return file.Name })

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
