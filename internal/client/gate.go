// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"
)

// defaultCertificateSettle is how long certificate create and delete keep
// other writes out after the request returns. The connection drop lands
// inside this window.
const defaultCertificateSettle = 1500 * time.Millisecond

type certificateGateKey struct{}

// SetCertificateSettle overrides how long certificate create and delete
// keep other writes waiting. A non-positive duration is ignored.
func (c *Client) SetCertificateSettle(d time.Duration) {
	if c == nil || d <= 0 {
		return
	}
	c.certificateSettle = d
}

func (c *Client) certificateSettleDuration() time.Duration {
	if c != nil && c.certificateSettle > 0 {
		return c.certificateSettle
	}
	return defaultCertificateSettle
}

// WithCertificateMutation runs fn while every other write waits, then keeps
// waiting until the certificate connection drop has passed.
func (c *Client) WithCertificateMutation(ctx context.Context, fn func(context.Context) error) error {
	if c == nil {
		return errors.New("vergeio client is nil")
	}
	if ctx == nil {
		return errors.New("missing request context")
	}
	if fn == nil {
		return errors.New("missing certificate operation")
	}
	c.mutationGate.Lock()
	defer c.mutationGate.Unlock()
	defer c.closeIdleConnections()

	err := fn(context.WithValue(ctx, certificateGateKey{}, struct{}{}))
	log.Printf("[DEBUG] waiting %s after certificate create or delete before other writes", c.certificateSettleDuration())
	c.waitCertificateSettle(ctx)
	return err
}

func (c *Client) waitCertificateSettle(ctx context.Context) {
	timer := time.NewTimer(c.certificateSettleDuration())
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (c *Client) closeIdleConnections() {
	if c == nil || c.httpClient == nil {
		return
	}
	c.httpClient.CloseIdleConnections()
}

func certificateGateHeld(ctx context.Context) bool {
	return ctx != nil && ctx.Value(certificateGateKey{}) != nil
}

func mutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
		return true
	default:
		return false
	}
}

// guardMutation takes the read side for a write. Reads do not take it.
// The caller that holds the certificate write side does not take it again.
func (c *Client) guardMutation(ctx context.Context, method string) func() {
	if c == nil || !mutatingMethod(method) || certificateGateHeld(ctx) {
		return func() {}
	}
	c.mutationGate.RLock()
	return c.mutationGate.RUnlock
}

// mutationRoundTripper applies the certificate gate to govergeos requests.
// The provider's own Client.Do applies the same gate.
type mutationRoundTripper struct {
	base   http.RoundTripper
	client *Client
}

func (t *mutationRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if t == nil || t.base == nil {
		return nil, errors.New("HTTP transport is not initialized")
	}
	if req == nil {
		return nil, errors.New("missing request")
	}
	release := t.client.guardMutation(req.Context(), req.Method)
	defer release()
	return t.base.RoundTrip(req)
}

func (t *mutationRoundTripper) CloseIdleConnections() {
	if t == nil || t.base == nil {
		return
	}
	type idleCloser interface {
		CloseIdleConnections()
	}
	closer, ok := t.base.(idleCloser)
	if !ok {
		return
	}
	closer.CloseIdleConnections()
}
