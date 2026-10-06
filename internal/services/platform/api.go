// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/verge-io/govergeos"
)

const (
	certificateCollection = "api/v4/certificates"
	settingsCollection    = "api/v4/settings"
)

// API reads and writes certificates, webhook destinations, webhook
// deliveries, and system settings.
//
// webhookTries and webhookWait bound the wait for a delivery row after
// WebhookURLs.Send. Send queues the message and does not return its key.
type API struct {
	sdk          *vergeos.Client
	http         *vergeio.Client
	webhookTries int
	webhookWait  time.Duration
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
		http:         c,
		webhookTries: 20,
		webhookWait:  500 * time.Millisecond,
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

func (a *API) putJSON(ctx context.Context, endpoint string, payload any) error {
	if a == nil || a.http == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := a.http.Put(ctx, endpoint, bytes.NewBuffer(buf))
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return err
}

func (a *API) getJSON(ctx context.Context, endpoint string, params *vergeio.Options, dest any) error {
	if a == nil || a.http == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	resp, err := a.http.Get(ctx, endpoint, params)
	if err != nil {
		if resp != nil && resp.Body != nil {
			defer func() { _ = resp.Body.Close() }()
			_, _ = io.Copy(io.Discard, resp.Body)
		}
		return err
	}
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("empty response from %s", endpoint)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(dest); err != nil {
		return fmt.Errorf("decode %s: %w", endpoint, err)
	}
	return nil
}

func escapeFilterValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "'", `\'`)
	s = strings.ReplaceAll(s, "{", `\{`)
	return s
}
