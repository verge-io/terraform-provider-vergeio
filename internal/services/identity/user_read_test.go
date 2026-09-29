package identity

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

// Create calls readUser, which stores change_password from the API user.
// An unset Optional+Computed attribute is unknown in the plan. It must be
// known after that read or Terraform rejects the apply and taints the user.
func TestApplyUserLeavesChangePasswordKnown(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		{
			name: "false",
			raw:  `{"$key":7,"name":"ada","enabled":true,"displayname":"","email":"","type":"normal","change_password":false}`,
			want: false,
		},
		{
			name: "true",
			raw:  `{"$key":8,"name":"ada","enabled":true,"change_password":true}`,
			want: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var apiUser vergeos.User
			if err := json.Unmarshal([]byte(tc.raw), &apiUser); err != nil {
				t.Fatal(err)
			}

			data := &UserResourceModel{
				Password:       types.StringValue("secret"),
				ChangePassword: types.BoolUnknown(),
			}
			applyUser(data, &apiUser)

			if data.ChangePassword.IsNull() || data.ChangePassword.IsUnknown() {
				t.Fatalf("change_password still unknown: %#v", data.ChangePassword)
			}
			if data.ChangePassword.ValueBool() != tc.want {
				t.Fatalf("change_password = %v, want %v", data.ChangePassword.ValueBool(), tc.want)
			}
			if data.Password.ValueString() != "secret" {
				t.Fatalf("password = %q, want the value last applied", data.Password.ValueString())
			}
			if data.Id.IsUnknown() || data.Id.ValueString() == "" {
				t.Fatalf("id = %#v, want the API key", data.Id)
			}
		})
	}
}

func TestApplyUserDropsUnreadPassword(t *testing.T) {
	data := &UserResourceModel{
		Password:       types.StringUnknown(),
		ChangePassword: types.BoolUnknown(),
	}
	applyUser(data, &vergeos.User{
		Key:            9,
		Name:           "ada",
		ChangePassword: false,
	})

	if !data.Password.IsNull() {
		t.Fatalf("password = %#v, want null when the API and state have none", data.Password)
	}
	if data.ChangePassword.IsNull() || data.ChangePassword.IsUnknown() || data.ChangePassword.ValueBool() {
		t.Fatalf("change_password = %#v, want known false", data.ChangePassword)
	}
	if data.Id.ValueString() != "9" {
		t.Fatalf("id = %q, want 9", data.Id.ValueString())
	}
}
