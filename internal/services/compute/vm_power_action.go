// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

const (
	vmPowerShutdown = "shutdown"
	vmPowerReset    = "reset"
	vmPowerOn       = "power_on"
)

var (
	_ action.Action                   = &VMPowerAction{}
	_ action.ActionWithConfigure      = &VMPowerAction{}
	_ action.ActionWithValidateConfig = &VMPowerAction{}
)

func NewVMPowerAction() action.Action {
	return &VMPowerAction{}
}

// VMPowerAction changes VM power outside vergeio_vm.powerstate.
// A later plan of that resource restores the declared power state when it is set.
type VMPowerAction struct {
	sdk *vergeos.Client
	api *VMApi
}

type vmPowerActionModel struct {
	VMID           types.String `tfsdk:"vm_id"`
	Operation      types.String `tfsdk:"operation"`
	TimeoutSeconds types.Int64  `tfsdk:"timeout_seconds"`
	Force          types.Bool   `tfsdk:"force"`
}

func (a *VMPowerAction) Metadata(ctx context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vm_power"
}

func (a *VMPowerAction) Schema(ctx context.Context, req action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Shut down, reset, or power on a VergeOS VM without changing vergeio_vm.powerstate. shutdown sends one ACPI poweroff and waits until the guest stops. reset is the reset button. power_on posts poweron when the machine is not running and waits until it is running. The VM powerstate column is not used for that decision. A later plan of vergeio_vm restores a declared powerstate. Requires Terraform 1.14 or later. OpenTofu does not implement actions, and no resource behavior depends on this action.",
		Attributes: map[string]schema.Attribute{
			"vm_id": schema.StringAttribute{
				MarkdownDescription: "VM id, the same value as vergeio_vm.id.",
				Required:            true,
			},
			"operation": schema.StringAttribute{
				MarkdownDescription: "shutdown, reset, or power_on.",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.OneOf(vmPowerShutdown, vmPowerReset, vmPowerOn),
				},
			},
			"timeout_seconds": schema.Int64Attribute{
				MarkdownDescription: "How long shutdown waits for the guest to stop, in seconds. When omitted, the client power-wait timeout is used (150 seconds unless the client changed it). Only used when operation is shutdown.",
				Optional:            true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"force": schema.BoolAttribute{
				MarkdownDescription: "When shutdown is still running at timeout_seconds, kill the VM and wait again for the same timeout. A cancelled run does not kill. Only used when operation is shutdown. Defaults to false when omitted.",
				Optional:            true,
			},
		},
	}
}

func (a *VMPowerAction) Configure(ctx context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	sdk, diags := shared.ActionSDK(req.ProviderData)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	a.sdk = sdk
	if sdk == nil {
		return
	}
	client, ok := req.ProviderData.(*vergeio.Client)
	if !ok {
		return
	}
	api, err := NewVMApi(client)
	if err != nil {
		resp.Diagnostics.AddError("Unable to Create VergeOS API Client", err.Error())
		return
	}
	a.api = api
}

func (a *VMPowerAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var data vmPowerActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, _, err := shared.KnownPositiveID(data.VMID, "vm_id"); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("vm_id"), "Invalid VM ID", err.Error())
	}
	if data.Operation.IsNull() || data.Operation.IsUnknown() || data.Operation.ValueString() == vmPowerShutdown {
		return
	}
	if !data.TimeoutSeconds.IsNull() && !data.TimeoutSeconds.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("timeout_seconds"),
			"Invalid VM Power Option",
			"timeout_seconds is only used when operation is shutdown.",
		)
	}
	if !data.Force.IsNull() && !data.Force.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("force"),
			"Invalid VM Power Option",
			"force is only used when operation is shutdown.",
		)
	}
}

func (a *VMPowerAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var data vmPowerActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if a.sdk == nil {
		resp.Diagnostics.AddError("VergeOS Client Missing", "The provider has not been configured.")
		return
	}
	vmID, known, err := shared.KnownPositiveID(data.VMID, "vm_id")
	if err != nil || !known {
		if err == nil {
			err = fmt.Errorf("vm_id is required")
		}
		resp.Diagnostics.AddAttributeError(path.Root("vm_id"), "Invalid VM ID", err.Error())
		return
	}
	if data.Operation.IsNull() || data.Operation.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("operation"), "Invalid VM Power Operation", "operation is required")
		return
	}
	operation := data.Operation.ValueString()
	shared.ReportProgress(resp, fmt.Sprintf("VM %d %s", vmID, operation))
	switch operation {
	case vmPowerShutdown:
		err = a.sdk.VMs.PowerOffWithOptions(ctx, vmID, powerOffOptions(data))
	case vmPowerReset:
		err = a.sdk.VMs.Reset(ctx, vmID)
	case vmPowerOn:
		err = a.powerOn(ctx, vmID)
	default:
		err = fmt.Errorf("operation %q is not shutdown, reset, or power_on", operation)
	}
	if err != nil {
		resp.Diagnostics.AddError("Error Changing VM Power", err.Error())
	}
}

// powerOn posts poweron when the machine is stopped. VMService.PowerOn
// returns without posting when the powerstate column is already true.
func (a *VMPowerAction) powerOn(ctx context.Context, vmID int) error {
	if a.api == nil {
		return errors.New("missing API client")
	}
	id := strconv.Itoa(vmID)
	running, _, err := a.api.readVMPowerStatus(ctx, id)
	if err != nil {
		return err
	}
	if running {
		return nil
	}
	if err := a.api.postVMAction(ctx, id, vmActionPowerOn); err != nil {
		return err
	}
	return a.api.waitForVMPower(ctx, id, true, vmPowerWaitTimeout, powerOnInterval)
}

func powerOffOptions(data vmPowerActionModel) *vergeos.VMPowerOffOptions {
	var opts *vergeos.VMPowerOffOptions
	if !data.TimeoutSeconds.IsNull() && !data.TimeoutSeconds.IsUnknown() {
		opts = &vergeos.VMPowerOffOptions{
			Timeout: time.Duration(data.TimeoutSeconds.ValueInt64()) * time.Second,
		}
	}
	if !data.Force.IsNull() && !data.Force.IsUnknown() && data.Force.ValueBool() {
		if opts == nil {
			opts = &vergeos.VMPowerOffOptions{}
		}
		opts.ForceAfterTimeout = true
	}
	return opts
}
