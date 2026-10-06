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

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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
var _ resource.ResourceWithModifyPlan = &VMResource{}
var _ resource.ResourceWithUpgradeState = &VMResource{}
var _ resource.ResourceWithValidateConfig = &VMResource{}

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

// cloudInitExpectLivePrivateKey marks that configured cloudinit_files should
// still exist in VergeOS. Create sets it when powerstate is false (or files
// were configured without a power-on detach). Power-on detach leaves it unset
// so refresh keeps the planned list when VergeOS has none. An external wipe
// of every file clears state only when this key is set. Read also seeds it
// when VergeOS still has attached files and the key is absent (2.x upgrade
// or import), so those VMs detect a later full wipe without a Create/Update.
const cloudInitExpectLivePrivateKey = "cloudinitExpectLive"

// shouldSeedCloudInitExpectLive reports whether Read should write the
// expect-live private marker. Live files with no marker means the VM came
// from upgrade or import; power-on detach leaves live empty so stays false.
func shouldSeedCloudInitExpectLive(expectLive, livePresent bool) bool {
	return !expectLive && livePresent
}

// CloudInitFile represents a cloud-init file with name and contents.
type CloudInitFile struct {
	Name              types.String `tfsdk:"name"`
	Contents          types.String `tfsdk:"contents"`
	ContentsWO        types.String `tfsdk:"contents_wo"`
	ContentsWOVersion types.Int64  `tfsdk:"contents_wo_version"`
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
	ConsolePassWO         types.String     `tfsdk:"console_pass_wo"`
	ConsolePassWOVersion  types.Int64      `tfsdk:"console_pass_wo_version"`
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
	// BootDisk is the one drive this resource owns. Other drives and every NIC
	// are vergeio_vm_drive and vergeio_vm_nic.
	BootDisk              *bootDiskModel         `tfsdk:"boot_disk"`
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
		// Version 4 drops inline vergeio_drive and vergeio_nic blocks.
		// The VM keeps an optional boot_disk. Drives and NICs are separate resources.
		// Version 3 stored drive preferred_tier as a number from 1 to 5.
		// Version 2 stored preferred_tier as a string and stored cluster,
		// preferred_node, and snapshot_profile as numbers.
		// Version 1 stored those keys as strings and widened disksize to float.
		// Version 0 stored disksize as an integer.
		Version: 4,

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
				MarkdownDescription: "Machine type, validated against the VergeOS system. q35 matches an expanded pc-q35 type and pc matches an expanded pc-i440fx type. Import stores the expanded type, and that pair does not plan a change once the boot disk is adopted.",
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
				MarkdownDescription: "Console password. The API does not return this value, so Terraform stores the configured value. Deprecated: use console_pass_wo so the password is not stored. Changing it updates the VM in place.",
				Optional:            true,
				Computed:            true,
				Sensitive:           true,
				DeprecationMessage:  "Deprecated. Terraform stores this value in state through v3.x. It will be removed in v4. Use console_pass_wo and console_pass_wo_version. Increment console_pass_wo_version to change the console password. That update does not replace the VM. console_pass_wo requires Terraform 1.11 or OpenTofu 1.11.",
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("console_pass_wo")),
				},
			},
			"console_pass_wo": schema.StringAttribute{
				MarkdownDescription: "Console password sent on create, and again when console_pass_wo_version changes. Terraform does not store it. The API does not return it. The VM is updated in place. Requires Terraform 1.11 or OpenTofu 1.11. Do not set console_pass as well.",
				Optional:            true,
				WriteOnly:           true,
				Sensitive:           true,
				Validators: []validator.String{
					stringvalidator.ConflictsWith(path.MatchRoot("console_pass")),
					stringvalidator.AlsoRequires(path.MatchRoot("console_pass_wo_version")),
				},
			},
			"console_pass_wo_version": schema.Int64Attribute{
				MarkdownDescription: "Version of console_pass_wo. Increment it to set a new console password. Terraform stores this number, not the password. Changing console_pass_wo without changing this version does not update the VM. The VM is not replaced.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AlsoRequires(path.MatchRoot("console_pass_wo")),
				},
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
				MarkdownDescription: "Key of a snapshot profile. Set this to tonumber(vergeio_snapshot_profile.example.id). Omit to leave the current profile unchanged.",
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
							MarkdownDescription: "File body stored in state. Deprecated when the file holds a secret: use contents_wo. One of contents or contents_wo is required.",
							Optional:            true,
							DeprecationMessage:  "Deprecated. Terraform stores this file body in state through v3.x. It will be removed in v4. Use contents_wo and contents_wo_version for a body that must stay out of state, including a secret. Increment contents_wo_version to write the body again. contents_wo requires Terraform 1.11 or OpenTofu 1.11.",
							Validators: []validator.String{
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("contents_wo")),
							},
						},
						"contents_wo": schema.StringAttribute{
							MarkdownDescription: "File body sent on create, and again when contents_wo_version changes. Terraform does not store it. Use this when the file contains a secret. Requires Terraform 1.11 or OpenTofu 1.11. Do not set contents as well.",
							Optional:            true,
							WriteOnly:           true,
							Sensitive:           true,
							Validators: []validator.String{
								stringvalidator.ConflictsWith(path.MatchRelative().AtParent().AtName("contents")),
								stringvalidator.AlsoRequires(path.MatchRelative().AtParent().AtName("contents_wo_version")),
							},
						},
						"contents_wo_version": schema.Int64Attribute{
							MarkdownDescription: "Version of contents_wo. Increment it to write the file body again. Terraform stores this number, not the body. Changing contents_wo without changing this version does not update the file.",
							Optional:            true,
							Validators: []validator.Int64{
								int64validator.AlsoRequires(path.MatchRelative().AtParent().AtName("contents_wo")),
							},
						},
					},
				},
			},
			"powerstate": schema.BoolAttribute{
				MarkdownDescription: "Whether the VM is powered on. Default = False on create, which leaves a new VM stopped. Apply powers the VM on, or sends one ACPI poweroff, when this differs from the VM's current power, including a power change made in the VergeOS UI. The provider reads the VM again after that change so state matches. Powering off waits until the guest stops. It does not hard-kill the VM. If the guest is still running when timeouts.update elapses (default 2 minutes), apply fails unless force_power_off is true. Omit this attribute to leave the current power unchanged.",
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
			"boot_disk": schema.SingleNestedBlock{
				MarkdownDescription: "Boot disk owned by this VM. Set size, and source when the disk is cloned or imported from media. The provider matches an existing drive by name and adopts it instead of creating a second disk. Other drives belong to vergeio_vm_drive. Do not give a vergeio_vm_drive the same name. Changing media or source on an owned boot disk replaces the VM. Adding media or source when adopting an existing disk after import or a 2.x upgrade does not.",
				Attributes: map[string]schema.Attribute{
					"key": schema.StringAttribute{
						MarkdownDescription: "Drive key assigned by VergeOS.",
						Computed:            true,
						// UseStateForUnknown copies a null prior value when this
						// block is added to a VM that is already in state. Apply
						// then stores the drive key and Terraform rejects the
						// plan as inconsistent. UseNonNullStateForUnknown keeps
						// a key that is already known and leaves a new key unknown.
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.UseNonNullStateForUnknown(),
						},
					},
					"name": schema.StringAttribute{
						MarkdownDescription: "Drive name used to adopt an existing disk. Defaults to boot. Set this to the current drive name when moving a single-disk VM off the old vergeio_drive block.",
						Optional:            true,
						Computed:            true,
						Default:             stringdefault.StaticString("boot"),
					},
					"size": schema.Float64Attribute{
						MarkdownDescription: "Boot disk size in GB. Supports fractional values.",
						Optional:            true,
						Computed:            true,
					},
					"source": schema.Int32Attribute{
						MarkdownDescription: "Media source id used to clone or import the boot disk. Changing source on an owned boot disk replaces the VM. Setting source when adopting after import or a 2.x upgrade does not.",
						Optional:            true,
						PlanModifiers: []planmodifier.Int32{
							bootDiskSourceRequiresReplace(),
						},
					},
					"media": schema.StringAttribute{
						MarkdownDescription: "Media type. Defaults to a new disk when omitted. Changing media on an owned boot disk replaces the VM. Setting media when adopting after import or a 2.x upgrade does not.",
						Optional:            true,
						PlanModifiers: []planmodifier.String{
							bootDiskMediaRequiresReplace(),
						},
						Validators: []validator.String{
							stringvalidator.OneOf(getValidDiskMedia()...),
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
							// UseStateForUnknown copies the device at this index,
							// including null when the block is new. Adding a
							// device, or inserting one above another, then
							// stores a different key and Terraform rejects the
							// plan. Copy the key from the device with the same
							// name, and leave a new device unknown.
							PlanModifiers: []planmodifier.String{
								deviceComputedModifier{},
							},
						},
						"machine": schema.Int32Attribute{
							Computed: true,
							Optional: true,
							PlanModifiers: []planmodifier.Int32{
								deviceComputedModifier{},
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
									// Same name match as the device key. A TPM
									// block added with the device must stay
									// unknown instead of copying null.
									PlanModifiers: []planmodifier.Int32{
										deviceComputedModifier{},
									},
								},
								"machine_device": schema.Int32Attribute{
									Optional: true,
									Computed: true,
									PlanModifiers: []planmodifier.Int32{
										deviceComputedModifier{},
									},
								},
								"model": schema.StringAttribute{
									Optional: true,
									Computed: true,
								},
								"version": schema.StringAttribute{
									MarkdownDescription: "TPM version. Display labels `\"2.0\"` and `\"1.2\"` are stored as `\"2\"` and `\"1\"`. Sent when the device is created. VergeOS treats version as read-only afterward, so updates omit it. Changing version on an existing device replaces the VM so create can send the new value. Adding a TPM device is still an in-place update.",
									Optional:            true,
									Computed:            true,
									CustomType:          TPMVersionType{},
									PlanModifiers: []planmodifier.String{
										tpmVersionRequiresReplace(),
									},
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
// Version 3 stores preferred_tier as a number and still nests drives and NICs.
// Version 4 removes those nested blocks. The drives and NICs stay in VergeOS.
// boot_disk is left null so the next apply adopts a configured boot disk by
// name instead of deleting every drive that used to be inline. Every prior
// version upgrades straight to the current schema. Numeric strings are
// accepted by the number decoder; blank strings become null so an unset key
// does not fail the upgrade. A preferred_tier that is not an integer still
// fails the upgrade before the drive list is dropped.
func (r *VMResource) UpgradeState(ctx context.Context) map[int64]resource.StateUpgrader {
	upgrader := func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
		tflog.Info(ctx, "Upgrading VM state to v4: numeric ids, then drop inline drives and NICs")

		normalized, err := upgradeVMStateJSON(req.RawState.JSON)
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
		3: {StateUpgrader: upgrader},
	}
}

// upgradeVMStateJSON normalizes prior VM state and removes the inline drive
// and NIC lists. Those objects are not deleted in VergeOS. vergeio_vm_drive
// and vergeio_vm_nic adopt them by name, and boot_disk adopts the one disk
// the VM still owns.
func upgradeVMStateJSON(raw []byte) ([]byte, error) {
	normalized, err := normalizeVMStateJSON(raw)
	if err != nil {
		return nil, err
	}
	var state map[string]json.RawMessage
	if err := json.Unmarshal(normalized, &state); err != nil {
		return nil, err
	}
	delete(state, "vergeio_drive")
	delete(state, "vergeio_nic")
	state["boot_disk"] = json.RawMessage("null")
	return json.Marshal(state)
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

	vmApi, err := NewVMApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	diskApi, err := NewDiskApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	nicApi, err := NewNICApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	deviceApi, err := NewDeviceApi(client)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Create VergeOS API Client",
			err.Error(),
		)
		return
	}
	r.vmApi = vmApi
	r.diskApi = diskApi
	r.nicApi = nicApi
	r.deviceApi = deviceApi
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

	// Let's presever the desired power state
	// as it will be used later to power on the VM if needed.
	desiredPowerState := false // Default to false
	if !data.PowerState.IsNull() {
		desiredPowerState = data.PowerState.ValueBool()
	}
	// readVM replaces cloud-init files with the live rows. Create still
	// stores the plan: power-on deletes the files after the guest boots,
	// and the apply has to match the configuration. Clone before write-only
	// secrets are copied on, so the saved list does not contain them.
	plannedCloudInitFiles := cloneCloudInitFiles(data.CloudInitFiles)
	var config VMResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	applyVMWriteOnly(&data, nil, &config)

	// Create a new VM. A name collision means a previous create left the VM
	// in VergeOS without writing state. Adopt it when it matches this plan.
	createError := r.vmApi.CreateVM(ctx, &data)
	if createError != nil && vmNameInUse(createError) {
		tflog.Debug(ctx, fmt.Sprintf("VM %q is already in use; looking for an orphan to adopt", data.Name.ValueString()))
		if err := r.adoptOrphanVM(ctx, &data); err != nil {
			// Lookup can fail after the id is known. Keep that id.
			r.createError(ctx, resp, &data, "Error creating VM", err.Error())
			return
		}
		createError = nil
	}
	if createError != nil {
		// CreateVM sets the id once the VM row exists. A later read can
		// still fail. Keep that id so the VM is not orphaned.
		r.createError(ctx, resp, &data, "Error creating VM", createError.Error())
		return
	}

	// Log the id only. The model includes console_pass.
	tflog.Debug(ctx, fmt.Sprintf("created a vm resource %s", data.Id.ValueString()))

	// Store the id before the boot disk, devices, and power-on. Terraform
	// keeps this state when a later step returns an error, and the next
	// plan updates or replaces the VM instead of creating another one.
	r.rememberVM(ctx, resp, &data)
	if resp.Diagnostics.HasError() {
		resp.Diagnostics.AddError("Error saving VM state", vmNotInStateDetail(&data))
		return
	}

	// Create the boot disk before devices and power-on so a new VM boots from it.
	// NICs and extra drives are their own resources and hotplug when the VM is already running.
	if data.BootDisk != nil {
		boot, bootErr := r.syncBootDisk(ctx, data.BootDisk, nil, data.Machine, data.Id)
		// A failed read can still return the drive once its key exists.
		if boot != nil {
			data.BootDisk = boot
		}
		if bootErr != nil {
			r.createError(ctx, resp, &data, "Error creating boot disk", bootErr.Error())
			return
		}
		r.rememberVM(ctx, resp, &data)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Create devices. Each one is stored after it succeeds so a later device
	// or power-on error does not drop the keys already assigned.
	if data.Devices != nil {
		for _, device := range data.Devices {
			device.Machine = data.Machine

			if deviceErr := r.deviceApi.createDevice(ctx, device); deviceErr != nil {
				tflog.Debug(ctx, fmt.Sprintf("Error creating device %v", deviceErr))
				// createDevice sets the key before the follow-up read. That
				// key is on this device, so the checkpoint keeps it.
				r.createError(ctx, resp, &data, "Error creating device", deviceErr.Error())
				return
			}
			r.rememberVM(ctx, resp, &data)
			if resp.Diagnostics.HasError() {
				return
			}
		}
	}

	// Now turn the power on if the power state is true
	if desiredPowerState {
		tflog.Debug(ctx, "Powering on the VM")
		if powerOnError := r.vmApi.powerOnVM(ctx, &data); powerOnError != nil {
			r.createError(ctx, resp, &data, "Error powering on VM", powerOnError.Error())
			return
		}
		// The row was read before power-on, so it still says stopped. Store
		// the running state before detach or the guest-agent wait can fail.
		data.PowerState = types.BoolValue(true)
		r.rememberVM(ctx, resp, &data)
		if resp.Diagnostics.HasError() {
			return
		}

		// Detach cloud-init after power-on if cloud-init files were configured.
		// The cloud-init files are already attached to the VM at creation time,
		// so the guest will read them during boot regardless. Deleting the
		// cloud-init files prevents the VM from depending on them on subsequent
		// boots.
		if len(plannedCloudInitFiles) > 0 {
			tflog.Debug(ctx, "Waiting before detaching cloud-init")
			if err := sleepContext(ctx, cloudInitDetachDelay); err != nil {
				r.createError(ctx, resp, &data, "Error detaching cloud-init", err.Error())
				return
			}

			if detachError := r.vmApi.detachCloudInit(ctx, &data); detachError != nil {
				r.createError(ctx, resp, &data, "Error detaching cloud-init", detachError.Error())
				return
			}
			// Detach cleared VergeOS. Leave cloudInitExpectLive unset so refresh
			// keeps the planned list when the live list is empty.
		}
	} else if len(plannedCloudInitFiles) > 0 {
		// Files stay attached until an external change or a later update.
		if resp.Private != nil {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, cloudInitExpectLivePrivateKey, []byte("true"))...)
			if resp.Diagnostics.HasError() {
				r.rememberVM(ctx, resp, &data)
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
				r.createError(ctx, resp, &data, "Error reading guest agent info", readError.Error())
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
				r.createError(ctx, resp, &data, "Error reading guest agent info", readError.Error())
				return
			}
		}

		fmt.Println(i)
	}

	// Preserve the cloud-init datasource before the final read.
	// If we detached cloud-init, the API now returns "none" but Terraform
	// expects the planned value ("nocloud"/"config_drive_v2") for consistency.
	// On subsequent Read() calls, ignore_changes prevents drift.
	// File rows are the plan captured above. The final read would otherwise
	// store the live list, which is empty after detach. Private state leaves
	// cloudInitExpectLive unset after detach so a later refresh keeps that
	// list when VergeOS has no rows, and the next plan does not create them
	// again. When create left the files attached, expect-live is set so an
	// external wipe of every file is still detected.
	plannedCloudInitDS := data.CloudInitDataSource

	// read the final state of the VM
	if _, readError := r.vmApi.readVM(ctx, &data, false); readError != nil {
		r.createError(ctx, resp, &data, "Error reading the VM", readError.Error())
		return
	}

	// Restore planned cloud-init values so Terraform's consistency check passes
	data.CloudInitDataSource = plannedCloudInitDS
	data.CloudInitFiles = plannedCloudInitFiles
	scrubVMWriteOnly(&data)

	if err := r.refreshBootDisk(ctx, &data); err != nil {
		r.createError(ctx, resp, &data, "Error reading boot disk", err.Error())
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
	if r.vmApi == nil {
		resp.Diagnostics.AddError("Error Fetching Data", "VM client is not configured")
		return
	}

	expectLive := false
	if req.Private != nil {
		raw, diags := req.Private.GetKey(ctx, cloudInitExpectLivePrivateKey)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		expectLive = string(raw) == "true"
	}

	// Read data into the model to get all the attributes
	livePresent, readDataError := r.vmApi.readVM(ctx, &data, expectLive)

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

	// Seed expect-live when files are still attached but the marker was never
	// set (UpgradeState cannot write Private; import starts without it). The
	// next external wipe of every file then clears state like a 3.0 create.
	// Power-on detach leaves live empty, so this does not arm #185's empty plan.
	if shouldSeedCloudInitExpectLive(expectLive, livePresent) && resp.Private != nil {
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, cloudInitExpectLivePrivateKey, []byte("true"))...)
		if resp.Diagnostics.HasError() {
			return
		}
	}

	// Import only sets id, so boot_disk stays empty until configuration adopts
	// a drive by name. Refreshing here must not pull every drive on the VM
	// into this resource.
	if err := r.refreshBootDisk(ctx, &data); err != nil {
		resp.Diagnostics.AddError(
			"Error reading boot disk",
			err.Error(),
		)
		return
	}

	scrubVMWriteOnly(&data)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update a VM.
func (r *VMResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var planData, stateData, config VMResourceModel

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &planData)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)

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

	tflog.Debug(ctx, fmt.Sprintf("Updating VM %s", planData.Name.ValueString()))

	// Read Terraform plan data into the model
	resp.Diagnostics.Append(req.State.Get(ctx, &stateData)...)

	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, fmt.Sprintf("Updating VM state %s", stateData.Id.ValueString()))

	cloudInitEqualBefore := cloudInitFilesEqual(planData.CloudInitFiles, stateData.CloudInitFiles)

	// Write-only secrets are null in the plan. Copy them onto a clone when
	// their version changed. The plan saved below stays without those secrets.
	apiPlan := planData
	apiPlan.CloudInitFiles = cloneCloudInitFiles(planData.CloudInitFiles)
	applyVMWriteOnly(&apiPlan, &stateData, &config)

	// Update VM attributes only. Power is applied after the boot disk
	// sync so a VM started in this apply boots with that disk.
	// Extra drives and NICs are separate resources. They hotplug when the
	// VM is already running.
	if updateError := r.vmApi.UpdateVM(ctx, &apiPlan, &stateData); updateError != nil {
		resp.Diagnostics.AddError(
			"Error updating VM",
			updateError.Error(),
		)
		return
	}

	boot, bootErr := r.syncBootDisk(ctx, planData.BootDisk, stateData.BootDisk, stateData.Machine, stateData.Id)
	if bootErr != nil {
		resp.Diagnostics.AddError(
			"Error syncing boot disk",
			bootErr.Error(),
		)
		return
	}
	stateData.BootDisk = boot

	// Power on only after the boot disk exists. A failed sync skips
	// the power change so the VM is not started without that disk.
	if err := r.vmApi.applyPlannedPowerState(ctx, &planData, &stateData); err != nil {
		resp.Diagnostics.AddError(
			"Error updating VM power state",
			err.Error(),
		)
		return
	}
	// Pause so a following read can see guest state after a power change.
	// Tests set vmUpdateSettle to zero. The power helpers already wait
	// until machine status matches the plan.
	time.Sleep(vmUpdateSettle)

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
	if _, readError := r.vmApi.readVM(ctx, &stateData, false); readError != nil {
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

	if err := r.refreshBootDisk(ctx, &stateData); err != nil {
		resp.Diagnostics.AddError(
			"Error reading boot disk",
			err.Error(),
		)
		return
	}

	// The file rows were written above. readVM loads those bodies, possibly
	// in API order. Saving the plan keeps the configured order and leaves
	// a write-only body out of state.
	usePlannedCloudInitFiles(&stateData, &planData)
	scrubVMWriteOnly(&stateData)

	// A cloud-init sync that wrote or removed rows updates whether refresh
	// should treat an empty live list as drift. Detach-only creates leave
	// expect-live unset; recreating files after drift sets it again.
	if !cloudInitEqualBefore && resp.Private != nil {
		if len(planData.CloudInitFiles) > 0 {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, cloudInitExpectLivePrivateKey, []byte("true"))...)
		} else {
			resp.Diagnostics.Append(resp.Private.SetKey(ctx, cloudInitExpectLivePrivateKey, nil)...)
		}
		if resp.Diagnostics.HasError() {
			return
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
	if req.ID == "" {
		resp.Diagnostics.AddError(
			"Invalid VM Import ID",
			"Import vergeio_vm with the VM id.",
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
