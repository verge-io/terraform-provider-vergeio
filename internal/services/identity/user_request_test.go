package identity

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestUserCreateBodyKeepsFalseAndEmpty(t *testing.T) {
	data := &UserResourceModel{
		Name:           types.StringValue("ada"),
		Enabled:        types.BoolValue(false),
		DisplayName:    types.StringValue(""),
		Email:          types.StringValue(""),
		ChangePassword: types.BoolValue(false),
		AuthSource:     types.Int32Value(0),
		RemoteName:     types.StringNull(),
	}

	model := jsonObject(t, userCreateModel(data))
	requireBool(t, model, "enabled", false)
	requireBool(t, model, "change_password", false)
	requireNumber(t, model, "auth_source", 0)
	requireString(t, model, "displayname", "")
	requireString(t, model, "email", "")
	requireAbsent(t, model, "remote_name")

	req, err := userCreateRequest(data)
	if err != nil {
		t.Fatal(err)
	}
	body := jsonObject(t, req)
	requireBool(t, body, "enabled", false)
	requireBool(t, body, "change_password", false)
	requireNumber(t, body, "auth_source", 0)
	requireAbsent(t, body, "remote_name")
}

func TestUserUpdateBodyKeepsFalseAndEmpty(t *testing.T) {
	plan := &UserResourceModel{
		Name:        types.StringValue("ada"),
		Enabled:     types.BoolValue(false),
		DisplayName: types.StringValue(""),
		Email:       types.StringValue(""),
		RemoteName:  types.StringValue("ada"),
	}
	state := &UserResourceModel{
		Name:        types.StringValue("ada"),
		Enabled:     types.BoolValue(true),
		DisplayName: types.StringValue("Ada"),
		Email:       types.StringValue("ada@example.com"),
		RemoteName:  types.StringValue("ada"),
	}

	req, err := userUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	body := jsonObject(t, req)
	requireBool(t, body, "enabled", false)
	requireString(t, body, "displayname", "")
	requireString(t, body, "email", "")
	requireAbsent(t, body, "name")
	requireAbsent(t, body, "remote_name")
	requireAbsent(t, body, "password")
	requireAbsent(t, body, "change_password")
}

func TestUserCreateSendsWriteOnlyPassword(t *testing.T) {
	data := &UserResourceModel{
		Name:              types.StringValue("ada"),
		PasswordWO:        types.StringValue("wo-secret"),
		PasswordWOVersion: types.Int64Value(1),
	}
	requireString(t, jsonObject(t, userCreateModel(data)), "password", "wo-secret")
}

func TestUserUpdateSendsChangedPassword(t *testing.T) {
	plan := &UserResourceModel{
		Name:           types.StringValue("ada"),
		Password:       types.StringValue("new-secret"),
		ChangePassword: types.BoolValue(false),
	}
	state := &UserResourceModel{
		Name:           types.StringValue("ada"),
		Password:       types.StringValue("old-secret"),
		ChangePassword: types.BoolValue(false),
	}

	req, err := userUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	// Users.Update json.Marshals this struct onto PUT /api/v4/users/{id}.
	// pyVergeOS users.update(..., password=...) sends the same object.
	// change_password stays off this body: it means the user must change
	// their password at next login, and read stores the API value.
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"password":"new-secret"}`
	if string(raw) != want {
		t.Fatalf("Users.Update body = %s, want %s", raw, want)
	}
}

func TestUserUpdateOmitsUnchangedPassword(t *testing.T) {
	plan := &UserResourceModel{
		Name:     types.StringValue("ada"),
		Password: types.StringValue("same-secret"),
		Enabled:  types.BoolValue(false),
	}
	state := &UserResourceModel{
		Name:     types.StringValue("ada"),
		Password: types.StringValue("same-secret"),
		Enabled:  types.BoolValue(true),
	}

	req, err := userUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	body := jsonObject(t, req)
	requireBool(t, body, "enabled", false)
	requireAbsent(t, body, "password")
}

func TestUserUpdateSendsPasswordWhenStateHasNone(t *testing.T) {
	plan := &UserResourceModel{
		Name:     types.StringValue("ada"),
		Password: types.StringValue("first-secret"),
	}
	state := &UserResourceModel{
		Name:     types.StringValue("ada"),
		Password: types.StringNull(),
	}

	req, err := userUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"password":"first-secret"}`
	if string(raw) != want {
		t.Fatalf("Users.Update body = %s, want %s", raw, want)
	}
}

func TestUserUpdateOmitsUnsetEnabled(t *testing.T) {
	plan := &UserResourceModel{
		Name:    types.StringValue("ada"),
		Enabled: types.BoolNull(),
	}
	state := &UserResourceModel{
		Name:    types.StringValue("ada"),
		Enabled: types.BoolValue(true),
	}
	req, err := userUpdateRequest(plan, state)
	if err != nil {
		t.Fatal(err)
	}
	body := jsonObject(t, req)
	requireAbsent(t, body, "enabled")
	requireAbsent(t, body, "name")
}

func jsonObject(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	obj := map[string]any{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	return obj
}

func requireBool(t *testing.T, obj map[string]any, key string, want bool) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %v", key, got, want)
	}
}

func requireString(t *testing.T, obj map[string]any, key, want string) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %q", key, got, want)
	}
}

func requireNumber(t *testing.T, obj map[string]any, key string, want float64) {
	t.Helper()
	got, ok := obj[key]
	if !ok {
		t.Fatalf("missing %q in %#v", key, obj)
	}
	if got != want {
		t.Fatalf("%s = %#v, want %v", key, got, want)
	}
}

func requireAbsent(t *testing.T, obj map[string]any, key string) {
	t.Helper()
	if _, ok := obj[key]; ok {
		t.Fatalf("field %q was sent: %#v", key, obj[key])
	}
}
