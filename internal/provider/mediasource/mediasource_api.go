// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package mediasource

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// Mediasource API Endpoint.

// IClient interface.
var _ vergeio.IClient = &MediasourceApi{}

func NewMediasourceApi(c *vergeio.Client) *MediasourceApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &MediasourceApi{
		name:   "Mediasource Api",
		client: c,
		sdk:    sdk,
	}
}

type MediasourceApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *MediasourceApi) Name() string {
	return nc.name
}

// Mediasource API DataSource Model.
type MediasourceAPIDataSourceModel struct {
	Id          int32  `json:"$key,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	Filesize    int64  `json:"filesize,omitempty"`
}

// Read the VM from the API.
func (va *MediasourceApi) readMediasources(ctx context.Context, data *MediasourceDataSourceModel) error {

	tflog.Debug(ctx, "Reading the mediasource data")

	opts := vergeio.Options{Fields: "$key,name,description,filesize"}

	// Build filter
	if fn := data.FilterName.ValueString(); fn != "" {
		opts.Filter = fmt.Sprintf("name eq '%s'", fn)
	}

	// Call the SDK API
	var listOpts []vergeos.ListOption
	if opts.Filter != "" {
		listOpts = append(listOpts, vergeos.WithFilter(opts.Filter))
	}

	files, err := va.sdk.Files.List(ctx, listOpts...)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the resource %v", files))

	// Convert SDK files to API model for existing field mapping logic
	var mediasourceAPIResp []MediasourceAPIDataSourceModel
	for _, file := range files {
		mediasourceAPIResp = append(mediasourceAPIResp, MediasourceAPIDataSourceModel{
			Id:          int32(file.ID.Int()),
			Name:        file.Name,
			Description: file.Description,
			Filesize:    file.Filesize,
		})
	}

	// save into the resource model
	for _, mediasourceAPIResp := range mediasourceAPIResp {
		data.Mediasources = append(data.Mediasources, &MediasourceModel{
			Id:          types.Int32Value(mediasourceAPIResp.Id),
			Name:        types.StringValue(mediasourceAPIResp.Name),
			Description: types.StringValue(mediasourceAPIResp.Description),
			Filesize:    types.Int64Value(mediasourceAPIResp.Filesize),
		})

	}

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
