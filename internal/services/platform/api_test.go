// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/verge-io/govergeos"

	"terraform-provider-vergeio/internal/client"
)

func TestPlatformSchema(t *testing.T) {
	cert := schemaOf(t, NewCertificateResource())
	for _, name := range []string{"expires", "created", "modified"} {
		attr, ok := cert.Attributes[name].(resschema.Int64Attribute)
		if !ok || len(attr.PlanModifiers) != 0 {
			t.Fatalf("certificate %s must be computed without UseStateForUnknown", name)
		}
	}
	kind, ok := cert.Attributes["type"].(resschema.StringAttribute)
	if !ok || !kind.Required || !requiresReplace(kind.PlanModifiers) {
		t.Fatal("certificate type must be required and require replace")
	}
	domainName, ok := cert.Attributes["domain_name"].(resschema.StringAttribute)
	if !ok || len(domainName.PlanModifiers) != 2 {
		t.Fatal("domain_name must copy state before it decides on replace")
	}
	if !strings.Contains(domainName.PlanModifiers[0].Description(context.Background()), "will not change") {
		t.Fatal("UseStateForUnknown must run before RequiresReplace")
	}
	if !strings.Contains(domainName.PlanModifiers[1].Description(context.Background()), "configured and changes") {
		t.Fatal("domain_name must replace only when a configured value changes")
	}
	privateKey, ok := cert.Attributes["private_key_wo"].(resschema.StringAttribute)
	if !ok || !privateKey.WriteOnly || !privateKey.Sensitive {
		t.Fatal("private_key_wo must be write-only and sensitive")
	}
	hmac, ok := cert.Attributes["eab_hmac_key_wo"].(resschema.StringAttribute)
	if !ok || !hmac.WriteOnly || !hmac.Sensitive {
		t.Fatal("eab_hmac_key_wo must be write-only and sensitive")
	}

	urlSchema := schemaOf(t, NewWebhookURLResource())
	auth, ok := urlSchema.Attributes["authorization_value_wo"].(resschema.StringAttribute)
	if !ok || !auth.WriteOnly || !auth.Sensitive {
		t.Fatal("authorization_value_wo must be write-only and sensitive")
	}

	hook := schemaOf(t, NewWebhookResource())
	for _, name := range []string{"last_attempt", "created"} {
		attr, ok := hook.Attributes[name].(resschema.Int64Attribute)
		if !ok || len(attr.PlanModifiers) != 0 {
			t.Fatalf("webhook %s must be computed without UseStateForUnknown", name)
		}
	}
	message, ok := hook.Attributes["message"].(resschema.StringAttribute)
	if !ok || !message.Required || !requiresReplace(message.PlanModifiers) {
		t.Fatal("webhook message must be required and require replace")
	}
	urlID, ok := hook.Attributes["webhook_url_id"].(resschema.StringAttribute)
	if !ok || !urlID.Required || !requiresReplace(urlID.PlanModifiers) {
		t.Fatal("webhook_url_id must be required and require replace")
	}

	setting := schemaOf(t, NewSettingResource())
	key, ok := setting.Attributes["key"].(resschema.StringAttribute)
	if !ok || !key.Required || !requiresReplace(key.PlanModifiers) {
		t.Fatal("setting key must be required and require replace")
	}
	valueWO, ok := setting.Attributes["value_wo"].(resschema.StringAttribute)
	if !ok || !valueWO.WriteOnly || !valueWO.Sensitive {
		t.Fatal("value_wo must be write-only and sensitive")
	}
}

func TestSettingKeyAndPEMHelpers(t *testing.T) {
	if err := validSettingKey("max_connections"); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"", "a/b", `a\b`, "a b", "a?b", "a#b"} {
		if err := validSettingKey(key); err == nil {
			t.Fatalf("key %q should be rejected", key)
		}
	}
	prior := types.StringValue("-----BEGIN CERTIFICATE-----\r\nabc\r\n-----END CERTIFICATE-----")
	kept := keepPEM("-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n", prior)
	if kept.ValueString() != prior.ValueString() {
		t.Fatalf("keepPEM changed equivalent PEM to %q", kept.ValueString())
	}
	changed := keepPEM("-----BEGIN CERTIFICATE-----\nother\n-----END CERTIFICATE-----\n", prior)
	if !strings.Contains(changed.ValueString(), "other") {
		t.Fatalf("keepPEM kept a different certificate: %q", changed.ValueString())
	}
}

func TestSettingConfigRequiresOneValue(t *testing.T) {
	ctx := context.Background()
	item := &settingResource{}
	schemaResp := &resource.SchemaResponse{}
	item.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	validators := item.ConfigValidators(ctx)

	both := &settingModel{Key: types.StringValue("max_connections"), Value: types.StringValue("1"), ValueWO: types.StringValue("2"), ValueWOVersion: types.Int64Value(1)}
	if resp := validateConfig(t, schemaResp.Schema, both, validators); !resp.Diagnostics.HasError() {
		t.Fatal("value and value_wo together should fail")
	}
	neither := &settingModel{Key: types.StringValue("max_connections")}
	if resp := validateConfig(t, schemaResp.Schema, neither, validators); !resp.Diagnostics.HasError() {
		t.Fatal("missing value should fail")
	}
	slash := &settingModel{Key: types.StringValue("a/b"), Value: types.StringValue("1")}
	if resp := validateConfig(t, schemaResp.Schema, slash, validators); !resp.Diagnostics.HasError() {
		t.Fatal("slash in the key should fail")
	}
	okModel := &settingModel{Key: types.StringValue("max_connections"), Value: types.StringValue("1")}
	if resp := validateConfig(t, schemaResp.Schema, okModel, validators); resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
}

