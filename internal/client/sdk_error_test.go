// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/verge-io/govergeos"
)

func TestNewVergeosClientNilClient(t *testing.T) {
	sdk, err := (*Client)(nil).NewVergeosClient()
	if err == nil || sdk != nil {
		t.Fatalf("sdk=%v err=%v", sdk, err)
	}
	if !strings.Contains(err.Error(), "vergeio client is nil") {
		t.Fatalf("error = %v", err)
	}
}

func TestCachedVergeosClientNilClient(t *testing.T) {
	sdk, err := (*Client)(nil).CachedVergeosClient()
	if err == nil || sdk != nil {
		t.Fatalf("sdk=%v err=%v", sdk, err)
	}
	if !strings.Contains(err.Error(), "vergeio client is nil") {
		t.Fatalf("error = %v", err)
	}
}

func TestNewVergeosClientRejectsUnsupportedVersion(t *testing.T) {
	sdk, err := versionClient(t, "25.0.0", true).NewVergeosClient()
	if err == nil || sdk != nil {
		t.Fatalf("sdk=%v err=%v", sdk, err)
	}
	if !vergeos.IsUnsupportedVersionError(err) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "failed to create VergeOS client") {
		t.Fatalf("error = %v", err)
	}
}

func TestCachedVergeosClientRetriesAfterFailure(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}

		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = w.Write([]byte(`{"version":"25.0.0"}`))
			return
		}
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "user", "pass", true)
	sdk, err := client.CachedVergeosClient()
	if err == nil || sdk != nil {
		t.Fatalf("first sdk=%v err=%v", sdk, err)
	}
	sdk, err = client.CachedVergeosClient()
	if err != nil || sdk == nil {
		t.Fatalf("retry sdk=%v err=%v", sdk, err)
	}
	again, err := client.CachedVergeosClient()
	if err != nil || again != sdk {
		t.Fatalf("cached client was not reused: sdk=%v again=%v err=%v", sdk, again, err)
	}
}

func TestNewVergeosClientRejectsUntrustedCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClientWithConfig(ClientConfig{
		Host:     server.URL,
		Username: "user",
		Password: "pass",
		Insecure: false,
	})
	sdk, err := client.NewVergeosClient()
	if err == nil || sdk != nil {
		t.Fatalf("sdk=%v err=%v", sdk, err)
	}
	if !strings.Contains(err.Error(), "certificate") && !strings.Contains(err.Error(), "failed to check server version") {
		t.Fatalf("error = %v", err)
	}
}

func versionClient(t *testing.T, version string, insecure bool) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"` + version + `"}`))
	}))
	t.Cleanup(server.Close)
	return NewClient(server.URL, "user", "pass", insecure)
}
