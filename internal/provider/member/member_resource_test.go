package member

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	resschema "github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"terraform-provider-vergeio/internal/provider/vergeio"
)

// Unit Tests
func TestMemberResource(t *testing.T) {
	memberResource := NewMemberResource()
	if memberResource == nil {
		t.Fatal("member resource should not be nil")
	}
}

func TestMemberResource_Metadata(t *testing.T) {
	memberResource := NewMemberResource()
	req := fwresource.MetadataRequest{
		ProviderTypeName: "vergeio",
	}
	resp := &fwresource.MetadataResponse{}

	memberResource.Metadata(context.Background(), req, resp)

	if resp.TypeName != "vergeio_member" {
		t.Errorf("expected TypeName 'vergeio_member', got '%s'", resp.TypeName)
	}
}

func TestMemberResource_Schema(t *testing.T) {
	memberResource := NewMemberResource()
	req := fwresource.SchemaRequest{}
	resp := &fwresource.SchemaResponse{}

	memberResource.Schema(context.Background(), req, resp)

	// Check that required attributes exist
	if resp.Schema.Attributes == nil {
		t.Fatal("schema attributes should not be nil")
	}

	// Check computed id attribute
	if idAttr, ok := resp.Schema.Attributes["id"]; !ok {
		t.Error("id attribute should exist")
	} else if !idAttr.IsComputed() {
		t.Error("id should be computed")
	}

	// Check required group attribute
	if groupAttr, ok := resp.Schema.Attributes["group"]; !ok {
		t.Error("group attribute should exist")
	} else if !groupAttr.IsRequired() || groupAttr.IsOptional() || groupAttr.IsComputed() {
		t.Error("group should be required")
	}

	// Check required member attribute
	if memberAttr, ok := resp.Schema.Attributes["member"]; !ok {
		t.Error("member attribute should exist")
	} else if !memberAttr.IsRequired() || memberAttr.IsOptional() || memberAttr.IsComputed() {
		t.Error("member should be required")
	}

	// Check schema description
	if resp.Schema.MarkdownDescription != "Member or Vnet resource in VergeIO" {
		t.Errorf("expected description 'Member or Vnet resource in VergeIO', got '%s'", resp.Schema.MarkdownDescription)
	}
}

func TestMemberResource_Configure_WithValidClient(t *testing.T) {
	memberResource := &MemberResource{}
	client := vergeio.NewClient("test.example.com", "testuser", "testpass", true)

	req := fwresource.ConfigureRequest{
		ProviderData: client,
	}
	resp := &fwresource.ConfigureResponse{}

	memberResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors, got: %v", resp.Diagnostics.Errors())
	}

	if memberResource.memberApi == nil {
		t.Error("memberApi should be configured")
	}

	if memberResource.memberApi.Name() != "Member Api" {
		t.Errorf("expected memberApi name 'Member Api', got '%s'", memberResource.memberApi.Name())
	}
}

func TestMemberResource_Configure_WithInvalidClient(t *testing.T) {
	memberResource := &MemberResource{}

	req := fwresource.ConfigureRequest{
		ProviderData: "invalid",
	}
	resp := &fwresource.ConfigureResponse{}

	memberResource.Configure(context.Background(), req, resp)

	if !resp.Diagnostics.HasError() {
		t.Error("expected error for invalid client type")
	}

	if memberResource.memberApi != nil {
		t.Error("memberApi should not be configured with invalid client")
	}
}

func TestMemberResource_Configure_WithNilClient(t *testing.T) {
	memberResource := &MemberResource{}

	req := fwresource.ConfigureRequest{
		ProviderData: nil,
	}
	resp := &fwresource.ConfigureResponse{}

	memberResource.Configure(context.Background(), req, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("expected no errors with nil client, got: %v", resp.Diagnostics.Errors())
	}

	if memberResource.memberApi != nil {
		t.Error("memberApi should not be configured with nil client")
	}
}

