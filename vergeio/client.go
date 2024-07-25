package vergeio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
)

// Options represents an option from the Verge.IO api.
type Options struct {
	Limit  string
	Offset string
	Sort   string
	Fields string
	Filter string
}

// VergeResponse structure
type VergeResponse struct {
	Key   string `json:"$key,omitempty"`
	Error string `json:"err,omitempty"`
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

// Client is the base internal Client to talk to the Verge.IO API. This should be a username and password and host
type Client struct {
	Username   string
	Password   string
	Host       string
	HTTPClient *http.Client
}

// Do Will just call the Verge.IO api but also add auth to it and some extra headers
func (c *Client) Do(method string, endpoint string, payload *bytes.Buffer, params *Options) (*http.Response, error) {

	absoluteendpoint := c.Host + "/" + endpoint
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
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] Resp: %v Err: %v", resp, err)

	if resp.StatusCode >= 400 || resp.StatusCode < 200 {
		apiError := Error{
			StatusCode: resp.StatusCode,
			Endpoint:   endpoint,
		}

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		log.Printf("[DEBUG] Resp Body: %s", string(body))

		test := VergeResponse{}
		err = json.Unmarshal(body, &test)
		if err != nil {
			log.Printf("UNMARSHALL ERROR: %s", string(err.Error()))
			apiError.VergeError = string(body)
		} else {
			apiError.VergeError = test.Error
		}

		return nil, error(apiError)

	}
	return resp, err
}

// Get is just a helper method to do but with a GET verb
func (c *Client) Get(endpoint string, params *Options) (*http.Response, error) {

	return c.Do("GET", endpoint, nil, params)
}

// Post is just a helper method to do but with a POST verb
func (c *Client) Post(endpoint string, jsonpayload *bytes.Buffer) (*http.Response, error) {
	return c.Do("POST", endpoint, jsonpayload, nil)
}

// Put is just a helper method to do but with a PUT verb
func (c *Client) Put(endpoint string, jsonpayload *bytes.Buffer) (*http.Response, error) {
	return c.Do("PUT", endpoint, jsonpayload, nil)
}

// Delete is just a helper to Do but with a DELETE verb
func (c *Client) Delete(endpoint string) (*http.Response, error) {
	return c.Do("DELETE", endpoint, nil, nil)
}
