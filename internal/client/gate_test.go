// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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

func TestDefaultCertificateSettleCoversDeferredReload(t *testing.T) {
	if certificateReloadGap < 3*time.Second {
		t.Fatalf("reload gap = %s, want at least 3s", certificateReloadGap)
	}
	if certificateReloadDuration < 600*time.Millisecond {
		t.Fatalf("reload duration = %s, want at least 600ms", certificateReloadDuration)
	}
	if defaultCertificateSettle < certificateReloadGap+certificateReloadDuration {
		t.Fatalf("settle = %s, want at least %s", defaultCertificateSettle, certificateReloadGap+certificateReloadDuration)
	}
	if defaultCertificateSettle <= 1500*time.Millisecond {
		t.Fatalf("settle = %s still ends before a deferred reload", defaultCertificateSettle)
	}
}

func TestCertificateReloadError(t *testing.T) {
	reset := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "reset", err: reset, want: true},
		{name: "refused", err: refused, want: true},
		{name: "eof", err: io.EOF, want: true},
		{name: "reset text", err: errors.New("read tcp 10.0.0.1:443: connection reset by peer"), want: true},
		{name: "refused text", err: errors.New("dial tcp 10.0.0.1:443: connect: connection refused"), want: true},
		{name: "probe timeout", err: context.DeadlineExceeded, want: true},
		{name: "dial timeout", err: timeoutNetError{}, want: true},
		{name: "canceled", err: context.Canceled, want: false},
		{name: "api status", err: Error{StatusCode: http.StatusInternalServerError, VergeError: "no"}, want: false},
		{name: "tls", err: errors.New("tls: handshake failure"), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := certificateReloadError(tc.err); got != tc.want {
				t.Fatalf("certificateReloadError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestWritesWaitUntilCertificateAPIAnswers(t *testing.T) {
	reset := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	entered := make(chan struct{})
	release := make(chan struct{})
	transport := &scriptedTransport{
		outcomes: []error{reset, nil},
		onCall: func(n int) {
			if n == 1 {
				close(entered)
				<-release
			}
		},
	}
	client := certificateProbeClient(t, transport)

	done := make(chan error, 1)
	go func() {
		done <- client.WithCertificateMutation(context.Background(), func(context.Context) error {
			return nil
		})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("version probe did not start")
	}

	acquired := make(chan struct{})
	go func() {
		client.mutationGate.RLock()
		close(acquired)
		client.mutationGate.RUnlock()
	}()
	select {
	case <-acquired:
		t.Fatal("a write acquired the gate while the API was still resetting connections")
	case <-time.After(40 * time.Millisecond):
	}
	close(release)
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("write did not acquire the gate after the API answered")
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if transport.calls < 2 {
		t.Fatalf("probes = %d, want at least 2", transport.calls)
	}
}

func TestCertificateReadyRetriesClosedConnections(t *testing.T) {
	cases := []struct {
		name    string
		outcome error
	}{
		{name: "reset", outcome: &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}},
		{name: "refused", outcome: &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}},
		{name: "eof", outcome: io.EOF},
		{name: "reset text", outcome: errors.New("read tcp 10.0.0.1:443: connection reset by peer")},
		{name: "probe timeout", outcome: context.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &scriptedTransport{outcomes: []error{tc.outcome, nil}}
			client := certificateProbeClient(t, transport)
			if err := client.WithCertificateMutation(context.Background(), func(context.Context) error {
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if transport.calls != 2 {
				t.Fatalf("probes = %d, want 2", transport.calls)
			}
		})
	}
}

func TestCertificateReadyAcceptsAnHTTPResponse(t *testing.T) {
	transport := &scriptedTransport{outcomes: []error{statusError(http.StatusInternalServerError)}}
	client := certificateProbeClient(t, transport)
	client.certificateReadyTimeout = time.Second

	start := time.Now()
	if err := client.WithCertificateMutation(context.Background(), func(context.Context) error {
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 {
		t.Fatalf("probes = %d, want 1", transport.calls)
	}
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("hold = %s, want the settle plus one answered probe", elapsed)
	}
}

func TestCertificateReadyReturnsOtherErrors(t *testing.T) {
	transport := &scriptedTransport{outcomes: []error{errors.New("tls: handshake failure"), nil}}
	client := certificateProbeClient(t, transport)
	err := client.WithCertificateMutation(context.Background(), func(context.Context) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "tls: handshake failure") {
		t.Fatal(err)
	}
	if transport.calls != 1 {
		t.Fatalf("probes = %d, want 1", transport.calls)
	}
}

func TestCertificateReadyGivesUp(t *testing.T) {
	transport := &resetTransport{}
	client := certificateProbeClient(t, transport)
	client.certificateSettle = 5 * time.Millisecond
	client.certificateReadyTimeout = 40 * time.Millisecond
	client.certificateReadyInterval = 5 * time.Millisecond

	err := client.WithCertificateMutation(context.Background(), func(context.Context) error {
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "API did not accept connections after the certificate change") {
		t.Fatal(err)
	}
	if !certificateReloadError(err) {
		t.Fatalf("error = %v, want the connection reset", err)
	}
	if transport.calls.Load() < 2 {
		t.Fatalf("probes = %d, want more than one", transport.calls.Load())
	}
}

func TestCertificateReadyStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := &resetTransport{}
	client := certificateProbeClient(t, transport)
	client.certificateSettle = time.Second
	client.certificateReadyTimeout = time.Second

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	err := client.WithCertificateMutation(ctx, func(context.Context) error { return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("quiet wait ignored context cancellation")
	}
	if transport.calls.Load() != 0 {
		t.Fatalf("probes = %d, want 0", transport.calls.Load())
	}
}

func TestOptionalCertificateMutationSkipsReloadWhenUnchanged(t *testing.T) {
	client := NewClient("https://example.test", "user", "pass", true)
	client.certificateSettle = 2 * time.Second

	started := make(chan struct{})
	acquired := make(chan struct{})
	go func() {
		<-started
		client.mutationGate.RLock()
		close(acquired)
		client.mutationGate.RUnlock()
	}()

	start := time.Now()
	err := client.WithOptionalCertificateMutation(context.Background(), func(context.Context) (bool, error) {
		close(started)
		select {
		case <-acquired:
			return false, errors.New("write entered during the certificate read")
		case <-time.After(50 * time.Millisecond):
			return false, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("an unchanged certificate operation waited out the reload")
	}
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("write did not acquire the gate after the unchanged operation")
	}
}

func TestOptionalCertificateMutationReturnsReadErrorWithoutWaiting(t *testing.T) {
	client := NewClient("https://example.test", "user", "pass", true)
	client.certificateSettle = 2 * time.Second
	start := time.Now()
	err := client.WithOptionalCertificateMutation(context.Background(), func(context.Context) (bool, error) {
		return false, errors.New("read failed")
	})
	if err == nil || err.Error() != "read failed" {
		t.Fatalf("error = %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("a failed certificate read waited out the reload")
	}
}

func TestOptionalCertificateMutationWaitsAfterAChange(t *testing.T) {
	client := NewClient("https://example.test", "user", "pass", true)
	client.certificateSettle = 200 * time.Millisecond
	start := time.Now()
	err := client.WithOptionalCertificateMutation(context.Background(), func(context.Context) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("a certificate change did not wait out the reload")
	}
}

func TestCertificateReadStaysInsideTheGate(t *testing.T) {
	client := NewClient("https://example.test", "user", "pass", true)
	client.certificateSettle = 20 * time.Millisecond

	started := make(chan struct{})
	acquired := make(chan struct{})
	go func() {
		<-started
		client.mutationGate.RLock()
		close(acquired)
		client.mutationGate.RUnlock()
	}()

	err := client.WithCertificateMutationAfter(context.Background(), func(context.Context) error {
		return nil
	}, func(context.Context) error {
		close(started)
		select {
		case <-acquired:
			return errors.New("write entered during the certificate read")
		case <-time.After(50 * time.Millisecond):
			return nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-acquired:
	case <-time.After(2 * time.Second):
		t.Fatal("write did not acquire the gate after the certificate read")
	}
}

func TestCertificateReadDoesNotRunWhileTheAPIIsDown(t *testing.T) {
	transport := &resetTransport{}
	client := certificateProbeClient(t, transport)
	client.certificateSettle = 5 * time.Millisecond
	client.certificateReadyTimeout = 25 * time.Millisecond
	client.certificateReadyInterval = 5 * time.Millisecond

	called := false
	err := client.WithCertificateMutationAfter(context.Background(), func(context.Context) error {
		return nil
	}, func(context.Context) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "API did not accept connections after the certificate change") {
		t.Fatal(err)
	}
	if called {
		t.Fatal("certificate read ran while the API was down")
	}
}

func certificateProbeClient(t *testing.T, transport http.RoundTripper) *Client {
	t.Helper()
	client := NewClient("https://example.test", "user", "pass", true)
	client.httpClient.Transport = transport
	client.certificateSettle = 15 * time.Millisecond
	client.certificateReadyTimeout = time.Second
	client.certificateReadyInterval = 5 * time.Millisecond
	client.certificateProbeTimeout = 50 * time.Millisecond
	return client
}

type resetTransport struct {
	calls atomic.Int32
}

func (t *resetTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return nil, &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
}
