// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package shared

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/verge-io/govergeos"
)

func TestNameConflict(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "webhook unique constraint",
			err:  &vergeos.ConflictError{APIError: vergeos.APIError{StatusCode: 409, Message: "Validation error: unique constraint"}},
			want: true,
		},
		{
			name: "network name in use",
			err:  &vergeos.ConflictError{APIError: vergeos.APIError{StatusCode: 409, Message: "This name is already in use"}},
			want: true,
		},
		{
			name: "wrapped",
			err:  fmt.Errorf("create: %w", &vergeos.APIError{StatusCode: 422, Message: "This name is already in use"}),
			want: true,
		},
		{
			name: "plain conflict text",
			err:  errors.New("vergeos: conflict: Validation error: unique constraint"),
			want: true,
		},
		{
			name: "busy 409",
			err:  &vergeos.ConflictError{APIError: vergeos.APIError{StatusCode: 409, Message: "resource is busy"}},
			want: false,
		},
		{
			name: "not found",
			err:  &vergeos.NotFoundError{Resource: "WebhookURL", ID: 1},
			want: false,
		},
		{name: "nil", err: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NameConflict(tt.err); got != tt.want {
				t.Fatalf("NameConflict() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCommitContextSurvivesParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	committed := CommitContext(parent)
	cancel()
	if committed.Err() != nil {
		t.Fatalf("committed context ended: %v", committed.Err())
	}
	if err := parent.Err(); err == nil {
		t.Fatal("parent was not canceled")
	}
	if got := CommitContext(parent); got.Err() == nil {
		t.Fatal("an already canceled context was detached")
	}
}

func TestInterruptAfterCreate(t *testing.T) {
	if !InterruptAfterCreate(context.Canceled, true) {
		t.Fatal("cancel after the id is known should stay in state")
	}
	if InterruptAfterCreate(context.Canceled, false) {
		t.Fatal("cancel before the id is known should still be an error")
	}
	if InterruptAfterCreate(errors.New("read failed"), true) {
		t.Fatal("an API error should still be returned")
	}
}

func TestWritableContextKeepsValuesAfterCancel(t *testing.T) {
	type key struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "kept"))
	cancel()
	got := WritableContext(parent)
	if got.Err() != nil {
		t.Fatalf("err = %v", got.Err())
	}
	if got.Value(key{}) != "kept" {
		t.Fatalf("value = %#v", got.Value(key{}))
	}
}
