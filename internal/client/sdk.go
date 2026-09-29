// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
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