func TestMemberResource_RequiresReplace(t *testing.T) {
	memberResource := NewMemberResource()
	resp := &fwresource.SchemaResponse{}
	memberResource.Schema(context.Background(), fwresource.SchemaRequest{}, resp)

	groupAttr, ok := resp.Schema.Attributes["group"].(resschema.Int32Attribute)
	if !ok {
		t.Fatal("group should be an int32 attribute")
	}
	if len(groupAttr.PlanModifiers) != 1 {
		t.Fatalf("group plan modifiers = %d, want 1", len(groupAttr.PlanModifiers))
	}
	wantGroup := int32planmodifier.RequiresReplace().Description(context.Background())
	if got := groupAttr.PlanModifiers[0].Description(context.Background()); got != wantGroup {
		t.Errorf("group plan modifier %q, want %q", got, wantGroup)
	}
	assertInt32RequiresReplace(t, "group", groupAttr.PlanModifiers[0], 1, 2)

	memberAttr, ok := resp.Schema.Attributes["member"].(resschema.StringAttribute)
	if !ok {
		t.Fatal("member should be a string attribute")
	}
	if len(memberAttr.PlanModifiers) != 1 {
		t.Fatalf("member plan modifiers = %d, want 1", len(memberAttr.PlanModifiers))
	}
	wantMember := stringplanmodifier.RequiresReplace().Description(context.Background())
	if got := memberAttr.PlanModifiers[0].Description(context.Background()); got != wantMember {
		t.Errorf("member plan modifier %q, want %q", got, wantMember)
	}
	assertStringRequiresReplace(t, "member", memberAttr.PlanModifiers[0], "users/1", "users/2")
}

func TestMemberResource_UpdateRejectsInPlaceChange(t *testing.T) {
	memberResource := &MemberResource{}
	resp := &fwresource.UpdateResponse{}

	memberResource.Update(context.Background(), fwresource.UpdateRequest{}, resp)

	if !resp.Diagnostics.HasError() {
		t.Fatal("in-place member update should be rejected")
	}
}

func assertInt32RequiresReplace(t *testing.T, name string, mod planmodifier.Int32, prior, next int32) {
	t.Helper()

	createResp := &planmodifier.Int32Response{PlanValue: types.Int32Value(next)}
	mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
		PlanValue:   types.Int32Value(next),
		ConfigValue: types.Int32Value(next),
		Plan:        tfsdk.Plan{Raw: knownPlanValue()},
	}, createResp)
	if createResp.RequiresReplace {
		t.Errorf("%s create planned a replacement", name)
	}

	sameResp := &planmodifier.Int32Response{PlanValue: types.Int32Value(prior)}
	mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
		StateValue:  types.Int32Value(prior),
		PlanValue:   types.Int32Value(prior),
		ConfigValue: types.Int32Value(prior),
		State:       tfsdk.State{Raw: knownPlanValue()},
		Plan:        tfsdk.Plan{Raw: knownPlanValue()},
	}, sameResp)
	if sameResp.RequiresReplace {
		t.Errorf("%s unchanged value planned a replacement", name)
	}

	changeResp := &planmodifier.Int32Response{PlanValue: types.Int32Value(next)}
	mod.PlanModifyInt32(context.Background(), planmodifier.Int32Request{
		StateValue:  types.Int32Value(prior),
		PlanValue:   types.Int32Value(next),
		ConfigValue: types.Int32Value(next),
		State:       tfsdk.State{Raw: knownPlanValue()},
		Plan:        tfsdk.Plan{Raw: knownPlanValue()},
	}, changeResp)
	if !changeResp.RequiresReplace {
		t.Errorf("%s changed value did not plan a replacement", name)
	}
	if changeResp.Diagnostics.HasError() {
		t.Errorf("%s plan diagnostics: %v", name, changeResp.Diagnostics)
	}
}

