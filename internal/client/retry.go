// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

const (
	defaultDirectRetryAttempts   = 3
	defaultDirectRetryBackoff    = 100 * time.Millisecond
	defaultDirectRetryMaxBackoff = 2 * time.Second
)

func (c *Client) directRetryCount() int {
	if c != nil && c.directRetryAttempts > 0 {
		return c.directRetryAttempts
	}
	return defaultDirectRetryAttempts
}

func (c *Client) retryDelay(failedAttempt int) time.Duration {
	if failedAttempt < 1 {
		failedAttempt = 1
	}
	backoff := defaultDirectRetryBackoff
	if c != nil && c.directRetryBackoff > 0 {
		backoff = c.directRetryBackoff
	}
	delay := backoff
	for i := 1; i < failedAttempt; i++ {
		if delay >= defaultDirectRetryMaxBackoff/2 {
			return defaultDirectRetryMaxBackoff
		}
		delay *= 2
	}
	if delay > defaultDirectRetryMaxBackoff {
		return defaultDirectRetryMaxBackoff
	}
	return delay
}

func (c *Client) sleepRetry(ctx context.Context, failedAttempt int) error {
	delay := c.retryDelay(failedAttempt)
	if delay <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryableDirectMethod is GET, PUT, and DELETE. POST is not retried:
// a create may already have been stored.
func retryableDirectMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func retryableDirectError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr Error
	if errors.As(err, &apiErr) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "server closed idle connection")
}

func drainResponse(resp *http.Response) error {
	if resp == nil || resp.Body == nil {
		return nil
	}
	_, readErr := io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	closeErr := resp.Body.Close()
	if readErr != nil {
		return readErr
	}
	return closeErr
}
