// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &VMDriveResource{}
var _ resource.ResourceWithImportState = &VMDriveResource{}
var _ resource.ResourceWithMoveState = &VMDriveResource{}

func NewVMDriveResource() resource.Resource {
	return &VMDriveResource{}
}

// VMDriveResource is one machine drive. It references a VM and is matched by name.
type VMDriveResource struct {
	vmApi   *VMApi
	diskApi *DiskApi
}

type vmDriveResourceModel struct {
	ID                  types.String  `tfsdk:"id"`
	VMID                types.String  `tfsdk:"vm_id"`
	Machine             types.Int32   `tfsdk:"machine"`
	Name                types.String  `tfsdk:"name"`
	Description         types.String  `tfsdk:"description"`
	Interface           types.String  `tfsdk:"interface"`
	Media               types.String  `tfsdk:"media"`
	MediaSource         types.Int32   `tfsdk:"media_source"`
	DiskSize            types.Float64 `tfsdk:"disksize"`
	PreferredTier       types.Int32   `tfsdk:"preferred_tier"`
	Enabled             types.Bool    `tfsdk:"enabled"`
	ReadOnly            types.Bool    `tfsdk:"readonly"`
	Serial              types.String  `tfsdk:"serial"`
	Asset               types.String  `tfsdk:"asset"`
	OrderId             types.Int32   `tfsdk:"orderid"`
	PreserveDriveFormat types.Bool    `tfsdk:"preserve_drive_format"`
}

func (m *vmDriveResourceModel) toDisk() *diskResourceModel {
	if m == nil {
		return nil
	}
	return &diskResourceModel{
		Key:                 m.ID,
		Machine:             m.Machine,
		Name:                m.Name,
		Description:         m.Description,
		Interface:           m.Interface,
		Media:               m.Media,
		MediaSource:         m.MediaSource,
		DiskSize:            m.DiskSize,
		PreferredTier:       m.PreferredTier,
		Enabled:             m.Enabled,
		ReadOnly:            m.ReadOnly,
		Serial:              m.Serial,
		Asset:               m.Asset,
		OrderId:             m.OrderId,
		PreserveDriveFormat: m.PreserveDriveFormat,
	}
}

func (m *vmDriveResourceModel) applyDisk(disk *diskResourceModel) {
	if m == nil || disk == nil {
		return
	}
	if !disk.Key.IsNull() && !disk.Key.IsUnknown() && disk.Key.ValueString() != "" {
		m.ID = disk.Key
	}
	if !disk.Machine.IsNull() && !disk.Machine.IsUnknown() {
		m.Machine = disk.Machine
	}
	if !disk.Name.IsNull() && !disk.Name.IsUnknown() && disk.Name.ValueString() != "" {
		m.Name = disk.Name
	}
	m.Description = disk.Description
	m.Interface = disk.Interface
	m.DiskSize = disk.DiskSize
	m.PreferredTier = disk.PreferredTier
	m.Enabled = disk.Enabled
	m.ReadOnly = disk.ReadOnly
	m.Serial = disk.Serial
	m.Asset = disk.Asset
	m.OrderId = disk.OrderId
	m.PreserveDriveFormat = disk.PreserveDriveFormat
	if !disk.Media.IsNull() && !disk.Media.IsUnknown() {
		m.Media = disk.Media
	}
	if !disk.MediaSource.IsNull() && !disk.MediaSource.IsUnknown() {
		m.MediaSource = disk.MediaSource
	}
}

func (r *VMDriveResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_drive"
}

