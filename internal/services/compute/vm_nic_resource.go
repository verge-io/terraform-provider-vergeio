// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ resource.Resource = &VMNICResource{}
var _ resource.ResourceWithImportState = &VMNICResource{}
var _ resource.ResourceWithMoveState = &VMNICResource{}

func NewVMNICResource() resource.Resource {
	return &VMNICResource{}
}

// VMNICResource is one machine NIC. It references a VM and is matched by name.
type VMNICResource struct {
	vmApi  *VMApi
	nicApi *NICApi
}

type vmNICResourceModel struct {
	ID              types.String `tfsdk:"id"`
	VMID            types.String `tfsdk:"vm_id"`
	Machine         types.Int32  `tfsdk:"machine"`
	Name            types.String `tfsdk:"name"`
	Description     types.String `tfsdk:"description"`
	Interface       types.String `tfsdk:"interface"`
	Driver          types.String `tfsdk:"driver"`
	Model           types.String `tfsdk:"model"`
	Vendor          types.String `tfsdk:"vendor"`
	Port            types.Int32  `tfsdk:"port"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	VNET            types.Int32  `tfsdk:"vnet"`
	MAC             types.String `tfsdk:"macaddress"`
	IPAddress       types.String `tfsdk:"ipaddress"`
	AssignIPAddress types.Bool   `tfsdk:"assign_ipaddress"`
	Asset           types.String `tfsdk:"asset"`
}

func (m *vmNICResourceModel) toNIC() *nicResourceModel {
	if m == nil {
		return nil
	}
	return &nicResourceModel{
		Id:              m.ID,
		Machine:         m.Machine,
		Name:            m.Name,
		Description:     m.Description,
		Interface:       m.Interface,
		Driver:          m.Driver,
		Model:           m.Model,
		Vendor:          m.Vendor,
		Port:            m.Port,
		Enabled:         m.Enabled,
		VNET:            m.VNET,
		MAC:             m.MAC,
		IPAddress:       m.IPAddress,
		AssignIPAddress: m.AssignIPAddress,
		Asset:           m.Asset,
	}
}

func (m *vmNICResourceModel) applyNIC(nic *nicResourceModel) {
	if m == nil || nic == nil {
		return
	}
	if !nic.Id.IsNull() && !nic.Id.IsUnknown() && nic.Id.ValueString() != "" {
		m.ID = nic.Id
	}
	if !nic.Machine.IsNull() && !nic.Machine.IsUnknown() {
		m.Machine = nic.Machine
	}
	if !nic.Name.IsNull() && !nic.Name.IsUnknown() && nic.Name.ValueString() != "" {
		m.Name = nic.Name
	}
	m.Description = nic.Description
	m.Interface = nic.Interface
	m.Driver = nic.Driver
	m.Model = nic.Model
	m.Vendor = nic.Vendor
	m.Port = nic.Port
	m.Enabled = nic.Enabled
	m.VNET = nic.VNET
	m.MAC = nic.MAC
	m.Asset = nic.Asset
	if !nic.IPAddress.IsNull() && !nic.IPAddress.IsUnknown() {
		m.IPAddress = nic.IPAddress
	}
	if !nic.AssignIPAddress.IsNull() && !nic.AssignIPAddress.IsUnknown() {
		m.AssignIPAddress = nic.AssignIPAddress
	}
}

func (r *VMNICResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_nic"
}

func (r *VMNICResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A NIC attached to a VergeOS VM. The NIC is matched to the VM by name. A NIC that already has that name is adopted instead of created again.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "NIC id assigned by VergeOS.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"vm_id": schema.StringAttribute{
				MarkdownDescription: "ID of the vergeio_vm this NIC is attached to. Changing it replaces the NIC.",
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
				MarkdownDescription: "NIC name, unique on the VM. Renaming the NIC updates it in place and keeps its id and MAC address. Create adopts an existing NIC with this name.",
				Required:            true,
			},
			"description": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"interface": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"driver": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"model": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"vendor": schema.StringAttribute{
				Optional: true,
				Computed: true,
			},
			"port": schema.Int32Attribute{
				Optional: true,
				Computed: true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "Whether the NIC is enabled. Omit this attribute to leave the VergeOS default, which is enabled. Set false to create the NIC disabled.",
				Optional:            true,
				Computed:            true,
			},
			"vnet": schema.Int32Attribute{
				MarkdownDescription: "Key of the vNET this NIC attaches to.",
				Optional:            true,
				Computed:            true,
			},
			"macaddress": schema.StringAttribute{
				MarkdownDescription: "MAC address. Renaming the NIC keeps this address.",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"ipaddress": schema.StringAttribute{
				MarkdownDescription: "Address assigned when `assign_ipaddress` is true. Set it to request that address from the vNET. Omit it and VergeOS chooses the next free address. Null when no address is assigned.",
				Optional:            true,
				Computed:            true,
			},
			"assign_ipaddress": schema.BoolAttribute{
				Optional: true,
			},
			"asset": schema.StringAttribute{
				Optional: true,
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *VMNICResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	r.nicApi = NewNICApi(client)
}

func (r *VMNICResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data vmNICResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.prepareMachine(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading VM for NIC", err.Error())
		return
	}

	existing, err := r.nicApi.findNICByName(ctx, data.Machine.ValueInt32(), data.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error looking up NIC", err.Error())
		return
	}
	planned := data.toNIC()
	if existing != nil {
		tflog.Info(ctx, fmt.Sprintf("Adopting NIC %q (id %s) on VM %s", data.Name.ValueString(), existing.Id.ValueString(), data.VMID.ValueString()))
		planned.Id = existing.Id
		if existing.IPAddress.IsNull() || existing.IPAddress.IsUnknown() {
			existing.IPAddress = data.IPAddress
		}
		if nicNeedsUpdate(planned, existing) {
			if err := r.nicApi.updateNIC(ctx, planned, existing); err != nil {
				resp.Diagnostics.AddError("Error updating adopted NIC", err.Error())
				return
			}
			data.applyNIC(existing)
		} else {
			data.applyNIC(existing)
		}
		if data.IPAddress.IsNull() || data.IPAddress.IsUnknown() {
			data.IPAddress = existing.IPAddress
		}
		if data.AssignIPAddress.ValueBool() && (data.IPAddress.IsNull() || data.IPAddress.IsUnknown() || data.IPAddress.ValueString() == "") {
			nic := data.toNIC()
			if err := r.nicApi.assignIP(ctx, nic); err != nil {
				resp.Diagnostics.AddError("Error assigning IP to NIC", err.Error())
				return
			}
			data.IPAddress = nic.IPAddress
		}
	} else {
		if err := r.nicApi.createNIC(ctx, planned); err != nil {
			resp.Diagnostics.AddError("Error creating NIC", err.Error())
			return
		}
		data.applyNIC(planned)
		running, err := readVMPowerState(ctx, r.nicApi.client, data.VMID)
		if err != nil {
			resp.Diagnostics.AddError("Error reading VM power state", err.Error())
			return
		}
		if running {
			if err := r.nicApi.attachCreatedNIC(ctx, planned, data.VMID); err != nil {
				resp.Diagnostics.AddError("Error hotplugging NIC", err.Error())
				return
			}
		}
	}

	if err := r.readInto(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error reading NIC", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VMNICResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data vmNICResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.readInto(ctx, &data); err != nil {
		if notFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading NIC", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VMNICResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state vmNICResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID = state.ID
	plan.Machine = state.Machine
	if plan.AssignIPAddress.IsNull() || plan.AssignIPAddress.IsUnknown() {
		plan.AssignIPAddress = state.AssignIPAddress
	}
	if plan.Machine.IsNull() || plan.Machine.IsUnknown() {
		if err := r.prepareMachine(ctx, &plan); err != nil {
			resp.Diagnostics.AddError("Error reading VM for NIC", err.Error())
			return
		}
	}
	planned := plan.toNIC()
	current := state.toNIC()
	if nicNeedsUpdate(planned, current) {
		if err := r.nicApi.updateNIC(ctx, planned, current); err != nil {
			resp.Diagnostics.AddError("Error updating NIC", err.Error())
			return
		}
		plan.applyNIC(current)
	}
	if plan.IPAddress.IsNull() || plan.IPAddress.IsUnknown() {
		plan.IPAddress = state.IPAddress
	}
	if err := r.readInto(ctx, &plan); err != nil {
		resp.Diagnostics.AddError("Error reading NIC", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *VMNICResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data vmNICResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if data.ID.IsNull() || data.ID.ValueString() == "" {
		return
	}
	if err := r.nicApi.deleteNIC(ctx, data.toNIC(), data.VMID); err != nil && !notFound(err) {
		resp.Diagnostics.AddError("Error deleting NIC", err.Error())
	}
}

func (r *VMNICResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	vmID, name, byName, err := parseVMScopedImportID(req.ID)
	if err != nil {
		summary, detail := vmScopedImportError("NIC", req.ID, err)
		resp.Diagnostics.AddError(summary, detail)
		return
	}
	if !byName {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), vmID)...)
		return
	}
	if r.nicApi == nil || r.vmApi == nil {
		resp.Diagnostics.AddError("Error importing NIC", "Provider is not configured.")
		return
	}
	machine, err := r.vmApi.machineIDForVM(ctx, vmID)
	if err != nil {
		resp.Diagnostics.AddError("Error importing NIC", err.Error())
		return
	}
	nic, err := r.nicApi.findNICByName(ctx, machine.ValueInt32(), name)
	if err != nil {
		resp.Diagnostics.AddError("Error importing NIC", err.Error())
		return
	}
	if nic == nil {
		resp.Diagnostics.AddError("Error importing NIC", fmt.Sprintf("VM %s has no NIC named %q.", vmID, name))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), nic.Id.ValueString())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("vm_id"), vmID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), name)...)
}

// MoveState rejects moving a whole VM into a NIC.
// Terraform moved blocks address a resource, not one nested block.
func (r *VMNICResource) MoveState(ctx context.Context) []resource.StateMover {
	return []resource.StateMover{{
		StateMover: func(ctx context.Context, req resource.MoveStateRequest, resp *resource.MoveStateResponse) {
			if !strings.HasSuffix(req.SourceTypeName, "vergeio_vm") || strings.HasSuffix(req.SourceTypeName, "vergeio_vm_drive") || strings.HasSuffix(req.SourceTypeName, "vergeio_vm_nic") {
				return
			}
			resp.Diagnostics.AddError(
				"Cannot move a VM into a NIC",
				"A moved block addresses the whole vergeio_vm resource, so it cannot pull one inline NIC out of it. Keep the VM at its current address. Add vergeio_vm_nic with the same vm_id and name, or import it as <vm_id>/<name>. The VM state upgrade drops the old inline blocks and leaves the NIC in VergeOS.",
			)
		},
	}}
}

func (r *VMNICResource) prepareMachine(ctx context.Context, data *vmNICResourceModel) error {
	machine, err := r.vmApi.machineIDForVM(ctx, data.VMID.ValueString())
	if err != nil {
		return err
	}
	data.Machine = machine
	return nil
}

func (r *VMNICResource) readInto(ctx context.Context, data *vmNICResourceModel) error {
	if data.ID.IsNull() || data.ID.IsUnknown() || data.ID.ValueString() == "" {
		return fmt.Errorf("NIC %q has no id", data.Name.ValueString())
	}
	priorIP := data.IPAddress
	priorAssign := data.AssignIPAddress
	priorVM := data.VMID
	nic := data.toNIC()
	if err := r.nicApi.readNIC(ctx, nic); err != nil {
		return err
	}
	data.applyNIC(nic)
	if data.IPAddress.IsNull() || data.IPAddress.IsUnknown() {
		data.IPAddress = priorIP
	}
	if data.AssignIPAddress.IsNull() || data.AssignIPAddress.IsUnknown() {
		data.AssignIPAddress = priorAssign
	}
	if data.VMID.IsNull() || data.VMID.IsUnknown() || data.VMID.ValueString() == "" {
		data.VMID = priorVM
	}
	if (data.IPAddress.IsNull() || data.IPAddress.IsUnknown()) && !data.Machine.IsNull() && !data.Machine.IsUnknown() {
		if err := r.fillIPFromList(ctx, data); err != nil {
			return err
		}
	}
	if data.VMID.IsNull() || data.VMID.IsUnknown() || data.VMID.ValueString() == "" {
		if err := r.fillVMID(ctx, data); err != nil {
			return err
		}
	}
	return nil
}

func (r *VMNICResource) fillIPFromList(ctx context.Context, data *vmNICResourceModel) error {
	nics, err := r.nicApi.readNICsByMachine(ctx, data.Machine.ValueInt32())
	if err != nil {
		return err
	}
	for _, nic := range nics {
		if nic != nil && nic.Id.ValueString() == data.ID.ValueString() {
			data.IPAddress = nic.IPAddress
			return nil
		}
	}
	return nil
}

func (r *VMNICResource) fillVMID(ctx context.Context, data *vmNICResourceModel) error {
	if data.Machine.IsNull() || data.Machine.IsUnknown() {
		return fmt.Errorf("NIC %s has no VM id", data.ID.ValueString())
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