func TestCertificateCreateKeepsPrivateKeyOutOfState(t *testing.T) {
	fake := newPlatformFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)
	ctx := context.Background()

	pub := "-----BEGIN CERTIFICATE-----\nabc\n-----END CERTIFICATE-----\n"
	priv := "-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----\n"
	manual := &certificateModel{
		Type:                types.StringValue(vergeos.CertificateTypeManual),
		DomainName:          types.StringValue("tf-acc.example"),
		Description:         types.StringValue("uploaded"),
		PublicCertificate:   types.StringValue(pub),
		PrivateKeyWOVersion: types.Int64Value(1),
	}
	if err := api.createCertificate(ctx, manual, priv, ""); err != nil {
		t.Fatal(err)
	}
	if manual.ID.ValueString() == "" || manual.Type.ValueString() != vergeos.CertificateTypeManual {
		t.Fatalf("manual certificate state = %#v", manual)
	}
	if manual.PublicCertificate.ValueString() != pub {
		t.Fatalf("public certificate = %q", manual.PublicCertificate.ValueString())
	}
	var created vergeos.CertificateCreateRequest
	if err := json.Unmarshal([]byte(fake.certCreateBody), &created); err != nil {
		t.Fatal(err)
	}
	if created.Private != priv || created.Public != pub {
		t.Fatalf("create body = %s", fake.certCreateBody)
	}
	if fake.privateReads != 0 {
		t.Fatalf("private key was read %d times", fake.privateReads)
	}
	stored := certificateForState(manual)
	stored.PrivateKeyWO = types.StringValue(priv)
	stored = certificateForState(&stored)
	if !stored.PrivateKeyWO.IsNull() || stored.PrivateKeyWOVersion.ValueInt64() != 1 {
		t.Fatal("state must keep the version and drop the private key")
	}

	lets := &certificateModel{
		Type:       types.StringValue(vergeos.CertificateTypeLetsEncrypt),
		DomainName: types.StringValue("ui.example.com"),
		Contact:    types.Int64Value(9),
		AgreeTOS:   types.BoolValue(true),
	}
	fake.certCreateBody = ""
	if err := api.createCertificate(ctx, lets, "", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fake.certCreateBody, "PRIVATE KEY") || !strings.Contains(fake.certCreateBody, `"agree_tos":true`) || !strings.Contains(fake.certCreateBody, `"contact":9`) {
		t.Fatalf("letsencrypt body = %s", fake.certCreateBody)
	}

	self := &certificateModel{
		Type:       types.StringValue(vergeos.CertificateTypeSelfSigned),
		DomainName: types.StringValue("tf-acc-ui.local"),
		KeyType:    types.StringValue(vergeos.CertificateKeyTypeECDSA),
	}
	fake.certCreateBody = ""
	if err := api.createCertificate(ctx, self, "", ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.certCreateBody, `"domainname":"tf-acc-ui.local"`) || !strings.Contains(fake.certCreateBody, `"type":"self_signed"`) {
		t.Fatalf("self signed body = %s", fake.certCreateBody)
	}
	if self.DomainName.ValueString() != "tf-acc-ui.local" || self.Domain.ValueString() != "tf-acc-ui.local" {
		t.Fatalf("domain after create = %s / %s", self.DomainName.ValueString(), self.Domain.ValueString())
	}

	updated := *manual
	updated.Description = types.StringValue("replaced text")
	if err := api.updateCertificate(ctx, &updated, manual, "", ""); err != nil {
		t.Fatal(err)
	}
	if updated.Description.ValueString() != "replaced text" || !strings.Contains(fake.certUpdateBody, `"description":"replaced text"`) || strings.Contains(fake.certUpdateBody, "PRIVATE KEY") {
		t.Fatalf("update body = %s", fake.certUpdateBody)
	}
}

