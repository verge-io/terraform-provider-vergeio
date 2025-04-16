// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/provider/vergeio"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &VMResource{}
var _ resource.ResourceWithImportState = &VMResource{}

func NewVMResource() resource.Resource {
	return &VMResource{}
}

// VMResource defines the resource implementation.
type VMResource struct {
	vmApi   *VMApi
	diskApi *DiskApi
	nicApi  *NICApi
}

// CloudInitFile represents a cloud-init file with name and contents.
type CloudInitFile struct {
	Name     types.String `tfsdk:"name"`
	Contents types.String `tfsdk:"contents"`
}

// VMResourceModel describes the resource data model.
type VMResourceModel struct {
	Id                    types.String    `tfsdk:"id"`
	Machine               types.Int32     `tfsdk:"machine"`
	Name                  types.String    `tfsdk:"name"`
	Cluster               types.String    `tfsdk:"cluster"`
	Description           types.String    `tfsdk:"description"`
	Enabled               types.Bool      `tfsdk:"enabled"`
	MachineType           types.String    `tfsdk:"machine_type"`
	AllowHotplug          types.Bool      `tfsdk:"allow_hotplug"`
	DisablePowercycle     types.Bool      `tfsdk:"disable_powercycle"`
	CPUCores              types.Int32     `tfsdk:"cpu_cores"`
	CPUType               types.String    `tfsdk:"cpu_type"`
	RAM                   types.Int32     `tfsdk:"ram"`
	Console               types.String    `tfsdk:"console"`
	Display               types.String    `tfsdk:"display"`
	Video                 types.String    `tfsdk:"video"`
	Sound                 types.String    `tfsdk:"sound"`
	OSFamily              types.String    `tfsdk:"os_family"`
	OSDescription         types.String    `tfsdk:"os_description"`
	RTCBase               types.String    `tfsdk:"rtc_base"`
	BootOrder             types.String    `tfsdk:"boot_order"`
	ConsolePassEnabled    types.Bool      `tfsdk:"console_pass_enabled"`
	ConsolePass           types.String    `tfsdk:"console_pass"`
	USBTablet             types.Bool      `tfsdk:"usb_tablet"`
	UEFI                  types.Bool      `tfsdk:"uefi"`
	SecureBoot            types.Bool      `tfsdk:"secure_boot"`
	SerialPort            types.Bool      `tfsdk:"serial_port"`
	BootDelay             types.Int32     `tfsdk:"boot_delay"`
	PreferredNode         types.String    `tfsdk:"preferred_node"`
	SnapshotProfile       types.String    `tfsdk:"snapshot_profile"`
	CloudInitDataSource   types.String    `tfsdk:"cloudinit_datasource"`
	HAGroup               types.String    `tfsdk:"ha_group"`
	CloudInitFiles        []CloudInitFile `tfsdk:"cloudinit_files"`
	PowerState            types.String    `tfsdk:"powerstate"`
	GuestAgent            types.Bool      `tfsdk:"guest_agent"`
	Advanced              types.String    `tfsdk:"advanced"`
	WaitForGuestAgentInfo types.Int32     `tfsdk:"wait_for_guest_agent_info"`
	// GuestAgentIp          types.String         `tfsdk:"guest_agent_ip"`
	Disks                []*diskResourceModel `tfsdk:"vergeio_drive"`
	NICs                 []*nicResourceModel  `tfsdk:"vergeio_nic"`
	GuestAgentIPs        types.List           `tfsdk:"guest_agent_ips"`
	NestedVirtualization types.Bool           `tfsdk:"nested_virtualization"`
	DisableHypervisor    types.Bool           `tfsdk:"disable_hypervisor"`
}

// Metadata returns the resource type name.
func (r *VMResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm"
}

