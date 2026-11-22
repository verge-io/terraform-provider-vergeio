// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func TestSelfSignedIssuedFieldsRequireReplace(t *testing.T) {
	self := certificateModel{
		ID:         types.StringValue("1"),
		Type:       types.StringValue(vergeos.CertificateTypeSelfSigned),
		DomainName: types.StringValue("tf-acc.example"),
		DomainList: types.StringValue("tf-acc.example"),
		KeyType:    types.StringValue(vergeos.CertificateKeyTypeECDSA),
	}
	lets := self
	lets.Type = types.StringValue(vergeos.CertificateTypeLetsEncrypt)
	manual := self
	manual.Type = types.StringValue(vergeos.CertificateTypeManual)

	names := self
	names.DomainList = types.StringValue("tf-acc.example,tf-acc-api.example")
	if planned, replaced := planCertificateString(t, "domain_list", &self, &names, names.DomainList, names.DomainList, self.DomainList); !replaced || planned.ValueString() != names.DomainList.ValueString() {
		t.Fatalf("self_signed domain_list replaced=%t plan=%#v", replaced, planned)
	}
	if _, replaced := planCertificateString(t, "domain_list", &self, &self, self.DomainList, self.DomainList, self.DomainList); replaced {
		t.Fatal("unchanged self_signed domain_list planned a replace")
	}
	omitted := self
	omitted.DomainList = types.StringUnknown()
	if planned, replaced := planCertificateString(t, "domain_list", &self, &omitted, types.StringNull(), types.StringUnknown(), self.DomainList); replaced || planned.ValueString() != self.DomainList.ValueString() {
		t.Fatalf("omitted domain_list replaced=%t plan=%#v", replaced, planned)
	}
	letsNames := lets
	letsNames.DomainList = names.DomainList
	if _, replaced := planCertificateString(t, "domain_list", &lets, &letsNames, names.DomainList, names.DomainList, lets.DomainList); replaced {
		t.Fatal("letsencrypt domain_list planned a replace")
	}
	manualNames := manual
	manualNames.DomainList = names.DomainList
	if _, replaced := planCertificateString(t, "domain_list", &manual, &manualNames, names.DomainList, names.DomainList, manual.DomainList); replaced {
		t.Fatal("manual domain_list planned a replace")
	}

	rsa := self
	rsa.KeyType = types.StringValue(vergeos.CertificateKeyTypeRSA)
	rsa.RSAKeySize = types.StringValue("2048")
	if _, replaced := planCertificateString(t, "key_type", &self, &rsa, rsa.KeyType, rsa.KeyType, self.KeyType); !replaced {
		t.Fatal("self_signed key_type change did not plan a replace")
	}
	letsKey := lets
	letsKey.KeyType = rsa.KeyType
	if _, replaced := planCertificateString(t, "key_type", &lets, &letsKey, rsa.KeyType, rsa.KeyType, lets.KeyType); replaced {
		t.Fatal("letsencrypt key_type planned a replace")
	}
	unknownKey := self
	unknownKey.KeyType = types.StringUnknown()
	if planned, replaced := planCertificateString(t, "key_type", &self, &unknownKey, types.StringNull(), types.StringUnknown(), self.KeyType); replaced || planned.ValueString() != self.KeyType.ValueString() {
		t.Fatalf("omitted key_type replaced=%t plan=%#v", replaced, planned)
	}
	if _, replaced := planCertificateString(t, "rsa_key_size", &self, &rsa, rsa.RSAKeySize, rsa.RSAKeySize, types.StringNull()); !replaced {
		t.Fatal("self_signed rsa_key_size change did not plan a replace")
	}
	letsSize := lets
	letsSize.RSAKeySize = rsa.RSAKeySize
	if _, replaced := planCertificateString(t, "rsa_key_size", &lets, &letsSize, rsa.RSAKeySize, rsa.RSAKeySize, types.StringNull()); replaced {
		t.Fatal("letsencrypt rsa_key_size planned a replace")
	}

	described := self
	described.Description = types.StringValue("UI certificate")
	if _, replaced := planCertificateString(t, "description", &self, &described, described.Description, described.Description, types.StringNull()); replaced {
		t.Fatal("self_signed description planned a replace")
	}
}

