// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// continueRequest keeps a create that has already started from being
// dropped when Terraform cancels the caller. VergeOS stores the row
// before it sends the response. Aborting the POST then returns
// context.Canceled with no key, and the row is not written to state.
//
// A POST whose context is already canceled is left alone, so a create
// that has not started is not sent. A GET whose context is already
// canceled is continued: govergeos Create reads the new row with the
// same context, and dropping that read throws away the key the POST
// just returned. An in-flight GET or DELETE still stops.
//
// The caller's deadline is kept. http.Client.Timeout is applied before
// RoundTrip, and dropping it here would let a stuck POST run forever.
func continueRequest(req *http.Request) (*http.Request, func()) {
	stop := func() {}
	if req == nil {
		return req, stop
	}
	ctx := req.Context()
	if ctx == nil {
		return req, stop
	}
	switch req.Method {
	case http.MethodPost:
		if ctx.Err() != nil {
			return req, stop
		}
	case http.MethodGet:
		if ctx.Err() == nil || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return req, stop
		}
	default:
		return req, stop
	}

	detached := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		if !time.Now().Before(deadline) {
			return req, stop
		}
		var cancel context.CancelFunc
		detached, cancel = context.WithDeadline(detached, deadline)
		stop = cancel
	}
	return req.Clone(detached), stop
}
