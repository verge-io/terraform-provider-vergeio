// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestAuthSourceCreateRequestSendsSecret(t *testing.T) {
	req, err := authSourceCreateRequest(&AuthSourceResourceModel{
		Name:         types.StringValue("Corporate Azure"),
		Driver:       types.StringValue(vergeos.AuthSourceDriverAzure),
		Settings:     types.StringValue(`{"client_id":"app","tenant_id":"tenant"}`),
		Menu:         types.BoolValue(true),
		ButtonFAIcon: types.StringValue("bi-microsoft"),
	}, "super-secret")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{`"name":"Corporate Azure"`, `"driver":"azure"`, `"client_id":"app"`, `"client_secret":"super-secret"`, `"menu":true`, `"button_fa_icon":"bi-microsoft"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("create body missing %s: %s", want, body)
		}
	}
	if strings.Contains(req.String(), "super-secret") {
		t.Fatal("create request printed the client secret")
	}
}

func TestAuthSourceUpdateRequestOmitsUnchangedSecret(t *testing.T) {
	state := &AuthSourceResourceModel{
		Name:     types.StringValue("Corporate Azure"),
		Driver:   types.StringValue(vergeos.AuthSourceDriverAzure),
		Settings: types.StringValue(`{"client_id":"app","scope":"openid"}`),
		Menu:     types.BoolValue(false),
	}
	plan := &AuthSourceResourceModel{
		Name:     types.StringValue("Corporate Azure"),
		Driver:   types.StringValue(vergeos.AuthSourceDriverAzure),
		Settings: types.StringValue(`{"client_id":"app"}`),
		Menu:     types.BoolValue(true),
	}
	req, err := authSourceUpdateRequest(plan, state, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "client_secret") || strings.Contains(body, "scope") {
		t.Fatalf("partial update included a removed key or the secret: %s", body)
	}
	if !strings.Contains(body, `"client_id":"app"`) || !strings.Contains(body, `"menu":true`) {
		t.Fatalf("update body = %s", body)
	}
}

func TestAuthSourceUpdateRequestRotatesSecret(t *testing.T) {
	state := &AuthSourceResourceModel{
		Name:     types.StringValue("Corporate Azure"),
		Settings: types.StringValue(`{"client_id":"app"}`),
	}
	plan := &AuthSourceResourceModel{
		Name:     types.StringValue("Renamed"),
		Settings: types.StringValue(`{"client_id":"app"}`),
	}
	req, err := authSourceUpdateRequest(plan, state, "new-secret")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if !strings.Contains(body, `"client_secret":"new-secret"`) || !strings.Contains(body, `"name":"Renamed"`) {
		t.Fatalf("update body = %s", body)
	}
	if strings.Contains(req.String(), "new-secret") {
		t.Fatal("update request printed the client secret")
	}
}

func TestAuthSourceUpdateRequestSkipsUnchanged(t *testing.T) {
	model := &AuthSourceResourceModel{
		Name:     types.StringValue("Corporate Azure"),
		Settings: types.StringValue(`{"client_id":"app"}`),
		Menu:     types.BoolValue(false),
	}
	req, err := authSourceUpdateRequest(model, model, "")
	if err != nil {
		t.Fatal(err)
	}
	if req != nil {
		t.Fatalf("unchanged auth source produced %#v", req)
	}
}

func TestApplyAuthSourceKeepsTrackedSettings(t *testing.T) {
	data := &AuthSourceResourceModel{Settings: types.StringValue(`{"client_id":"app"}`)}
	err := applyAuthSource(data, &vergeos.AuthSource{
		Key:                   7,
		Name:                  "Corporate Azure",
		Driver:                vergeos.AuthSourceDriverAzure,
		Menu:                  true,
		ButtonFAIcon:          "bi-microsoft",
		ButtonBackgroundColor: "#4285F4",
		Settings: vergeos.AuthSourceSettings{
			"client_id":     "app",
			"client_secret": "super-secret",
			"debug":         false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if data.Id.ValueString() != "7" || data.Driver.ValueString() != "azure" || !data.Menu.ValueBool() {
		t.Fatalf("source = id %s driver %s menu %s", data.Id, data.Driver, data.Menu)
	}
	if data.Settings.ValueString() != `{"client_id":"app"}` {
		t.Fatalf("settings = %s", data.Settings.ValueString())
	}
}
