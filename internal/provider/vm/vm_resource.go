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
var _ resource.ResourceWithUpgradeState = &VMResource{}

func NewVMResource() resource.Resource {
	return &VMResource{}
}

// VMResource defines the resource implementation.
type VMResource struct {
	vmApi     *VMApi
	diskApi   *DiskApi
	nicApi    *NICApi
	deviceApi *DeviceApi
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
	PowerState            types.Bool      `tfsdk:"powerstate"`
	GuestAgent            types.Bool      `tfsdk:"guest_agent"`
	Advanced              types.String    `tfsdk:"advanced"`
	WaitForGuestAgentInfo types.Int32     `tfsdk:"wait_for_guest_agent_info"`
	// GuestAgentIp          types.String         `tfsdk:"guest_agent_ip"`
	Disks                 []*diskResourceModel   `tfsdk:"vergeio_drive"`
	NICs                  []*nicResourceModel    `tfsdk:"vergeio_nic"`
	Devices               []*deviceResourceModel `tfsdk:"vergeio_device"`
	GuestAgentIPs         types.List             `tfsdk:"guest_agent_ips"`
	NestedVirtualization  types.Bool             `tfsdk:"nested_virtualization"`
	DisableHypervisor     types.Bool             `tfsdk:"disable_hypervisor"`
	WaitForGuestIPTimeout types.Int32            `tfsdk:"wait_for_guest_ip_timeout"`
	IgnoredGuestIPs       types.String           `tfsdk:"ignored_guest_ips"`
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
		Version:             1,

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
				MarkdownDescription: "Machine type (validated dynamically against VergeOS API)",
				Optional:            true,
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					MachineTypeSemanticEquality(),
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
			"powerstate": schema.BoolAttribute{
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
			"wait_for_guest_ip_timeout": schema.Int32Attribute{
				MarkdownDescription: "Wait time in seconds for guest ip to be ready",
				Optional:            true,
			},
			"ignored_guest_ips": schema.StringAttribute{
				MarkdownDescription: "Ignored guest ips (CIDR)",
				Optional:            true,
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
							// Note: Dynamic validation performed in Create/Update methods
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
						"disksize": schema.Float64Attribute{
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
			// Nested blocks for devices
			"vergeio_device": schema.ListNestedBlock{
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
						// "machine_type": schema.StringAttribute{
						// 	Optional: true,
						// 	Computed: true,
						// },
						"type": schema.StringAttribute{
							MarkdownDescription: "Type of the device",
							Optional:            true,
							Computed:            true,
							Validators: []validator.String{
								// Validate string value must be one of the allowed values
								stringvalidator.OneOf(getValidDeviceTypes()...),
							},
						},
						"name": schema.StringAttribute{
							Required: true,
						},
						"description": schema.StringAttribute{
							Optional: true,
							Computed: true,
						},
						"resource_group": schema.StringAttribute{
							MarkdownDescription: "Resource group of the device",
							Optional:            true,
							Computed:            true,
						},
						"enabled": schema.BoolAttribute{
							Optional: true,
							Computed: true,
						},
						"status": schema.Int32Attribute{
							MarkdownDescription: "Status of the device",
							Optional:            true,
							Computed:            true,
						},
						"usb_settings": schema.SingleNestedAttribute{
							Optional: true,
							// PlanModifiers: []planmodifier.List{
							// 	listplanmodifier.UseStateForUnknown(),
							// },
							Attributes: map[string]schema.Attribute{
								"key": schema.Int32Attribute{
									Optional: true,
									Computed: true,
								},
								"machine_device": schema.Int32Attribute{
									Optional: true,
									Computed: true,
								},
								"guest_reset": schema.BoolAttribute{
									Optional: true,
									Computed: true,
								},
								"guest_resets_all": schema.BoolAttribute{
									Optional: true,
									Computed: true,
								},
							},
						},
						"tpm_settings": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"key": schema.Int32Attribute{
									Optional: true,
									Computed: true,
								},
								"machine_device": schema.Int32Attribute{
									Optional: true,
									Computed: true,
								},
								"model": schema.StringAttribute{
									Optional: true,
									Computed: true,
								},
								"version": schema.StringAttribute{
									Optional: true,
									Computed: true,
								},
							},
						},
						"nvidia_vgpu_settings": schema.SingleNestedAttribute{
							Optional: true,
							Attributes: map[string]schema.Attribute{
								"key": schema.Int32Attribute{
									Optional: true,
									Computed: true,
								},
								"machine_device": schema.Int32Attribute{
									Optional: true,
									Computed: true,
								},
								"profile_type": schema.StringAttribute{
									Optional: true,
									Computed: true,
								},
								// "attach_drivers": schema.BoolAttribute{
								// 	Optional: true,
								// 	Computed: true,
								// },
								"frame_rate_limiter": schema.Int32Attribute{
									Optional: true,
									Computed: true,
								},
								"disable_vnc": schema.BoolAttribute{
									Optional: true,
									Computed: true,
								},
								"enable_uvm": schema.BoolAttribute{
									Optional: true,
									Computed: true,
								},
								"enable_debugging": schema.BoolAttribute{
									Optional: true,
									Computed: true,
								},
								"enable_profiling": schema.BoolAttribute{
									Optional: true,
									Computed: true,
								},
							},
						},
					},
				},
			},
		},
	}
}

