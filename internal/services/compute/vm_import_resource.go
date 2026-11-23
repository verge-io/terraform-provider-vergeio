// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

const vmImportDescription = "Brings an OVA or OVF in as a virtual machine, or a disk image in as a media-catalog file. " +
	"target vm posts vm_imports and waits until the import finishes. A bare disk image (qcow2, vmdk, vhd, vhdx, raw, img, vdi, iso) is rejected by VergeOS on that table. " +
	"target media downloads the image into the catalog and waits until the file has a size. Attach that file with vergeio_vm_drive media = \"import\" and media_source set to file_id. " +
	"Destroy removes the import row and the VM it created, unless delete_vm is false. An import that adopted a VM which already existed deletes neither the VM nor the vm_imports row, so the resource that created the VM can keep tracking that row. " +
	"It removes a catalog file this resource downloaded unless delete_file is false. A disk image resource is the file, and destroy always removes that file. " +
	"Import reads file, volume, volume_path, shared_object, and the preserve, tier, and interface options from the vm_imports row. url is not a column on that row. A file this resource downloaded is recognized by its description, and url is read from that catalog file. " +
	"An omitted option keeps the stored value, so a plan after import does not replace the VM. file_name is not a vm_imports column and is not restored. " +
	"preserve_bios_serial, guest_agent_preinstalled, and replicate_tags are VergeOS 26.2 fields. On an older system they stay at the configured value because the API does not return them. " +
	"url is stored in state. Do not put a credential in it. " +
	"Running a migration, an image pipeline, or a VM export stays with the Ansible collection. vergeio_vm_export only keeps the volume export configuration."

var (
	_ resource.Resource                   = &vmImportResource{}
	_ resource.ResourceWithImportState    = &vmImportResource{}
	_ resource.ResourceWithIdentity       = &vmImportResource{}
	_ resource.ResourceWithValidateConfig = &vmImportResource{}
)

func NewVMImportResource() resource.Resource {
	return &vmImportResource{}
}

type vmImportResource struct {
	api *importAPI
}

func (r *vmImportResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_import"
}

func (r *vmImportResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := vmImportSourceAttributes()
	for name, attr := range vmImportOptionAttributes() {
		attrs[name] = attr
	}
	for name, attr := range vmImportComputedAttributes() {
		attrs[name] = attr
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: vmImportDescription,
		Attributes:          attrs,
		Blocks:              vmImportTimeoutBlocks(),
	}
}

func vmImportSourceAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "vm_imports key, a 40 character hex string, or the media file id when target is media.",
			Computed:            true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			MarkdownDescription: "VM name to create, or the catalog file name when target is media and file_name is omitted. Changing it replaces the import.",
			Required:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			Validators: []validator.String{
				stringvalidator.LengthAtLeast(1),
			},
		},
		"target": schema.StringAttribute{
			MarkdownDescription: "vm imports an OVA or OVF as a virtual machine. media downloads a disk image into the media catalog. Defaults to vm. Changing it replaces the import.",
			Optional:            true,
			Computed:            true,
			Default:             stringdefault.StaticString(vmImportTargetVM),
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			Validators: []validator.String{
				stringvalidator.OneOf(vmImportTargetVM, vmImportTargetMedia),
			},
		},
		"file": schema.Int64Attribute{
			MarkdownDescription: "Media-catalog file id of an OVA or OVF that is already on VergeOS. Import reads this from the vm_imports file column. Changing it replaces the import.",
			Optional:            true,
			PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			Validators: []validator.Int64{
				int64validator.AtLeast(1),
			},
		},
		"url": schema.StringAttribute{
			MarkdownDescription: "http or https address of an OVA, OVF, or disk image. VergeOS downloads it into the media catalog. The address is stored in state. vm_imports has no url column. Import reads it from the catalog file when that file's description is the one this resource writes for a download. Changing it replaces the import.",
			Optional:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"file_name": schema.StringAttribute{
			MarkdownDescription: "Catalog name for the file downloaded from url. The default for a VM import is the URL's file name. The default for a media import is name. vm_imports has no file_name column, and import does not restore it. Changing it replaces the import.",
			Optional:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"volume": schema.StringAttribute{
			MarkdownDescription: "NAS volume key, a 40 character hex string, that holds the OVA or OVF. Import reads this from the vm_imports row. Changing it replaces the import.",
			Optional:            true,
			PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
		},
		"volume_path": schema.StringAttribute{
			MarkdownDescription: "Path of the OVA or OVF on volume. Import reads this from the vm_imports row. An omitted value keeps the stored value. Changing a stored value replaces the import.",
			Optional:            true,
			PlanModifiers: []planmodifier.String{
				importStoredModifier{},
				stringplanmodifier.RequiresReplace(),
			},
		},
		"shared_object": schema.Int64Attribute{
			MarkdownDescription: "Shared-object id for a tenant import. Import reads this from the vm_imports row. Changing it replaces the import.",
			Optional:            true,
			PlanModifiers:       []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			Validators: []validator.Int64{
				int64validator.AtLeast(1),
			},
		},
	}
}

func vmImportOptionAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"preserve_macs":            importBool("Keep source MAC addresses. Omit on create to leave the platform default. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import."),
		"preserve_drive_format":    importBool("Keep the source disk format. Omit on create to leave the platform default. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import."),
		"preserve_bios_serial":     importBool("Keep the source SMBIOS serial. VergeOS 26.2. Omit on create to leave the platform default. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import."),
		"guest_agent_preinstalled": importBool("Mark the guest agent as already installed. VergeOS 26.2. Omit on create to leave the platform default. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import."),
		"replicate_tags":           importBool("Copy VMware tags and categories onto the imported VM. VergeOS 26.2. Omit on create to leave the platform default. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import."),
		"preferred_tier": schema.StringAttribute{
			MarkdownDescription: "Storage tier, from 1 through 5. Omit on create to leave the platform default. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import.",
			Optional:            true,
			PlanModifiers: []planmodifier.String{
				importStoredModifier{},
				stringplanmodifier.RequiresReplace(),
			},
			Validators: []validator.String{
				stringvalidator.OneOf("1", "2", "3", "4", "5"),
			},
		},
		"no_optical_drives": importBool("Skip optical drives from an OVA. Omit on create to leave the platform default. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import."),
		"override_drive_interface": schema.StringAttribute{
			MarkdownDescription: "Drive interface to use instead of the source interface. default leaves the source value. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import.",
			Optional:            true,
			PlanModifiers: []planmodifier.String{
				importStoredModifier{},
				stringplanmodifier.RequiresReplace(),
			},
		},
		"override_nic_interface": schema.StringAttribute{
			MarkdownDescription: "NIC interface to use instead of the source interface. default leaves the source value. Import stores the vm_imports value. An omitted value keeps the stored value. Changing a stored value replaces the import.",
			Optional:            true,
			PlanModifiers: []planmodifier.String{
				importStoredModifier{},
				stringplanmodifier.RequiresReplace(),
			},
		},
		"delete_vm": schema.BoolAttribute{
			MarkdownDescription: "Destroy deletes the VM this import created. A VM that already had this name is left in place. Defaults to true. Has no effect when target is media.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
		},
		"delete_file": schema.BoolAttribute{
			MarkdownDescription: "Destroy deletes the catalog file downloaded from url for a VM import. A file passed in file is left in place. Defaults to true. A media import is the file, and destroy always removes it.",
			Optional:            true,
			Computed:            true,
			Default:             booldefault.StaticBool(true),
		},
	}
}

func vmImportComputedAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"status": schema.StringAttribute{
			MarkdownDescription: "Import status: initializing, importing, complete, aborted, error, or warning. A media file is complete once it has a size.",
			Computed:            true,
		},
		"status_info": schema.StringAttribute{
			MarkdownDescription: "Detail VergeOS reported for status.",
			Computed:            true,
		},
		"vm_id": schema.Int64Attribute{
			MarkdownDescription: "Key of the VM the import created. Null for a media file, and null until VergeOS assigns one.",
			Computed:            true,
		},
		"file_id": schema.Int64Attribute{
			MarkdownDescription: "Media-catalog file id used for the import. For target media this is the file the resource created.",
			Computed:            true,
		},
		"uuid": schema.StringAttribute{
			MarkdownDescription: "Source VM UUID reported by VergeOS.",
			Computed:            true,
		},
		"importing": schema.BoolAttribute{
			MarkdownDescription: "True while VergeOS is still importing.",
			Computed:            true,
		},
		"aborted": schema.BoolAttribute{
			MarkdownDescription: "True when the import was aborted.",
			Computed:            true,
		},
		"failed_drive_count": schema.Int64Attribute{
			MarkdownDescription: "How many drives VergeOS has already failed.",
			Computed:            true,
		},
		"adopted": schema.BoolAttribute{
			MarkdownDescription: "True when a VM with this name already existed and no new import was posted. Destroy does not delete that VM or the vm_imports row. The resource that created the VM still tracks the row.",
			Computed:            true,
		},
		"owns_file": schema.BoolAttribute{
			MarkdownDescription: "True when this resource downloaded the catalog file and delete_file may remove it.",
			Computed:            true,
		},
	}
}