func TestCertificateImportReadFillsReplaceFields(t *testing.T) {
	fake := newPlatformFake()
	fake.seedCertificate(vergeos.Certificate{
		Domain:      "tf-acc-import.local",
		Description: "imported",
		Type:        vergeos.CertificateTypeSelfSigned,
		KeyType:     vergeos.CertificateKeyTypeECDSA,
		Valid:       true,
	})
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	ctx := context.Background()
	item := configuredCertificate(t, server.URL)
	schemaResp := &resource.SchemaResponse{}
	item.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	imported := importState(ctx, t, schemaResp.Schema, item, "1")
	var data certificateModel
	imported.Diagnostics.Append(imported.State.Get(ctx, &data)...)
	if imported.Diagnostics.HasError() {
		t.Fatal(imported.Diagnostics)
	}
	read := &resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	item.Read(ctx, resource.ReadRequest{State: imported.State}, read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	read.Diagnostics.Append(read.State.Get(ctx, &data)...)
	if data.Type.ValueString() != vergeos.CertificateTypeSelfSigned || data.DomainName.ValueString() != "tf-acc-import.local" {
		t.Fatalf("imported type=%s domain_name=%s", data.Type.ValueString(), data.DomainName.ValueString())
	}
}

func TestCertificateImportReadFillsDomainNameFromDomainList(t *testing.T) {
	fake := newPlatformFake()
	fake.seedCertificate(vergeos.Certificate{
		DomainList:  "tf-acc-list.local, other.local",
		Description: "imported",
		Type:        vergeos.CertificateTypeSelfSigned,
	})
	data := importCertificate(t, fake, "1")
	if data.DomainName.ValueString() != "tf-acc-list.local" {
		t.Fatalf("domain_name = %q", data.DomainName.ValueString())
	}
}

func TestCertificateImportReadFillsDomainNameFromCertificate(t *testing.T) {
	fake := newPlatformFake()
	fake.seedCertificate(vergeos.Certificate{
		Description: "imported",
		Type:        vergeos.CertificateTypeSelfSigned,
		Public:      testCertificatePEM(t, "tf-acc-cn.local", ""),
	})
	data := importCertificate(t, fake, "1")
	if data.DomainName.ValueString() != "tf-acc-cn.local" {
		t.Fatalf("domain_name = %q", data.DomainName.ValueString())
	}

	dnsOnly := newPlatformFake()
	dnsOnly.seedCertificate(vergeos.Certificate{
		Type:   vergeos.CertificateTypeSelfSigned,
		Public: testCertificatePEM(t, "", "tf-acc-dns.local"),
	})
	fromDNS := importCertificate(t, dnsOnly, "1")
	if fromDNS.DomainName.ValueString() != "tf-acc-dns.local" {
		t.Fatalf("dns domain_name = %q", fromDNS.DomainName.ValueString())
	}
}

func TestCertificateImportReadPrefersStoredDomainName(t *testing.T) {
	fake := newPlatformFake()
	id := fake.seedCertificate(vergeos.Certificate{
		Description: "imported",
		Type:        vergeos.CertificateTypeSelfSigned,
		Public:      testCertificatePEM(t, "verge-api", ""),
	})
	fake.setDomainName(id, "tf-acc-cert.local")
	data := importCertificate(t, fake, strconv.Itoa(id))
	if data.DomainName.ValueString() != "tf-acc-cert.local" {
		t.Fatalf("domain_name = %q", data.DomainName.ValueString())
	}
	if data.Domain.ValueString() != "" {
		t.Fatalf("domain = %q", data.Domain.ValueString())
	}
	sawDomainName := false
	for _, fields := range fake.fieldQueries() {
		if fields == "domainname" {
			sawDomainName = true
		}
	}
	if !sawDomainName {
		t.Fatalf("domainname was not requested: %#v", fake.fieldQueries())
	}
}

func TestDomainNameOmitDoesNotRequireReplace(t *testing.T) {
	ctx := context.Background()
	schemaResp := &resource.SchemaResponse{}
	NewCertificateResource().Schema(ctx, resource.SchemaRequest{}, schemaResp)
	attr, ok := schemaResp.Schema.Attributes["domain_name"].(resschema.StringAttribute)
	if !ok {
		t.Fatal("missing domain_name")
	}

	state := tfsdk.State{Schema: schemaResp.Schema}
	if diags := state.Set(ctx, &certificateModel{
		ID:         types.StringValue("1"),
		Type:       types.StringValue(vergeos.CertificateTypeLetsEncrypt),
		DomainName: types.StringValue("ui.example.com"),
		DomainList: types.StringValue("ui.example.com"),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	if diags := plan.Set(ctx, &certificateModel{
		ID:         types.StringValue("1"),
		Type:       types.StringValue(vergeos.CertificateTypeLetsEncrypt),
		DomainName: types.StringUnknown(),
		DomainList: types.StringValue("ui.example.com"),
	}); diags.HasError() {
		t.Fatal(diags)
	}
	req := planmodifier.StringRequest{
		ConfigValue: types.StringNull(),
		Plan:        plan,
		PlanValue:   types.StringUnknown(),
		State:       state,
		StateValue:  types.StringValue("ui.example.com"),
	}
	for _, mod := range attr.PlanModifiers {
		resp := &planmodifier.StringResponse{PlanValue: req.PlanValue}
		mod.PlanModifyString(ctx, req, resp)
		if resp.RequiresReplace {
			t.Fatal("omitted domain_name planned a replace")
		}
		req.PlanValue = resp.PlanValue
	}
	if req.PlanValue.ValueString() != "ui.example.com" {
		t.Fatalf("planned domain_name = %#v", req.PlanValue)
	}

	changed := planmodifier.StringRequest{
		ConfigValue: types.StringValue("other.example.com"),
		Plan:        plan,
		PlanValue:   types.StringValue("other.example.com"),
		State:       state,
		StateValue:  types.StringValue("ui.example.com"),
	}
	replaced := false
	for _, mod := range attr.PlanModifiers {
		resp := &planmodifier.StringResponse{PlanValue: changed.PlanValue}
		mod.PlanModifyString(ctx, changed, resp)
		replaced = replaced || resp.RequiresReplace
		changed.PlanValue = resp.PlanValue
	}
	if !replaced || changed.PlanValue.ValueString() != "other.example.com" {
		t.Fatalf("configured domain_name change replaced=%t plan=%#v", replaced, changed.PlanValue)
	}
}

func TestSettingPUTRetriesUnavailable(t *testing.T) {
	fake := newPlatformFake()
	fake.failPuts = 1
	fake.failPutStatus = http.StatusServiceUnavailable
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)

	current := &settingModel{Key: types.StringValue("max_connections"), Value: types.StringValue("100")}
	if err := api.createSetting(context.Background(), current, ""); err != nil {
		t.Fatal(err)
	}
	updated := *current
	updated.Value = types.StringValue("501")
	if err := api.updateSetting(context.Background(), &updated, current, ""); err != nil {
		t.Fatal(err)
	}
	if fake.putAttempts != 2 || len(fake.puts) != 1 || fake.puts[0].body != `{"value":"501"}` {
		t.Fatalf("attempts=%d puts=%+v", fake.putAttempts, fake.puts)
	}

	rejected := newPlatformFake()
	rejected.failPuts = 1
	rejected.failPutStatus = http.StatusBadRequest
	rejectedServer := httptest.NewServer(rejected)
	t.Cleanup(rejectedServer.Close)
	rejectedAPI := testAPI(t, rejectedServer.URL)
	again := updated
	again.Value = types.StringValue("502")
	// The rejected server has its own settings still at 100, so this is a real change.
	base := &settingModel{Key: types.StringValue("max_connections"), Value: types.StringValue("100")}
	err := rejectedAPI.updateSetting(context.Background(), &again, base, "")
	if err == nil {
		t.Fatal("expected the 400 to fail")
	}
	if rejected.putAttempts != 1 {
		t.Fatalf("400 was retried %d times", rejected.putAttempts)
	}
}

func TestWebhookURLAndDelivery(t *testing.T) {
	fake := newPlatformFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)
	ctx := context.Background()

	dest := &webhookURLModel{
		Name:                        types.StringValue("tf-acc-chat"),
		URL:                         types.StringValue("https://example.com/hook"),
		AuthorizationType:           types.StringValue(vergeos.WebhookAuthBearer),
		AuthorizationValueWOVersion: types.Int64Value(1),
		Timeout:                     types.Int64Value(10),
	}
	if err := api.createWebhookURL(ctx, dest, "s3cret"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(fake.urlCreateBody, `"authorization_value":"s3cret"`) {
		t.Fatalf("url create body = %s", fake.urlCreateBody)
	}
	if dest.ID.ValueString() == "" || !dest.AuthorizationValueWO.IsNull() || dest.AuthorizationValueWOVersion.ValueInt64() != 1 {
		t.Fatalf("destination state id=%s secret null=%t", dest.ID.ValueString(), dest.AuthorizationValueWO.IsNull())
	}
	stored := webhookURLForState(dest)
	stored.AuthorizationValueWO = types.StringValue("s3cret")
	stored = webhookURLForState(&stored)
	if !stored.AuthorizationValueWO.IsNull() {
		t.Fatal("webhook destination state stored the credential")
	}

	next := *dest
	next.Timeout = types.Int64Value(30)
	next.URL = types.StringValue("https://example.com/hook/v2")
	if err := api.updateWebhookURL(ctx, &next, dest, ""); err != nil {
		t.Fatal(err)
	}
	if next.Timeout.ValueInt64() != 30 || next.URL.ValueString() != "https://example.com/hook/v2" {
		t.Fatalf("updated destination = %#v", next)
	}
	if strings.Contains(fake.urlUpdateBody, "s3cret") || !strings.Contains(fake.urlUpdateBody, `"timeout":30`) {
		t.Fatalf("url update body = %s", fake.urlUpdateBody)
	}

	hook := &webhookModel{
		WebhookURLID: next.ID,
		Message:      types.StringValue(`{"text":"lab"}`),
	}
	if err := api.createWebhook(ctx, hook); err != nil {
		t.Fatal(err)
	}
	if hook.ID.ValueString() == "" || hook.Message.ValueString() != `{"text":"lab"}` || hook.WebhookURLID.ValueString() != next.ID.ValueString() {
		t.Fatalf("delivery = %#v", hook)
	}
	if hook.Status.ValueString() != vergeos.WebhookStatusQueued {
		t.Fatalf("status = %s", hook.Status.ValueString())
	}

	id, err := parseID(next.ID, "webhook destination")
	if err != nil {
		t.Fatal(err)
	}
	if err := api.deleteWebhookURL(ctx, id); err != nil {
		t.Fatal(err)
	}
	if len(fake.urls) != 0 || len(fake.hooks) != 0 {
		t.Fatalf("destination delete left urls=%d hooks=%d", len(fake.urls), len(fake.hooks))
	}
}

func TestSettingUpdatesOneRowAndDestroyRestoresDefault(t *testing.T) {
	fake := newPlatformFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)
	ctx := context.Background()

	same := &settingModel{Key: types.StringValue("max_connections"), Value: types.StringValue("100")}
	if err := api.createSetting(ctx, same, ""); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 0 {
		t.Fatalf("unchanged value was written: %+v", fake.puts)
	}
	if same.ID.ValueString() != "max_connections" || same.Value.ValueString() != "100" || same.DefaultValue.ValueString() != "100" {
		t.Fatalf("setting = %#v", same)
	}

	missing := &settingModel{Key: types.StringValue("not_a_setting"), Value: types.StringValue("1")}
	err := api.createSetting(ctx, missing, "")
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing key error = %v", err)
	}

	changed := *same
	changed.Value = types.StringValue("501")
	if err := api.updateSetting(ctx, &changed, same, ""); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 1 || fake.puts[0].path != "/api/v4/settings/7" || fake.puts[0].body != `{"value":"501"}` {
		t.Fatalf("puts = %+v", fake.puts)
	}
	if fake.settings["cloud_name"].Value != "lab" {
		t.Fatalf("cloud_name changed to %q", fake.settings["cloud_name"].Value)
	}
	again := changed
	if err := api.updateSetting(ctx, &again, &changed, ""); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != 1 {
		t.Fatalf("repeat update wrote again: %+v", fake.puts)
	}

	blank := changed
	blank.Value = types.StringValue("")
	if err := api.updateSetting(ctx, &blank, &changed, ""); err != nil {
		t.Fatal(err)
	}
	if fake.puts[len(fake.puts)-1].body != `{"value":""}` || blank.Value.ValueString() != "" {
		t.Fatalf("empty value put = %+v state=%q", fake.puts[len(fake.puts)-1], blank.Value.ValueString())
	}

	secretSetting := &settingModel{
		Key:            types.StringValue("max_connections"),
		ValueWOVersion: types.Int64Value(1),
	}
	if err := api.createSetting(ctx, secretSetting, "hidden"); err != nil {
		t.Fatal(err)
	}
	if !secretSetting.Value.IsNull() || secretSetting.ValueWOVersion.ValueInt64() != 1 {
		t.Fatalf("secret setting stored value %#v", secretSetting.Value)
	}
	if !strings.Contains(fake.puts[len(fake.puts)-1].body, `"value":"hidden"`) {
		t.Fatalf("secret put = %+v", fake.puts[len(fake.puts)-1])
	}
	held := *secretSetting
	if err := api.updateSetting(ctx, &held, secretSetting, ""); err != nil {
		t.Fatal(err)
	}
	putsAfterHold := len(fake.puts)
	bumped := held
	bumped.ValueWOVersion = types.Int64Value(2)
	if err := api.updateSetting(ctx, &bumped, &held, "hidden-2"); err != nil {
		t.Fatal(err)
	}
	if len(fake.puts) != putsAfterHold+1 || !strings.Contains(fake.puts[len(fake.puts)-1].body, `"value":"hidden-2"`) {
		t.Fatalf("version bump puts = %+v", fake.puts)
	}

	if err := api.deleteSetting(ctx, "max_connections"); err != nil {
		t.Fatal(err)
	}
	if fake.settings["max_connections"].Value != "100" || fake.settings["cloud_name"].Value != "lab" {
		t.Fatalf("after destroy max=%q cloud=%q", fake.settings["max_connections"].Value, fake.settings["cloud_name"].Value)
	}
	if len(fake.deletes) != 0 {
		t.Fatalf("setting destroy deleted rows: %+v", fake.deletes)
	}
	if err := api.deleteSetting(ctx, "not_a_setting"); err != nil {
		t.Fatalf("missing setting on destroy: %v", err)
	}
}

