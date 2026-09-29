// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int32validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int32planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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
	Id                    types.String     `tfsdk:"id"`
	Machine               types.Int32      `tfsdk:"machine"`
	Name                  types.String     `tfsdk:"name"`
	Cluster               types.Int32      `tfsdk:"cluster"`
	Description           types.String     `tfsdk:"description"`
	Enabled               types.Bool       `tfsdk:"enabled"`
	MachineType           types.String     `tfsdk:"machine_type"`
	AllowHotplug          types.Bool       `tfsdk:"allow_hotplug"`
	DisablePowercycle     types.Bool       `tfsdk:"disable_powercycle"`
	OnPowerLoss           types.String     `tfsdk:"on_power_loss"`
	CPUCores              types.Int32      `tfsdk:"cpu_cores"`
	CPUType               types.String     `tfsdk:"cpu_type"`
	RAM                   types.Int32      `tfsdk:"ram"`
	Console               types.String     `tfsdk:"console"`
	Display               types.String     `tfsdk:"display"`
	Video                 types.String     `tfsdk:"video"`
	Sound                 types.String     `tfsdk:"sound"`
	OSFamily              types.String     `tfsdk:"os_family"`
	OSDescription         types.String     `tfsdk:"os_description"`
	RTCBase               types.String     `tfsdk:"rtc_base"`
	BootOrder             types.String     `tfsdk:"boot_order"`
	ConsolePassEnabled    types.Bool       `tfsdk:"console_pass_enabled"`
	ConsolePass           types.String     `tfsdk:"console_pass"`
	USBTablet             types.Bool       `tfsdk:"usb_tablet"`
	UEFI                  types.Bool       `tfsdk:"uefi"`
	SecureBoot            types.Bool       `tfsdk:"secure_boot"`
	SerialPort            types.Bool       `tfsdk:"serial_port"`
	BootDelay             types.Int32      `tfsdk:"boot_delay"`
	PreferredNode         types.Int32      `tfsdk:"preferred_node"`
	SnapshotProfile       types.Int32      `tfsdk:"snapshot_profile"`
	CloudInitDataSource   types.String     `tfsdk:"cloudinit_datasource"`
	HAGroup               types.String     `tfsdk:"ha_group"`
	CloudInitFiles        []CloudInitFile  `tfsdk:"cloudinit_files"`
	PowerState            types.Bool       `tfsdk:"powerstate"`
	ForcePowerOff         types.Bool       `tfsdk:"force_power_off"`
	ShutdownOnDestroy     types.String     `tfsdk:"shutdown_on_destroy"`
	Timeouts              *vmTimeoutsModel `tfsdk:"timeouts"`
	GuestAgent            types.Bool       `tfsdk:"guest_agent"`
	Advanced              types.String     `tfsdk:"advanced"`
	WaitForGuestAgentInfo types.Int32      `tfsdk:"wait_for_guest_agent_info"`
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
		// Version 3: drive preferred_tier is a number from 1 to 5.
		// Version 2 stored preferred_tier as a string and stored cluster,
		// preferred_node, and snapshot_profile as numbers.
		// Version 1 stored those keys as strings and widened disksize to float.
		// Version 0 stored disksize as an integer.
		Version: 3,

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
				// Assigned once. Without this, every VM update plans it as unknown.
				PlanModifiers: []planmodifier.Int32{
					int32planmodifier.UseStateForUnknown(),
				},
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
					shared.MachineTypeSemanticEquality(),
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
			"on_power_loss": schema.StringAttribute{
				MarkdownDescription: "What to do on power loss",
				Validators: []validator.String{
					// Validate string value must be "power_on", "leave_off", or "last_state"
					stringvalidator.OneOf([]string{"power_on", "leave_off", "last_state"}...),
				},
				Optional: true,
				Computed: true,
			},
			"cpu_cores": schema.Int32Attribute{
				MarkdownDescription: "CPU cores. When omitted on create, the VM is created with 1 core.",
				Optional:            true,
				Computed:            true,
			},
			"cpu_type": schema.StringAttribute{
				MarkdownDescription: "CPU type",
				Optional:            true,
				Computed:            true,
			},
			"ram": schema.Int32Attribute{
				MarkdownDescription: "RAM in MiB. When omitted on create, the VM is created with 1024 MiB.",
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
				MarkdownDescription: "Console password. The API does not return this value, so Terraform stores the configured value.",
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
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
			"preferred_node": schema.Int32Attribute{
				MarkdownDescription: "Preferred node",
				Optional:            true,
				Computed:            true,
			},
			"snapshot_profile": schema.Int32Attribute{
				MarkdownDescription: "Snapshot profile",
				Optional:            true,
				Computed:            true,
			},
			"cluster": schema.Int32Attribute{
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
				MarkdownDescription: "Whether the VM is powered on. On update, false sends one ACPI poweroff and waits until the guest stops. It does not hard-kill the VM. If the guest is still running when timeouts.update elapses (default 2 minutes), apply fails unless force_power_off is true. Omit this attribute to leave the current power unchanged.",
				Optional:            true,
				Computed:            true,
				// Keep the prior value when configuration omits powerstate.
				// Otherwise the framework marks it unknown on every update.
				PlanModifiers: []planmodifier.Bool{
					boolplanmodifier.UseStateForUnknown(),
				},
			},
			"force_power_off": schema.BoolAttribute{
				MarkdownDescription: "Kill the VM if a graceful ACPI poweroff does not stop it before the update timeout. Defaults to false. Guests without ACPI support need this set to true. This does not change destroy; use shutdown_on_destroy for that.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
			"shutdown_on_destroy": schema.StringAttribute{
				MarkdownDescription: "How destroy and replace stop a running VM. graceful_then_kill (default) sends one ACPI poweroff and waits timeouts.delete (default 2 minutes), then kills the VM if it is still running. graceful waits the same way and fails destroy if the guest does not stop. kill sends kill immediately. A guest that ignores ACPI waits out the full delete timeout before the kill fallback.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString(shutdownOnDestroyGracefulThenKill),
				Validators: []validator.String{
					stringvalidator.OneOf([]string{
						shutdownOnDestroyGracefulThenKill,
						shutdownOnDestroyGraceful,
						shutdownOnDestroyKill,
					}...),
				},
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
			"timeouts": schema.SingleNestedBlock{
				MarkdownDescription: "How long to wait for a VM power change. update applies when powerstate changes to false. delete applies when a running VM is destroyed or replaced.",
				Attributes: map[string]schema.Attribute{
					"update": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "How long to wait for a graceful ACPI poweroff when powerstate changes to false. A duration such as \"90s\" or \"2m\". Defaults to 2m when unset.",
						Validators: []validator.String{
							durationValidator{},
						},
					},
					"delete": schema.StringAttribute{
						Optional:            true,
						MarkdownDescription: "How long destroy and replace wait for a graceful ACPI poweroff before the shutdown_on_destroy fallback. A duration such as \"90s\" or \"2m\". Defaults to 2m when unset. A guest that ignores ACPI stays running for this whole wait before kill.",
						Validators: []validator.String{
							durationValidator{},
						},
					},
				},
			},
			"vergeio_nic": schema.ListNestedBlock{
				MarkdownDescription: "NICs for the VM. A NIC added while the VM is running is hotplugged and brought up. A NIC added in the same apply that powers the VM on is created before power-on. If hotplug is refused, apply fails and the VM must be power cycled before the guest sees the NIC.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							MarkdownDescription: "NIC id assigned by VergeOS. Renaming the NIC keeps this id. Changing a configured id replaces the VM.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								// Computed ids become unknown on update unless copied
								// from state. syncNICs pairs NICs by this id, so a
								// rename must keep it in the plan.
								stringplanmodifier.UseStateForUnknown(),
								stringplanmodifier.RequiresReplaceIfConfigured(),
							},
						},
						"machine": schema.Int32Attribute{
							Optional: true,
							Computed: true,
							// The NIC stays on the same machine across an in-place VM update.
							PlanModifiers: []planmodifier.Int32{
								int32planmodifier.UseStateForUnknown(),
							},
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "NIC name. Renaming the NIC updates it in place and keeps its id and MAC address.",
							Optional:            true,
							Computed:            true,
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
							Optional: true,
							Computed: true,
						},
						"macaddress": schema.StringAttribute{
							MarkdownDescription: "MAC address. Renaming the NIC keeps this address.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								// Copies state only when configuration omits the address.
								// A configured value, including an unknown one, is left alone.
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
				},
			},
			// Nested blocks for drives
			"vergeio_drive": schema.ListNestedBlock{
				MarkdownDescription: "Drives for the VM. A drive added while the VM is running is hotplugged. A drive added in the same apply that powers the VM on is created before power-on. Interfaces that cannot be hotplugged, such as IDE, stay offline until the VM is power cycled, and apply fails with that error.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							MarkdownDescription: "Drive key assigned by VergeOS. Renaming the drive keeps this key. Changing a configured key replaces the VM.",
							Optional:            true,
							Computed:            true,
							PlanModifiers: []planmodifier.String{
								// Computed keys become unknown on update unless copied
								// from state. syncDisks pairs drives by this key, so a
								// rename must keep it in the plan.
								stringplanmodifier.UseStateForUnknown(),
								stringplanmodifier.RequiresReplaceIfConfigured(),
							},
						},
						"machine": schema.Int32Attribute{
							Computed: true,
							Optional: true,
							// The drive stays on the same machine across an in-place VM update.
							PlanModifiers: []planmodifier.Int32{
								int32planmodifier.UseStateForUnknown(),
							},
						},
						"name": schema.StringAttribute{
							MarkdownDescription: "Drive name. Renaming the drive updates it in place and keeps its key.",
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
							MarkdownDescription: "Media type. Changing media replaces the VM, because an existing drive cannot change media.",
							Optional:            true,
							// Computed: true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.RequiresReplace(),
							},
							Validators: []validator.String{
								// Validate string value must be one of the allowed values
								stringvalidator.OneOf(getValidDiskMedia()...),
							},
						},
						"media_source": schema.Int32Attribute{
							MarkdownDescription: "Source used to create a cloned, imported, or CD-ROM drive. Changing media_source replaces the VM.",
							Optional:            true,
							// Computed: true,
							PlanModifiers: []planmodifier.Int32{
								int32planmodifier.RequiresReplace(),
							},
						},
						"disksize": schema.Float64Attribute{
							Optional: true,
							Computed: true,
						},
						"preferred_tier": schema.Int32Attribute{
							MarkdownDescription: "Storage tier from 1 to 5. VergeOS assigns the system default when this is omitted, and a later VM update does not move the drive.",
							Optional:            true,
							Computed:            true,
							// VergeOS assigns the system default tier when this is omitted,
							// and a later VM update does not move the drive.
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
				},
			},
			// Nested blocks for devices
			"vergeio_device": schema.ListNestedBlock{
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Optional: true,
							Computed: true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"machine": schema.Int32Attribute{
							Computed: true,
							Optional: true,
							PlanModifiers: []planmodifier.Int32{
								int32planmodifier.UseStateForUnknown(),
							},
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
									PlanModifiers: []planmodifier.Int32{
										int32planmodifier.UseStateForUnknown(),
									},
								},
								"machine_device": schema.Int32Attribute{
									Optional: true,
									Computed: true,
									PlanModifiers: []planmodifier.Int32{
										int32planmodifier.UseStateForUnknown(),
									},
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
									PlanModifiers: []planmodifier.Int32{
										int32planmodifier.UseStateForUnknown(),
									},
								},
								"machine_device": schema.Int32Attribute{
									Optional: true,
									Computed: true,
									PlanModifiers: []planmodifier.Int32{
										int32planmodifier.UseStateForUnknown(),
									},
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
									PlanModifiers: []planmodifier.Int32{
										int32planmodifier.UseStateForUnknown(),
									},
								},
								"machine_device": schema.Int32Attribute{
									Optional: true,
									Computed: true,
									PlanModifiers: []planmodifier.Int32{
										int32planmodifier.UseStateForUnknown(),
									},
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
//
// Version 0 stored disksize as an integer and cluster, preferred_node, and
// snapshot_profile as strings. Version 1 widened disksize to float (JSON
// numbers need no rewrite) and still stored those keys as strings. Version 2
// stores those keys as numbers and preferred_tier as a string such as "4".
// Version 3 stores preferred_tier as a number. Every prior version upgrades
// straight to the current schema. Numeric strings are accepted by the number
// decoder; blank strings become null so an unset key does not fail the upgrade.
func (r *VMResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	upgrader := func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
		tflog.Info(ctx, "Upgrading VM state to v3: numeric cluster, preferred_node, snapshot_profile, and drive preferred_tier")

		normalized, err := normalizeVMStateJSON(req.RawState.JSON)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error upgrading VM state",
				fmt.Sprintf("Failed to normalize state: %s", err),
			)
			return
		}

		newSchemaType := resp.State.Schema.Type().TerraformType(ctx)
		newStateValue, err := tftypes.ValueFromJSON(normalized, newSchemaType)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error upgrading VM state",
				fmt.Sprintf("Failed to parse state: %s", err),
			)
			return
		}

		resp.State.Raw = newStateValue
	}

	return map[int64]resource.StateUpgrader{
		0: {StateUpgrader: upgrader},
		1: {StateUpgrader: upgrader},
		2: {StateUpgrader: upgrader},
	}
}

