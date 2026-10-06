// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"fmt"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/verge-io/govergeos"
)

// nasPollInterval and nasPollAttempts bound the wait after a volume disable.
// Delete is refused while the volume is still enabled. Tests set the interval
// to zero.
//
// nasServiceWaitInterval and nasServiceWaitAttempts bound the wait for the
// service row after the Services recipe is deployed. The recipe creates the
// virtual machine first. The vm_services row appears after that.
//
// nasRecipeDownloadInterval and nasRecipeDownloadAttempts bound the wait
// while VergeOS downloads that recipe the first time it is used.
var (
	nasPollInterval           = time.Second
	nasPollAttempts           = 60
	nasServiceWaitInterval    = 2 * time.Second
	nasServiceWaitAttempts    = 90
	nasRecipeDownloadInterval = 5 * time.Second
	nasRecipeDownloadAttempts = 120
)

// API reads and writes NAS services, volumes, and shares through govergeos.
type API struct {
	sdk  *vergeos.Client
	http *vergeio.Client
}

func NewAPI(c *vergeio.Client) (*API, error) {
	if c == nil {
		return nil, fmt.Errorf("vergeio client is nil")
	}
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	return &API{sdk: sdk, http: c}, nil
}

func configure(providerData any) (*API, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	c, ok := providerData.(*vergeio.Client)
	if !ok {
		diags.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, diags
	}
	api, err := NewAPI(c)
	if err != nil {
		diags.AddError("Unable to Create VergeOS API Client", err.Error())
		return nil, diags
	}
	return api, diags
}

func (a *API) requireSDK() error {
	if a == nil || a.sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
