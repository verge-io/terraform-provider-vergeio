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
	"testing"
	"time"

	"github.com/verge-io/govergeos"
)

func TestReadinessSDKCreateKeepsResponseBodyAlive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		if r.URL.Path == "/version.json" {
			writeResponse(t, w, `{"version":"26.2.2.1"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			flushResponse(t, w)
			time.Sleep(20 * time.Millisecond)
			writeResponse(t, w, `{"$key":1}`)
			return
		}
		writeResponse(t, w, `{"$key":1,"name":"response-body-probe","url":"https://example.invalid/hook"}`)
	}))
	defer server.Close()

	c := NewClientWithConfig(ClientConfig{Host: server.URL, APIKey: "test-token", Timeout: time.Second})
	sdk, err := c.SDK()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := sdk.WebhookURLs.Create(ctx, &vergeos.WebhookURLCreateRequest{
		Name: "response-body-probe",
		URL:  "https://example.invalid/hook",
	})
	if err != nil {
		t.Fatalf("normal create with live caller context failed: %v; caller context error: %v", err, ctx.Err())
	}
	if created == nil || created.Key.Int() != 1 || created.Name != "response-body-probe" {
		t.Fatalf("created = %#v", created)
	}
}

func TestSDKCreateReadsDelayedBodyAfterInterrupt(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx := context.WithoutCancel(parent)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		if r.URL.Path == "/version.json" {
			writeResponse(t, w, `{"version":"26.2.2.1"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			cancel()
			if err := r.Context().Err(); err != nil {
				t.Errorf("POST context ended after cancel: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			flushResponse(t, w)
			time.Sleep(20 * time.Millisecond)
			if err := r.Context().Err(); err != nil {
				t.Errorf("POST context ended before the body: %v", err)
			}
			writeResponse(t, w, `{"$key":1}`)
			return
		}
		if err := r.Context().Err(); err != nil {
			t.Errorf("%s context ended before the body: %v", r.Method, err)
		}
		w.WriteHeader(http.StatusOK)
		flushResponse(t, w)
		time.Sleep(20 * time.Millisecond)
		if err := r.Context().Err(); err != nil {
			t.Errorf("%s context ended before the delayed body: %v", r.Method, err)
		}
		writeResponse(t, w, `{"$key":1,"name":"response-body-probe","url":"https://example.invalid/hook"}`)
	}))
	defer server.Close()

	c := NewClientWithConfig(ClientConfig{Host: server.URL, APIKey: "test-token", Timeout: time.Second})
	sdk, err := c.SDK()
	if err != nil {
		t.Fatal(err)
	}
	created, err := sdk.WebhookURLs.Create(ctx, &vergeos.WebhookURLCreateRequest{
		Name: "response-body-probe",
		URL:  "https://example.invalid/hook",
	})
	if err != nil {
		t.Fatalf("interrupted create failed: %v; caller context error: %v", err, parent.Err())
	}
	if parent.Err() == nil {
		t.Fatal("caller context was not canceled")
	}
	if ctx.Err() != nil {
		t.Fatalf("create context error: %v", ctx.Err())
	}
	if created == nil || created.Key.Int() != 1 || created.Name != "response-body-probe" {
		t.Fatalf("created = %#v", created)
	}
}

func TestPostFinishesDelayedBodyAfterCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		cancel()
		if err := r.Context().Err(); err != nil {
			t.Errorf("POST context ended after cancel: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		flushResponse(t, w)
		time.Sleep(20 * time.Millisecond)
		if err := r.Context().Err(); err != nil {
			t.Errorf("POST context ended before the body: %v", err)
		}
		writeResponse(t, w, `{"$key":4}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	resp, err := client.Post(ctx, "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-int","url":"https://example.invalid/hook"}`))
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
	if string(body) != `{"$key":4}` {
		t.Fatalf("body = %s", body)
	}
}

func TestPostDeadlineReadsDelayedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		flushResponse(t, w)
		time.Sleep(20 * time.Millisecond)
		writeResponse(t, w, `{"$key":4}`)
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resp, err := client.Post(ctx, "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-int","url":"https://example.invalid/hook"}`))
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
	if ctx.Err() != nil {
		t.Fatalf("caller context error: %v", ctx.Err())
	}
	if string(body) != `{"$key":4}` {
		t.Fatalf("body = %s", body)
	}
}

func TestPostDeadlineStillBoundsDelayedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		flushResponse(t, w)
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
			writeResponse(t, w, `{"$key":4}`)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	resp, err := client.Post(ctx, "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-int"}`))
	if err == nil {
		body, readErr := io.ReadAll(resp.Body)
		closeErr := resp.Body.Close()
		if closeErr != nil {
			t.Error(closeErr)
		}
		err = readErr
		if err == nil {
			t.Fatalf("read delayed body = %s", body)
		}
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("POST took %s after the caller deadline", elapsed)
	}
}

func flushResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	flusher, ok := w.(http.Flusher)
	if !ok {
		t.Error("response writer cannot flush headers")
		return
	}
	flusher.Flush()
}

func writeResponse(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write response: %v", err)
	}
}