func TestSelfSignedKeySettingsWarn(t *testing.T) {
	schema := schemaOf(t, NewCertificateResource())
	r := &certificateResource{}
	self := certificateModel{
		Type:       types.StringValue(vergeos.CertificateTypeSelfSigned),
		DomainName: types.StringValue("ui.example.com"),
		KeyType:    types.StringValue(vergeos.CertificateKeyTypeECDSA),
	}
	sizeOnly := self
	sizeOnly.KeyType = types.StringNull()
	sizeOnly.RSAKeySize = types.StringValue("4096")
	unknownKey := self
	unknownKey.KeyType = types.StringUnknown()
	omitted := self
	omitted.KeyType = types.StringNull()
	omitted.RSAKeySize = types.StringNull()
	lets := self
	lets.Type = types.StringValue(vergeos.CertificateTypeLetsEncrypt)
	manual := self
	manual.Type = types.StringValue(vergeos.CertificateTypeManual)
	unknownType := self
	unknownType.Type = types.StringUnknown()

	for _, tc := range []struct {
		name   string
		config certificateModel
		warn   bool
	}{
		{name: "key_type", config: self, warn: true},
		{name: "rsa_key_size", config: sizeOnly, warn: true},
		{name: "unknown key_type", config: unknownKey, warn: true},
		{name: "omitted", config: omitted, warn: false},
		{name: "letsencrypt", config: lets, warn: false},
		{name: "manual", config: manual, warn: false},
		{name: "unknown type", config: unknownType, warn: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			config := certificateConfig(t, schema, &tc.config)
			plan := tfsdk.Plan{Schema: schema}
			planned := tc.config
			if diags := plan.Set(ctx, &planned); diags.HasError() {
				t.Fatal(diags)
			}
			before := plan.Raw.Copy()
			resp := &resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: config, Plan: plan}, resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			if got := hasWarning(resp.Diagnostics, "always issues RSA 2048"); got != tc.warn {
				t.Fatalf("warning = %t, want %t: %v", got, tc.warn, resp.Diagnostics)
			}
			if !resp.Plan.Raw.Equal(before) {
				t.Fatal("plan changed")
			}
		})
	}

	ctx := context.Background()
	config := certificateConfig(t, schema, &self)
	resp := &resource.ModifyPlanResponse{Plan: tfsdk.Plan{Schema: schema}}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: config, Plan: resp.Plan}, resp)
	if resp.Diagnostics.HasError() || resp.Diagnostics.WarningsCount() != 0 {
		t.Fatalf("destroy plan diagnostics = %v", resp.Diagnostics)
	}
}

func TestCertificateKeySchemaMatchesDocs(t *testing.T) {
	schema := schemaOf(t, NewCertificateResource())
	key, ok := schema.Attributes["key_type"].(resschema.StringAttribute)
	if !ok {
		t.Fatal("missing key_type")
	}
	size, ok := schema.Attributes["rsa_key_size"].(resschema.StringAttribute)
	if !ok {
		t.Fatal("missing rsa_key_size")
	}
	if key.MarkdownDescription != certificateKeyTypeDescription {
		t.Fatalf("key_type description = %s", key.MarkdownDescription)
	}
	if size.MarkdownDescription != certificateRSAKeySizeDescription {
		t.Fatalf("rsa_key_size description = %s", size.MarkdownDescription)
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate certificate plan test")
	}
	docs := filepath.Join(filepath.Dir(file), "../../../docs/resources/certificate.md")
	body, err := os.ReadFile(docs)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, key.MarkdownDescription) || !strings.Contains(text, size.MarkdownDescription) {
		t.Fatal("docs/resources/certificate.md schema text differs from the resource schema")
	}
	if strings.Contains(text, `key_type    = "ecdsa"`) {
		t.Fatal("self_signed example still sets key_type ecdsa")
	}
}

func certificateConfig(t *testing.T, schema resschema.Schema, model *certificateModel) tfsdk.Config {
	t.Helper()
	state := tfsdk.State{Schema: schema}
	if diags := state.Set(context.Background(), model); diags.HasError() {
		t.Fatal(diags)
	}
	return tfsdk.Config{Schema: schema, Raw: state.Raw}
}

func hasWarning(diags diag.Diagnostics, detail string) bool {
	for _, item := range diags {
		if item.Severity() == diag.SeverityWarning && strings.Contains(item.Detail(), detail) {
			return true
		}
	}
	return false
}

func planCertificateString(t *testing.T, name string, stateModel, planModel *certificateModel, config, planValue, stateValue types.String) (types.String, bool) {
	t.Helper()
	ctx := context.Background()
	schema := schemaOf(t, NewCertificateResource())
	attr, ok := schema.Attributes[name].(resschema.StringAttribute)
	if !ok {
		t.Fatalf("missing %s", name)
	}
	state := tfsdk.State{Schema: schema}
	plan := tfsdk.Plan{Schema: schema}
	if diags := state.Set(ctx, stateModel); diags.HasError() {
		t.Fatal(diags)
	}
	if diags := plan.Set(ctx, planModel); diags.HasError() {
		t.Fatal(diags)
	}
	req := planmodifier.StringRequest{
		ConfigValue: config,
		Plan:        plan,
		PlanValue:   planValue,
		State:       state,
		StateValue:  stateValue,
	}
	replaced := false
	for _, mod := range attr.PlanModifiers {
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		mod.PlanModifyString(ctx, req, resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		replaced = replaced || resp.RequiresReplace
		req.PlanValue = resp.PlanValue
	}
	return req.PlanValue, replaced
}