// UpgradeState handles state migrations between schema versions.
// Version 0→1: disksize in vergeio_drive changed from Int64 to Float64
// to support fractional GB sizes (e.g., 8.5 GB imported disks).
func (r *VMResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: nil,
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				// JSON numbers are untyped, so an integer value like 10 in the
				// prior state is automatically valid as float64 (10.0) in the
				// new schema. Terraform will refresh the resource on the next
				// plan, which re-reads from the API with the new float64 type.
				tflog.Info(ctx, "Upgrading VM state from v0 to v1: disksize changed from int64 to float64")
			},
		},
	}
}

// validateMachineType validates that the machine_type value is supported by the VergeOS API
func (r *VMResource) validateMachineType(ctx context.Context, machineType types.String) error {
	// If machine_type is null or unknown, skip validation
	if machineType.IsNull() || machineType.IsUnknown() {
		return nil
	}

	value := machineType.ValueString()
	if value == "" {
		return nil
	}

	// Fetch valid machine types from API
	machineTypes, err := r.vmApi.GetMachineTypesFromAPI(ctx)
	if err != nil {
		return fmt.Errorf("unable to fetch valid machine types from VergeOS API: %w", err)
	}

	// Check if the value is valid
	for _, mt := range machineTypes {
		if value == mt {
			return nil
		}
	}

	return fmt.Errorf("machine type '%s' is not supported by this VergeOS version", value)
}

// validateDiskInterface validates that the disk interface value is supported by the VergeOS API
func (r *VMResource) validateDiskInterface(ctx context.Context, diskInterface types.String) error {
	// If interface is null or unknown, skip validation
	if diskInterface.IsNull() || diskInterface.IsUnknown() {
		return nil
	}

	value := diskInterface.ValueString()
	if value == "" {
		return nil
	}

	// Fetch valid disk interfaces from API
	diskInterfaces, err := r.diskApi.GetDiskInterfacesFromAPI(ctx)
	if err != nil {
		return fmt.Errorf("unable to fetch valid disk interfaces from VergeOS API: %w", err)
	}

	// Check if the value is valid
	for _, di := range diskInterfaces {
		if value == di {
			return nil
		}
	}

	return fmt.Errorf("disk interface '%s' is not supported by this VergeOS version", value)
}

