// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"errors"
	"fmt"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/verge-io/govergeos"
)

// API reads and writes sites and site syncs through govergeos.
//
// codeInterval and codeAttempts bound the wait for an incoming registration
// code after create. A full wait returns the sync with an empty code.
// Tests set a short interval and one attempt.
//
// nowUnix is the clock for sync lag. Nil uses time.Now.
type API struct {
	sdk          *vergeos.Client
	codeInterval time.Duration
	codeAttempts int
	nowUnix      func() int64
}

func NewAPI(c *vergeio.Client) (*API, error) {
	if c == nil {
		return nil, fmt.Errorf("vergeio client is nil")
	}
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	return &API{
		sdk:          sdk,
		codeInterval: time.Second,
		codeAttempts: 30,
	}, nil
}

func configureAPI(providerData any) (*API, diag.Diagnostics) {
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

func (a *API) now() int64 {
	if a != nil && a.nowUnix != nil {
		return a.nowUnix()
	}
	return time.Now().Unix()
}

func (a *API) requireSDK() error {
	if a == nil || a.sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	return nil
}

func ignoreMissing(err error) error {
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	return err
}

func listMissing(err error) bool {
	if err == nil {
		return false
	}
	if vergeos.IsNotFoundError(err) {
		return true
	}
	var apiErr *vergeos.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}
