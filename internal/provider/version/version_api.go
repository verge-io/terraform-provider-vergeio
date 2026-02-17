// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package version

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)


var _ vergeio.IClient = &VersionApi{}

func NewVersionApi(c *vergeio.Client) *VersionApi {
	sdk, _ := vergeos.NewClient(
		vergeos.WithBaseURL(vergeio.EnsureHTTPSPrefix(c.Host)),
		vergeos.WithCredentials(c.Username, c.Password),
		vergeos.WithInsecureTLS(c.Insecure),
	)
	return &VersionApi{
		name:   "Version Api",
		client: c,
		sdk:    sdk,
	}
}

type VersionApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
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

	// Call the SDK API
	version, err := va.sdk.System.GetVersion(ctx)
	if err != nil {
		return err
	}

	tflog.Debug(ctx, fmt.Sprintf("Read the version resource %v", version))

	// Convert SDK version to API model for existing field mapping logic
	versionAPIResp := VersionAPIDataSourceModel{
		Name:    version,
		Version: version, // SDK returns version string directly
		Hash:    "",      // Hash may not be available in SDK
	}

	data.Name = types.StringValue(versionAPIResp.Name)
	data.Version = types.StringValue(versionAPIResp.Version)
	data.Hash = types.StringValue(versionAPIResp.Hash)

	tflog.Debug(ctx, "Data was successfully converted to a resource")

	return nil
}
