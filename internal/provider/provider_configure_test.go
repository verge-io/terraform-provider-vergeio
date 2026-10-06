package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestProviderSchemaAttributesOptional(t *testing.T) {
	ctx := context.Background()
	p := &vergeioProvider{version: "test"}
	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}

	want := []string{"host", "username", "password", "api_key", "insecure", "timeout"}
	if len(schemaResp.Schema.Attributes) != len(want) {
		t.Fatalf("attributes = %d, want %d", len(schemaResp.Schema.Attributes), len(want))
	}
	for _, name := range want {
		attr, ok := schemaResp.Schema.Attributes[name]
		if !ok {
			t.Fatalf("missing attribute %s", name)
		}
		if !attr.IsOptional() || attr.IsRequired() {
			t.Fatalf("%s optional=%v required=%v", name, attr.IsOptional(), attr.IsRequired())
		}
	}
	if !schemaResp.Schema.Attributes["password"].IsSensitive() || !schemaResp.Schema.Attributes["api_key"].IsSensitive() {
		t.Fatal("password and api_key must be sensitive")
	}
}

func TestConfigureBuildsHTTPClient(t *testing.T) {
	clearProviderEnv(t)
	client, diags := configureProvider(t, map[string]tftypes.Value{
		"host":     tftypes.NewValue(tftypes.String, "example.test"),
		"username": tftypes.NewValue(tftypes.String, "api-user"),
		"password": tftypes.NewValue(tftypes.String, "api-pass"),
		"insecure": tftypes.NewValue(tftypes.Bool, true),
	})
	if diags.HasError() {
		t.Fatalf("configure diagnostics: %v", diags)
	}
	if client.Timeout() != vergeio.DefaultTimeout || client.Timeout() <= 0 {
		t.Fatalf("Timeout() = %s, want %s", client.Timeout(), vergeio.DefaultTimeout)
	}
	if client.Host != "example.test" || client.Username != "api-user" || client.Password != "api-pass" || client.APIKey != "" || !client.Insecure {
		t.Fatalf("client = host %q user %q api_key set %v insecure %v", client.Host, client.Username, client.APIKey != "", client.Insecure)
	}
	if client.FieldCache == nil {
		t.Fatal("field cache was not initialized")
	}
}

func TestConfigureUsesAPIKeyInsteadOfPassword(t *testing.T) {
	clearProviderEnv(t)
	client, diags := configureProvider(t, map[string]tftypes.Value{
		"host":     tftypes.NewValue(tftypes.String, "example.test"),
		"username": tftypes.NewValue(tftypes.String, "api-user"),
		"password": tftypes.NewValue(tftypes.String, "api-pass"),
		"api_key":  tftypes.NewValue(tftypes.String, "token-value"),
	})
	if diags.HasError() {
		t.Fatalf("configure diagnostics: %v", diags)
	}
	if client.APIKey != "token-value" || client.Username != "api-user" || client.Password != "api-pass" {
		t.Fatalf("client api_key %q user %q", client.APIKey, client.Username)
	}
}

func TestConfigureAPIKeyDoesNotRequirePassword(t *testing.T) {
	clearProviderEnv(t)
	client, diags := configureProvider(t, map[string]tftypes.Value{
		"host":    tftypes.NewValue(tftypes.String, "example.test"),
		"api_key": tftypes.NewValue(tftypes.String, "token-value"),
	})
	if diags.HasError() {
		t.Fatalf("configure diagnostics: %v", diags)
	}
	if client.APIKey != "token-value" || client.Username != "" || client.Password != "" {
		t.Fatalf("client = %+v", client)
	}
}

func TestConfigureEnvironmentFallback(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envHost, "env.example.test")
	t.Setenv(envUsername, "env-user")
	t.Setenv(envPassword, "env-pass")
	t.Setenv(envInsecure, "true")
	t.Setenv(envTimeout, "15")

	client, diags := configureProvider(t, nil)
	if diags.HasError() {
		t.Fatalf("configure diagnostics: %v", diags)
	}
	if client.Host != "env.example.test" || client.Username != "env-user" || client.Password != "env-pass" || !client.Insecure {
		t.Fatalf("client = host %q user %q insecure %v", client.Host, client.Username, client.Insecure)
	}
	if client.Timeout() != 15*time.Second {
		t.Fatalf("Timeout() = %s, want 15s", client.Timeout())
	}
}