func (r *VMDriveResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A drive attached to a VergeOS VM. The drive is matched to the VM by name. A drive that already has that name is adopted instead of created again. Do not use the same name as the VM boot_disk.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Drive key assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vm_id": schema.StringAttribute{
				MarkdownDescription: "ID of the vergeio_vm this drive is attached to. Changing it replaces the drive.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					replaceWhenVMChanges(),
				},
			},
			"machine": schema.Int32Attribute{
				MarkdownDescription: "Machine id of the VM. Assigned from the VM.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Drive name, unique on the VM. Renaming the drive updates it in place and keeps its key. Create adopts an existing drive with this name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"interface": schema.StringAttribute{
				MarkdownDescription: "Drive interface. Must be one of the machine_drives interfaces VergeOS documents, including usb.",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(diskInterfaces()...),
				},
			},
			"media": schema.StringAttribute{
				MarkdownDescription: "Media type. Changing media replaces the drive, because an existing drive cannot change media.",
				Optional:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.OneOf(getValidDiskMedia()...),
				},
			},
			"media_source": schema.Int32Attribute{
				MarkdownDescription: "Source used to create a cloned, imported, or CD-ROM drive. Changing media_source replaces the drive.",
				Optional:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.RequiresReplace(),
				},
			},
			"disksize": schema.Float64Attribute{
				MarkdownDescription: "Size in GB. Supports fractional values.",
				Optional:            true,
				Computed:            true,
			},
			"preferred_tier": schema.Int32Attribute{
				MarkdownDescription: "Storage tier from 1 to 5. VergeOS assigns the system default when this is omitted, and a later update does not move the drive.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
				Validators: []validator.Int32{
					int32validator.OneOf(1, 2, 3, 4, 5),
				},
			},
			"enabled": schema.BoolAttribute{
				Optional: true,
				Computed: true,
			},
			"readonly": schema.BoolAttribute{
				Optional: true,
				Computed: true,
			},
			"serial": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"asset": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"orderid": schema.Int32Attribute{
				Optional: true,
				Computed: true,
			},
			"preserve_drive_format": schema.BoolAttribute{
				Optional: true,
				Computed: true,
			},
		},
	}
}

func (r *VMDriveResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.vmApi = NewVMApi(client)
	r.diskApi = NewDiskApi(client)
}

func (r *VMDriveResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data vmDriveResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.prepareMachine(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading VM for drive", err.Error())
		return
	}
	if err := r.validateInterface(ctx, data.Interface); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("interface"), "Invalid Drive Interface", err.Error())
		return
	}

	existing, err := r.diskApi.findDiskByName(ctx, data.Machine.ValueInt32(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error looking up drive", err.Error())
		return
	}
	planned := data.toDisk()
	if existing != nil {
		tflog.Info(ctx, fmt.Sprintf("Adopting drive %q (key %s) on VM %s", data.Name.ValueString(), existing.Key.ValueString(), data.VMID.ValueString()))
		if knownMediaChanged(planned, existing) {
			resp.Diagnostics.AddError(
				"Drive already exists",
				fmt.Sprintf("VM %s already has a drive named %q (key %s) with different media or media_source. Import it, or choose another name.", data.VMID.ValueString(), data.Name.ValueString(), existing.Key.ValueString()),
			)
			return
		}
		planned.Key = existing.Key
		if diskNeedsUpdate(planned, existing) {
			if err := r.diskApi.updateDisk(ctx, planned, existing); err != nil {
				resp.Diagnostics.AddError("Error updating adopted drive", err.Error())
				return
			}
			data.applyDisk(existing)
		} else {
			data.applyDisk(existing)
		}
	} else {
		if err := r.diskApi.createDisk(ctx, planned); err != nil {
			resp.Diagnostics.AddError("Error creating drive", err.Error())
			return
		}
		data.applyDisk(planned)
		running, err := readVMPowerState(ctx, r.diskApi.client, data.VMID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading VM power state", err.Error())
			return
		}
		if running {
			if err := r.diskApi.attachCreatedDrive(ctx, planned, data.VMID); err != nil {
				resp.Diagnostics.AddError("Error hotplugging drive", err.Error())
				return
			}
		}
	}

	if err := r.readInto(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading drive", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VMDriveResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data vmDriveResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &data); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading drive", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VMDriveResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmDriveResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.validateInterface(ctx, plan.Interface); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("interface"), "Invalid Drive Interface", err.Error())
		return
	}
	plan.ID = state.ID
	plan.Machine = state.Machine
	if plan.Machine.IsNull() || plan.Machine.IsUnknown() {
		if err := r.prepareMachine(ctx, &plan); err != nil {
			resp.Diagnostics.AddError("Error reading VM for drive", err.Error())
			return
		}
	}
	planned := plan.toDisk()
	current := state.toDisk()
	if knownMediaChanged(planned, current) {
		resp.Diagnostics.AddError(
			"Cannot change drive media",
			fmt.Sprintf("Drive %q: changing media or media_source replaces the drive.", plan.Name.ValueString()),
		)
		return
	}
	if diskNeedsUpdate(planned, current) {
		if err := r.diskApi.updateDisk(ctx, planned, current); err != nil {
			resp.Diagnostics.AddError("Error updating drive", err.Error())
			return
		}
		plan.applyDisk(current)
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading drive", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VMDriveResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data vmDriveResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.ID.IsNull() || data.ID.ValueString() == "" {
		return
	}
	if err := r.diskApi.deleteDisk(ctx, data.toDisk(), data.VMID); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Error deleting drive", err.Error())
	}
}

