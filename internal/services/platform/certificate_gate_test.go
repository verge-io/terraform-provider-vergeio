// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestCertificateCreateHoldsTheGate(t *testing.T) {
	api, client, fake := gateAPI(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	otherEntered := make(chan struct{})
	fake.pause = func(r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/certificates":
			once.Do(func() { close(entered) })
			<-release
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/webhook_urls":
			close(otherEntered)
		}
	}

	done := make(chan error, 1)
	go func() {
		data := &certificateModel{
			Type:       types.StringValue(vergeos.CertificateTypeSelfSigned),
			DomainName: types.StringValue("tf-acc-gate.example.com"),
			KeyType:    types.StringValue(vergeos.CertificateKeyTypeECDSA),
		}
		done <- api.createCertificate(context.Background(), data, "", "")
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("certificate create did not reach the API")
	}

	writeDone := make(chan error, 1)
	go func() {
		resp, err := client.Post(context.Background(), "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-hook","url":"https://example.com/hook"}`))
		if err != nil {
			writeDone <- err
			return
		}
		writeDone <- resp.Body.Close()
	}()
	select {
	case <-otherEntered:
		t.Fatal("webhook create ran while the certificate create held the gate")
	case err := <-writeDone:
		t.Fatalf("webhook create finished while the certificate create held the gate: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook create did not finish after the certificate gate opened")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("certificate create did not finish")
	}
}

func TestCertificateCreateHoldsTheGateThroughNameRead(t *testing.T) {
	api, client, fake := gateAPI(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	otherEntered := make(chan struct{})
	fake.pause = func(r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/certificates/"):
			once.Do(func() { close(entered) })
			<-release
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/webhook_urls":
			close(otherEntered)
		}
	}

	done := make(chan error, 1)
	go func() {
		data := &certificateModel{
			Type:       types.StringValue(vergeos.CertificateTypeSelfSigned),
			DomainName: types.StringValue("tf-acc-gate.example.com"),
			KeyType:    types.StringValue(vergeos.CertificateKeyTypeECDSA),
		}
		done <- api.createCertificate(context.Background(), data, "", "")
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("certificate read did not reach the API")
	}

	writeDone := make(chan error, 1)
	go func() {
		resp, err := client.Post(context.Background(), "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-hook","url":"https://example.com/hook"}`))
		if err != nil {
			writeDone <- err
			return
		}
		writeDone <- resp.Body.Close()
	}()
	select {
	case <-otherEntered:
		t.Fatal("webhook create ran while the certificate read held the gate")
	case err := <-writeDone:
		t.Fatalf("webhook create finished while the certificate read held the gate: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook create did not finish after the certificate gate opened")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("certificate create did not finish")
	}
}

func TestCertificateDeleteHoldsTheGate(t *testing.T) {
	api, client, fake := gateAPI(t)
	id := fake.seedCertificate(vergeos.Certificate{
		Domain:  "tf-acc-gate.example.com",
		Type:    vergeos.CertificateTypeSelfSigned,
		KeyType: vergeos.CertificateKeyTypeECDSA,
	})
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	otherEntered := make(chan struct{})
	fake.pause = func(r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/certificates/"):
			once.Do(func() { close(entered) })
			<-release
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/webhook_urls":
			close(otherEntered)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- api.deleteCertificate(context.Background(), &certificateModel{ID: types.StringValue(strconv.Itoa(id))})
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("certificate delete did not reach the API")
	}

	writeDone := make(chan error, 1)
	go func() {
		resp, err := client.Post(context.Background(), "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-hook","url":"https://example.com/hook"}`))
		if err != nil {
			writeDone <- err
			return
		}
		writeDone <- resp.Body.Close()
	}()
	select {
	case <-otherEntered:
		t.Fatal("webhook create ran while the certificate delete held the gate")
	case err := <-writeDone:
		t.Fatalf("webhook create finished while the certificate delete held the gate: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook create did not finish after the certificate gate opened")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("certificate delete did not finish")
	}
}

func TestCertificateUpdateDoesNotExcludeOtherWrites(t *testing.T) {
	api, client, fake := gateAPI(t)
	id := fake.seedCertificate(vergeos.Certificate{
		Domain:      "tf-acc-gate.example.com",
		Description: "before",
		Type:        vergeos.CertificateTypeSelfSigned,
		KeyType:     vergeos.CertificateKeyTypeECDSA,
	})
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	otherEntered := make(chan struct{})
	fake.pause = func(r *http.Request) {
		switch {
		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/certificates/"):
			once.Do(func() { close(entered) })
			<-release
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/webhook_urls":
			close(otherEntered)
		}
	}

	done := make(chan error, 1)
	go func() {
		state := &certificateModel{
			ID:          idString(id),
			Type:        types.StringValue(vergeos.CertificateTypeSelfSigned),
			DomainName:  types.StringValue("tf-acc-gate.example.com"),
			Description: types.StringValue("before"),
			KeyType:     types.StringValue(vergeos.CertificateKeyTypeECDSA),
		}
		plan := *state
		plan.Description = types.StringValue("after")
		done <- api.updateCertificate(context.Background(), &plan, state, "", "", "tf-acc-gate.example.com")
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("certificate update did not reach the API")
	}

	writeDone := make(chan error, 1)
	go func() {
		resp, err := client.Post(context.Background(), "api/v4/webhook_urls", bytes.NewBufferString(`{"name":"tf-acc-hook","url":"https://example.com/hook"}`))
		if err != nil {
			writeDone <- err
			return
		}
		writeDone <- resp.Body.Close()
	}()
	select {
	case <-otherEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook create waited for the certificate update")
	}
	close(release)

	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook create did not finish")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("certificate update did not finish")
	}
}

func gateAPI(t *testing.T) (*API, *vergeio.Client, *platformFake) {
	t.Helper()
	fake := newPlatformFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	client := vergeio.NewClient(server.URL, "user", "pass", true)
	client.SetCertificateSettle(time.Millisecond)
	api, err := NewAPI(client)
	if err != nil {
		t.Fatal(err)
	}
	return api, client, fake
}
