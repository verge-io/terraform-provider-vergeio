// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewClientSetsTimeout(t *testing.T) {
	client := NewClient("example.test", "user", "pass", true)
	if client.Timeout() != DefaultTimeout {
		t.Fatalf("Timeout() = %s, want %s", client.Timeout(), DefaultTimeout)
	}
	if client.Timeout() <= 0 {
		t.Fatal("HTTP client timeout is zero")
	}

	transport, ok := client.httpClient.Transport.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil || !transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("insecure flag was not applied to the HTTP transport")
	}
}

func TestDoTimesOut(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	client.httpClient.Timeout = 200 * time.Millisecond

	start := time.Now()
	_, err := client.Get(context.Background(), "api/v4/version", nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context deadline exceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("request took %s; the client timeout should have stopped it", elapsed)
	}
}

func TestDoHonorsContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := client.Get(ctx, "api/v4/version", nil)
		done <- err
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight request was not canceled")
	}
}

func TestDoRequiresContext(t *testing.T) {
	client := NewClient("https://example.invalid", "user", "pass", true)
	_, err := client.Get(nil, "api/v4/version", nil)
	if err == nil || !strings.Contains(err.Error(), "missing request context") {
		t.Fatalf("error = %v, want a missing context error", err)
	}
}

func TestDoDoesNotLazilyCreateHTTPClient(t *testing.T) {
	client := &Client{
		Host:     "https://example.invalid",
		Username: "user",
		Password: "pass",
		Insecure: true,
	}

	const workers = 16
	var wg sync.WaitGroup
	wg.Add(workers)
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			_, err := client.Get(context.Background(), "api/v4/version", nil)
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err == nil || !strings.Contains(err.Error(), "not initialized") {
			t.Errorf("error = %v, want an uninitialized client error", err)
		}
	}
	if client.httpClient != nil {
		t.Fatal("Do created an HTTP client on first use")
	}
	if client.Timeout() != 0 {
		t.Fatalf("Timeout() = %s, want 0 for a client that was not built with NewClient", client.Timeout())
	}
}

func TestConcurrentDoDoesNotRace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"response":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)

	const workers = 32
	var wg sync.WaitGroup
	wg.Add(workers)
	errCh := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			resp, err := client.Get(context.Background(), "api/v4/version", nil)
			if err != nil {
				errCh <- err
				return
			}
			errCh <- resp.Body.Close()
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Error(err)
		}
	}
}
