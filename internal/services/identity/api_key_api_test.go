// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package identity

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestAPIKeyCreateRequestNeverExpires(t *testing.T) {
	req := apiKeyCreateRequest(&APIKeyResourceModel{
		UserID:      types.Int32Value(10),
		Name:        types.StringValue("ci"),
		Description: types.StringValue("runner"),
		IPAllowList: types.StringValue("192.0.2.0/24"),
		IPDenyList:  types.StringNull(),
		Expires:     types.Int64Null(),
	})
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"user":10,"name":"ci","description":"runner","ip_allow_list":"192.0.2.0/24","expires_type":"never"}`
	if string(raw) != want {
		t.Fatalf("create body = %s, want %s", raw, want)
	}
}

func TestAPIKeyCreateRequestExpiresOnDate(t *testing.T) {
	req := apiKeyCreateRequest(&APIKeyResourceModel{
		UserID:     types.Int32Value(10),
		Name:       types.StringValue("ci"),
		IPDenyList: types.StringValue("198.51.100.10"),
		Expires:    types.Int64Value(1893456000),
	})
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"user":10,"name":"ci","ip_deny_list":"198.51.100.10","expires_type":"date","expires":1893456000}`
	if string(raw) != want {
		t.Fatalf("create body = %s, want %s", raw, want)
	}
}

func TestAPIKeyUpdateRequestSendsOnlyChanges(t *testing.T) {
	state := &APIKeyResourceModel{
		Name:        types.StringValue("ci"),
		Description: types.StringValue("runner"),
		IPAllowList: types.StringValue("192.0.2.0/24"),
		IPDenyList:  types.StringValue(""),
		Expires:     types.Int64Value(1893456000),
	}
	plan := &APIKeyResourceModel{
		Name:        types.StringValue("ci"),
		Description: types.StringValue("updated"),
		IPAllowList: types.StringValue("192.0.2.10/32"),
		IPDenyList:  types.StringValue(""),
		Expires:     types.Int64Value(1893456000),
	}
	req := apiKeyUpdateRequest(plan, state)
	if req == nil {
		t.Fatal("expected an update body")
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"description":"updated","ip_allow_list":"192.0.2.10/32"}`
	if string(raw) != want {
		t.Fatalf("update body = %s, want %s", raw, want)
	}
	if apiKeyUpdateRequest(plan, plan) != nil {
		t.Fatal("unchanged API key should not produce an update body")
	}
}

func TestAPIKeyUpdateRequestClearsExpiry(t *testing.T) {
	state := &APIKeyResourceModel{
		Name:    types.StringValue("ci"),
		Expires: types.Int64Value(1893456000),
	}
	plan := &APIKeyResourceModel{
		Name:    types.StringValue("ci"),
		Expires: types.Int64Null(),
	}
	req := apiKeyUpdateRequest(plan, state)
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"expires_type":"never","expires":0}`
	if string(raw) != want {
		t.Fatalf("update body = %s, want %s", raw, want)
	}
}

func TestApplyAPIKeyDropsZeroExpiry(t *testing.T) {
	data := &APIKeyResourceModel{}
	applyAPIKey(data, &vergeos.UserAPIKey{
		Key:            5,
		User:           10,
		Name:           "ci",
		Description:    "runner",
		IPAllowList:    "192.0.2.0/24",
		Created:        1700000000,
		LastLoginStamp: 0,
		Expires:        0,
	})
	if data.Id.ValueString() != "5" || data.UserID.ValueInt32() != 10 {
		t.Fatalf("id=%s user=%s", data.Id, data.UserID)
	}
	if !data.Expires.IsNull() {
		t.Fatalf("expires = %#v, want null", data.Expires)
	}
	if data.Created.ValueInt64() != 1700000000 || data.LastLoginStamp.ValueInt64() != 0 {
		t.Fatalf("created=%s lastlogin=%s", data.Created, data.LastLoginStamp)
	}
}

func TestAPIKeyReusedUser(t *testing.T) {
	data := &APIKeyResourceModel{UserID: types.Int32Value(10)}
	if apiKeyReused(data, &vergeos.UserAPIKey{User: 10}) {
		t.Fatal("same user was treated as reused")
	}
	if !apiKeyReused(data, &vergeos.UserAPIKey{User: 11}) {
		t.Fatal("different user was kept")
	}
	if apiKeyReused(data, &vergeos.UserAPIKey{}) {
		t.Fatal("missing user was treated as reused")
	}
}
