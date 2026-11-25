// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

var (
	_ resource.Resource                   = &VMCloneResource{}
	_ resource.ResourceWithImportState    = &VMCloneResource{}
	_ resource.ResourceWithConfigure      = &VMCloneResource{}
	_ resource.ResourceWithValidateConfig = &VMCloneResource{}
)

const vmCloneDescription = "A VM cloned from a machine_snapshots row. Create clones the snapshot VM (the vms row whose machine is snap_machine and is_snapshot is true). The live VM is not the action target. preserve_macs is false. Every NIC on the copy is moved to network_id before create returns. A copy with no NIC gets one virtio NIC named lan on that network. Destroy powers the copy off and deletes it. The source VM, its NICs, and the snapshot row are not written. Changing snapshot_id, name, or network_id replaces the copy. An apply that is interrupted after VergeOS stores the copy keeps that key in state. A later destroy removes it instead of leaving a second VM with the same name. created is the time VergeOS stored the copy. Refresh drops this resource when the key's name or created time differs. Destroy without a refresh leaves that other VM."

func NewVMCloneResource() resource.Resource {
	return &VMCloneResource{}
}

// VMCloneResource is vergeio_vm_clone.
type VMCloneResource struct {
	api *VMApi
}

// VMCloneResourceModel is vergeio_vm_clone.
type VMCloneResourceModel struct {
	Id         types.String `tfsdk:"id"`
	SnapshotID types.String `tfsdk:"snapshot_id"`
	Name       types.String `tfsdk:"name"`
	NetworkID  types.String `tfsdk:"network_id"`
	SourceVMID types.String `tfsdk:"source_vm_id"`
	Created    types.Int64  `tfsdk:"created"`
}

func (r *VMCloneResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_clone"
}

func (r *VMCloneResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: vmCloneDescription,
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "VM $key of the copy.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"snapshot_id": schema.StringAttribute{
				MarkdownDescription: "machine_snapshots key to clone. Changing it replaces the copy.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Name of the copy. Changing it replaces the copy.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Network key every NIC on the copy is attached to. vergeio_network.id is this value. Changing it replaces the copy.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source_vm_id": schema.StringAttribute{
				MarkdownDescription: "VM $key of the live VM the snapshot belongs to. Null when that VM is gone, and null after import when the source is unknown. Destroy does not delete this VM. Read and destroy work when this is null.",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"created": schema.Int64Attribute{
				MarkdownDescription: "Unix time when VergeOS created the copy. Distinct from id, which VergeOS can reuse. A different name or created time means the key now points at another VM, and refresh drops this resource. Destroy without a refresh leaves the other VM. Import leaves this empty, and the first read stores it.",
				Computed:            true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *VMCloneResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	api, err := NewVMApi(client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	r.api = api
}

func (r *VMCloneResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data VMCloneResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.SnapshotID, "snapshot_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("snapshot_id"), "Invalid Snapshot ID", err.Error())
	}
	if _, _, err := shared.KnownPositiveID(data.NetworkID, "network_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("network_id"), "Invalid Network ID", err.Error())
	}
	if !data.Name.IsUnknown() && data.Name.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "Invalid Clone Name", "name is required")
	}
}

func (r *VMCloneResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VMCloneResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.api.createVMClone(ctx, &data)
	if vmCloneIDSet(&data) {
		r.keepVMClone(ctx, resp, &data)
	}
	if err != nil {
		if vmCloneIDSet(&data) && vmCloneStateID(ctx, resp) == "" {
			resp.Diagnostics.AddError("Error saving VM clone state", vmCloneNotInState(&data))
		}
		resp.Diagnostics.AddError("Error creating VM clone", err.Error())
		return
	}
	if resp.Diagnostics.HasError() {
		return
	}
}

func vmCloneIDSet(data *VMCloneResourceModel) bool {
	return data != nil && !data.Id.IsNull() && !data.Id.IsUnknown() && strings.TrimSpace(data.Id.ValueString()) != ""
}

// keepVMClone writes the copy once its key is known. Terraform keeps that
// state when create also returns an error, so the next apply destroys this
// copy instead of posting the same name again.
func (r *VMCloneResource) keepVMClone(ctx context.Context, resp *resource.CreateResponse, data *VMCloneResourceModel) {
	if resp == nil || !vmCloneIDSet(data) {
		return
	}
	stored := vmCloneForState(data)
	resp.Diagnostics.Append(resp.State.Set(shared.WritableContext(ctx), &stored)...)
}

func vmCloneForState(data *VMCloneResourceModel) VMCloneResourceModel {
	stored := *data
	stored.Id = knownString(data.Id)
	stored.SnapshotID = knownString(data.SnapshotID)
	stored.Name = knownString(data.Name)
	stored.NetworkID = knownString(data.NetworkID)
	stored.SourceVMID = knownString(data.SourceVMID)
	stored.Created = knownInt64(data.Created)
	return stored
}

func vmCloneNotInState(data *VMCloneResourceModel) string {
	name := ""
	id := ""
	if data != nil {
		name = data.Name.ValueString()
		id = data.Id.ValueString()
	}
	return fmt.Sprintf("VM clone %q (id %s) exists in VergeOS but could not be stored in Terraform state. Import it with `terraform import vergeio_vm_clone.<name> %s`, or delete it in VergeOS and apply again.", name, id, id)
}

func vmCloneStateID(ctx context.Context, resp *resource.CreateResponse) string {
	ctx = shared.WritableContext(ctx)
	if resp == nil || resp.State.Raw.Type() == nil || resp.State.Raw.IsNull() {
		return ""
	}
	var got VMCloneResourceModel
	if diags := resp.State.Get(ctx, &got); diags.HasError() {
		return ""
	}
	if !vmCloneIDSet(&got) {
		return ""
	}
	return got.Id.ValueString()
}

func (r *VMCloneResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VMCloneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.readVMClone(ctx, &data); err != nil {
		if vergeos.IsNotFoundError(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Error reading VM clone", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *VMCloneResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError("VM Clone Cannot Be Updated", "Changing snapshot_id, name, or network_id replaces the copy.")
}

func (r *VMCloneResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VMCloneResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.api.deleteVMClone(ctx, &data); err != nil {
		resp.Diagnostics.AddError("Error deleting VM clone", err.Error())
	}
}

func (r *VMCloneResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