// validateDrives validates all drives and their interfaces
func (r *VMResource) validateDrives(ctx context.Context, drives []*diskResourceModel) error {
	for i, drive := range drives {
		if err := r.validateDiskInterface(ctx, drive.Interface); err != nil {
			return fmt.Errorf("drive %d: %w", i, err)
		}
	}
	return nil
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
	r.deviceApi = NewDeviceApi(client)
}

// Create a new VM.
func (r *VMResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data VMResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Validate machine type against VergeOS API
	if err := r.validateMachineType(ctx, data.MachineType); err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("machine_type"),
			"Invalid Machine Type",
			err.Error(),
		)
		return
	}

	// Validate drives and their interfaces against VergeOS API
	if err := r.validateDrives(ctx, data.Disks); err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("vergeio_drive"),
			"Invalid Drive Interface",
			err.Error(),
		)
		return
	}

	// Let's presever the desired power state
	// as it will be used later to power on the VM if needed.
	desiredPowerState := false // Default to false
	if !data.PowerState.IsNull() {
		desiredPowerState = data.PowerState.ValueBool()
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

	// Create devices
	if data.Devices != nil {
		for _, device := range data.Devices {
			device.Machine = data.Machine

			createError := r.deviceApi.createDevice(ctx, device)
			if createError != nil {
				tflog.Debug(ctx, fmt.Sprintf("Error creating device %v", createError))
				resp.Diagnostics.AddError(
					"Error creating device",
					createError.Error(),
				)
				return
			}
		}
	}

	// Now turn the power on if the power state is true
	if desiredPowerState {
		tflog.Debug(ctx, "Powering on the VM")
		if powerOnError := r.vmApi.powerOnVM(ctx, &data); powerOnError != nil {
			resp.Diagnostics.AddError(
				"Error powering on VM",
				powerOnError.Error(),
			)
			return
		}

		// Detach cloud-init after power-on if cloud-init files were configured.
		// The cloud-init files are already attached to the VM at creation time,
		// so the guest will read them during boot regardless. Deleting the
		// cloud-init files prevents the VM from depending on them on subsequent
		// boots.
		if data.CloudInitFiles != nil && len(data.CloudInitFiles) > 0 {
			tflog.Debug(ctx, "Waiting 1 second before detaching cloud-init")
			time.Sleep(1 * time.Second)

			if detachError := r.vmApi.detachCloudInit(ctx, &data); detachError != nil {
				resp.Diagnostics.AddError(
					"Error detaching cloud-init",
					detachError.Error(),
				)
				return
			}
		}
	}

	// First get both guest agent info and timeout values
	waitForGuestAgentInfo := data.WaitForGuestAgentInfo.ValueInt32()
	waitForGuestIPTimeout := data.WaitForGuestIPTimeout.ValueInt32()
	toIgnoreCidr := data.IgnoredGuestIPs.ValueString()

	ipFound := false                 // to break the loop when IP is found
	var retryCheckInterval int32 = 5 // seconds
	var i int32 = 0

	// run a loop until the max of var1 and var2
	for i = 0; i <= max(waitForGuestAgentInfo, waitForGuestIPTimeout); i++ {
		time.Sleep(1 * time.Second)

		// check if i is a multiple of retryCheckInterval and less than waitForGuestIPTimeout
		if i%retryCheckInterval == 0 && i < waitForGuestIPTimeout && !ipFound {
			fmt.Println("Checking the IP address after ", i, " seconds")

			// Read the guest agent info.
			if readError := r.vmApi.readGuestAgentInfo(ctx, &data, toIgnoreCidr); readError != nil {
				resp.Diagnostics.AddError(
					"Error reading guest agent info",
					readError.Error(),
				)
				return
			}

			// Check if the IP address is found
			if len(data.GuestAgentIPs.Elements()) > 0 {
				ipFound = true
				waitForGuestIPTimeout = 0
			}
		}

		// call the waitForGuestAgentInfo function when i = waitForGuestAgentInfo
		if i == waitForGuestAgentInfo {
			fmt.Println("Checking the guest agent info after ", i, " seconds")
			// Read the guest agent info.
			if readError := r.vmApi.readGuestAgentInfo(ctx, &data, ""); readError != nil {
				resp.Diagnostics.AddError(
					"Error reading guest agent info",
					readError.Error(),
				)
				return
			}
		}

		fmt.Println(i)
	}

	// Preserve cloud-init config values before the final read.
	// If we detached cloud-init, the API now returns "none" but Terraform
	// expects the planned value ("nocloud"/"config_drive_v2") for consistency.
	// On subsequent Read() calls, ignore_changes prevents drift.
	plannedCloudInitDS := data.CloudInitDataSource
	plannedCloudInitFiles := data.CloudInitFiles

	// read the final state of the VM
	if readError := r.vmApi.readVM(ctx, &data); readError != nil {
		resp.Diagnostics.AddError(
			"Error reading the VM",
			readError.Error(),
		)
		return
	}

	// Restore planned cloud-init values so Terraform's consistency check passes
	data.CloudInitDataSource = plannedCloudInitDS
	data.CloudInitFiles = plannedCloudInitFiles

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read VM information.
func (r *VMResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data VMResourceModel

	// Read Terraform prior state data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	tflog.Debug(ctx, fmt.Sprintf("auto read %v", ctx))

	if resp.Diagnostics.HasError() {
		return
	}

	// Read data into the model to get all the attributes
	readDataError := r.vmApi.readVM(ctx, &data)

	if readDataError != nil {
		// if the resource was not found, likely deleted outside of terraform
		// remove the resource from the state
		// and return
		if strings.Contains(readDataError.Error(), "not found") {
			resp.State.RemoveResource(ctx)
			return
		}

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

	// Validate machine type against VergeOS API
	if err := r.validateMachineType(ctx, planData.MachineType); err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("machine_type"),
			"Invalid Machine Type",
			err.Error(),
		)
		return
	}

	// Validate drives and their interfaces against VergeOS API
	if err := r.validateDrives(ctx, planData.Disks); err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root("vergeio_drive"),
			"Invalid Drive Interface",
			err.Error(),
		)
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

	tflog.Debug(ctx, fmt.Sprintf("Updating the devices with the plan data %v", planData.Devices))
	// update devices
	if planData.Devices != nil && stateData.Devices != nil {
		tflog.Debug(ctx, "Syncing devices ran")
		if err := r.deviceApi.syncDevices(ctx, &planData.Devices, &stateData.Devices, stateData.Machine); err != nil {
			resp.Diagnostics.AddError(
				"Error syncing devices",
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

	// Reconcile machine_type: if the plan value is semantically equivalent to what
	// readVM set, use the plan value. This handles the case where a user explicitly
	// changes from a short form (q35) to an expanded form (pc-q35-10.0).
	if !planData.MachineType.IsNull() && !planData.MachineType.IsUnknown() {
		planVal := planData.MachineType.ValueString()
		stateVal := stateData.MachineType.ValueString()
		if planVal != stateVal &&
			(machineTypesAreEquivalent(planVal, stateVal) || machineTypesAreEquivalent(stateVal, planVal)) {
			stateData.MachineType = planData.MachineType
		}
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

	// make sure the VM is in a power state that can be deleted
	var currentPowerState *bool
	var err error

	if currentPowerState, err = r.vmApi.isVMRunning(ctx, data.Id.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Failed to check Power State before deletion:",
			err.Error(),
		)
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Current vm power state is %v", *currentPowerState))

	// If the VM is running, we need to power it off before deletion
	if *currentPowerState {
		tflog.Debug(ctx, "VM is running, powering it off before deletion")
		// Power the vm off
		if err := r.vmApi.killVM(ctx, &data); err != nil {
			resp.Diagnostics.AddError(
				"Failed to kill VM before deletion:",
				err.Error(),
			)
			return
		}
	}

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
