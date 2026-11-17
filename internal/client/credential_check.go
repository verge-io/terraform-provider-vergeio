// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import "net/http"

// AnswerCredentialCheck writes the empty list govergeos reads while creating
// a client. The probe is GET /api/v4/clusters?fields=$key&limit=1. It returns
// true when it handled the request.
func AnswerCredentialCheck(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != "/api/v4/clusters" {
		return false
	}
	q := r.URL.Query()
	if q.Get("limit") != "1" || q.Get("fields") != "$key" || q.Get("filter") != "" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`[]`))
	return true
}