// normalizeVMStateJSON rewrites prior VM state so it matches schema version 3.
// cluster, preferred_node, snapshot_profile, and each drive preferred_tier
// may be JSON strings. A blank string becomes null; a decimal integer string
// becomes a JSON number. Values that are already numbers are left alone.
func normalizeVMStateJSON(raw []byte) ([]byte, error) {
	var state map[string]json.RawMessage
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, err
	}

	for _, key := range []string{"cluster", "preferred_node", "snapshot_profile"} {
		rawVal, ok := state[key]
		if !ok {
			continue
		}
		converted, err := numericIDOrNull(rawVal)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		state[key] = converted
	}
	if err := convertDrivePreferredTiers(state); err != nil {
		return nil, err
	}

	return json.Marshal(state)
}

// convertDrivePreferredTiers rewrites vergeio_drive[].preferred_tier from the
// string stored by schema versions 0-2 into the number stored by version 3.
func convertDrivePreferredTiers(state map[string]json.RawMessage) error {
	rawVal, ok := state["vergeio_drive"]
	if !ok {
		return nil
	}
	trimmed := bytes.TrimSpace(rawVal)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}

	var drives []map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &drives); err != nil {
		return fmt.Errorf("vergeio_drive: %w", err)
	}
	for i := range drives {
		rawTier, ok := drives[i]["preferred_tier"]
		if !ok {
			continue
		}
		converted, err := numericIDOrNull(rawTier)
		if err != nil {
			return fmt.Errorf("vergeio_drive[%d].preferred_tier: %w", i, err)
		}
		drives[i]["preferred_tier"] = converted
	}
	encoded, err := json.Marshal(drives)
	if err != nil {
		return err
	}
	state["vergeio_drive"] = encoded
	return nil
}