func TestSettingImportReadFillsKey(t *testing.T) {
	fake := newPlatformFake()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	ctx := context.Background()
	item := configuredSetting(t, server.URL)
	schemaResp := &resource.SchemaResponse{}
	item.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	imported := importState(ctx, t, schemaResp.Schema, item, "max_connections")
	read := &resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	item.Read(ctx, resource.ReadRequest{State: imported.State}, read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var data settingModel
	read.Diagnostics.Append(read.State.Get(ctx, &data)...)
	if data.Key.ValueString() != "max_connections" || data.Value.ValueString() != "100" || data.ID.ValueString() != "max_connections" {
		t.Fatalf("imported setting = %#v", data)
	}

	bad := &resource.ImportStateResponse{State: tfsdk.State{
		Schema: schemaResp.Schema,
		Raw:    tftypes.NewValue(schemaResp.Schema.Type().TerraformType(ctx), nil),
	}}
	item.ImportState(ctx, resource.ImportStateRequest{ID: "a/b"}, bad)
	if !bad.Diagnostics.HasError() {
		t.Fatal("import id with a slash should fail")
	}
}

func TestSweepRemovesPrefixedCertificatesAndWebhookURLs(t *testing.T) {
	fake := newPlatformFake()
	fake.seedCertificate(vergeos.Certificate{Domain: "tf-acc-old.local", Description: "leftover", Type: vergeos.CertificateTypeSelfSigned})
	fake.seedCertificate(vergeos.Certificate{Domain: "ui.example.com", Description: "keep", Type: vergeos.CertificateTypeManual})
	fake.seedURL("tf-acc-old", "https://example.com/old")
	fake.seedURL("alerts", "https://example.com/keep")
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	api := testAPI(t, server.URL)
	ctx := context.Background()
	urlID := 0
	for id, item := range fake.urls {
		if item.Name == "tf-acc-old" {
			urlID = id
		}
	}
	fake.hooks[1] = &vergeos.Webhook{Key: 1, WebhookURL: vergeos.FlexInt(urlID), Message: "old", Status: vergeos.WebhookStatusQueued}

	if err := SweepTestRows(ctx, api.sdk, func(name string) bool { return strings.HasPrefix(name, "tf-acc-") }); err != nil {
		t.Fatal(err)
	}
	if left := Leftovers(ctx, api.sdk, func(name string) bool { return strings.HasPrefix(name, "tf-acc-") }); len(left) != 0 {
		t.Fatalf("leftovers = %v", left)
	}
	if _, ok := fake.certsByDomain("ui.example.com"); !ok {
		t.Fatal("unrelated certificate was deleted")
	}
	if _, ok := fake.certsByDomain("tf-acc-old.local"); ok {
		t.Fatal("prefixed certificate remains")
	}
	if _, ok := fake.urlByName("alerts"); !ok {
		t.Fatal("unrelated webhook destination was deleted")
	}
	if _, ok := fake.urlByName("tf-acc-old"); ok || len(fake.hooks) != 0 {
		t.Fatal("prefixed destination or its delivery remains")
	}
	if fake.settings["max_connections"].Value != "100" || fake.settings["cloud_name"].Value != "lab" {
		t.Fatal("sweep changed system settings")
	}
}

func TestImportRejectsBadIDs(t *testing.T) {
	ctx := context.Background()
	cert := &certificateResource{}
	schemaResp := &resource.SchemaResponse{}
	cert.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	bad := importResponse(ctx, schemaResp.Schema)
	cert.ImportState(ctx, resource.ImportStateRequest{ID: "domain"}, bad)
	if !bad.Diagnostics.HasError() {
		t.Fatal("non numeric certificate import should fail")
	}
	ok := importResponse(ctx, schemaResp.Schema)
	cert.ImportState(ctx, resource.ImportStateRequest{ID: "4"}, ok)
	if ok.Diagnostics.HasError() {
		t.Fatal(ok.Diagnostics)
	}

	hook := &webhookResource{}
	hookSchema := &resource.SchemaResponse{}
	hook.Schema(ctx, resource.SchemaRequest{}, hookSchema)
	rejected := importResponse(ctx, hookSchema.Schema)
	hook.ImportState(ctx, resource.ImportStateRequest{ID: "0"}, rejected)
	if !rejected.Diagnostics.HasError() {
		t.Fatal("webhook import id 0 should fail")
	}
}

func schemaOf(t *testing.T, item resource.Resource) resschema.Schema {
	t.Helper()
	resp := &resource.SchemaResponse{}
	item.Schema(context.Background(), resource.SchemaRequest{}, resp)
	return resp.Schema
}

func requiresReplace(mods []planmodifier.String) bool {
	for _, mod := range mods {
		if strings.Contains(mod.Description(context.Background()), "destroy and recreate") {
			return true
		}
	}
	return false
}

func validateConfig(t *testing.T, schema resschema.Schema, model *settingModel, validators []resource.ConfigValidator) *resource.ValidateConfigResponse {
	t.Helper()
	ctx := context.Background()
	state := tfsdk.State{Schema: schema}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatal(diags)
	}
	resp := &resource.ValidateConfigResponse{}
	req := resource.ValidateConfigRequest{Config: tfsdk.Config{Schema: schema, Raw: state.Raw}}
	for _, validator := range validators {
		validator.ValidateResource(ctx, req, resp)
	}
	return resp
}