func TestConfigureEnvironmentAPIKeyAndVerifySSL(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envHost, "env.example.test")
	t.Setenv(envUsername, "env-user")
	t.Setenv(envPassword, "env-pass")
	t.Setenv(envAPIKey, "env-token")
	t.Setenv(envVerifySSL, "false")

	client, diags := configureProvider(t, nil)
	if diags.HasError() {
		t.Fatalf("configure diagnostics: %v", diags)
	}
	if client.APIKey != "env-token" || !client.Insecure {
		t.Fatalf("api_key set %v insecure %v", client.APIKey != "", client.Insecure)
	}
}

func TestConfigureProviderBlockWinsOverEnvironment(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envHost, "env.example.test")
	t.Setenv(envUsername, "env-user")
	t.Setenv(envPassword, "env-pass")
	t.Setenv(envAPIKey, "env-token")
	t.Setenv(envInsecure, "true")
	t.Setenv(envTimeout, "90")

	client, diags := configureProvider(t, map[string]tftypes.Value{
		"host":     tftypes.NewValue(tftypes.String, "hcl.example.test"),
		"username": tftypes.NewValue(tftypes.String, "hcl-user"),
		"password": tftypes.NewValue(tftypes.String, "hcl-pass"),
		"api_key":  tftypes.NewValue(tftypes.String, "hcl-token"),
		"insecure": tftypes.NewValue(tftypes.Bool, false),
		"timeout":  tftypes.NewValue(tftypes.Number, 12),
	})
	if diags.HasError() {
		t.Fatalf("configure diagnostics: %v", diags)
	}
	if client.Host != "hcl.example.test" || client.Username != "hcl-user" || client.Password != "hcl-pass" || client.APIKey != "hcl-token" || client.Insecure {
		t.Fatalf("client = host %q user %q insecure %v", client.Host, client.Username, client.Insecure)
	}
	if client.Timeout() != 12*time.Second {
		t.Fatalf("Timeout() = %s, want 12s", client.Timeout())
	}
}

func TestConfigureEmptyAPIKeyDoesNotUseEnvironment(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envAPIKey, "env-token")

	client, diags := configureProvider(t, map[string]tftypes.Value{
		"host":    tftypes.NewValue(tftypes.String, "example.test"),
		"api_key": tftypes.NewValue(tftypes.String, ""),
	})
	if client != nil {
		t.Fatal("an empty api_key used VERGEOS_API_KEY")
	}
	assertSingleMissing(t, diags, "api_key or username and password")
}

func TestConfigureMixesProviderAndEnvironment(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envPassword, "env-pass")

	client, diags := configureProvider(t, map[string]tftypes.Value{
		"host":     tftypes.NewValue(tftypes.String, "example.test"),
		"username": tftypes.NewValue(tftypes.String, "hcl-user"),
	})
	if diags.HasError() {
		t.Fatalf("configure diagnostics: %v", diags)
	}
	if client.Username != "hcl-user" || client.Password != "env-pass" {
		t.Fatalf("user %q password set %v", client.Username, client.Password != "")
	}
}

func TestConfigureMissingValuesOneMessage(t *testing.T) {
	clearProviderEnv(t)
	client, diags := configureProvider(t, nil)
	if client != nil {
		t.Fatal("client was created without configuration")
	}
	assertSingleMissing(t, diags, "host", "api_key or username and password")
	text := diagText(diags)
	if strings.Contains(strings.ToLower(text), "401") || strings.Contains(strings.ToLower(text), "unauthorized") {
		t.Fatalf("missing configuration reported an auth error: %s", text)
	}
}

func TestConfigureMissingPasswordNamesIt(t *testing.T) {
	clearProviderEnv(t)
	_, diags := configureProvider(t, map[string]tftypes.Value{
		"host":     tftypes.NewValue(tftypes.String, "example.test"),
		"username": tftypes.NewValue(tftypes.String, "api-user"),
	})
	assertSingleMissing(t, diags, "password or api_key")
	text := diagText(diags)
	if strings.Contains(text, "host") {
		t.Fatalf("host was reported missing: %s", text)
	}
}