func numericIDOrNull(raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return json.RawMessage("null"), nil
	}
	if trimmed[0] != '"' {
		return json.RawMessage(trimmed), nil
	}

	var s string
	if err := json.Unmarshal(trimmed, &s); err != nil {
		return nil, err
	}
	if s == "" {
		return json.RawMessage("null"), nil
	}
	if _, err := strconv.ParseInt(s, 10, 32); err != nil {
		return nil, fmt.Errorf("cannot convert %q to an integer id", s)
	}
	return json.RawMessage(s), nil
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

	// Create a new VM. A name collision means a previous create left the VM
	// in VergeOS without writing state. Adopt it when it matches this plan.
	createError := r.vmApi.CreateVM(ctx, &data)
	if createError != nil && vmNameInUse(createError) {
		tflog.Debug(ctx, fmt.Sprintf("VM %q is already in use; looking for an orphan to adopt", data.Name.ValueString()))
		if err := r.adoptOrphanVM(ctx, &data); err != nil {
			r.rememberVM(ctx, resp, &data)
			resp.Diagnostics.AddError(
				"Error creating VM",
				err.Error(),
			)
			return
		}
		createError = nil
	}
	if createError != nil {
		// CreateVM sets the id once the VM row exists. A later read can
		// still fail. Keep that id so the VM is not orphaned.
		r.rememberVM(ctx, resp, &data)
		resp.Diagnostics.AddError(
			"Error creating VM",
			createError.Error(),
		)
		return
	}

	// Log the id only. The model includes console_pass.
	tflog.Debug(ctx, fmt.Sprintf("created a vm resource %s", data.Id.ValueString()))

	// Store the id before drives, NICs, and devices. Terraform keeps this
	// state when a later step fails or the apply is interrupted, and the
	// next apply updates the VM instead of colliding on the name.
	r.rememberVM(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError(
			"Error saving VM state",
			fmt.Sprintf("VM %q (id %s) exists in VergeOS but could not be stored in Terraform state. Import it with `terraform import vergeio_vm.<name> %s`, or delete it in VergeOS and apply again.", data.Name.ValueString(), data.Id.ValueString(), data.Id.ValueString()),
		)
		return
	}

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

	// Store drives and NICs from the same read used after import, so the
	// state written here matches a later refresh.
	if err := r.readDrivesAndNICs(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Drives and NICs",
			err.Error(),
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

	// Drives and NICs live in machine_drives and machine_nics, keyed by the
	// VM's machine id. ImportState only sets id, so this is what puts the
	// nested blocks back into state. Without it the next plan adds devices
	// that already exist and apply fails the consistency check.
	if err := r.readDrivesAndNICs(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Drives and NICs",
			err.Error(),
		)
		return
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// readDrivesAndNICs replaces nested drive and NIC blocks from the API.
// Attributes that the device APIs do not return are copied from prior state.
func (r *VMResource) readDrivesAndNICs(ctx context.Context, data *VMResourceModel) error {
	if data.Machine.IsNull() || data.Machine.IsUnknown() {
		return nil
	}

	priorDisks := data.Disks
	priorNICs := data.NICs
	machineID := data.Machine.ValueInt32()

	disks, err := r.diskApi.readDisksByMachine(ctx, machineID)
	if err != nil {
		return err
	}
	nics, err := r.nicApi.readNICsByMachine(ctx, machineID)
	if err != nil {
		return err
	}

	preserveDiskConfigFields(priorDisks, disks)
	preserveNICConfigFields(priorNICs, nics)
	data.Disks = disks
	data.NICs = nics
	return nil
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

	tflog.Debug(ctx, fmt.Sprintf("Updating VM %s", planData.Name.ValueString()))

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &stateData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Updating VM state %s", stateData.Id.ValueString()))

	// Update VM attributes only. Power is applied after the drive and NIC
	// sync so a VM started in this apply boots with the new devices.
	// Drives and NICs created while the VM is already running are hotplugged
	// inside the sync.
	if updateError := r.vmApi.UpdateVM(ctx, &planData, &stateData); updateError != nil {
		resp.Diagnostics.AddError(
			"Error updating VM",
			updateError.Error(),
		)
		return
	}

	// An empty nested list decodes to a nil slice. Sync still has to run so
	// the first drive, NIC, or device can be added and the last one removed.
	tflog.Debug(ctx, fmt.Sprintf("Updating the disk with the plan data %v", planData.Disks))
	tflog.Debug(ctx, "Syncing disks ran")
	diskErr := r.diskApi.syncDisks(ctx, &planData.Disks, &stateData.Disks, stateData.Machine, stateData.Id)
	if diskErr != nil {
		resp.Diagnostics.AddError(
			"Error syncing disks",
			diskErr.Error(),
		)
	}

	nicErr := r.nicApi.syncNICs(ctx, &planData.NICs, &stateData.NICs, stateData.Machine, stateData.Id)
	if nicErr != nil {
		resp.Diagnostics.AddError(
			"Error syncing NICs",
			nicErr.Error(),
		)
	}

	// Power on only after the new drives and NICs exist. A failed sync skips
	// the power change so the VM is not started without those devices.
	if diskErr == nil && nicErr == nil {
		if err := r.vmApi.applyPlannedPowerState(ctx, &planData, &stateData); err != nil {
			resp.Diagnostics.AddError(
				"Error updating VM power state",
				err.Error(),
			)
			return
		}
		// UpdateVM used to wait here so a following read sees the VM running.
		time.Sleep(5 * time.Second)
	}

	tflog.Debug(ctx, fmt.Sprintf("Updating the devices with the plan data %v", planData.Devices))
	tflog.Debug(ctx, "Syncing devices ran")
	if err := r.deviceApi.syncDevices(ctx, &planData.Devices, &stateData.Devices, stateData.Machine); err != nil {
		resp.Diagnostics.AddError(
			"Error syncing devices",
			err.Error(),
		)
	}

	// readVM refreshes prior state. console_pass is not in the API response,
	// so the planned value has to be on the model before that read. Leaving
	// the previous password in place would fail the apply consistency check
	// whenever the configuration changes it.
	usePlannedConsolePass(&stateData, &planData)
	// force_power_off and timeouts are provider settings. The API read does
	// not return them, so the planned values have to be copied onto state.
	usePlannedShutdownSettings(&stateData, &planData)

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
			(shared.MachineTypesAreEquivalent(planVal, stateVal) || shared.MachineTypesAreEquivalent(stateVal, planVal)) {
			stateData.MachineType = planData.MachineType
		}
	}

	if err := r.readDrivesAndNICs(ctx, &stateData); err != nil {
		resp.Diagnostics.AddError(
			"Error Fetching Drives and NICs",
			err.Error(),
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

	// Replace, taint, and force-new all run this Delete on the old VM.
	tflog.Debug(ctx, fmt.Sprintf("Deleting VM %s", data.Id.ValueString()))
	if err := r.stopVMForDelete(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Failed to shut down VM before deletion:",
			err.Error(),
		)
		return
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