// Schema defines the schema for the resource.
func (r *VMResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "VM resource in VergeIO",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "VM id (returned as the key) in VergeIO",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"machine": schema.Int32Attribute{
				MarkdownDescription: "Machine",
				Computed:            true,
			},

			"name": schema.StringAttribute{
				MarkdownDescription: "Unique vm name",
				Required:            true,
			},
			"description": schema.StringAttribute{
				MarkdownDescription: "Description",
				Optional:            true,
				Computed:            true,
			},
			"enabled": schema.BoolAttribute{
				MarkdownDescription: "VM state",
				Optional:            true,
				Computed:            true,
			},
			"machine_type": schema.StringAttribute{
				MarkdownDescription: "Machine type",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					// Validate string value must be one of the allowed values
					stringvalidator.OneOf(getValidMachineTypes()...),
				},
			},
			"allow_hotplug": schema.BoolAttribute{
				MarkdownDescription: "Allow hotplug",
				Optional:            true,
				Computed:            true,
			},
			"disable_powercycle": schema.BoolAttribute{
				MarkdownDescription: "Disable powercycle",
				Optional:            true,
				Computed:            true,
			},
			"cpu_cores": schema.Int32Attribute{
				MarkdownDescription: "CPU cores",
				Optional:            true,
				Computed:            true,
			},
			"cpu_type": schema.StringAttribute{
				MarkdownDescription: "CPU type",
				Optional:            true,
				Computed:            true,
			},
			"ram": schema.Int32Attribute{
				MarkdownDescription: "RAM",
				Optional:            true,
				Computed:            true,
			},
			"console": schema.StringAttribute{
				MarkdownDescription: "Console",
				Optional:            true,
				Computed:            true,
			},
			"display": schema.StringAttribute{
				MarkdownDescription: "Display",
				Optional:            true,
				Computed:            true,
			},
			"video": schema.StringAttribute{
				MarkdownDescription: "Video",
				Optional:            true,
				Computed:            true,
			},
			"sound": schema.StringAttribute{
				MarkdownDescription: "Sound",
				Optional:            true,
				Computed:            true,
			},
			"os_family": schema.StringAttribute{
				MarkdownDescription: "USB",
				Optional:            true,
				Computed:            true,
				Validators: []validator.String{
					// Validate string value must be one of the allowed values
					stringvalidator.OneOf(getValidOSFamilies()...),
				},
			},
			"os_description": schema.StringAttribute{
				MarkdownDescription: "OS description",
				Optional:            true,
				Computed:            true,
			},
			"rtc_base": schema.StringAttribute{
				MarkdownDescription: "RTC base",
				Optional:            true,
				Computed:            true,
			},
			"boot_order": schema.StringAttribute{
				MarkdownDescription: "Boot order",
				Optional:            true,
				Computed:            true,
			},
			"console_pass_enabled": schema.BoolAttribute{
				MarkdownDescription: "Console pass enabled",
				Optional:            true,
				Computed:            true,
			},
			"console_pass": schema.StringAttribute{
				MarkdownDescription: "Console pass",
				Optional:            true,
				Computed:            true,
			},
			"usb_tablet": schema.BoolAttribute{
				MarkdownDescription: "USB tablet",
				Optional:            true,
				Computed:            true,
			},
			"uefi": schema.BoolAttribute{
				MarkdownDescription: "UEFI",
				Optional:            true,
				Computed:            true,
			},
			"secure_boot": schema.BoolAttribute{
				MarkdownDescription: "Secure boot",
				Optional:            true,
				Computed:            true,
			},
			"serial_port": schema.BoolAttribute{
				MarkdownDescription: "Serial port",
				Optional:            true,
				Computed:            true,
			},
			"boot_delay": schema.Int32Attribute{
				MarkdownDescription: "Boot delay",
				Optional:            true,
				Computed:            true,
			},
			"preferred_node": schema.StringAttribute{
				MarkdownDescription: "Preferred node",
				Optional:            true,
				Computed:            true,
			},
			"snapshot_profile": schema.StringAttribute{
				MarkdownDescription: "Snapshot profile",
				Optional:            true,
				Computed:            true,
			},
			"cluster": schema.StringAttribute{
				MarkdownDescription: "Cluster",
				Optional:            true,
				Computed:            true,
			},
			"guest_agent": schema.BoolAttribute{
				MarkdownDescription: "Guest agent",
				Optional:            true,
				Computed:            true,
			},
			"cloudinit_datasource": schema.StringAttribute{
				MarkdownDescription: "Cloudinit datasource",
				Optional:            true,
				Computed:            true,
			},
			"ha_group": schema.StringAttribute{
				MarkdownDescription: "HA group",
				Optional:            true,
				Computed:            true,
			},
			"cloudinit_files": schema.ListNestedAttribute{
				Optional: true,
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required: true,
						},
						"contents": schema.StringAttribute{
							Required: true,
						},
					},
				},
			},
			"powerstate": schema.StringAttribute{
				MarkdownDescription: "Power state of the vm",
				Optional:            true,
				Computed:            true,
			},
			"advanced": schema.StringAttribute{
				MarkdownDescription: "Propery and value separated by '\n', e.g. 'tag1=val1\ntag2=val2'",
				Optional:            true,
				Computed:            true,
			},
			"wait_for_guest_agent_info": schema.Int32Attribute{
				MarkdownDescription: "Wait time in seconds for guest agent to be ready",
				Optional:            true,
			},
			"guest_agent_ips": schema.ListAttribute{
				ElementType:         types.StringType,
				MarkdownDescription: "Guest agent Ips",
				Optional:            true,
				Computed:            true,
			},
			"nested_virtualization": schema.BoolAttribute{
				MarkdownDescription: "Nested virtualization",
				Optional:            true,
				Computed:            true,
			},
			"disable_hypervisor": schema.BoolAttribute{
				MarkdownDescription: "Disable hypervisor",
				Optional:            true,
				Computed:            true,
			},
			"enable_bonding": schema.BoolAttribute{
				MarkdownDescription: "Enable bonding",
				Optional:            true,
				Computed:            true,
			},
			"bond_interfaces_args": schema.ListAttribute{
				ElementType:         types.Int32Type,
				MarkdownDescription: "Bond interfaces args",
				Optional:            true,
				Computed:            true,
			},
		},
		// Nested blocks for NICs
		Blocks: map[string]schema.Block{
			"vergeio_nic": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
						"machine": schema.Int32Attribute{
							Optional: true,
							Computed: true,
						},
						"name": schema.StringAttribute{
							Optional: true,
							Computed: true,
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
							Optional: true,
							Computed: true,
						},
						"vnet": schema.Int32Attribute{
							Optional: true,
							Computed: true,
						},
						"macaddress": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
						"ipaddress": schema.StringAttribute{
							MarkdownDescription: "IP address assigned to nic. For this attribute to be set, `assign_ip_address` must be set to `true` and vent id should be set to an Internal Vnet.",
							Optional:            true,
							Computed:            true,
						},
						"assign_ipaddress": schema.BoolAttribute{
							Optional: true,
						},
						"asset": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
					},
				},
			},
			// Nested blocks for drives
			"vergeio_drive": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
						"machine": schema.Int32Attribute{
							Computed: true,
							Optional: true,
						},
						"name": schema.StringAttribute{
							Required: true,
						},
						"description": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
						"interface": schema.StringAttribute{
							Optional: true,
							Computed: true,
							Validators: []validator.String{
								// Validate string value must be one of the allowed values
								stringvalidator.OneOf(getValidDiskInterfaces()...),
							},
						},
						"media": schema.StringAttribute{
							Optional: true,
							// Computed: true,
							Validators: []validator.String{
								// Validate string value must be one of the allowed values
								stringvalidator.OneOf(getValidDiskMedia()...),
							},
						},
						"media_source": schema.Int32Attribute{
							Optional: true,
							// Computed: true,
						},
						"disksize": schema.Int64Attribute{
							Optional: true,
							Computed: true,
						},
						"preferred_tier": schema.StringAttribute{
							Optional: true,
							Computed: true,
							Validators: []validator.String{
								// Validate string value must be one of the allowed values
								stringvalidator.OneOf("1", "2", "3", "4", "5"),
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
				},
			},
		},
	}
}

