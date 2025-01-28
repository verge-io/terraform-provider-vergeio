// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	VersionEndpoint = "/version.json"
)

var _ vergeio.IClient = &VersionApi{}

func NewVersionApi(c *vergeio.Client) *VersionApi {
	return &VersionApi{
		name:   "Version Api",
		client: c,
	}
}

type VersionApi struct {
	name   string
	client *vergeio.Client
}

func (nc *VersionApi) Name() string {
	return nc.name
}

type VersionAPIDataSourceModel struct {
	Name    string `json:"name,omitempty"`
	Version string `json:"version,omitempty"`
	Hash    string `json:"hash,omitempty"`
}

// Read the VM from the API.
func (va *VersionApi) readVersion(ctx context.Context, data *VersionDataSourceModel) error {

	tflog.Debug(ctx, "Reading the version data")

	apiResp, err := va.client.Get(VersionEndpoint,
		nil)

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
	var versionAPIResp VersionAPIDataSourceModel
	if err := json.NewDecoder(apiResp.Body).Decode(&versionAPIResp); err != nil {
		return errors.New("invalid format received for VM Item")
	}

	data.Name = types.StringValue(versionAPIResp.Name)
	data.Version = types.StringValue(versionAPIResp.Version)
	data.Hash = types.StringValue(versionAPIResp.Hash)

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
