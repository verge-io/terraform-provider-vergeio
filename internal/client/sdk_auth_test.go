// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/verge-io/govergeos"
)

func TestSDKOptionsUsesAPIKeyOverPassword(t *testing.T) {
	var auth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/clusters" {
			auth = r.Header.Get("Authorization")
			if _, _, ok := r.BasicAuth(); ok {
				t.Error("basic auth was sent with an API key")
			}
		}
		if AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	defer server.Close()

	client := NewClientWithConfig(ClientConfig{
		Host:     server.URL,
		Username: "user",
		Password: "pass",
		APIKey:   "token-value",
		Insecure: true,
		Timeout:  5 * time.Second,
	})
	sdk, err := vergeos.NewClient(client.SDKOptions()...)
	if err != nil {
		t.Fatal(err)
	}
	if sdk == nil {
		t.Fatal("sdk client is nil")
	}
	if auth != "Bearer token-value" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestSDKOptionsUsesBasicAuth(t *testing.T) {
	var user, pass string
	var ok bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v4/clusters" {
			user, pass, ok = r.BasicAuth()
		}
		if AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	if _, err := vergeos.NewClient(client.SDKOptions()...); err != nil {
		t.Fatal(err)
	}
	if !ok || user != "user" || pass != "pass" {
		t.Fatalf("basic auth = %q %q ok=%v", user, pass, ok)
	}
}

func TestSDKOptionsInsecureTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	defer server.Close()

	client := NewClientWithConfig(ClientConfig{
		Host:     server.URL,
		Username: "user",
		Password: "pass",
		Insecure: true,
	})
	if _, err := vergeos.NewClient(client.SDKOptions()...); err != nil {
		t.Fatal(err)
	}

	secure := NewClientWithConfig(ClientConfig{
		Host:     server.URL,
		Username: "user",
		Password: "pass",
		Insecure: false,
	})
	_, err := vergeos.NewClient(secure.SDKOptions()...)
	if err == nil {
		t.Fatal("expected TLS verification to reject the test certificate")
	}
	if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "failed to check server version") {
		t.Fatalf("error = %v, want a TLS verification failure", err)
	}
}

func TestSDKOptionsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}

		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	defer server.Close()

	client := NewClientWithConfig(ClientConfig{
		Host:    server.URL,
		APIKey:  "token-value",
		Timeout: 100 * time.Millisecond,
	})
	start := time.Now()
	_, err := vergeos.NewClient(client.SDKOptions()...)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected the version check to time out")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("version check took %s; WithTimeout was not applied", elapsed)
	}
}
