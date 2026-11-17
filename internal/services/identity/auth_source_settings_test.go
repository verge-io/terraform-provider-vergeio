// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestCanonicalSettingsSortsKeysAndRejectsSecret(t *testing.T) {
	got, err := canonicalSettings(`{"scope":"openid","client_id":"app","update_user_email":true}`)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"client_id":"app","scope":"openid","update_user_email":true}`
	if got != want {
		t.Fatalf("canonical = %s, want %s", got, want)
	}
	if _, err := canonicalSettings(`{"client_id":"app","client_secret":"super-secret"}`); err == nil {
		t.Fatal("client_secret was accepted")
	} else if strings.Contains(err.Error(), "super-secret") {
		t.Fatal("settings error included the client secret")
	}
	if _, err := canonicalSettings(`["nope"]`); err == nil {
		t.Fatal("array settings were accepted")
	}
}

func TestSettingsPlanModifierCanonicalizes(t *testing.T) {
	modifier := settingsCanonicalModifier{}
	req := planmodifier.StringRequest{
		Path:      path.Root("settings"),
		PlanValue: types.StringValue(`{"b":"2","a":"1"}`),
	}
	resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
	modifier.PlanModifyString(context.Background(), req, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if resp.PlanValue.ValueString() != `{"a":"1","b":"2"}` {
		t.Fatalf("plan = %s", resp.PlanValue.ValueString())
	}

	req.PlanValue = types.StringNull()
	resp = &planmodifier.StringResponse{PlanValue: req.PlanValue}
	modifier.PlanModifyString(context.Background(), req, resp)
	if resp.Diagnostics.HasError() || !resp.PlanValue.IsNull() {
		t.Fatalf("null plan = %#v diags=%v", resp.PlanValue, resp.Diagnostics)
	}
}

func TestReconcileSettingsDropsSecretAndDebug(t *testing.T) {
	tracked := types.StringValue(`{"client_id":"app","scope":"openid"}`)
	got, err := reconcileSettings(tracked, vergeos.AuthSourceSettings{
		"client_id":     "app",
		"client_secret": "super-secret",
		"scope":         "openid profile",
		"debug":         false,
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"client_id":"app","scope":"openid profile"}`
	if got.ValueString() != want {
		t.Fatalf("settings = %s, want %s", got.ValueString(), want)
	}
	if strings.Contains(got.ValueString(), "super-secret") || strings.Contains(got.ValueString(), "debug") {
		t.Fatalf("tracked settings kept a secret or debug: %s", got.ValueString())
	}
	untouched, err := reconcileSettings(types.StringNull(), vergeos.AuthSourceSettings{"client_id": "app"})
	if err != nil || !untouched.IsNull() {
		t.Fatalf("null settings = %#v err=%v", untouched, err)
	}
}

func TestSettingsDocumentAddsSecretOnlyWhenAsked(t *testing.T) {
	doc, err := settingsDocument(types.StringValue(`{"client_id":"app"}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc[clientSecretSettingsKey]; ok {
		t.Fatal("secret was added without a write-only value")
	}
	if doc["client_id"] != "app" {
		t.Fatalf("client_id = %#v", doc["client_id"])
	}

	doc, err = settingsDocument(types.StringValue(`{"client_id":"app","debug":false}`), "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	secret, ok := doc[clientSecretSettingsKey].(vergeos.WriteOnlySecret)
	if !ok || secret.Value() != "super-secret" {
		t.Fatal("write-only secret was not attached")
	}
	if _, ok := doc[settingsDebugKey]; ok {
		t.Fatal("server debug key was sent")
	}
	printed := doc.String()
	if strings.Contains(printed, "super-secret") {
		t.Fatal("settings document printed the client secret")
	}
}

func TestWriteOnlySecretFollowsVersion(t *testing.T) {
	secret := types.StringValue("super-secret")
	version := types.Int64Value(1)
	if got := writeOnlySecret(version, types.Int64Null(), secret, false); got != "super-secret" {
		t.Fatalf("create secret = %q", got)
	}
	if got := writeOnlySecret(version, version, secret, true); got != "" {
		t.Fatalf("same version sent %q", got)
	}
	if got := writeOnlySecret(types.Int64Value(2), version, secret, true); got != "super-secret" {
		t.Fatalf("rotated secret = %q", got)
	}
	if got := writeOnlySecret(version, types.Int64Null(), types.StringNull(), false); got != "" {
		t.Fatalf("missing secret = %q", got)
	}
}
