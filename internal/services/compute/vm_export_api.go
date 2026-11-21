// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

// exportAPI keeps one volume_vm_exports row. It does not start an export.
// A run is a procedure and stays with vergeio.vergeos.vm_export.
type exportAPI struct {
	sdk *vergeos.Client
}

func newExportAPI(c *vergeio.Client) (*exportAPI, error) {
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	return &exportAPI{sdk: sdk}, nil
}

type exportModel struct {
	ID            types.String `tfsdk:"id"`
	Volume        types.String `tfsdk:"volume"`
	VolumeName    types.String `tfsdk:"volume_name"`
	Quiesced      types.Bool   `tfsdk:"quiesced"`
	CreateCurrent types.Bool   `tfsdk:"create_current"`
	MaxExports    types.Int64  `tfsdk:"max_exports"`
	Status        types.String `tfsdk:"status"`
	StatusInfo    types.String `tfsdk:"status_info"`
}

func validateExportModel(data *exportModel) diag.Diagnostics {
	var diags diag.Diagnostics
	if data == nil {
		diags.AddError("Missing VM export", "The export configuration was empty.")
		return diags
	}
	if stringSet(data.Volume) && !hexKeyPattern.MatchString(strings.TrimSpace(data.Volume.ValueString())) {
		diags.AddAttributeError(path.Root("volume"), "Invalid volume key", "volume is the NAS volume key, a 40 character hex string.")
	}
	if intSet(data.MaxExports) {
		n := data.MaxExports.ValueInt64()
		if n < 1 || n > 100 {
			diags.AddAttributeError(path.Root("max_exports"), "Invalid export retention", "max_exports must be from 1 to 100.")
		}
	}
	return diags
}

func (a *exportAPI) create(ctx context.Context, data *exportModel) error {
	volume := strings.TrimSpace(stringValue(data.Volume))
	if volume == "" {
		return fmt.Errorf("volume is required")
	}
	req := &vergeos.VMExportCreateRequest{
		Volume:        volume,
		Quiesced:      vergeio.KnownBool(data.Quiesced),
		CreateCurrent: vergeio.KnownBool(data.CreateCurrent),
	}
	if intSet(data.MaxExports) {
		n := int(data.MaxExports.ValueInt64())
		req.MaxExports = &n
	}
	created, err := a.sdk.VMExports.Create(ctx, req)
	if err != nil {
		return err
	}
	if created == nil || created.Key.Int() <= 0 {
		return fmt.Errorf("VergeOS did not return a volume_vm_exports id for volume %s", volume)
	}
	data.ID = types.StringValue(strconv.Itoa(created.Key.Int()))
	return applyExport(data, created)
}

func (a *exportAPI) read(ctx context.Context, data *exportModel) error {
	id, err := exportID(data.ID)
	if err != nil {
		return err
	}
	row, err := a.sdk.VMExports.Get(ctx, id)
	if err != nil {
		return err
	}
	if row == nil || row.Key.Int() <= 0 {
		return &vergeos.NotFoundError{Resource: "VMExport", ID: id}
	}
	if volume := strings.TrimSpace(stringValue(data.Volume)); volume != "" && row.Volume != volume {
		return &vergeos.NotFoundError{Resource: "VMExport", ID: id}
	}
	return applyExport(data, row)
}

func (a *exportAPI) update(ctx context.Context, plan, state *exportModel) error {
	id, err := exportID(state.ID)
	if err != nil {
		return err
	}
	current, err := a.sdk.VMExports.Get(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return &vergeos.NotFoundError{Resource: "VMExport", ID: id}
	}
	if volume := strings.TrimSpace(stringValue(state.Volume)); volume != "" && current.Volume != volume {
		return &vergeos.NotFoundError{Resource: "VMExport", ID: id}
	}
	req := &vergeos.VMExportUpdateRequest{}
	changed := false
	if boolChanged(plan.Quiesced, state.Quiesced) {
		req.Quiesced = vergeio.KnownBool(plan.Quiesced)
		changed = true
	}
	if boolChanged(plan.CreateCurrent, state.CreateCurrent) {
		req.CreateCurrent = vergeio.KnownBool(plan.CreateCurrent)
		changed = true
	}
	if intChanged(plan.MaxExports, state.MaxExports) {
		if !intSet(plan.MaxExports) {
			return fmt.Errorf("max_exports cannot be cleared after it is set")
		}
		n := int(plan.MaxExports.ValueInt64())
		req.MaxExports = &n
		changed = true
	}
	plan.ID = state.ID
	if !changed {
		return a.read(ctx, plan)
	}
	updated, err := a.sdk.VMExports.Update(ctx, id, req)
	if err != nil {
		return err
	}
	return applyExport(plan, updated)
}

func (a *exportAPI) delete(ctx context.Context, id int) error {
	if err := a.sdk.VMExports.Delete(ctx, id); err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	return nil
}

func applyExport(data *exportModel, row *vergeos.VMExport) error {
	if data == nil || row == nil || row.Key.Int() <= 0 {
		return fmt.Errorf("vm export read returned no row")
	}
	volume := data.Volume
	data.ID = types.StringValue(strconv.Itoa(row.Key.Int()))
	if row.Volume != "" {
		data.Volume = types.StringValue(row.Volume)
	} else {
		data.Volume = volume
	}
	data.VolumeName = stringOrNull(row.VolumeName)
	data.Quiesced = types.BoolValue(row.Quiesced)
	data.CreateCurrent = types.BoolValue(row.CreateCurrent)
	if row.MaxExports > 0 {
		data.MaxExports = types.Int64Value(int64(row.MaxExports))
	} else if intSet(data.MaxExports) {
		// Keep a configured value when the row has not filled max_exports yet.
	} else {
		data.MaxExports = types.Int64Null()
	}
	data.Status = stringOrNull(row.Status)
	data.StatusInfo = stringOrNull(row.StatusInfo)
	return nil
}

func exportID(id types.String) (int, error) {
	text := strings.TrimSpace(stringValue(id))
	if text == "" {
		return 0, fmt.Errorf("vm export id is empty")
	}
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("vm export id %q is not a positive integer", text)
	}
	return n, nil
}

func boolChanged(plan, state types.Bool) bool {
	if plan.IsUnknown() {
		return false
	}
	if plan.IsNull() && state.IsNull() {
		return false
	}
	if plan.IsNull() || state.IsNull() || plan.IsUnknown() || state.IsUnknown() {
		return !plan.Equal(state)
	}
	return plan.ValueBool() != state.ValueBool()
}

func intChanged(plan, state types.Int64) bool {
	if plan.IsUnknown() {
		return false
	}
	return !plan.Equal(state)
}

func exportIDSet(data *exportModel) bool {
	return data != nil && stringSet(data.ID)
}