func configuredCertificate(t *testing.T, host string) *certificateResource {
	t.Helper()
	item := &certificateResource{}
	configured := &resource.ConfigureResponse{}
	item.Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, configured)
	if configured.Diagnostics.HasError() {
		t.Fatal(configured.Diagnostics)
	}
	return item
}

func configuredSetting(t *testing.T, host string) *settingResource {
	t.Helper()
	item := &settingResource{}
	configured := &resource.ConfigureResponse{}
	item.Configure(context.Background(), resource.ConfigureRequest{
		ProviderData: vergeio.NewClient(host, "user", "pass", true),
	}, configured)
	if configured.Diagnostics.HasError() {
		t.Fatal(configured.Diagnostics)
	}
	return item
}

func importState(ctx context.Context, t *testing.T, schema resschema.Schema, item resource.ResourceWithImportState, id string) *resource.ImportStateResponse {
	t.Helper()
	resp := importResponse(ctx, schema)
	item.ImportState(ctx, resource.ImportStateRequest{ID: id}, resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	return resp
}

func importCertificate(t *testing.T, fake *platformFake, id string) certificateModel {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	ctx := context.Background()
	item := configuredCertificate(t, server.URL)
	schemaResp := &resource.SchemaResponse{}
	item.Schema(ctx, resource.SchemaRequest{}, schemaResp)
	imported := importState(ctx, t, schemaResp.Schema, item, id)
	read := &resource.ReadResponse{State: tfsdk.State{Schema: schemaResp.Schema}}
	item.Read(ctx, resource.ReadRequest{State: imported.State}, read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var data certificateModel
	read.Diagnostics.Append(read.State.Get(ctx, &data)...)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	return data
}

func testCertificatePEM(t *testing.T, commonName, dnsName string) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if commonName != "" {
		tmpl.Subject = pkix.Name{CommonName: commonName}
	}
	if dnsName != "" {
		tmpl.DNSNames = []string{dnsName}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func importResponse(ctx context.Context, schema resschema.Schema) *resource.ImportStateResponse {
	return &resource.ImportStateResponse{State: tfsdk.State{
		Schema: schema,
		Raw:    tftypes.NewValue(schema.Type().TerraformType(ctx), nil),
	}}
}

func testAPI(t *testing.T, host string) *API {
	t.Helper()
	api, err := NewAPI(vergeio.NewClient(host, "user", "pass", true))
	if err != nil {
		t.Fatal(err)
	}
	api.webhookTries = 2
	api.webhookWait = 0
	api.putTries = 3
	api.putBackoff = 0
	return api
}

type settingRecord struct {
	Key          string
	Value        string
	DefaultValue string
	Description  string
	Dollar       int
}

type recordedPut struct {
	path string
	body string
}

type fakeCertBody struct {
	vergeos.Certificate
	DomainName string `json:"domainname,omitempty"`
}

type platformFake struct {
	mu               sync.Mutex
	next             int
	certs            map[int]vergeos.Certificate
	urls             map[int]vergeos.WebhookURL
	hooks            map[int]*vergeos.Webhook
	settings         map[string]*settingRecord
	puts             []recordedPut
	deletes          []string
	privateReads     int
	domainNames      map[int]string
	certFieldQueries []string
	failPuts         int
	failPutStatus    int
	putAttempts      int
	certCreateBody   string
	certUpdateBody   string
	urlCreateBody    string
	urlUpdateBody    string
}

func newPlatformFake() *platformFake {
	return &platformFake{
		next:  1,
		certs: map[int]vergeos.Certificate{},
		urls:  map[int]vergeos.WebhookURL{},
		hooks: map[int]*vergeos.Webhook{},
		settings: map[string]*settingRecord{
			"max_connections": {Key: "max_connections", Value: "100", DefaultValue: "100", Description: "Maximum concurrent connections", Dollar: 7},
			"cloud_name":      {Key: "cloud_name", Value: "lab", DefaultValue: "verge", Description: "Cloud name", Dollar: 8},
		},
	}
}

func (f *platformFake) seedCertificate(cert vergeos.Certificate) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.next
	f.next++
	cert.Key = vergeos.FlexInt(id)
	if cert.Public == "" {
		cert.Public = "-----BEGIN CERTIFICATE-----\nfake\n-----END CERTIFICATE-----\n"
	}
	f.certs[id] = cert
	return id
}

func (f *platformFake) setDomainName(id int, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.domainNames == nil {
		f.domainNames = map[int]string{}
	}
	f.domainNames[id] = name
}

func (f *platformFake) fieldQueries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.certFieldQueries...)
}

