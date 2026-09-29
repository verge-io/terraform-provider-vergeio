// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// All the Verge.IO endpoints.
const (
	APIEndpoint = "api/v4"

	// DefaultTimeout is the overall limit for one HTTP request, including
	// connection, redirects, and reading the body. Five seconds is too short
	// for clones and imports. A stalled response still has to give up so
	// Terraform does not wait forever.
	DefaultTimeout = 60 * time.Second
)

// IClient interface.
type IClient interface {
	Name() string
}

// Client is the base internal Client to talk to the Verge.IO API. This should be a username and password and host.
type Client struct {
	name       string
	Username   string
	Password   string
	Host       string
	Insecure   bool
	httpClient *http.Client
	FieldCache *FieldCache
}

// Name returns the name of the client.
func (c *Client) Name() string {
	return c.name
}

// serverURL returns the server URL using host and endpoint.
func (c *Client) serverURL(endpoint string) string {
	return EnsureHTTPSPrefix(c.Host) + "/" + endpoint
}

// NewClient returns a new Verge.IO client.
func NewClient(host string,
	username string,
	password string,
	insecure bool,
) *Client {
	return &Client{
		name:     "Base Client",
		Host:     host,
		Username: username,
		Password: password,
		Insecure: insecure,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
				TLSClientConfig:     &tls.Config{InsecureSkipVerify: insecure},
			},
			Timeout: DefaultTimeout,
		},
	}
}

// Timeout reports the overall HTTP client timeout. It is zero when the
// client was not built with NewClient.
func (c *Client) Timeout() time.Duration {
	if c == nil || c.httpClient == nil {
		return 0
	}
	return c.httpClient.Timeout
}

// Options represents an option from the Verge.IO api.
type Options struct {
	Limit  string
	Offset string
	Sort   string
	Fields string
	Filter string
}

// VergeResponse structure.
type VergeResponse struct {
	Key      string `json:"$key,omitempty"`
	Response string `json:"response,omitempty"`
	Error    string `json:"err,omitempty"`
}

// Error represents a error from the Verge.IO api.
type Error struct {
	VergeError string
	StatusCode int
	Endpoint   string
}

func (e Error) Error() string {
	return fmt.Sprintf("[ API Error %d ] @ %s - %s", e.StatusCode, e.Endpoint, e.VergeError)
}

// Do calls the Verge.IO API, adding auth and extra headers.
// ctx is the Terraform request context. Canceling it (Ctrl+C / SIGINT)
// cancels the HTTP request. The client itself is created in NewClient;
// Do does not build one.
func (c *Client) Do(ctx context.Context, method string, endpoint string, payload *bytes.Buffer, params *Options) (*http.Response, error) {
	if c == nil || c.httpClient == nil {
		return nil, errors.New("HTTP client is not initialized")
	}
	if ctx == nil {
		return nil, errors.New("missing request context")
	}

	absoluteendpoint := c.serverURL(endpoint)
	log.Printf("[DEBUG] Sending %s request to %s", method, absoluteendpoint)

	var bodyreader io.Reader

	if payload != nil {
		// Secret fields stay in the request. Only the debug line is masked.
		log.Printf("[DEBUG] With payload %s", maskSecretPayload(payload.String()))
		bodyreader = payload
	}

	req, err := http.NewRequestWithContext(ctx, method, absoluteendpoint, bodyreader)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(c.Username, c.Password)
	qs := req.URL.Query()
	if method == "GET" {
		log.Printf("[DEBUG] params %#v", params)
		qs.Set("fields", "most")
		if params != nil {
			if params.Fields != "" {
				qs.Set("fields", params.Fields)
			}
			if params.Filter != "" {
				qs.Set("filter", params.Filter)
			}
			if params.Sort != "" {
				qs.Set("sort", params.Sort)
			}
			if params.Limit != "" {
				qs.Set("limit", params.Limit)
			}
			if params.Offset != "" {
				qs.Set("offset", params.Offset)
			}
		}
		req.URL.RawQuery = qs.Encode()
	}
	if payload != nil {
		req.Header.Add("Content-Type", "application/json")
	}
	req.Close = true

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] Resp: %v Err: %v", resp, err)

	if resp.StatusCode >= 400 || resp.StatusCode < 200 {
		apiError := Error{
			StatusCode: resp.StatusCode,
			Endpoint:   endpoint,
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		log.Printf("[DEBUG] Resp Body: %s", maskSecretPayload(string(body)))

		test := VergeResponse{}
		err = json.Unmarshal(body, &test)
		if err != nil {
			log.Printf("UNMARSHALL ERROR: %s", err.Error())
			apiError.VergeError = string(body)
		} else {
			apiError.VergeError = test.Error
		}

		return nil, error(apiError)

	}
	return resp, err
}

// Get is just a helper method to do but with a GET verb.
func (c *Client) Get(ctx context.Context, endpoint string, params *Options) (*http.Response, error) {
	return c.Do(ctx, "GET", endpoint, nil, params)
}

// Post is just a helper method to do but with a POST verb.
func (c *Client) Post(ctx context.Context, endpoint string, jsonpayload *bytes.Buffer) (*http.Response, error) {
	return c.Do(ctx, "POST", endpoint, jsonpayload, nil)
}

// Put is just a helper method to do but with a PUT verb.
func (c *Client) Put(ctx context.Context, endpoint string, jsonpayload *bytes.Buffer) (*http.Response, error) {
	return c.Do(ctx, "PUT", endpoint, jsonpayload, nil)
}

// Delete is just a helper to Do but with a DELETE verb.
func (c *Client) Delete(ctx context.Context, endpoint string) (*http.Response, error) {
	return c.Do(ctx, "DELETE", endpoint, nil, nil)
}