func TestConfigureUnknownValueDoesNotReadEnvironment(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envHost, "env.example.test")
	t.Setenv(envAPIKey, "env-token")

	client, diags := configureProvider(t, map[string]tftypes.Value{
		"host": tftypes.NewValue(tftypes.String, tftypes.UnknownValue),
	})
	if client != nil {
		t.Fatal("client was created with an unknown host")
	}
	if errorCount(diags) != 1 {
		t.Fatalf("errors = %d, want 1: %s", errorCount(diags), diagText(diags))
	}
	text := diagText(diags)
	if !strings.Contains(text, "host") || !strings.Contains(text, "Unknown") {
		t.Fatalf("diagnostic = %s", text)
	}
	if strings.Contains(text, "Missing required") {
		t.Fatalf("unknown host was also reported missing: %s", text)
	}
}

func TestConfigureTLSEnvironment(t *testing.T) {
	t.Run("insecure true", func(t *testing.T) {
		clearProviderEnv(t)
		t.Setenv(envHost, "example.test")
		t.Setenv(envAPIKey, "token")
		t.Setenv(envInsecure, "yes")
		client, diags := configureProvider(t, nil)
		if diags.HasError() {
			t.Fatal(diags)
		}
		if !client.Insecure {
			t.Fatal("VERGEOS_INSECURE=yes left verification enabled")
		}
	})

	t.Run("verify ssl true", func(t *testing.T) {
		clearProviderEnv(t)
		t.Setenv(envHost, "example.test")
		t.Setenv(envAPIKey, "token")
		t.Setenv(envVerifySSL, "true")
		client, diags := configureProvider(t, nil)
		if diags.HasError() {
			t.Fatal(diags)
		}
		if client.Insecure {
			t.Fatal("VERGEOS_VERIFY_SSL=true skipped verification")
		}
	})

	t.Run("agree", func(t *testing.T) {
		clearProviderEnv(t)
		t.Setenv(envHost, "example.test")
		t.Setenv(envAPIKey, "token")
		t.Setenv(envInsecure, "true")
		t.Setenv(envVerifySSL, "false")
		client, diags := configureProvider(t, nil)
		if diags.HasError() {
			t.Fatal(diags)
		}
		if !client.Insecure {
			t.Fatal("matching TLS environment variables were not applied")
		}
	})

	t.Run("provider wins over disagreeing environment", func(t *testing.T) {
		clearProviderEnv(t)
		t.Setenv(envInsecure, "true")
		t.Setenv(envVerifySSL, "true")
		client, diags := configureProvider(t, map[string]tftypes.Value{
			"host":     tftypes.NewValue(tftypes.String, "example.test"),
			"api_key":  tftypes.NewValue(tftypes.String, "token"),
			"insecure": tftypes.NewValue(tftypes.Bool, false),
		})
		if diags.HasError() {
			t.Fatalf("configure diagnostics: %s", diagText(diags))
		}
		if client.Insecure {
			t.Fatal("provider insecure=false did not win")
		}
	})
}

func TestConfigureRejectsDisagreeingTLSEnvironment(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envHost, "example.test")
	t.Setenv(envAPIKey, "token")
	t.Setenv(envInsecure, "true")
	t.Setenv(envVerifySSL, "true")

	client, diags := configureProvider(t, nil)
	if client != nil {
		t.Fatal("client was created with disagreeing TLS settings")
	}
	text := diagText(diags)
	if errorCount(diags) != 1 || !strings.Contains(text, envInsecure) || !strings.Contains(text, envVerifySSL) {
		t.Fatalf("diagnostic = %s", text)
	}
}

