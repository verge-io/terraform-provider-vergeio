// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

const (
	// certificateReloadGap is the time from one web server reload to the
	// next when another certificate change lands in between. On 26.2.2.1
	// the second reload started about three seconds after the first, not
	// when the second change arrived. One measured pair was 3.7s apart.
	certificateReloadGap = 4 * time.Second

	// certificateReloadDuration is how long the restart keeps connections
	// closed after a reload starts. A quiet system finished in about 0.6s.
	certificateReloadDuration = 1 * time.Second

	// defaultCertificateSettle keeps other writes out until a deferred
	// reload can no longer be in progress. 1.5s covered a single change
	// on a quiet system and released writers before the later restart.
	defaultCertificateSettle = certificateReloadGap + certificateReloadDuration

	// defaultCertificateReadyTimeout is how long to keep waiting after the
	// settle when version probes still see a closed connection.
	defaultCertificateReadyTimeout  = 10 * time.Second
	defaultCertificateReadyInterval = 200 * time.Millisecond
	defaultCertificateProbeTimeout  = 2 * time.Second
)

type certificateGateKey struct{}

// SetCertificateSettle overrides how long certificate create and delete
// keep other writes waiting. A non-positive duration is ignored. A
// positive duration skips the version probe so a test can shorten the hold.
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

func (c *Client) probeAfterCertificateChange() bool {
	if c == nil {
		return false
	}
	if c.certificateReadyTimeout > 0 {
		return true
	}
	return c.certificateSettle <= 0
}

func (c *Client) certificateReadyBudget() time.Duration {
	if c != nil && c.certificateReadyTimeout > 0 {
		return c.certificateReadyTimeout
	}
	return defaultCertificateReadyTimeout
}

func (c *Client) probeTimeout() time.Duration {
	if c != nil && c.certificateProbeTimeout > 0 {
		return c.certificateProbeTimeout
	}
	return defaultCertificateProbeTimeout
}

func (c *Client) readyInterval() time.Duration {
	if c != nil && c.certificateReadyInterval > 0 {
		return c.certificateReadyInterval
	}
	return defaultCertificateReadyInterval
}

// WithCertificateMutation runs fn while every other write waits, then keeps
// waiting until the certificate reload can no longer reset those writes.
func (c *Client) WithCertificateMutation(ctx context.Context, fn func(context.Context) error) error {
	if fn == nil {
		return errors.New("missing certificate operation")
	}
	return c.WithCertificateMutationAfter(ctx, fn, nil)
}

// WithOptionalCertificateMutation runs fn while other writes wait. The reload
// wait runs only when fn reports that it changed a certificate. A read that
// finds a foreign row, or a read that fails, does not wait.
func (c *Client) WithOptionalCertificateMutation(ctx context.Context, fn func(context.Context) (bool, error)) error {
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

	gated := context.WithValue(ctx, certificateGateKey{}, struct{}{})
	changed, err := fn(gated)
	if ctx.Err() != nil {
		if err != nil {
			return err
		}
		return ctx.Err()
	}
	if !changed {
		return err
	}
	quietErr := c.waitCertificateQuiet(ctx)
	if err != nil {
		if quietErr != nil {
			return errors.Join(err, quietErr)
		}
		return err
	}
	return quietErr
}

// WithCertificateMutationAfter runs change, waits out the web server reload,
// then runs after while other writes are still held. after is the
// certificate read that follows create. The next certificate change has to
// stay behind that read, or its reload resets it.
func (c *Client) WithCertificateMutationAfter(ctx context.Context, change, after func(context.Context) error) error {
	if c == nil {
		return errors.New("vergeio client is nil")
	}
	if ctx == nil {
		return errors.New("missing request context")
	}
	if change == nil {
		return errors.New("missing certificate operation")
	}
	c.mutationGate.Lock()
	defer c.mutationGate.Unlock()
	defer c.closeIdleConnections()

	gated := context.WithValue(ctx, certificateGateKey{}, struct{}{})
	err := change(gated)
	if ctx.Err() != nil {
		if err != nil {
			return err
		}
		return ctx.Err()
	}
	quietErr := c.waitCertificateQuiet(ctx)
	if err != nil {
		if quietErr != nil {
			return errors.Join(err, quietErr)
		}
		return err
	}
	if quietErr != nil {
		return quietErr
	}
	if after == nil {
		return nil
	}
	return after(gated)
}

func (c *Client) waitCertificateQuiet(ctx context.Context) error {
	hold := c.certificateSettleDuration()
	log.Printf("[DEBUG] waiting %s after certificate create or delete before other writes", hold)
	if err := sleepCertificate(ctx, hold); err != nil {
		return err
	}
	if !c.probeAfterCertificateChange() {
		return nil
	}
	return c.waitCertificateReady(ctx)
}

func (c *Client) waitCertificateReady(ctx context.Context) error {
	deadline := time.Now().Add(c.certificateReadyBudget())
	interval := c.readyInterval()
	var last error
	logged := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := c.probeWebServer(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err == nil {
			return nil
		}
		if !certificateReloadError(err) {
			return err
		}
		last = err
		if !logged {
			log.Printf("[DEBUG] API connection closed after certificate create or delete: %s", err.Error())
			logged = true
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("API did not accept connections after the certificate change: %w", last)
		}
		if err := sleepCertificate(ctx, interval); err != nil {
			return err
		}
	}
}

// probeWebServer asks for /version.json. A response means the web server
// is accepting connections again. A reset or a refused connection does not.
func (c *Client) probeWebServer(ctx context.Context) error {
	if c == nil || c.httpClient == nil {
		return errors.New("HTTP client is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.closeIdleConnections()
	probeCtx, cancel := context.WithTimeout(ctx, c.probeTimeout())
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, c.serverURL("version.json"), nil)
	if err != nil {
		return err
	}
	req.Close = true
	resp, err := c.httpClient.Do(req)
	if ctx.Err() != nil {
		closeErr := closeBody(resp)
		if closeErr != nil {
			return errors.Join(ctx.Err(), closeErr)
		}
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	if resp.Body == nil {
		return nil
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	closeErr := resp.Body.Close()
	if readErr != nil {
		if closeErr != nil {
			return errors.Join(readErr, closeErr)
		}
		return readErr
	}
	return closeErr
}

func closeBody(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	return resp.Body.Close()
}

func certificateReloadError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	if retryableDirectError(err) || errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "connection refused")
}

func sleepCertificate(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
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
	req, stop := continueRequest(req)
	defer func() {
		if stop != nil {
			stop()
		}
	}()
	resp, err := t.base.RoundTrip(req)
	resp, err = releaseContinued(resp, err, stop)
	stop = nil
	return resp, err
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
