// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

const (
	// ACPI shutdown. VMService.PowerOff sends kill instead, so callers post
	// this action themselves.
	vmActionPowerOff = "poweroff"
	vmActionKill     = "kill"
)

// gracefulShutdownTimeout is how long a powerstate=false update, and a
// destroy or replace, waits after one ACPI poweroff. Guests without ACPI
// need force_power_off on update. Destroy falls back to kill unless
// shutdown_on_destroy is graceful. Tests that go through gracefulPowerOff
// shorten gracefulShutdownInterval.
var (
	gracefulShutdownTimeout  = 2 * time.Minute
	gracefulShutdownInterval = 2 * time.Second
)

const (
	// shutdownOnDestroyGracefulThenKill sends one ACPI poweroff, waits
	// timeouts.delete, then kills a guest that is still running.
	shutdownOnDestroyGracefulThenKill = "graceful_then_kill"
	// shutdownOnDestroyGraceful sends one ACPI poweroff and fails destroy
	// if the guest does not stop. It does not kill.
	shutdownOnDestroyGraceful = "graceful"
	// shutdownOnDestroyKill sends kill and does not wait for ACPI shutdown.
	shutdownOnDestroyKill = "kill"
)

// vmTimeoutsModel is the timeouts block. Update is the graceful poweroff
// wait when powerstate changes to false. Delete is the wait before destroy
// or replace falls back from that poweroff.
type vmTimeoutsModel struct {
	Update types.String `tfsdk:"update"`
	Delete types.String `tfsdk:"delete"`
}

// shutdownTimeoutFromPlan reads timeouts.update. An unset value uses
// gracefulShutdownTimeout.
func shutdownTimeoutFromPlan(plan *VMResourceModel) (time.Duration, error) {
	if plan == nil || plan.Timeouts == nil {
		return gracefulShutdownTimeout, nil
	}
	return parseShutdownTimeout(plan.Timeouts.Update, "timeouts.update")
}

// deleteTimeoutFromState reads timeouts.delete. An unset value uses
// gracefulShutdownTimeout, the same 2 minute default as an update.
func deleteTimeoutFromState(data *VMResourceModel) (time.Duration, error) {
	if data == nil || data.Timeouts == nil {
		return gracefulShutdownTimeout, nil
	}
	return parseShutdownTimeout(data.Timeouts.Delete, "timeouts.delete")
}

func parseShutdownTimeout(value types.String, name string) (time.Duration, error) {
	if value.IsNull() || value.IsUnknown() {
		return gracefulShutdownTimeout, nil
	}
	raw := value.ValueString()
	timeout, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("%s %q is not a duration: %w", name, raw, err)
	}
	if timeout < 0 {
		return 0, fmt.Errorf("%s must not be negative", name)
	}
	return timeout, nil
}

// shutdownOnDestroyMode reads shutdown_on_destroy. Null and unknown use
// graceful_then_kill so a VM stored before the attribute existed still gets
// an ACPI shutdown on destroy.
func shutdownOnDestroyMode(data *VMResourceModel) (string, error) {
	if data == nil || data.ShutdownOnDestroy.IsNull() || data.ShutdownOnDestroy.IsUnknown() {
		return shutdownOnDestroyGracefulThenKill, nil
	}
	switch mode := data.ShutdownOnDestroy.ValueString(); mode {
	case shutdownOnDestroyGracefulThenKill, shutdownOnDestroyGraceful, shutdownOnDestroyKill:
		return mode, nil
	default:
		return "", fmt.Errorf("shutdown_on_destroy %q must be %s, %s, or %s",
			mode, shutdownOnDestroyGracefulThenKill, shutdownOnDestroyGraceful, shutdownOnDestroyKill)
	}
}

// stopVMForDelete stops a running VM before Delete removes it. Replace uses
// the same path. The default posts one ACPI poweroff through gracefulPowerOff
// and, if the guest is still running when timeouts.delete elapses, kills it.
// A guest that ignores ACPI waits out that timeout. graceful fails instead of
// killing. kill skips the ACPI shutdown and cuts power immediately. A VM that
// is already stopped is left alone.
func (r *VMResource) stopVMForDelete(ctx context.Context, data *VMResourceModel) error {
	mode, err := shutdownOnDestroyMode(data)
	if err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("shutdown_on_destroy for VM %s is %s", data.Id.ValueString(), mode))
	if mode == shutdownOnDestroyKill {
		return r.killRunningVM(ctx, data)
	}

	timeout, err := deleteTimeoutFromState(data)
	if err != nil {
		return err
	}
	// Post poweroff and poll. Do not call VMService.PowerOff: it sends kill.
	return r.vmApi.gracefulPowerOff(ctx, data.Id.ValueString(), data.Name.ValueString(), timeout, mode == shutdownOnDestroyGracefulThenKill)
}

