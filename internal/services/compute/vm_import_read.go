// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/verge-io/govergeos"
)

func (a *importAPI) readFilled(ctx context.Context, data *importModel, fillOmitted bool) error {
	if data == nil {
		return fmt.Errorf("vm import is missing")
	}
	id := strings.TrimSpace(data.ID.ValueString())
	if id == "" {
		return fmt.Errorf("vm import id is empty")
	}
	target := ""
	if !data.Target.IsNull() && !data.Target.IsUnknown() {
		target = data.Target.ValueString()
	}
	if hexKeyPattern.MatchString(id) {
		if target == vmImportTargetMedia {
			return fmt.Errorf("vm import %s is a vm_imports key and target is media", id)
		}
		return a.readVMImport(ctx, data, id, fillOmitted)
	}
	fileID, err := strconv.Atoi(id)
	if err != nil || fileID <= 0 {
		return fmt.Errorf("vm import id %q is not a vm_imports key or a media file id", id)
	}
	if target == vmImportTargetVM {
		return fmt.Errorf("vm import %s is a media file id and target is vm", id)
	}
	return a.readMedia(ctx, data, fileID, fillOmitted)
}

func (a *importAPI) readVMImport(ctx context.Context, data *importModel, id string, fillOmitted bool) error {
	row, err := a.sdk.VMImports.Get(ctx, id)
	if err != nil {
		return err
	}
	if row == nil || vmImportKey(row) == "" {
		return &vergeos.NotFoundError{Resource: "VMImport", ID: id}
	}
	return a.applyVMImport(ctx, data, row, fillOmitted)
}

func (a *importAPI) readMedia(ctx context.Context, data *importModel, id int, fillOmitted bool) error {
	file, err := a.sdk.Files.Get(ctx, id)
	if err != nil {
		return err
	}
	if file == nil || file.Key.Int() <= 0 {
		return &vergeos.NotFoundError{Resource: "File", ID: id}
	}
	if !mediaOwned(data, file) {
		return &vergeos.NotFoundError{Resource: "File", ID: id}
	}
	return applyMediaFile(data, file, fillOmitted)
}
