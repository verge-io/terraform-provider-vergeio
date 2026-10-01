// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"fmt"

	"github.com/verge-io/govergeos"
)

// SDKOptions returns govergeos client options for this client.
// An API key is sent as a bearer token and takes precedence over username
// and password. TLS verification and the HTTP timeout are passed explicitly.
// The provider reads the environment itself. govergeos WithEnvConfig rejects
// a host without a scheme and ignores VERGEOS_INSECURE.
func (c *Client) SDKOptions() []vergeos.ClientOption {
	opts := []vergeos.ClientOption{
		vergeos.WithBaseURL(EnsureHTTPSPrefix(c.Host)),
		vergeos.WithInsecureTLS(c.Insecure),
	}
	if c.APIKey != "" {
		opts = append(opts, vergeos.WithAPIKey(c.APIKey))
	} else {
		opts = append(opts, vergeos.WithCredentials(c.Username, c.Password))
	}
	if timeout := c.Timeout(); timeout > 0 {
		opts = append(opts, vergeos.WithTimeout(timeout))
	}
	return opts
}

// SDK returns the govergeos client for this connection.
// The first successful client is kept. A failed attempt is not cached, so
// the next call can try again. Resource Configure and a later Read share
// this client instead of each opening one and discarding a setup error.
func (c *Client) SDK() (*vergeos.Client, error) {
	if c == nil {
		return nil, fmt.Errorf("vergeio client is nil")
	}
	c.sdkMu.Lock()
	defer c.sdkMu.Unlock()
	if c.sdk != nil {
		return c.sdk, nil
	}
	sdk, err := vergeos.NewClient(c.SDKOptions()...)
	if err != nil {
		return nil, err
	}
	c.sdk = sdk
	return sdk, nil
}

// NewVergeosClient creates a govergeos client for this connection.
// The client is not cached. A nil connection, a failed version check, and a
// nil SDK all return an error so callers never store a nil client.
func (c *Client) NewVergeosClient() (*vergeos.Client, error) {
	if c == nil {
		return nil, fmt.Errorf("vergeio client is nil")
	}
	return requireVergeosClient(vergeos.NewClient(c.SDKOptions()...))
}

// CachedVergeosClient returns the shared govergeos client for this connection.
// A failed attempt is not cached. The returned client is never nil.
func (c *Client) CachedVergeosClient() (*vergeos.Client, error) {
	if c == nil {
		return nil, fmt.Errorf("vergeio client is nil")
	}
	return requireVergeosClient(c.SDK())
}

func requireVergeosClient(sdk *vergeos.Client, err error) (*vergeos.Client, error) {
	if err != nil {
		return nil, fmt.Errorf("failed to create VergeOS client: %w", err)
	}
	if sdk == nil {
		return nil, fmt.Errorf("failed to create VergeOS client")
	}
	return sdk, nil
}
