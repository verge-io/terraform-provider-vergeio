// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const tokenEndpoint = "api/sys/tokens"

// UserLoginRejected posts {"login","password"} to /api/sys/tokens with no
// Authorization header. A 2xx response with a $key means that password is the
// stored credential. VergeOS returns 201 Created for a new token. Any other
// status means the password was rejected.
//
// GET /api/v4 with basic auth is not used. On the lab, that check still
// returned 200 for the password from the previous step after a rotation,
// while the new password also returned 200. The token call does not send
// the candidate as basic auth, so it cannot reuse that accepted pair.
func UserLoginRejected(host, username, password string) (bool, int, error) {
	endpoint := tokenURL(host)
	payload, err := json.Marshal(map[string]string{
		"login":    username,
		"password": password,
	})
	if err != nil {
		return false, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-JSON-Non-Compact", "1")

	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, // lab matches the provider insecure flag
		},
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return false, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return false, resp.StatusCode, err
	}

	rejected, token, err := TokenLoginOutcome(resp.StatusCode, respBody)
	if err != nil {
		return false, resp.StatusCode, fmt.Errorf("login as %s: %w", username, err)
	}
	if !rejected {
		deleteLoginToken(httpClient, endpoint, token)
	}
	return rejected, resp.StatusCode, nil
}

// TokenLoginOutcome classifies a POST /api/sys/tokens response.
// Any 2xx with a $key is a successful login. VergeOS uses 201 Created.
// Every other status is a rejected login.
func TokenLoginOutcome(status int, body []byte) (bool, string, error) {
	if status < 200 || status >= 300 {
		return true, "", nil
	}
	var parsed struct {
		Key any `json:"$key"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false, "", fmt.Errorf("status %d without a token", status)
	}
	token, ok := tokenKey(parsed.Key)
	if !ok {
		return false, "", fmt.Errorf("status %d without a token", status)
	}
	return false, token, nil
}

func tokenKey(v any) (string, bool) {
	switch k := v.(type) {
	case string:
		return k, k != ""
	case float64:
		return fmt.Sprintf("%.0f", k), true
	default:
		return "", false
	}
}

func tokenURL(host string) string {
	return strings.TrimRight(EnsureHTTPSPrefix(host), "/") + "/" + tokenEndpoint
}

func deleteLoginToken(client *http.Client, tokensEndpoint, token string) {
	if token == "" {
		return
	}
	req, err := http.NewRequest(http.MethodDelete, ObjectPath(tokensEndpoint, token), nil)
	if err != nil {
		return
	}
	req.Header.Set("x-yottabyte-token", token)
	resp, err := client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
}