// killRunningVM is the previous destroy behavior: kill a VM that is still
// running, and leave a stopped VM alone.
func (r *VMResource) killRunningVM(ctx context.Context, data *VMResourceModel) error {
	currentPowerState, err := r.vmApi.isVMRunning(ctx, data.Id.ValueString())
	if err != nil {
		return fmt.Errorf("checking power state before deletion: %w", err)
	}
	if currentPowerState == nil {
		return errors.New("checking power state before deletion: empty power state")
	}
	tflog.Debug(ctx, fmt.Sprintf("Current vm power state is %v", *currentPowerState))
	if !*currentPowerState {
		return nil
	}
	tflog.Debug(ctx, "VM is running, killing it before deletion")
	if err := r.vmApi.killVM(ctx, data); err != nil {
		return fmt.Errorf("killing VM before deletion: %w", err)
	}
	return nil
}

// durationValidator checks that a timeout is a Go duration, such as 90s or 2m.
type durationValidator struct{}

func (durationValidator) Description(context.Context) string {
	return `value must be a duration such as "90s" or "2m"`
}

func (v durationValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (durationValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := time.ParseDuration(strings.TrimSpace(req.ConfigValue.ValueString())); err != nil {
		resp.Diagnostics.AddAttributeError(
			req.Path,
			"Invalid timeout",
			fmt.Sprintf("Timeout %q is not a duration. Use a value such as \"90s\" or \"2m\".", req.ConfigValue.ValueString()),
		)
	}
}

// gracefulPowerOff sends one ACPI poweroff and polls until the VM stops.
// Destroy can call this with its own timeout and force flag. It posts the
// poweroff action itself and does not call VMService.PowerOff.
func (va *VMApi) gracefulPowerOff(ctx context.Context, vmID, vmName string, timeout time.Duration, force bool) error {
	return gracefulShutdown(ctx, vmID, vmName, timeout, gracefulShutdownInterval, force,
		func(ctx context.Context, action string) error {
			return va.postVMAction(ctx, vmID, action)
		},
		func(ctx context.Context) (bool, string, error) {
			return va.readVMPowerStatus(ctx, vmID)
		},
	)
}

// gracefulShutdown posts one ACPI poweroff, then polls until the VM is
// stopped or the timeout passes. send is called with "poweroff" once. It is
// called with "kill" only when force is true and the guest is still running
// at the deadline. A stopped VM is left alone. Context cancel returns
// ctx.Err() and does not kill.
func gracefulShutdown(ctx context.Context, vmID, vmName string, timeout, interval time.Duration, force bool, send func(context.Context, string) error, read func(context.Context) (running bool, status string, err error)) error {
	label := vmShutdownLabel(vmName, vmID)
	if err := ctx.Err(); err != nil {
		return err
	}
	if timeout < 0 {
		timeout = 0
	}

	running, status, err := read(ctx)
	if err != nil {
		return fmt.Errorf("reading power status for VM %s: %w", label, err)
	}
	if !running {
		tflog.Debug(ctx, fmt.Sprintf("VM %s is already stopped", label))
		return nil
	}

	tflog.Debug(ctx, fmt.Sprintf("Sending poweroff to VM %s", label))
	if err := send(ctx, vmActionPowerOff); err != nil {
		return fmt.Errorf("sending poweroff to VM %s: %w", label, err)
	}

	last, stopped, err := waitForVMStopped(ctx, read, timeout, interval)
	if err != nil {
		return shutdownReadError(label, err)
	}
	if stopped {
		return nil
	}
	if last == "" {
		last = powerStatusLabel(status, true)
	}
	if !force {
		return fmt.Errorf("VM %s stayed %q after poweroff and did not stop before the timeout", label, last)
	}

	tflog.Debug(ctx, fmt.Sprintf("Graceful poweroff of VM %s timed out at %q; sending kill", label, last))
	if err := send(ctx, vmActionKill); err != nil {
		return fmt.Errorf("VM %s stayed %q after poweroff; kill failed: %w", label, last, err)
	}

	last, stopped, err = waitForVMStopped(ctx, read, timeout, interval)
	if err != nil {
		return shutdownReadError(label, err)
	}
	if stopped {
		return nil
	}
	if last == "" {
		last = "running"
	}
	return fmt.Errorf("VM %s stayed %q after poweroff and kill", label, last)
}

func shutdownReadError(label string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("reading power status for VM %s: %w", label, err)
}

// waitForVMStopped polls until the VM is stopped or the deadline passes.
// The first read is immediate. A timeout returns the last status and
// stopped=false. A read or context error is returned as-is.
func waitForVMStopped(ctx context.Context, read func(context.Context) (bool, string, error), timeout, interval time.Duration) (string, bool, error) {
	deadline := time.Now().Add(timeout)
	var last string
	for {
		if err := ctx.Err(); err != nil {
			return last, false, err
		}
		running, status, err := read(ctx)
		if err != nil {
			return last, false, err
		}
		last = powerStatusLabel(status, running)
		if !running {
			return last, true, nil
		}
		if !time.Now().Before(deadline) {
			return last, false, nil
		}
		wait := interval
		if interval > 0 {
			if remaining := time.Until(deadline); remaining < wait {
				wait = remaining
			}
		}
		if err := sleepContext(ctx, wait); err != nil {
			return last, false, err
		}
	}
}

func powerStatusLabel(status string, running bool) string {
	status = strings.TrimSpace(status)
	if status != "" {
		return status
	}
	if running {
		return "running"
	}
	return "stopped"
}

func vmShutdownLabel(name, id string) string {
	name = strings.TrimSpace(name)
	id = strings.TrimSpace(id)
	switch {
	case name != "" && id != "":
		return fmt.Sprintf("%q (id %s)", name, id)
	case name != "":
		return fmt.Sprintf("%q", name)
	case id != "":
		return fmt.Sprintf("id %s", id)
	default:
		return "unknown"
	}
}

// postVMAction posts one vm_actions action. Poweroff and kill both go
// through here so neither calls VMService.PowerOff.
func (va *VMApi) postVMAction(ctx context.Context, vmID, action string) error {
	if va == nil || va.client == nil {
		return errors.New("missing API client")
	}
	id, err := strconv.Atoi(strings.TrimSpace(vmID))
	if err != nil {
		return fmt.Errorf("invalid VM ID format: %v", err)
	}
	body, err := json.Marshal(VMAction{
		VM:     int32(id),
		Action: action,
	})
	if err != nil {
		return err
	}
	resp, err := va.client.Post(ctx, VMActionEndpoint, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}
	if !vergeio.SuccessStatus(resp.StatusCode) {
		return fmt.Errorf("vm action %s for VM %s failed: status code %d", action, vmID, resp.StatusCode)
	}
	return nil
}

type vmPowerStatusBody struct {
	Status     string          `json:"status"`
	Running    *bool           `json:"running"`
	PowerState *bool           `json:"powerstate"`
	Machine    json.RawMessage `json:"machine"`
}

func (b vmPowerStatusBody) powerStatus() (running bool, status string, ok bool) {
	nestedStatus, nestedRunning := b.nestedMachineStatus()
	switch {
	case b.Running != nil:
		running = *b.Running
		ok = true
	case b.PowerState != nil:
		running = *b.PowerState
		ok = true
	case nestedRunning != nil:
		running = *nestedRunning
		ok = true
	}
	if !ok {
		return false, "", false
	}
	status = strings.TrimSpace(b.Status)
	if status == "" {
		status = strings.TrimSpace(nestedStatus)
	}
	return running, powerStatusLabel(status, running), true
}

func (b vmPowerStatusBody) nestedMachineStatus() (string, *bool) {
	trimmed := bytes.TrimSpace(b.Machine)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return "", nil
	}
	var machine struct {
		Status *struct {
			Status  string `json:"status"`
			Running *bool  `json:"running"`
		} `json:"status"`
	}
	if err := json.Unmarshal(trimmed, &machine); err != nil || machine.Status == nil {
		return "", nil
	}
	return machine.Status.Status, machine.Status.Running
}

// readVMPowerStatus reads the machine status string and whether it is running.
// The fields query asks for the status alias. A full VM payload that only
// has powerstate is accepted too, including when machine is a numeric id.
func (va *VMApi) readVMPowerStatus(ctx context.Context, vmID string) (bool, string, error) {
	if va == nil || va.client == nil {
		return false, "", errors.New("missing API client")
	}
	vmID = strings.TrimSpace(vmID)
	if vmID == "" {
		return false, "", errors.New("missing VM id")
	}
	apiResp, err := va.client.Get(ctx, vergeio.ObjectPath(VMEndpoint, vmID), &vergeio.Options{Fields: "machine#status#status as status,machine#status#running as running"})
	if err != nil {
		return false, "", err
	}
	if apiResp == nil || apiResp.Body == nil {
		return false, "", errors.New("missing response from the API")
	}
	defer func() { _ = apiResp.Body.Close() }()

	var body vmPowerStatusBody
	if err := json.NewDecoder(apiResp.Body).Decode(&body); err != nil {
		return false, "", fmt.Errorf("invalid VM power status response: %w", err)
	}
	running, status, ok := body.powerStatus()
	if !ok {
		return false, "", errors.New("VM power status response did not include a power state")
	}
	return running, status, nil
}