func assertStringRequiresReplace(t *testing.T, name string, mod planmodifier.String, prior, next string) {
	t.Helper()

	createResp := &planmodifier.StringResponse{PlanValue: types.StringValue(next)}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		PlanValue:   types.StringValue(next),
		ConfigValue: types.StringValue(next),
		Plan:        tfsdk.Plan{Raw: knownPlanValue()},
	}, createResp)
	if createResp.RequiresReplace {
		t.Errorf("%s create planned a replacement", name)
	}

	sameResp := &planmodifier.StringResponse{PlanValue: types.StringValue(prior)}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		StateValue:  types.StringValue(prior),
		PlanValue:   types.StringValue(prior),
		ConfigValue: types.StringValue(prior),
		State:       tfsdk.State{Raw: knownPlanValue()},
		Plan:        tfsdk.Plan{Raw: knownPlanValue()},
	}, sameResp)
	if sameResp.RequiresReplace {
		t.Errorf("%s unchanged value planned a replacement", name)
	}

	changeResp := &planmodifier.StringResponse{PlanValue: types.StringValue(next)}
	mod.PlanModifyString(context.Background(), planmodifier.StringRequest{
		StateValue:  types.StringValue(prior),
		PlanValue:   types.StringValue(next),
		ConfigValue: types.StringValue(next),
		State:       tfsdk.State{Raw: knownPlanValue()},
		Plan:        tfsdk.Plan{Raw: knownPlanValue()},
	}, changeResp)
	if !changeResp.RequiresReplace {
		t.Errorf("%s changed value did not plan a replacement", name)
	}
	if changeResp.Diagnostics.HasError() {
		t.Errorf("%s plan diagnostics: %v", name, changeResp.Diagnostics)
	}
}

// knownPlanValue is a non-null plan or state value. RequiresReplace treats a
// null raw value as create or destroy and does not mark a replacement.
func knownPlanValue() tftypes.Value {
	return tftypes.NewValue(tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}, map[string]tftypes.Value{})
}

func TestMemberResourceModel_Types(t *testing.T) {
	model := &MemberResourceModel{
		Id:     types.StringValue("123"),
		Group:  types.Int32Value(1),
		Member: types.StringValue("test-member"),
	}

	if model.Id.ValueString() != "123" {
		t.Errorf("expected Id '123', got '%s'", model.Id.ValueString())
	}

	if model.Group.ValueInt32() != 1 {
		t.Errorf("expected Group 1, got %d", model.Group.ValueInt32())
	}

	if model.Member.ValueString() != "test-member" {
		t.Errorf("expected Member 'test-member', got '%s'", model.Member.ValueString())
	}
}

func TestMemberResourceModel_NullValues(t *testing.T) {
	model := &MemberResourceModel{
		Id:     types.StringNull(),
		Group:  types.Int32Null(),
		Member: types.StringNull(),
	}

	if !model.Id.IsNull() {
		t.Error("expected Id to be null")
	}

	if !model.Group.IsNull() {
		t.Error("expected Group to be null")
	}

	if !model.Member.IsNull() {
		t.Error("expected Member to be null")
	}
}

// Acceptance Tests
func TestAccMemberResource_Basic(t *testing.T) {
	t.Skip("Member acceptance test temporarily disabled - API integration needs review")
	memberName := "tf-acc-test-member-basic"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMemberDestroy,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccMemberResourceConfig_basic(memberName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckMemberExists("vergeio_member.test"),
					resource.TestCheckResourceAttr("vergeio_member.test", "member", memberName),
					resource.TestCheckResourceAttrSet("vergeio_member.test", "id"),
					resource.TestCheckResourceAttrSet("vergeio_member.test", "group"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "vergeio_member.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccMemberResource_Update(t *testing.T) {
	t.Skip("Member acceptance test temporarily disabled - API integration needs review")
	memberName := "tf-acc-test-member-update"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckMemberDestroy,
		Steps: []resource.TestStep{
			// Create initial member
			{
				Config: testAccMemberResourceConfig_basic(memberName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckMemberExists("vergeio_member.test"),
					resource.TestCheckResourceAttr("vergeio_member.test", "member", memberName),
				),
			},
			// Changing member replaces the membership.
			{
				Config: testAccMemberResourceConfig_updated(memberName),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckMemberExists("vergeio_member.test"),
					resource.TestCheckResourceAttr("vergeio_member.test", "member", memberName+"-updated"),
				),
			},
		},
	})
}

