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
	"syscall"
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
	// putTries and putBackoff retry one settings PUT. The SDK retries PUT
	// when the connection is reset. This HTTP client does not, and a
	// settings write goes through it.
	putTries   int
	putBackoff time.Duration
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
		putTries:     3,
		putBackoff:   100 * time.Millisecond,
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
	tries := a.putTries
	if tries <= 0 {
		tries = 3
	}
	var last error
	for attempt := 1; attempt <= tries; attempt++ {
		if attempt > 1 {
			delay := a.putBackoff * time.Duration(attempt-1)
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		}
		resp, err := a.http.Put(ctx, endpoint, bytes.NewBuffer(buf))
		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		if err == nil || !retryablePUT(err) || attempt == tries {
			return err
		}
		last = err
	}
	return last
}

// retryablePUT reports a settings write that is safe to send again.
// 401 is not retried. A repeated rejected login counts toward lockout.
func retryablePUT(err error) bool {
	if err == nil {
		return false
	}
	var apiErr vergeio.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 429, 502, 503:
			return true
		default:
			return false
		}
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return strings.Contains(err.Error(), "connection reset")
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
