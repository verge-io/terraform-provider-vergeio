// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package mediasource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Mediasource API Endpoint.
const (
	MediasourceEndpoint = vergeio.APIEndpoint + "/files"
)

// IClient interface.
var _ vergeio.IClient = &MediasourceApi{}

func NewMediasourceApi(c *vergeio.Client) *MediasourceApi {
	return &MediasourceApi{
		name:   "Mediasource Api",
		client: c,
	}
}

type MediasourceApi struct {
	name   string
	client *vergeio.Client
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

	apiResp, err := va.client.Get(MediasourceEndpoint,
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
	var mediasourceAPIResp []MediasourceAPIDataSourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&mediasourceAPIResp); err != nil {
		return errors.New("invalid format received for VM Item")
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
