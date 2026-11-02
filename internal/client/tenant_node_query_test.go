// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTenantNodeTCPQuery(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		calls = append(calls, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenant_nodes/7":
			if _, err := w.Write([]byte(`{"$key":7,"db_version":"26.2.2.1"}`)); err != nil {
				t.Errorf("write: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/tenant_node_queries":
			if _, err := w.Write([]byte(`{"$key":15}`)); err != nil {
				t.Errorf("write: %v", err)
			}
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/tenant_node_queries/15":
			if _, err := w.Write([]byte(`{"$key":15,"status":"complete","result":"Connection successful"}`)); err != nil {
				t.Errorf("write: %v", err)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	client := NewClient(server.URL, "user", "pass", true)
	ctx := context.Background()

	version, err := client.TenantNodeDBVersion(ctx, 7)
	if err != nil || version != "26.2.2.1" {
		t.Fatalf("db_version = %q %v", version, err)
	}
	id, err := client.CreateTenantNodeTCPQuery(ctx, 7)
	if err != nil || id != "15" {
		t.Fatalf("query id = %q %v", id, err)
	}
	status, result, err := client.GetTenantNodeQuery(ctx, id)
	if err != nil || status != "complete" || !strings.Contains(result, "Connection successful") {
		t.Fatalf("query = %s %s %v", status, result, err)
	}
	joined := strings.Join(calls, "\n")
	for _, part := range []string{
		"fields=%24key%2Cdb_version",
		`"tenant_node":7`,
		`"query":"tcp_connect"`,
		`"host":"127.0.0.1"`,
		`"port":"443"`,
		"fields=%24key%2Cstatus%2Cresult",
	} {
		if !strings.Contains(joined, part) {
			t.Fatalf("calls = %s\nmissing %s", joined, part)
		}
	}
}
