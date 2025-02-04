// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// All the Verge.IO endpoints.
const (
	APIEndpoint = "api/v4"
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
}

// Name returns the name of the client.
func (c *Client) Name() string {
	return c.name
}

// serverURL returns the server URL using host and endpoint.
func (c *Client) serverURL(endpoint string) string {
	return "https://" + c.Host + "/" + endpoint
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
			Timeout: time.Duration(5) * time.Second,
		},
	}
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

// Do Will just call the Verge.IO api but also add auth to it and some extra headers.
func (c *Client) Do(method string, endpoint string, payload *bytes.Buffer, params *Options) (*http.Response, error) {

	absoluteendpoint := c.serverURL(endpoint)
	log.Printf("[DEBUG] Sending %s request to %s", method, absoluteendpoint)

	var bodyreader io.Reader

	if payload != nil {
		log.Printf("[DEBUG] With payload %s", payload.String())
		bodyreader = payload
	}

	req, err := http.NewRequest(method, absoluteendpoint, bodyreader)
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

	// Create a custom HTTP client with the insecure option if needed
	if c.httpClient == nil {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: c.Insecure},
		}
		c.httpClient = &http.Client{Transport: tr}
	}

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

		log.Printf("[DEBUG] Resp Body: %s", body)

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
func (c *Client) Get(endpoint string, params *Options) (*http.Response, error) {

	return c.Do("GET", endpoint, nil, params)
}

// Post is just a helper method to do but with a POST verb.
func (c *Client) Post(endpoint string, jsonpayload *bytes.Buffer) (*http.Response, error) {
	return c.Do("POST", endpoint, jsonpayload, nil)
}

// Put is just a helper method to do but with a PUT verb.
func (c *Client) Put(endpoint string, jsonpayload *bytes.Buffer) (*http.Response, error) {
	return c.Do("PUT", endpoint, jsonpayload, nil)
}

// Delete is just a helper to Do but with a DELETE verb.
func (c *Client) Delete(endpoint string) (*http.Response, error) {
	return c.Do("DELETE", endpoint, nil, nil)
}
