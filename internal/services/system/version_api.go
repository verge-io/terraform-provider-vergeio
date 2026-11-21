// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package system

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var _ vergeio.IClient = &VersionApi{}

func NewVersionApi(c *vergeio.Client) (*VersionApi, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &VersionApi{
		name:   "Version Api",
		client: c,
		sdk:    sdk,
	}, nil
}

type VersionApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
}

func (nc *VersionApi) Name() string {
	return nc.name
}

func (va *VersionApi) readVersion(ctx context.Context, data *VersionDataSourceModel) error {
	tflog.Debug(ctx, "Reading the version data")

	info, err := va.sdk.System.GetInfo(ctx)
	if err != nil {
		return err
	}
	if info == nil {
		return fmt.Errorf("empty VergeOS version response")
	}

	tflog.Debug(ctx, fmt.Sprintf("Read version %s name %s hash %s", info.Version, info.Name, info.Hash))

	data.Name = types.StringValue(info.Name)
	data.Version = types.StringValue(info.Version)
	data.Hash = types.StringValue(info.Hash)
	return nil
}
