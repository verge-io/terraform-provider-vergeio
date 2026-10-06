// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"regexp"
	"strings"

	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const serviceDescription = "A VergeOS NAS service on an existing virtual machine. Set vm_id to vergeio_vm.id so Terraform removes this service before the virtual machine. Removing the service does not remove the virtual machine. user blocks are the accounts on this service. A user left out of the configuration is deleted. Changing a user name replaces that user. password_wo is sent when the user is created and when password_wo_version changes. Terraform stores the version, not the password. If deleting the service is refused because a volume remains, Terraform disables that volume, removes its shares, and tries again so a partial create cannot leave a row that blocks destroy."

var (
	_ resource.Resource                = &serviceResource{}
	_ resource.ResourceWithConfigure   = &serviceResource{}
	_ resource.ResourceWithImportState = &serviceResource{}
	_ resource.ResourceWithIdentity    = &serviceResource{}
	_ resource.ResourceWithModifyPlan  = &serviceResource{}

	readAheadValues = []string{"", "0", "64", "128", "256", "512", "1024", "2048", "4096"}
	userNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)
)

func NewServiceResource() resource.Resource {
	return &serviceResource{}
}

// serviceResource is vergeio_nas_service.
type serviceResource struct {
	api *API
}

type serviceModel struct {
	ID                 types.String `tfsdk:"id"`
	VMID               types.String `tfsdk:"vm_id"`
	Name               types.String `tfsdk:"name"`
	MaxImports         types.Int64  `tfsdk:"max_imports"`
	MaxSyncs           types.Int64  `tfsdk:"max_syncs"`
	DisableSwap        types.Bool   `tfsdk:"disable_swap"`
	ReadAheadKBDefault types.String `tfsdk:"read_ahead_kb_default"`
	CIFSID             types.Int64  `tfsdk:"cifs_id"`
	NFSID              types.Int64  `tfsdk:"nfs_id"`
	AntivirusID        types.Int64  `tfsdk:"antivirus_id"`
	Users              []userModel  `tfsdk:"user"`
}

type userModel struct {
	ID                types.String `tfsdk:"id"`
	Name              types.String `tfsdk:"name"`
	PasswordWO        types.String `tfsdk:"password_wo"`
	PasswordWOVersion types.Int64  `tfsdk:"password_wo_version"`
	DisplayName       types.String `tfsdk:"display_name"`
	Description       types.String `tfsdk:"description"`
	Enabled           types.Bool   `tfsdk:"enabled"`
	HomeShare         types.Int64  `tfsdk:"home_share"`
	HomeDrive         types.String `tfsdk:"home_drive"`
	Created           types.Int64  `tfsdk:"created"`
}

func (r *serviceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nas_service"
}

