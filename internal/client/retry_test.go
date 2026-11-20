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
	"syscall"
	"testing"
	"time"
)

func TestDirectRetryPolicy(t *testing.T) {
	reset := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	cases := []struct {
		name      string
		method    string
		outcomes  []error
		wantCalls int
		wantErr   bool
		body      string
	}{
		{
			name:      "GET connection reset then success",
			method:    http.MethodGet,
			outcomes:  []error{reset, nil},
			wantCalls: 2,
		},
		{
			name:      "GET eof then success",
			method:    http.MethodGet,
			outcomes:  []error{io.EOF, nil},
			wantCalls: 2,
		},
		{
			name:      "GET unexpected eof then success",
			method:    http.MethodGet,
			outcomes:  []error{io.ErrUnexpectedEOF, nil},
			wantCalls: 2,
		},
		{
			name:      "GET connection reset text then success",
			method:    http.MethodGet,
			outcomes:  []error{errors.New("read tcp 10.0.0.1:443: connection reset by peer"), nil},
			wantCalls: 2,
		},
		{
			name:      "PUT connection reset replays the body",
			method:    http.MethodPut,
			outcomes:  []error{reset, nil},
			wantCalls: 2,
			body:      `{"value":"1"}`,
		},
		{
			name:      "DELETE connection reset then success",
			method:    http.MethodDelete,
			outcomes:  []error{reset, nil},
			wantCalls: 2,
		},
		{
			name:      "GET stops after three resets",
			method:    http.MethodGet,
			outcomes:  []error{reset, reset, reset, nil},
			wantCalls: 3,
			wantErr:   true,
		},
		{
			name:      "POST connection reset is not retried",
			method:    http.MethodPost,
			outcomes:  []error{reset, nil},
			wantCalls: 1,
			wantErr:   true,
			body:      `{"name":"tf-acc"}`,
		},
		{
			name:      "POST eof is not retried",
			method:    http.MethodPost,
			outcomes:  []error{io.EOF, nil},
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:      "GET timeout is not retried",
			method:    http.MethodGet,
			outcomes:  []error{timeoutNetError{}, nil},
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:      "GET 401 is not retried",
			method:    http.MethodGet,
			outcomes:  []error{statusError(http.StatusUnauthorized), nil},
			wantCalls: 1,
			wantErr:   true,
		},
		{
			name:      "GET 500 is not retried",
			method:    http.MethodGet,
			outcomes:  []error{statusError(http.StatusInternalServerError), nil},
			wantCalls: 1,
			wantErr:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transport := &scriptedTransport{outcomes: tc.outcomes}
			client := NewClient("https://example.test", "user", "pass", true)
			client.httpClient.Transport = transport
			client.directRetryBackoff = time.Millisecond

			ctx := context.Background()
			var (
				resp *http.Response
				err  error
			)
			payload := bytes.NewBufferString(tc.body)
			switch tc.method {
			case http.MethodGet:
				resp, err = client.Get(ctx, "api/v4/version", nil)
			case http.MethodPut:
				resp, err = client.Put(ctx, "api/v4/settings/x", payload)
			case http.MethodDelete:
				resp, err = client.Delete(ctx, "api/v4/webhook_urls/1")
			case http.MethodPost:
				resp, err = client.Post(ctx, "api/v4/webhook_urls", payload)
			default:
				t.Fatalf("method %s", tc.method)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if resp != nil && resp.Body != nil {
				if closeErr := resp.Body.Close(); closeErr != nil {
					t.Error(closeErr)
				}
			}
			if transport.calls != tc.wantCalls {
				t.Fatalf("calls = %d, want %d", transport.calls, tc.wantCalls)
			}
			if tc.body != "" {
				if len(transport.bodies) != tc.wantCalls {
					t.Fatalf("bodies = %d, want %d", len(transport.bodies), tc.wantCalls)
				}
				for _, body := range transport.bodies {
					if body != tc.body {
						t.Fatalf("body = %s, want %s", body, tc.body)
					}
				}
			}
		})
	}
}

func TestDirectRetryStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	transport := &scriptedTransport{outcomes: []error{
		&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET},
		nil,
	}, onCall: func(int) { cancel() }}
	client := NewClient("https://example.test", "user", "pass", true)
	client.httpClient.Transport = transport
	client.directRetryBackoff = time.Second

	_, err := client.Get(ctx, "api/v4/version", nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if transport.calls != 1 {
		t.Fatalf("calls = %d, want 1", transport.calls)
	}
}

type scriptedTransport struct {
	outcomes []error
	onCall   func(int)
	calls    int
	bodies   []string
}

func (s *scriptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	if s.onCall != nil {
		s.onCall(s.calls)
	}
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, err
		}
		s.bodies = append(s.bodies, string(body))
	}
	var outcome error
	if s.calls-1 < len(s.outcomes) {
		outcome = s.outcomes[s.calls-1]
	}
	if status, ok := outcome.(statusError); ok {
		return &http.Response{
			StatusCode: int(status),
			Status:     http.StatusText(int(status)),
			Body:       io.NopCloser(bytes.NewReader([]byte(`{"err":"no"}`))),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	}
	if outcome != nil {
		return nil, outcome
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(bytes.NewReader([]byte(`{"response":"ok"}`))),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

type statusError int

func (s statusError) Error() string { return http.StatusText(int(s)) }

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }
