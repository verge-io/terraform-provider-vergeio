// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostCreateMasksSecretsInDebugLog(t *testing.T) {
	const (
		userPassword   = "user-secret-9f3c1e7a"
		consolePass    = "console-secret-8e2b4d6c"
		authPassword   = "auth-secret-4d5e6f70"
		remotePassword = "remote-secret-1b2c3d4e"
		privateKey     = "private-key-secret-aabbccdd"
		ipsecSecrets   = "ipsec-secret-11223344"
		cifsPassword   = "cifs-secret-55667788"
		token          = "token-secret-5e6f7a8b"
		providerPass   = "provider-login-7a1c5e9d"
	)
	secrets := []string{
		userPassword, consolePass, authPassword, remotePassword,
		privateKey, ipsecSecrets, cifsPassword, token, providerPass,
	}

	payload := `{
		"name":"ada",
		"password":"` + userPassword + `",
		"console_pass":"` + consolePass + `",
		"console_pass_enabled":true,
		"change_password":false,
		"auth_password":"` + authPassword + `",
		"private_key":"` + privateKey + `",
		"ipsec_secrets":"` + ipsecSecrets + `",
		"cifs_password":"` + cifsPassword + `",
		"token":"` + token + `",
		"nested":{"remote_password":"` + remotePassword + `"}
	}`

	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read", http.StatusInternalServerError)
			return
		}
		sent = body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte(`{"$key":"1","response":"ok"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	logs := captureStdLog(t)
	client := NewClient(server.URL, "api-user", providerPass, true)
	resp, err := client.Post("api/v4/users", bytes.NewBufferString(payload))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close response: %v", err)
	}

	got := logs()
	for _, secret := range secrets {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q leaked into debug log:\n%s", secret, got)
		}
	}
	if !strings.Contains(got, `"name":"ada"`) {
		t.Fatalf("debug log dropped the non-secret name field:\n%s", got)
	}
	if !strings.Contains(got, `"console_pass_enabled":true`) {
		t.Fatalf("debug log dropped console_pass_enabled:\n%s", got)
	}
	if !strings.Contains(got, `"change_password":false`) {
		t.Fatalf("debug log dropped change_password:\n%s", got)
	}
	if !strings.Contains(got, `"password":"***"`) || !strings.Contains(got, `"console_pass":"***"`) {
		t.Fatalf("debug log did not mask password fields:\n%s", got)
	}

	sentBody := string(sent)
	for _, secret := range []string{userPassword, consolePass, remotePassword, token} {
		if !strings.Contains(sentBody, secret) {
			t.Fatalf("create request dropped secret %q; masking must not change the body", secret)
		}
	}
}

func TestPostCreateMasksSecretsInErrorBody(t *testing.T) {
	const userPassword = "user-secret-error-9f3c1e7a"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := w.Write([]byte(`{"password":"` + userPassword + `","err":"invalid"}`)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	logs := captureStdLog(t)
	client := NewClient(server.URL, "api-user", "provider-login", true)
	_, err := client.Post("api/v4/users", bytes.NewBufferString(`{"name":"ada","password":"`+userPassword+`"}`))
	if err == nil {
		t.Fatal("expected API error")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("error = %v, want the API err field", err)
	}

	got := logs()
	if strings.Contains(got, userPassword) {
		t.Fatalf("secret leaked into debug log:\n%s", got)
	}
}

func captureStdLog(t *testing.T) func() string {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() {
		log.SetOutput(orig)
	})
	return func() string {
		return buf.String()
	}
}