func (f *platformFake) seedURL(name, rawURL string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := f.next
	f.next++
	f.urls[id] = vergeos.WebhookURL{
		Key:               vergeos.FlexInt(id),
		Name:              name,
		URL:               rawURL,
		Type:              "custom",
		AuthorizationType: vergeos.WebhookAuthNone,
		Timeout:           5,
		Retries:           3,
	}
}

func (f *platformFake) certsByDomain(domain string) (vergeos.Certificate, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, cert := range f.certs {
		if cert.Domain == domain {
			return cert, true
		}
	}
	return vergeos.Certificate{}, false
}

func (f *platformFake) urlByName(name string) (vergeos.WebhookURL, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, item := range f.urls {
		if item.Name == name {
			return item, true
		}
	}
	return vergeos.WebhookURL{}, false
}

func (f *platformFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if vergeio.AnswerCredentialCheck(w, r) {
		return
	}
	if r.URL.Path == "/version.json" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"26.0.0"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/certificates":
		f.createCertificate(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/certificates":
		f.listCertificates(w)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/certificates/"):
		f.getCertificate(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/certificates/"):
		f.updateCertificate(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/certificates/"):
		f.deleteCertificate(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/v4/webhook_urls":
		f.createURL(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/webhook_urls":
		f.listURLs(w)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/send"):
		f.sendWebhook(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/webhook_urls/"):
		f.getURL(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/webhook_urls/"):
		f.updateURL(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/webhook_urls/"):
		f.deleteURL(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/webhooks":
		f.listWebhooks(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v4/webhooks/"):
		f.getWebhook(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/webhooks/"):
		f.deleteWebhook(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v4/settings":
		f.listSettings(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/v4/settings/"):
		f.putSetting(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/v4/settings/"):
		f.mu.Lock()
		f.deletes = append(f.deletes, r.URL.Path)
		f.mu.Unlock()
		http.Error(w, `{"err":"settings are not deleted"}`, http.StatusInternalServerError)
	default:
		http.NotFound(w, r)
	}
}

func (f *platformFake) createCertificate(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.CertificateCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	id := f.next
	f.next++
	domain := req.DomainName
	if domain == "" {
		domain = "generated.example"
	}
	f.certs[id] = vergeos.Certificate{
		Key:         vergeos.FlexInt(id),
		Domain:      domain,
		DomainList:  req.DomainList,
		Description: req.Description,
		Type:        req.Type,
		Public:      req.Public,
		Private:     req.Private,
		Chain:       req.Chain,
		ACMEServer:  req.ACMEServer,
		EABKid:      req.EABKid,
		KeyType:     req.KeyType,
		RSAKeySize:  req.RSAKeySize,
		AgreeTOS:    req.AgreeTOS != nil && *req.AgreeTOS,
		Valid:       true,
		Created:     1700000000,
		Modified:    1700000000,
	}
	if req.Contact != nil {
		f.certs[id] = withContact(f.certs[id], *req.Contact)
	}
	f.certCreateBody = string(body)
	f.mu.Unlock()
	writeJSON(w, map[string]int{"$key": id})
}

func withContact(cert vergeos.Certificate, contact int) vergeos.Certificate {
	cert.Contact = vergeos.FlexInt(contact)
	return cert
}

func (f *platformFake) listCertificates(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]vergeos.Certificate, 0, len(f.certs))
	for _, cert := range f.certs {
		out = append(out, cert)
	}
	writeJSON(w, out)
}

func (f *platformFake) getCertificate(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	fields := r.URL.Query().Get("fields")
	f.certFieldQueries = append(f.certFieldQueries, fields)
	if strings.Contains(fields, "private") {
		f.privateReads++
	} else {
		cert.Private = ""
	}
	writeJSON(w, fakeCertBody{
		Certificate: cert,
		DomainName:  f.domainNames[int(cert.Key)],
	})
}

func (f *platformFake) updateCertificate(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.CertificateUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cert, ok := f.certs[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if req.Description != nil {
		cert.Description = *req.Description
	}
	if req.Public != nil {
		cert.Public = *req.Public
	}
	if req.Private != nil {
		cert.Private = *req.Private
	}
	f.certs[cert.Key.Int()] = cert
	f.certUpdateBody = string(body)
	w.WriteHeader(http.StatusNoContent)
}

func (f *platformFake) deleteCertificate(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	if _, ok := f.certs[id]; !ok {
		http.NotFound(w, r)
		return
	}
	delete(f.certs, id)
	w.WriteHeader(http.StatusNoContent)
}

func (f *platformFake) createURL(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.WebhookURLCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	id := f.next
	f.next++
	timeout := 5
	if req.Timeout != nil {
		timeout = *req.Timeout
	}
	retries := 3
	if req.Retries != nil {
		retries = *req.Retries
	}
	kind := req.Type
	if kind == "" {
		kind = "custom"
	}
	auth := req.AuthorizationType
	if auth == "" {
		auth = vergeos.WebhookAuthNone
	}
	f.urls[id] = vergeos.WebhookURL{
		Key:               vergeos.FlexInt(id),
		Name:              req.Name,
		URL:               req.URL,
		Type:              kind,
		Headers:           req.Headers,
		AuthorizationType: auth,
		AllowInsecure:     req.AllowInsecure != nil && *req.AllowInsecure,
		Timeout:           timeout,
		Retries:           retries,
	}
	f.urlCreateBody = string(body)
	f.mu.Unlock()
	writeJSON(w, map[string]int{"$key": id})
}

func (f *platformFake) listURLs(w http.ResponseWriter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]vergeos.WebhookURL, 0, len(f.urls))
	for _, item := range f.urls {
		out = append(out, item)
	}
	writeJSON(w, out)
}

func (f *platformFake) getURL(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.urls[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, item)
}

func (f *platformFake) updateURL(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.WebhookURLUpdateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.urls[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	if req.Name != nil {
		item.Name = *req.Name
	}
	if req.URL != nil {
		item.URL = *req.URL
	}
	if req.Timeout != nil {
		item.Timeout = *req.Timeout
	}
	if req.Retries != nil {
		item.Retries = *req.Retries
	}
	if req.Headers != nil {
		item.Headers = *req.Headers
	}
	if req.AuthorizationType != nil {
		item.AuthorizationType = *req.AuthorizationType
	}
	if req.AllowInsecure != nil {
		item.AllowInsecure = *req.AllowInsecure
	}
	f.urls[item.Key.Int()] = item
	f.urlUpdateBody = string(body)
	w.WriteHeader(http.StatusNoContent)
}

func (f *platformFake) deleteURL(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	if _, ok := f.urls[id]; !ok {
		http.NotFound(w, r)
		return
	}
	delete(f.urls, id)
	w.WriteHeader(http.StatusNoContent)
}

func (f *platformFake) sendWebhook(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req vergeos.WebhookSendRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 2 {
		http.NotFound(w, r)
		return
	}
	urlID, _ := strconv.Atoi(parts[len(parts)-2])
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.urls[urlID]; !ok {
		http.NotFound(w, r)
		return
	}
	id := f.next
	f.next++
	f.hooks[id] = &vergeos.Webhook{
		Key:        vergeos.FlexInt(id),
		WebhookURL: vergeos.FlexInt(urlID),
		Message:    req.Message,
		Status:     vergeos.WebhookStatusQueued,
		Created:    1700000001,
	}
	w.WriteHeader(http.StatusNoContent)
}

func (f *platformFake) listWebhooks(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	filter := r.URL.Query().Get("filter")
	out := make([]vergeos.Webhook, 0, len(f.hooks))
	for _, item := range f.hooks {
		if filter != "" && !strings.Contains(filter, strconv.Itoa(item.WebhookURL.Int())) {
			continue
		}
		out = append(out, *item)
	}
	writeJSON(w, out)
}

func (f *platformFake) getWebhook(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	item, ok := f.hooks[pathID(r.URL.Path)]
	if !ok {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, item)
}

func (f *platformFake) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := pathID(r.URL.Path)
	if _, ok := f.hooks[id]; !ok {
		http.NotFound(w, r)
		return
	}
	delete(f.hooks, id)
	w.WriteHeader(http.StatusNoContent)
}

func (f *platformFake) listSettings(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	filter := r.URL.Query().Get("filter")
	var out []map[string]any
	for _, item := range f.settings {
		if filter != "" && !strings.Contains(filter, "'"+item.Key+"'") {
			continue
		}
		out = append(out, map[string]any{
			"$key":          item.Dollar,
			"key":           item.Key,
			"value":         item.Value,
			"default_value": item.DefaultValue,
			"description":   item.Description,
		})
	}
	writeJSON(w, out)
}

func (f *platformFake) putSetting(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	var req settingValueBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/v4/settings/")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putAttempts++
	if f.failPuts > 0 {
		f.failPuts--
		code := f.failPutStatus
		if code == 0 {
			code = http.StatusServiceUnavailable
		}
		http.Error(w, `{"err":"temporary"}`, code)
		return
	}
	var matched *settingRecord
	for _, item := range f.settings {
		if strconv.Itoa(item.Dollar) == id || item.Key == id {
			matched = item
			break
		}
	}
	if matched == nil {
		http.NotFound(w, r)
		return
	}
	matched.Value = req.Value
	f.puts = append(f.puts, recordedPut{path: r.URL.Path, body: string(body)})
	w.WriteHeader(http.StatusNoContent)
}

func readBody(r *http.Request) []byte {
	if r.Body == nil {
		return nil
	}
	defer func() { _ = r.Body.Close() }()
	buf, _ := io.ReadAll(r.Body)
	return buf
}

func writeJSON(w http.ResponseWriter, payload any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func pathID(path string) int {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	id, _ := strconv.Atoi(parts[len(parts)-1])
	return id
}
