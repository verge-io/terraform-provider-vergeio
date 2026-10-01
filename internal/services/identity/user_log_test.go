package identity

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-log/tfsdklog"

	"terraform-provider-vergeio/internal/client"
)

// TestUserCreateDoesNotLogPassword runs a user create with a known password
// and fails if that password shows up in the captured debug log.
func TestUserCreateDoesNotLogPassword(t *testing.T) {
	const (
		userPassword = "user-secret-9f3c1e7a"
		providerPass = "provider-login-7a1c5e9d"
	)

	var sent []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vergeio.AnswerCredentialCheck(w, r) {
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, "read", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		var response []byte
		switch {
		case r.URL.Path == "/version.json":
			response = []byte(`{"version":"26.0.0"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v4/users":
			sent = body
			w.WriteHeader(http.StatusOK)
			response = []byte(`{"$key":7}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v4/users/7":
			response = []byte(`{"$key":7,"name":"ada","enabled":true,"displayname":"Ada","email":"ada@example.com","type":"normal","change_password":false}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.RequestURI())
			http.Error(w, "unexpected", http.StatusNotFound)
			return
		}
		if _, err := w.Write(response); err != nil {
			t.Errorf("write response: %v", err)
		}
	}))
	defer server.Close()

	ctx, logs := captureProviderLog(t)

	userResource := &UserResource{}
	configure := &fwresource.ConfigureResponse{}
	userResource.Configure(ctx, fwresource.ConfigureRequest{
		ProviderData: vergeio.NewClient(server.URL, "api-user", providerPass, true),
	}, configure)
	if configure.Diagnostics.HasError() {
		t.Fatalf("configure: %v", configure.Diagnostics)
	}

	schemaResp := &fwresource.SchemaResponse{}
	userResource.Schema(ctx, fwresource.SchemaRequest{}, schemaResp)

	plan := tfsdk.Plan{Schema: schemaResp.Schema}
	diags := plan.Set(ctx, &UserResourceModel{
		Id:             types.StringNull(),
		AuthSource:     types.Int32Null(),
		Name:           types.StringValue("ada"),
		RemoteName:     types.StringNull(),
		Enabled:        types.BoolValue(true),
		DisplayName:    types.StringValue("Ada"),
		Email:          types.StringValue("ada@example.com"),
		Type:           types.StringValue("normal"),
		Password:       types.StringValue(userPassword),
		ChangePassword: types.BoolValue(false),
	})
	if diags.HasError() {
		t.Fatalf("plan: %v", diags)
	}

	resp := &fwresource.CreateResponse{
		State: tfsdk.State{Schema: schemaResp.Schema},
	}
	userResource.Create(ctx, fwresource.CreateRequest{Plan: plan}, resp)

	got := logs()
	if strings.Contains(got, userPassword) || strings.Contains(got, providerPass) {
		t.Fatalf("secret leaked into debug log:\n%s", got)
	}
	if !strings.Contains(got, "Created a user with Id") {
		t.Fatalf("create debug log was not captured:\n%s", got)
	}
	if resp.Diagnostics.HasError() {
		t.Fatalf("create: %v", resp.Diagnostics)
	}
	if !strings.Contains(string(sent), userPassword) {
		t.Fatal("create request dropped the password; the API body must still send it")
	}
}

// captureProviderLog records tflog output and the standard logger.
// tflog writes to the stderr value captured when the provider logger is created.
func captureProviderLog(t *testing.T) (context.Context, func() string) {
	t.Helper()

	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStderr := os.Stderr
	os.Stderr = writer
	ctx := tfsdklog.NewRootProviderLogger(context.Background(),
		tflog.WithLevel(hclog.Trace),
		tflog.WithoutLocation(),
	)
	os.Stderr = origStderr

	var std bytes.Buffer
	origLog := log.Writer()
	log.SetOutput(&std)
	t.Cleanup(func() {
		log.SetOutput(origLog)
		closePipe(t, writer)
		closePipe(t, reader)
	})

	return ctx, func() string {
		closePipe(t, writer)
		out, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("read provider log: %v", err)
		}
		return string(out) + std.String()
	}
}

func closePipe(t *testing.T, c io.Closer) {
	t.Helper()
	if err := c.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Errorf("close log pipe: %v", err)
	}
}
