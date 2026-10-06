// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeleteVMRecipeInstance(t *testing.T) {
	var method, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		method = r.Method
		path = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "user", "pass", true)
	if err := client.DeleteVMRecipeInstance(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodDelete || path != "/api/v4/vm_recipe_instances/9" {
		t.Fatalf("request = %s %s", method, path)
	}
}

func TestDeleteVMRecipeInstanceNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"err":"not found"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "user", "pass", true)
	if err := client.DeleteVMRecipeInstance(context.Background(), 9); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteVMRecipeInstanceServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if AnswerCredentialCheck(w, r) {
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"err":"unavailable"}`))
	}))
	t.Cleanup(server.Close)

	client := NewClient(server.URL, "user", "pass", true)
	err := client.DeleteVMRecipeInstance(context.Background(), 9)
	if err == nil {
		t.Fatal("expected the server error")
	}
	var apiErr Error
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("error = %v", err)
	}
}