// Helper functions for acceptance tests
func testAccPreCheck(t *testing.T) {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")

	if host == "" || username == "" || password == "" {
		t.Skip(`
Acceptance test skipped: VergeOS environment not configured.

To run acceptance tests, set these environment variables:
  export TF_ACC_VERGEIO_HOST="your-verge-host.com"
  export TF_ACC_VERGEIO_USERNAME="your-username"
  export TF_ACC_VERGEIO_PASSWORD="your-password"

Then run:
  TF_ACC=1 go test ./internal/provider/member -v -run=TestAccMember
`)
	}

	t.Logf("Acceptance test will connect to VergeOS at: %s (user: %s)", host, username)
}

func testAccCheckMemberExists(resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found: %s", resourceName)
		}

		if rs.Primary.ID == "" {
			return fmt.Errorf("no ID is set")
		}

		// In a real implementation, you would verify the member exists via API call
		// For now, we just verify that we have an ID
		return nil
	}
}

func testAccCheckMemberDestroy(s *terraform.State) error {
	// Check that all members with tf-acc-test prefix are destroyed
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "vergeio_member" {
			continue
		}

		// In a real implementation, you would verify the member no longer exists via API call
		// For now, we assume the destroy worked if no error occurred during test
	}

	return nil
}

// testProvider creates a minimal provider for testing member resource
type testProvider struct{}

func (p *testProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vergeio"
}

func (p *testProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: map[string]schema.Attribute{
			"host":     schema.StringAttribute{Optional: true},
			"username": schema.StringAttribute{Optional: true},
			"password": schema.StringAttribute{Optional: true, Sensitive: true},
			"insecure": schema.BoolAttribute{Optional: true},
		},
	}
}

func (p *testProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var data struct {
		Host     types.String `tfsdk:"host"`
		Username types.String `tfsdk:"username"`
		Password types.String `tfsdk:"password"`
		Insecure types.Bool   `tfsdk:"insecure"`
	}

	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Create client with configuration
	client := vergeio.NewClient(
		data.Host.ValueString(),
		data.Username.ValueString(),
		data.Password.ValueString(),
		data.Insecure.ValueBool(),
	)

	resp.DataSourceData = client
	resp.ResourceData = client
}

func (p *testProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{}
}

func (p *testProvider) Resources(ctx context.Context) []func() fwresource.Resource {
	return []func() fwresource.Resource{
		NewMemberResource,
	}
}

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"vergeio": providerserver.NewProtocol6WithError(&testProvider{}),
}

// Test configuration templates
func testAccMemberResourceConfig_basic(memberName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")

	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_member" "test" {
  member = "%s"
  group  = 1
}
`, memberName)
	}

	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_member" "test" {
  member = "%s"
  group  = 1
}
`, host, username, password, memberName)
}

func testAccMemberResourceConfig_updated(memberName string) string {
	host := os.Getenv("TF_ACC_VERGEIO_HOST")
	username := os.Getenv("TF_ACC_VERGEIO_USERNAME")
	password := os.Getenv("TF_ACC_VERGEIO_PASSWORD")

	if host == "" || username == "" || password == "" {
		return fmt.Sprintf(`
provider "vergeio" {
  # Environment variables required for acceptance testing
}

resource "vergeio_member" "test" {
  member = "%s-updated"
  group  = 1
}
`, memberName)
	}

	return fmt.Sprintf(`
provider "vergeio" {
  host     = "%s"
  username = "%s"
  password = "%s"
  insecure = true
}

resource "vergeio_member" "test" {
  member = "%s-updated"
  group  = 1
}
`, host, username, password, memberName)
}
