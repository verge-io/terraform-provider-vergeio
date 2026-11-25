// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"context"
	"errors"
	"strings"

	"github.com/verge-io/govergeos"
)

// NameConflict reports a create VergeOS rejected because the name is taken.
// Current releases answer HTTP 409. Older releases used 422. The message is
// "already in use" or "unique constraint". Other 409 responses are not a
// name collision.
func NameConflict(err error) bool {
	if err == nil {
		return false
	}
	var conflict *vergeos.ConflictError
	if errors.As(err, &conflict) {
		return nameTakenText(conflict.Message)
	}
	var apiErr *vergeos.APIError
	if errors.As(err, &apiErr) && (apiErr.StatusCode == 409 || apiErr.StatusCode == 422) {
		return nameTakenText(apiErr.Message)
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "unique constraint") {
		return true
	}
	if !strings.Contains(text, "already in use") {
		return false
	}
	return strings.Contains(text, "409") || strings.Contains(text, "422") || strings.Contains(text, "conflict")
}

func nameTakenText(msg string) bool {
	msg = strings.ToLower(msg)
	return strings.Contains(msg, "already in use") || strings.Contains(msg, "unique constraint")
}

// CommitContext is the context for an API call that inserts a row and
// then reads it back. Terraform cancels ctx on SIGINT. The govergeos
// client returns that error before the follow-up read is sent, which
// discards the key from the create response. CommitContext keeps the
// caller's values and is not canceled when ctx is. The HTTP client
// timeout still bounds the call. A context that is already canceled is
// returned unchanged so a create that has not started is not sent.
func CommitContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if ctx.Err() != nil {
		return ctx
	}
	return context.WithoutCancel(ctx)
}

// InterruptAfterCreate reports a cancellation after the remote row exists.
// The id is already in state. Returning the cancellation as an error would
// mark the resource tainted, and the next apply would delete it.
func InterruptAfterCreate(err error, idKnown bool) bool {
	return idKnown && err != nil && errors.Is(err, context.Canceled)
}

// WritableContext is a context State.Set can use after Terraform has
// canceled the request. Values from ctx are kept. Cancellation is not.
func WritableContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if ctx.Err() != nil {
		return context.WithoutCancel(ctx)
	}
	return ctx
}