func (r *serviceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: serviceDescription,
		Attributes: map[string]schema.Attribute{
			"id": idAttr("NAS service id."),
			"vm_id": schema.StringAttribute{
				MarkdownDescription: "Virtual machine id, the same value as vergeio_vm.id. Changing it replaces the service.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name":                  stableString("Service name, taken from the virtual machine."),
			"max_imports":           optInt("Maximum number of concurrent imports, from 1 to 200. Omit to leave the current value unchanged."),
			"max_syncs":             optInt("Maximum number of concurrent syncs, from 0 to 200. 0 disables sync. Omit to leave the current value unchanged."),
			"disable_swap":          optBool("Disable swap for this service. Omit to leave the current value unchanged."),
			"read_ahead_kb_default": optString("Default read ahead size in KB. One of `0`, `64`, `128`, `256`, `512`, `1024`, `2048`, or `4096`. Omit to leave the current value unchanged.", stringvalidator.OneOf(readAheadValues...)),
			"cifs_id":               stableInt("CIFS settings id for this service."),
			"nfs_id":                stableInt("NFS settings id for this service."),
			"antivirus_id":          stableInt("Antivirus settings id for this service."),
		},
		Blocks: map[string]schema.Block{
			"user": schema.ListNestedBlock{
				MarkdownDescription: "Accounts that can open CIFS shares on this service. A user left out of the configuration is deleted. Changing name replaces that user. password_wo is sent when the user is created and when password_wo_version changes. Terraform stores the version, not the password.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": computedString("User id."),
						"name": schema.StringAttribute{
							MarkdownDescription: "User name. 1 to 32 characters: letters, digits, underscore, and hyphen. Changing it replaces the user.",
							Required:            true,
							Validators: []validator.String{
								stringvalidator.RegexMatches(userNamePattern, "Use 1 to 32 characters: letters, digits, underscore, and hyphen."),
							},
						},
						"password_wo": schema.StringAttribute{
							MarkdownDescription: "Password sent when the user is created and when password_wo_version changes. Terraform does not store it. Requires Terraform 1.11 or OpenTofu 1.11.",
							Optional:            true,
							WriteOnly:           true,
							Sensitive:           true,
							Validators: []validator.String{
								stringvalidator.LengthAtLeast(1),
								stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("password_wo_version")),
							},
						},
						"password_wo_version": schema.Int64Attribute{
							MarkdownDescription: "Version of password_wo. Increment it to set a new password. Terraform stores this number, not the password.",
							Optional:            true,
							Validators: []validator.Int64{
								int64validator.AtLeast(1),
								int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("password_wo")),
							},
						},
						"display_name": schema.StringAttribute{
							MarkdownDescription: "Name shown for this user. Omit to leave the current value unchanged.",
							Optional:            true,
							Computed:            true,
						},
						"description": schema.StringAttribute{
							MarkdownDescription: "What this user is for. Omit to leave the current value unchanged.",
							Optional:            true,
							Computed:            true,
						},
						"enabled": schema.BoolAttribute{
							MarkdownDescription: "Whether the user can sign in. Defaults to true.",
							Optional:            true,
							Computed:            true,
							Default:             booldefault.StaticBool(true),
						},
						"home_share": schema.Int64Attribute{
							MarkdownDescription: "CIFS share id used as the home directory. Omit to leave the current value unchanged.",
							Optional:            true,
							Computed:            true,
						},
						"home_drive": schema.StringAttribute{
							MarkdownDescription: "Windows drive letter for the home directory, one letter from A through Z. Omit to leave the current value unchanged.",
							Optional:            true,
							Computed:            true,
							Validators: []validator.String{
								stringvalidator.RegexMatches(regexp.MustCompile(`^[A-Za-z]?$`), "Use one letter from A through Z, or leave it empty."),
							},
						},
						"created": timestampAttr("Time the user was created, as seconds since the epoch."),
					},
				},
			},
		},
	}
}

func (r *serviceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() || req.State.Raw.IsNull() {
		return
	}
	var plan, state serviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.Users = keepNamedUsers(plan.Users, state.Users)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *serviceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	api, diags := configure(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	r.api = api
}

func (r *serviceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, config serviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.createService(ctx, &plan, userSecrets(config.Users, nil, true)); err != nil {
		resp.Diagnostics.AddError("Error Creating NAS Service", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *serviceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data serviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readService(ctx, &data); err != nil {
		if missing(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error Reading NAS Service", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *serviceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, config serviceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.updateService(ctx, &plan, &state, userSecrets(config.Users, state.Users, false)); err != nil {
		resp.Diagnostics.AddError("Error Updating NAS Service", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *serviceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data serviceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteService(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error Deleting NAS Service", err.Error())
	}
}

func (r *serviceResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("NAS service id. Import vergeio_nas_service with this value.")
}

func (r *serviceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if id != "" {
		if _, err := parsePositiveID(types.StringValue(id), "NAS service"); err != nil {
			resp.Diagnostics.AddError("Invalid NAS Service Import ID", "Import vergeio_nas_service with the service id, a positive integer.")
			return
		}
	}
	shared.ImportByID(ctx, req, resp, "Invalid NAS Service Import ID", "Import vergeio_nas_service with the service id, a positive integer.")
}