// Configure the resource.
func (r *VMResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
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
	r.nicApi = NewNICApi(client)
}

// Create a new VM.
func (r *VMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VMResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Create a new VM
	createError := r.vmApi.CreateVM(ctx, &data)
	if createError != nil {
		resp.Diagnostics.AddError(
			"Error creating VM",
			createError.Error(),
		)
		return
	}

	// Write logs using the tflog package
	tflog.Debug(ctx, fmt.Sprintf("created a resource %v", data))

	// Create disks
	if data.Disks != nil {
		for _, disk := range data.Disks {
			disk.Machine = data.Machine

			createError := r.diskApi.createDisk(ctx, disk)
			if createError != nil {
				tflog.Debug(ctx, fmt.Sprintf("Error creating disk %v", createError))
				resp.Diagnostics.AddError(
					"Error creating disk",
					createError.Error(),
				)
				return
			}
		}
	}

	// Create NICs
	if data.NICs != nil {
		for _, nic := range data.NICs {
			nic.Machine = data.Machine

			createError := r.nicApi.createNIC(ctx, nic)
			if createError != nil {
				tflog.Debug(ctx, fmt.Sprintf("Error creating nic %v", createError))
				resp.Diagnostics.AddError(
					"Error creating nic",
					createError.Error(),
				)
				return
			}
		}
	}

	// Wait for the guest agent to be ready
	if data.WaitForGuestAgentInfo.ValueInt32() > 0 {
		tflog.Debug(ctx, fmt.Sprintf("Waiting for %v seconds for guest agent to be ready", data.WaitForGuestAgentInfo.ValueInt32()))
		time.Sleep(time.Duration(data.WaitForGuestAgentInfo.ValueInt32()) * time.Second)
	}

	// Read the VM from the API to get all the data.
	if readError := r.vmApi.readGuestAgentInfo(ctx, &data); readError != nil {
		resp.Diagnostics.AddError(
			"Error reading guest agent info",
			readError.Error(),
		)
		return
	}

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read VM information.
func (r *VMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VMResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read data into the model to get all the attributes
	readDataError := r.vmApi.readVM(ctx, &data)

	if readDataError != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Data",
			readDataError.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update a VM.
func (r *VMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planData, stateData VMResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Updateing the VM with the plan data %v", planData))

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &stateData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Updateing the VM with the state data %v", stateData))

	// Update the VM
	if updateError := r.vmApi.UpdateVM(ctx, &planData, &stateData); updateError != nil {
		resp.Diagnostics.AddError(
			"Error updating VM",
			updateError.Error(),
		)
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Updating the disk with the plan data %v", planData.Disks))
	// update disks
	if planData.Disks != nil && stateData.Disks != nil {
		tflog.Debug(ctx, "Syncing disks ran")
		if err := r.diskApi.syncDisks(ctx, &planData.Disks, &stateData.Disks, stateData.Machine, stateData.Id); err != nil {
			resp.Diagnostics.AddError(
				"Error syncing disks",
				err.Error(),
			)
		}
	}

	// update NICs
	if planData.NICs != nil && stateData.NICs != nil {
		if err := r.nicApi.syncNICs(ctx, &planData.NICs, &stateData.NICs, stateData.Machine, stateData.Id); err != nil {
			resp.Diagnostics.AddError(
				"Error syncing NICs",
				err.Error(),
			)
		}
	}

	// Read the VM from the API to get all the data.
	if readError := r.vmApi.readVM(ctx, &stateData); readError != nil {
		resp.Diagnostics.AddError(
			"Error reading the VM",
			readError.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &stateData)...)
}

func (r *VMResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data VMResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Deleting VM %v", data))

	// Call the API to check the current power state
	if err := r.vmApi.checkVMPowerState(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Failed to check Power State before deletion:",
			err.Error(),
		)
		return
	}

	// Call the API to check if the vm is in a power state that can be deleted
	for strings.ToLower(data.PowerState.ValueString()) != "stopped" {
		Retries := 1

		tflog.Debug(ctx, fmt.Sprintf("Current vm power state is %v", data))

		// Power the vm off
		if err := r.vmApi.killVM(ctx, &data); err != nil {
			resp.Diagnostics.AddError(
				"Failed to kill VM before deletion:",
				err.Error(),
			)
			return
		}

		// Wait for a short period to allow the kill operation to complete
		time.Sleep(1 * time.Second)

		// Call the API to check if the vm is in a power state that can be deleted
		if err := r.vmApi.checkVMPowerState(ctx, &data); err != nil {
			resp.Diagnostics.AddError(
				"Failed to check Power State before deletion:",
				err.Error(),
			)
			return
		}

		Retries += 1

		// We are only going to retry 5 times before giving up
		if Retries > 5 {
			resp.Diagnostics.AddError(
				"Failed to kill VM before deletion:",
				fmt.Sprintf("Failed to kill VM before deletion after %d retries", Retries),
			)
			return
		}
		continue
	}

	tflog.Debug(ctx, fmt.Sprintf("VM state before deletion %v", data.PowerState.ValueString()))

	// Proceed with vm deletion
	// IMPORTANT: we don't need to explicitly delete the nics and the disks. They will be deleted when the VM is deleted.
	if err := r.vmApi.deleteVM(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting Data",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, "VM was successfully deleted")
}

func (r *VMResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