func (r *VMDriveResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vmID, name, byName, err := parseVMScopedImportID(req.ID)
	if err != nil {
		summary, detail := vmScopedImportError("drive", req.ID, err)
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	if !byName {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), vmID)...)
		return
	}
	if r.diskApi == nil || r.vmApi == nil {
		resp.Diagnostics.AddError("Error importing drive", "Provider is not configured.")
		return
	}
	machine, err := r.vmApi.machineIDForVM(ctx, vmID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing drive", err.Error())
		return
	}
	disk, err := r.diskApi.findDiskByName(ctx, machine.ValueInt32(), name)
	if err != nil {
		resp.Diagnostics.AddError("Error importing drive", err.Error())
		return
	}
	if disk == nil {
		resp.Diagnostics.AddError("Error importing drive", fmt.Sprintf("VM %s has no drive named %q.", vmID, name))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), disk.Key.ValueString())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

// MoveState rejects moving a whole VM into a drive.
// Terraform moved blocks address a resource, not one nested block. Moving
// vergeio_vm would drop the VM from state. Import vm_id/name instead.
func (r *VMDriveResource) MoveState(ctx context.Context) []resource.StateMover {
	return []resource.StateMover{{
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if !strings.HasSuffix(req.SourceTypeName, "vergeio_vm") || strings.HasSuffix(req.SourceTypeName, "vergeio_vm_drive") || strings.HasSuffix(req.SourceTypeName, "vergeio_vm_nic") {
				return
			}
			resp.Diagnostics.AddError(
				"Cannot move a VM into a drive",
				"A moved block addresses the whole vergeio_vm resource, so it cannot pull one inline drive out of it. Keep the VM at its current address. Add vergeio_vm_drive with the same vm_id and name, or import it as <vm_id>/<name>. The VM state upgrade drops the old inline blocks and leaves the drive in VergeOS.",
			)
		},
	}}
}

func (r *VMDriveResource) prepareMachine(ctx context.Context, data *vmDriveResourceModel) error {
	machine, err := r.vmApi.machineIDForVM(ctx, data.VMID.ValueString())
	if err != nil {
		return err
	}
	data.Machine = machine
	return nil
}

func (r *VMDriveResource) validateInterface(ctx context.Context, diskInterface types.String) error {
	if diskInterface.IsNull() || diskInterface.IsUnknown() || diskInterface.ValueString() == "" {
		return nil
	}
	interfaces, err := r.diskApi.GetDiskInterfacesFromAPI(ctx)
	if err != nil {
		return fmt.Errorf("unable to fetch valid disk interfaces from VergeOS API: %w", err)
	}
	for _, iface := range interfaces {
		if diskInterface.ValueString() == iface {
			return nil
		}
	}
	return fmt.Errorf("disk interface '%s' is not supported by this VergeOS version", diskInterface.ValueString())
}

func (r *VMDriveResource) readInto(ctx context.Context, data *vmDriveResourceModel) error {
	if data.ID.IsNull() || data.ID.IsUnknown() || data.ID.ValueString() == "" {
		return fmt.Errorf("drive %q has no key", data.Name.ValueString())
	}
	priorMedia := data.Media
	priorSource := data.MediaSource
	priorVM := data.VMID
	disk := data.toDisk()
	if err := r.diskApi.readDisk(ctx, disk); err != nil {
		return err
	}
	data.applyDisk(disk)
	if data.Media.IsNull() || data.Media.IsUnknown() {
		data.Media = priorMedia
	}
	if data.MediaSource.IsNull() || data.MediaSource.IsUnknown() {
		data.MediaSource = priorSource
	}
	if data.VMID.IsNull() || data.VMID.IsUnknown() || data.VMID.ValueString() == "" {
		data.VMID = priorVM
	}
	if data.VMID.IsNull() || data.VMID.IsUnknown() || data.VMID.ValueString() == "" {
		if err := r.fillVMID(ctx, data); err != nil {
			return err
		}
	}
	return nil
}

func (r *VMDriveResource) fillVMID(ctx context.Context, data *vmDriveResourceModel) error {
	if data.Machine.IsNull() || data.Machine.IsUnknown() {
		return fmt.Errorf("drive %s has no VM id", data.ID.ValueString())
	}
	vm, err := r.vmApi.findVMByMachine(ctx, data.Machine.ValueInt32())
	if err != nil {
		return err
	}
	if vm == nil {
		return fmt.Errorf("no VM found for machine %d", data.Machine.ValueInt32())
	}
	data.VMID = types.StringValue(fmt.Sprintf("%d", vm.ID.Int()))
	return nil
}
