package compute

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestConsolePassSchemaIsSensitive(t *testing.T) {
	var resp resource.SchemaResponse
	NewVMResource().Schema(context.Background(), resource.SchemaRequest{}, &resp)

	attr, ok := resp.Schema.Attributes["console_pass"].(schema.StringAttribute)
	if !ok {
		t.Fatalf("console_pass type = %T", resp.Schema.Attributes["console_pass"])
	}
	if !attr.Sensitive {
		t.Fatal("console_pass is not Sensitive")
	}
	if !attr.Optional || !attr.Computed {
		t.Fatalf("console_pass optional=%v computed=%v", attr.Optional, attr.Computed)
	}
	if attr.GetDeprecationMessage() == "" {
		t.Fatal("console_pass should be deprecated in favor of console_pass_wo")
	}
	writeOnly, ok := resp.Schema.Attributes["console_pass_wo"].(schema.StringAttribute)
	if !ok || !writeOnly.WriteOnly || !writeOnly.Sensitive || !writeOnly.Optional || writeOnly.Computed {
		t.Fatalf("console_pass_wo = %#v", resp.Schema.Attributes["console_pass_wo"])
	}
	version := resp.Schema.Attributes["console_pass_wo_version"]
	if version == nil || !version.IsOptional() || version.IsWriteOnly() {
		t.Fatal("console_pass_wo_version should be stored")
	}
}

// Create and refresh call applyVM. The API never returns console_pass, so the
// value already on the model must survive that read. Storing the missing
// response as "" makes the next plan replace the VM.
func TestApplyVMKeepsConfiguredConsolePass(t *testing.T) {
	cases := []struct {
		name  string
		raw   string
		prior types.String
		want  string
	}{
		{
			name:  "omitted by api",
			raw:   `{"$key":7,"name":"vm","enabled":true,"machine":3,"cpu_cores":2,"ram":2048,"console_pass_enabled":true}`,
			prior: types.StringValue("configured"),
			want:  "configured",
		},
		{
			name:  "api value ignored",
			raw:   `{"$key":7,"name":"vm","enabled":true,"machine":3,"cpu_cores":2,"ram":2048,"console_pass_enabled":true,"console_pass":"from-api"}`,
			prior: types.StringValue("configured"),
			want:  "configured",
		},
		{
			name:  "explicit empty kept",
			raw:   `{"$key":7,"name":"vm","enabled":true,"machine":3,"cpu_cores":1,"ram":1024,"console_pass":"from-api"}`,
			prior: types.StringValue(""),
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var apiVM vergeos.VM
			if err := json.Unmarshal([]byte(tc.raw), &apiVM); err != nil {
				t.Fatal(err)
			}

			data := &VMResourceModel{
				ConsolePass: tc.prior,
				MachineType: types.StringValue("q35"),
			}
			applyVM(data, &apiVM)

			if data.ConsolePass.IsNull() || data.ConsolePass.IsUnknown() {
				t.Fatalf("console_pass = %#v, want %q", data.ConsolePass, tc.want)
			}
			if data.ConsolePass.ValueString() != tc.want {
				t.Fatalf("console_pass = %q, want %q", data.ConsolePass.ValueString(), tc.want)
			}
			if data.Name.ValueString() != "vm" {
				t.Fatalf("name = %q, want vm", data.Name.ValueString())
			}
			if data.Id.ValueString() != "" {
				t.Fatalf("id = %q, applyVM must not overwrite the resource id", data.Id.ValueString())
			}
			if tc.name != "explicit empty kept" && !data.ConsolePassEnabled.ValueBool() {
				t.Fatal("console_pass_enabled was not read from the API")
			}
		})
	}
}

func TestUsePlannedConsolePassSurvivesRead(t *testing.T) {
	plan := &VMResourceModel{
		ConsolePass: types.StringValue("new-secret"),
		MachineType: types.StringValue("q35"),
	}
	state := &VMResourceModel{
		ConsolePass: types.StringValue("old-secret"),
		MachineType: types.StringValue("q35"),
	}

	usePlannedConsolePass(state, plan)
	applyVM(state, &vergeos.VM{
		Name:               "vm",
		ConsolePass:        "from-api",
		ConsolePassEnabled: true,
	})

	if state.ConsolePass.IsNull() || state.ConsolePass.IsUnknown() || state.ConsolePass.ValueString() != "new-secret" {
		t.Fatalf("console_pass = %#v, want the planned password", state.ConsolePass)
	}
	if !state.ConsolePassEnabled.ValueBool() {
		t.Fatal("console_pass_enabled was not read from the API")
	}
}

func TestApplyVMDropsUnreadConsolePass(t *testing.T) {
	for _, prior := range []types.String{types.StringNull(), types.StringUnknown()} {
		data := &VMResourceModel{
			ConsolePass: prior,
			MachineType: types.StringNull(),
		}
		applyVM(data, &vergeos.VM{
			Key:         9,
			Name:        "vm",
			ConsolePass: "from-api",
		})
		if !data.ConsolePass.IsNull() {
			t.Fatalf("console_pass = %#v, want null when configuration has none", data.ConsolePass)
		}
		if data.Name.ValueString() != "vm" {
			t.Fatalf("name = %q, want vm", data.Name.ValueString())
		}
	}
}