func TestConfigureRejectsInvalidTimeout(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envHost, "example.test")
	t.Setenv(envAPIKey, "token")

	t.Run("environment", func(t *testing.T) {
		t.Setenv(envTimeout, "15s")
		client, diags := configureProvider(t, nil)
		if client != nil {
			t.Fatal("client was created")
		}
		text := diagText(diags)
		if errorCount(diags) != 1 || !strings.Contains(text, envTimeout) || !strings.Contains(text, "15s") {
			t.Fatalf("diagnostic = %s", text)
		}
	})

	t.Run("provider", func(t *testing.T) {
		client, diags := configureProvider(t, map[string]tftypes.Value{
			"timeout": tftypes.NewValue(tftypes.Number, 0),
		})
		if client != nil {
			t.Fatal("client was created")
		}
		text := diagText(diags)
		if errorCount(diags) != 1 || !strings.Contains(text, "timeout") {
			t.Fatalf("diagnostic = %s", text)
		}
	})
}

func TestConfigureRejectsInvalidBoolEnvironment(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(envHost, "example.test")
	t.Setenv(envAPIKey, "token")
	t.Setenv(envInsecure, "maybe")

	_, diags := configureProvider(t, nil)
	text := diagText(diags)
	if errorCount(diags) != 1 || !strings.Contains(text, envInsecure) || !strings.Contains(text, "maybe") {
		t.Fatalf("diagnostic = %s", text)
	}
}

func clearProviderEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{envHost, envUsername, envPassword, envAPIKey, envInsecure, envVerifySSL, envTimeout} {
		t.Setenv(name, "")
	}
}

func configureProvider(t *testing.T, values map[string]tftypes.Value) (*vergeio.Client, diag.Diagnostics) {
	t.Helper()
	ctx := context.Background()
	p := &vergeioProvider{version: "test"}
	resp := &provider.ConfigureResponse{}
	p.Configure(ctx, provider.ConfigureRequest{
		Config: configFromMap(t, p, values),
	}, resp)
	client, _ := resp.ResourceData.(*vergeio.Client)
	if resp.DataSourceData != nil && resp.DataSourceData != client {
		t.Fatal("data sources and resources were given different clients")
	}
	if resp.EphemeralResourceData != nil && resp.EphemeralResourceData != client {
		t.Fatal("ephemeral resources were given a different client")
	}
	return client, resp.Diagnostics
}

func configFromMap(t *testing.T, p *vergeioProvider, values map[string]tftypes.Value) tfsdk.Config {
	t.Helper()
	ctx := context.Background()
	schemaResp := &provider.SchemaResponse{}
	p.Schema(ctx, provider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema diagnostics: %v", schemaResp.Diagnostics)
	}
	tfType := schemaResp.Schema.Type().TerraformType(ctx)
	obj, ok := tfType.(tftypes.Object)
	if !ok {
		t.Fatalf("schema type %T", tfType)
	}
	attrs := make(map[string]tftypes.Value, len(obj.AttributeTypes))
	for name, attrType := range obj.AttributeTypes {
		if v, exists := values[name]; exists {
			attrs[name] = v
			continue
		}
		attrs[name] = tftypes.NewValue(attrType, nil)
	}
	return tfsdk.Config{
		Raw:    tftypes.NewValue(tfType, attrs),
		Schema: schemaResp.Schema,
	}
}

func assertSingleMissing(t *testing.T, diags diag.Diagnostics, names ...string) {
	t.Helper()
	if errorCount(diags) != 1 {
		t.Fatalf("errors = %d, want 1: %s", errorCount(diags), diagText(diags))
	}
	text := diagText(diags)
	if !strings.Contains(text, "Missing required provider configuration") {
		t.Fatalf("diagnostic = %s", text)
	}
	for _, name := range names {
		if !strings.Contains(text, name) {
			t.Fatalf("diagnostic %q does not name %q", text, name)
		}
	}
}

func diagText(diags diag.Diagnostics) string {
	var b strings.Builder
	for _, d := range diags {
		b.WriteString(d.Summary())
		b.WriteString("\n")
		b.WriteString(d.Detail())
		b.WriteString("\n")
	}
	return b.String()
}

func errorCount(diags diag.Diagnostics) int {
	n := 0
	for _, d := range diags {
		if d.Severity() == diag.SeverityError {
			n++
		}
	}
	return n
}
