// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

func shortenNetworkStop(t *testing.T, timeout, interval time.Duration) {
	t.Helper()
	origTimeout, origInterval := networkStopTimeout, networkStopInterval
	t.Cleanup(func() {
		networkStopTimeout = origTimeout
		networkStopInterval = origInterval
	})
	networkStopTimeout = timeout
	networkStopInterval = interval
}

func TestStopNetworkBeforeDeleteKillsOnceThenWaits(t *testing.T) {
	shortenNetworkStop(t, time.Second, 0)

	var killBodies []string
	reads := 0
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
			reads++
			payload := `[{"$key":12,"name":"lan","running":true}]`
			if reads > 2 {
				payload = `[{"$key":12,"name":"lan","running":false}]`
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(payload)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			killBodies = append(killBodies, string(body))
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	data := &NetworkResourceModel{
		Id:   types.StringValue("12"),
		Name: types.StringValue("lan"),
	}
	if err := api.stopNetworkBeforeDelete(t.Context(), data); err != nil {
		t.Fatal(err)
	}
	if data.PowerState.IsNull() || data.PowerState.ValueBool() {
		t.Fatalf("power state = %#v, want false", data.PowerState)
	}
	if reads < 3 {
		t.Fatalf("status reads = %d, want polls while the network is still running", reads)
	}
	if len(killBodies) != 1 {
		t.Fatalf("kill calls = %d, want 1: %#v", len(killBodies), killBodies)
	}
	body := killBodies[0]
	if !strings.Contains(body, `"action":"kill"`) || !strings.Contains(body, `"vnet":12`) {
		t.Fatalf("kill body = %s, want one kill for vnet 12", body)
	}
}

func TestStopNetworkBeforeDeleteTimeoutNamesNetworkAndStatus(t *testing.T) {
	shortenNetworkStop(t, 0, time.Hour)

	kills := 0
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`[{"$key":12,"name":"lan","running":true}]`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/vnet_actions":
			kills++
			w.WriteHeader(http.StatusCreated)
			if _, err := w.Write([]byte(`{}`)); err != nil {
				t.Errorf("write response: %v", err)
			}
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusInternalServerError)
		}
	})

	err := api.stopNetworkBeforeDelete(t.Context(), &NetworkResourceModel{
		Id:   types.StringValue("12"),
		Name: types.StringValue("lan"),
	})
	if err == nil {
		t.Fatal("expected a timeout")
	}
	msg := err.Error()
	for _, want := range []string{"lan", "12", "running", "stopped"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q missing %q", msg, want)
		}
	}
	if kills != 1 {
		t.Fatalf("kill calls = %d, want 1", kills)
	}
}

func TestStopNetworkBeforeDeleteSkipsKillWhenStopped(t *testing.T) {
	api := newRestartTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		if r.Method == http.MethodPost {
			t.Errorf("stopped network was killed")
			http.Error(w, "unexpected kill", http.StatusInternalServerError)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v4/vnets" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if _, err := w.Write([]byte(`[{"$key":12,"name":"lan","running":false}]`)); err != nil {
				t.Errorf("write response: %v", err)
			}
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
		http.Error(w, "unexpected", http.StatusInternalServerError)
	})

	if err := api.stopNetworkBeforeDelete(t.Context(), &NetworkResourceModel{
		Id:   types.StringValue("12"),
		Name: types.StringValue("lan"),
	}); err != nil {
		t.Fatal(err)
	}
}
