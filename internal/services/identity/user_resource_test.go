package identity

import (
	"context"
	"testing"

	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-vergeio/internal/client"
)

// Unit Tests
func TestUserResource(t *testing.T) {
	userResource := NewUserResource()
	if userResource == nil {
		t.Fatal("user resource should not be nil")
	}
}

func TestUserResource_Metadata(t *testing.T) {
	userResource := NewUserResource()
	req := fwresource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &fwresource.MetadataResponse{}

	userResource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_user" {
		t.Errorf("expected TypeName 'vergeio_user', got '%s'", resp.TypeName)
	}
}

func TestUserResource_Schema(t *testing.T) {
	userResource := NewUserResource()
	req := fwresource.SchemaRequest{}
	resp := &fwresource.SchemaResponse{}

	userResource.Schema(context.Background(), req, resp)

	// Check that required attributes exist
	if resp.Schema.Attributes == nil {
		t.Fatal("schema attributes should not be nil")
	}

	// Check required name attribute
	if nameAttr, ok := resp.Schema.Attributes["name"]; !ok {
		t.Error("name attribute should exist")
	} else if !nameAttr.IsRequired() {
		t.Error("name should be required")
	}

	// Check computed id attribute
	if idAttr, ok := resp.Schema.Attributes["id"]; !ok {
		t.Error("id attribute should exist")
	} else if !idAttr.IsComputed() {
		t.Error("id should be computed")
	}

	// Check optional enabled attribute
	if enabledAttr, ok := resp.Schema.Attributes["enabled"]; !ok {
		t.Error("enabled attribute should exist")
	} else if !enabledAttr.IsOptional() {
		t.Error("enabled should be optional")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "User resource in VergeIO" {
		t.Errorf("expected description 'User resource in VergeIO', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestUserResource_Configure_WithValidClient(t *testing.T) {
	userResource := &UserResource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)

	req := fwresource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &fwresource.ConfigureResponse{}

	userResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if userResource.userApi == nil {
		t.Error("userApi should be configured")
	}

	if userResource.userApi.Name() != "User Api" {
		t.Errorf("expected userApi name 'User Api', got '%s'", userResource.userApi.Name())
	}
}

func TestUserResource_Configure_WithInvalidClient(t *testing.T) {
	userResource := &UserResource{}

	req := fwresource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &fwresource.ConfigureResponse{}

	userResource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if userResource.userApi != nil {
		t.Error("userApi should not be configured with invalid client")
	}
}

func TestUserResource_Configure_WithNilClient(t *testing.T) {
	userResource := &UserResource{}

	req := fwresource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &fwresource.ConfigureResponse{}

	userResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if userResource.userApi != nil {
		t.Error("userApi should not be configured with nil client")
	}
}

func TestUserResourceModel_Types(t *testing.T) {
	model := &UserResourceModel{
		Id:             types.StringValue("123"),
		AuthSource:     types.Int32Value(1),
		Name:           types.StringValue("testuser"),
		RemoteName:     types.StringValue("remote-testuser"),
		Enabled:        types.BoolValue(true),
		DisplayName:    types.StringValue("Test User"),
		Email:          types.StringValue("test@example.com"),
		Type:           types.StringValue("local"),
		Password:       types.StringValue("secretpassword"),
		ChangePassword: types.BoolValue(false),
	}

	if model.Id.ValueString() != "123" {
		t.Errorf("expected Id '123', got '%s'", model.Id.ValueString())
	}

	if model.Name.ValueString() != "testuser" {
		t.Errorf("expected Name 'testuser', got '%s'", model.Name.ValueString())
	}

	if model.AuthSource.ValueInt32() != 1 {
		t.Errorf("expected AuthSource 1, got %d", model.AuthSource.ValueInt32())
	}

	if !model.Enabled.ValueBool() {
		t.Error("expected Enabled to be true")
	}

	if model.DisplayName.ValueString() != "Test User" {
		t.Errorf("expected DisplayName 'Test User', got '%s'", model.DisplayName.ValueString())
	}

	if model.Email.ValueString() != "test@example.com" {
		t.Errorf("expected Email 'test@example.com', got '%s'", model.Email.ValueString())
	}
}

func TestUserResourceModel_NullValues(t *testing.T) {
	model := &UserResourceModel{
		Id:             types.StringNull(),
		AuthSource:     types.Int32Null(),
		Name:           types.StringValue("testuser"), // Required field
		RemoteName:     types.StringNull(),
		Enabled:        types.BoolNull(),
		DisplayName:    types.StringNull(),
		Email:          types.StringNull(),
		Type:           types.StringNull(),
		Password:       types.StringNull(),
		ChangePassword: types.BoolNull(),
	}

	if !model.Id.IsNull() {
		t.Error("expected Id to be null")
	}

	if !model.AuthSource.IsNull() {
		t.Error("expected AuthSource to be null")
	}

	if !model.RemoteName.IsNull() {
		t.Error("expected RemoteName to be null")
	}

	if !model.Enabled.IsNull() {
		t.Error("expected Enabled to be null")
	}

	// Name should not be null as it's required
	if model.Name.IsNull() {
		t.Error("Name should not be null")
	}
}
