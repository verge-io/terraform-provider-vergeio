// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPostFinishesAfterCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		cancel()
		time.Sleep(30 * time.Millisecond)
		if err := r.Context().Err(); err != nil {
			t.Errorf("POST context ended after cancel: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"$key":4}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	resp, err := client.Post(ctx, "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-int","url":"https://example.com/hook"}`))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if closeErr != nil {
		t.Error(closeErr)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"$key":4`) {
		t.Fatalf("body = %s", body)
	}
}

func TestPostDoesNotStartWhenAlreadyCanceled(t *testing.T) {
	hit := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Post(ctx, "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-int"}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if hit {
		t.Fatal("POST was sent after the context was already canceled")
	}
}

func TestPostTimeoutStillApplies(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		select {
		case <-r.Context().Done():
		case <-time.After(time.Second):
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	client.httpClient.Timeout = 200 * time.Millisecond
	start := time.Now()
	_, err := client.Post(context.Background(), "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-int"}`))
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("POST took %s after the client timeout", elapsed)
	}
}

func TestInFlightGetStillCancels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.Get(ctx, "api/v4/webhook_urls/1", nil)
		done <- err
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("in-flight GET was not canceled")
	}
}