func vmImportTimeoutBlocks() map[string]schema.Block {
	return map[string]schema.Block{
		"timeouts": schema.SingleNestedBlock{
			MarkdownDescription: "How long create waits for the import or the catalog download, and how long destroy waits for a guest to stop.",
			Attributes: map[string]schema.Attribute{
				"create": schema.StringAttribute{
					MarkdownDescription: "How long create waits. A duration such as \"20m\". Defaults to 10m. A failed import returns as soon as VergeOS reports it.",
					Optional:            true,
					Validators:          []validator.String{durationValidator{}},
				},
				"delete": schema.StringAttribute{
					MarkdownDescription: "How long destroy waits after one ACPI poweroff before killing the VM. A duration such as \"2m\". Defaults to 2m.",
					Optional:            true,
					Validators:          []validator.String{durationValidator{}},
				},
			},
		},
	}
}

func importBool(description string) schema.Attribute {
	return schema.BoolAttribute{
		MarkdownDescription: description,
		Optional:            true,
		PlanModifiers: []planmodifier.Bool{
			importStoredModifier{},
			boolplanmodifier.RequiresReplace(),
		},
	}
}

type importStoredModifier struct{}

func (importStoredModifier) Description(context.Context) string {
	return "Keeps a stored value when the configuration omits the attribute. A configured value that differs replaces the import."
}

func (m importStoredModifier) MarkdownDescription(ctx context.Context) string {
	return m.Description(ctx)
}

func (importStoredModifier) PlanModifyString(ctx context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	switch {
	case req.ConfigValue.IsUnknown():
		resp.PlanValue = req.ConfigValue
	case !req.ConfigValue.IsNull():
		resp.PlanValue = req.ConfigValue
	case !req.StateValue.IsNull() && !req.StateValue.IsUnknown():
		resp.PlanValue = req.StateValue
	default:
		resp.PlanValue = types.StringNull()
	}
}

func (importStoredModifier) PlanModifyBool(ctx context.Context, req planmodifier.BoolRequest, resp *planmodifier.BoolResponse) {
	switch {
	case req.ConfigValue.IsUnknown():
		resp.PlanValue = req.ConfigValue
	case !req.ConfigValue.IsNull():
		resp.PlanValue = req.ConfigValue
	case !req.StateValue.IsNull() && !req.StateValue.IsUnknown():
		resp.PlanValue = req.StateValue
	default:
		resp.PlanValue = types.BoolNull()
	}
}

func (r *vmImportResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	conn, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	api, err := newImportAPI(conn)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.api = api
}

func (r *vmImportResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data importModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(validateImportModel(&data)...)
}

func (r *vmImportResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data importModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.create(ctx, &data); err != nil {
		if importIDSet(&data) {
			r.remember(ctx, resp, &data)
		}
		resp.Diagnostics.AddError("Error creating VM import", err.Error())
		return
	}
	r.remember(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *vmImportResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data importModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.read(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
			if resp.Diagnostics.HasError() {
				return
			}
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VM import", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func (r *vmImportResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state importModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.update(ctx, &plan, &state); err != nil {
		resp.Diagnostics.AddError("Error updating VM import", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, plan.ID)
}

func (r *vmImportResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data importModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.delete(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting VM import", err.Error())
		return
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted vm import %s", data.ID.ValueString()))
}

func (r *vmImportResource) IdentitySchema(ctx context.Context, req resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = shared.KeyIdentitySchema("vm_imports key, or the media file id when target is media.")
}

func (r *vmImportResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := strings.TrimSpace(req.ID)
	if id == "" && req.Identity != nil {
		var got types.String
		resp.Diagnostics.Append(req.Identity.GetAttribute(ctx, path.Root("id"), &got)...)
		if resp.Diagnostics.HasError() {
			return
		}
		if !got.IsNull() && !got.IsUnknown() {
			id = strings.TrimSpace(got.ValueString())
		}
	}
	if !validImportID(id) {
		resp.Diagnostics.AddError(
			"Invalid VM Import ID",
			"Import vergeio_vm_import with the vm_imports key, a 40 character hex string, or with the media file id, a positive integer.",
		)
		return
	}
	shared.ImportByID(ctx, resource.ImportStateRequest{ID: id, Identity: req.Identity}, resp, "Invalid VM Import ID", "Import vergeio_vm_import with the vm_imports key or the media file id.")
}

func (r *vmImportResource) remember(ctx context.Context, resp *resource.CreateResponse, data *importModel) {
	if !importIDSet(data) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
	shared.RememberIdentity(ctx, &resp.Diagnostics, resp.Identity, data.ID)
}

func validImportID(id string) bool {
	if hexKeyPattern.MatchString(id) {
		return true
	}
	n, err := positiveDecimal(id)
	return err == nil && n > 0
}

func positiveDecimal(id string) (int, error) {
	if id == "" || id[0] == '-' || id[0] == '+' {
		return 0, fmt.Errorf("not a positive integer")
	}
	n, err := strconv.Atoi(id)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("not a positive integer")
	}
	return n, nil
}
