// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/verge-io/govergeos"
)

func TestCertificateGateExcludesWritesAndNotReads(t *testing.T) {
	var writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/version.json":
			_, err := io.WriteString(w, `{"version":"26.0.0"}`)
			if err != nil {
				t.Errorf("write version: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/version":
			_, err := io.WriteString(w, `{"response":"ok"}`)
			if err != nil {
				t.Errorf("write version api: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/webhook_urls":
			writes.Add(1)
			_, err := io.WriteString(w, `{"$key":1}`)
			if err != nil {
				t.Errorf("write webhook: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/webhook_urls/1":
			_, err := io.WriteString(w, `{"$key":1,"name":"tf-acc-hook","url":"https://example.com/hook"}`)
			if err != nil {
				t.Errorf("write webhook read: %v", err)
			}
		default:
			if r.Method != http.MethodGet {
				writes.Add(1)
			}
			_, err := io.WriteString(w, `{"response":"ok"}`)
			if err != nil {
				t.Errorf("write response: %v", err)
			}
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	client.certificateSettle = time.Millisecond
	client.directRetryBackoff = time.Millisecond

	inFn := make(chan struct{})
	releaseFn := make(chan struct{})
	gateDone := make(chan error, 1)
	go func() {
		gateDone <- client.WithCertificateMutation(context.Background(), func(context.Context) error {
			close(inFn)
			<-releaseFn
			return nil
		})
	}()

	select {
	case <-inFn:
	case <-time.After(2 * time.Second):
		t.Fatal("certificate operation did not start")
	}

	readCtx, cancelRead := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelRead()
	resp, err := client.Get(readCtx, "api/v4/version", nil)
	if err != nil {
		t.Fatalf("GET during certificate operation: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}

	sdk, err := client.CachedVergeosClient()
	if err != nil {
		t.Fatalf("SDK reads during certificate operation: %v", err)
	}

	writeDone := make(chan error, 4)
	startWrite := func(fn func() error) {
		go func() { writeDone <- fn() }()
	}
	startWrite(func() error {
		response, err := client.Post(context.Background(), "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-a"}`))
		if err != nil {
			return err
		}
		return response.Body.Close()
	})
	startWrite(func() error {
		response, err := client.Put(context.Background(), "api/v4/settings/x", bytes.NewBufferString(`{"value":"1"}`))
		if err != nil {
			return err
		}
		return response.Body.Close()
	})
	startWrite(func() error {
		response, err := client.Delete(context.Background(), "api/v4/webhook_urls/1")
		if err != nil {
			return err
		}
		return response.Body.Close()
	})
	startWrite(func() error {
		_, err := sdk.WebhookURLs.Create(context.Background(), &vergeos.WebhookURLCreateRequest{
			Name: "tf-acc-hook",
			URL:  "https://example.com/hook",
		})
		return err
	})

	time.Sleep(150 * time.Millisecond)
	if got := writes.Load(); got != 0 {
		t.Fatalf("writes during certificate operation = %d", got)
	}

	close(releaseFn)
	for i := 0; i < 4; i++ {
		select {
		case err := <-writeDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("write did not finish after the certificate gate opened")
		}
	}
	if err := <-gateDone; err != nil {
		t.Fatal(err)
	}
	if got := writes.Load(); got != 4 {
		t.Fatalf("writes after the gate opened = %d, want 4", got)
	}
}

func TestCertificateOperationsExcludeEachOther(t *testing.T) {
	client := NewClient("https://example.test", "user", "pass", true)
	client.certificateSettle = time.Millisecond

	firstIn := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondIn := make(chan struct{})
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		firstDone <- client.WithCertificateMutation(context.Background(), func(context.Context) error {
			close(firstIn)
			<-releaseFirst
			return nil
		})
	}()
	select {
	case <-firstIn:
	case <-time.After(2 * time.Second):
		t.Fatal("first certificate operation did not start")
	}

	go func() {
		secondDone <- client.WithCertificateMutation(context.Background(), func(context.Context) error {
			close(secondIn)
			return nil
		})
	}()
	select {
	case <-secondIn:
		t.Fatal("second certificate operation overlapped the first")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseFirst)
	select {
	case <-secondIn:
	case <-time.After(2 * time.Second):
		t.Fatal("second certificate operation did not start")
	}
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
}

func TestWritesShareTheCertificateGate(t *testing.T) {
	both := make(chan struct{})
	var arrived atomic.Int32
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if arrived.Add(1) == 2 {
				close(both)
			}
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		_, err := io.WriteString(w, `{"response":"ok"}`)
		if err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "user", "pass", true)
	var wg sync.WaitGroup
	wg.Add(2)
	errCh := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			resp, err := client.Post(context.Background(), "api/v4/things", bytes.NewBufferString(`{"name":"tf-acc"}`))
			if err != nil {
				errCh <- err
				return
			}
			errCh <- resp.Body.Close()
		}()
	}

	select {
	case <-both:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent writes did not overlap")
	}
	close(release)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestCertificateGateClosesIdleConnections(t *testing.T) {
	transport := &closeCountingTransport{}
	client := NewClient("https://example.test", "user", "pass", true)
	client.httpClient.Transport = transport
	client.certificateSettle = time.Millisecond

	err := client.WithCertificateMutation(context.Background(), func(context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if transport.closed.Load() != 1 {
		t.Fatalf("CloseIdleConnections calls = %d, want 1", transport.closed.Load())
	}
}

func TestCertificateSettleHoldsWriters(t *testing.T) {
	client := NewClient("https://example.test", "user", "pass", true)
	client.certificateSettle = 200 * time.Millisecond

	release := make(chan struct{})
	inFn := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- client.WithCertificateMutation(context.Background(), func(context.Context) error {
			close(inFn)
			<-release
			return nil
		})
	}()
	select {
	case <-inFn:
	case <-time.After(2 * time.Second):
		t.Fatal("certificate operation did not start")
	}

	acquired := make(chan time.Time, 1)
	go func() {
		client.mutationGate.RLock()
		acquired <- time.Now()
		client.mutationGate.RUnlock()
	}()

	select {
	case <-acquired:
		t.Fatal("a write acquired the gate while a certificate operation was running")
	case <-time.After(50 * time.Millisecond):
	}

	returned := time.Now()
	close(release)
	select {
	case when := <-acquired:
		if when.Sub(returned) < 120*time.Millisecond {
			t.Fatalf("write acquired %s after the certificate request, want at least 120ms", when.Sub(returned))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("write did not acquire the gate after the settle window")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

type closeCountingTransport struct {
	closed atomic.Int32
}

func (t *closeCountingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, io.EOF
}

func (t *closeCountingTransport) CloseIdleConnections() {
	t.closed.Add(1)
}
