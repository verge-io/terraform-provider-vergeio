// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package compute

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func NewVMRecipeInstanceApi(c *vergeio.Client) (*VMRecipeInstanceApi, error) {
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	vm, err := NewVMApi(c)
	if err != nil {
		return nil, err
	}
	return &VMRecipeInstanceApi{
		name:   "VM Recipe Instance Api",
		client: c,
		sdk:    sdk,
		vm:     vm,
	}, nil
}

// VMRecipeInstanceApi deploys a VM from a recipe and deletes that VM.
type VMRecipeInstanceApi struct {
	name   string
	client *vergeio.Client
	sdk    *vergeos.Client
	vm     *VMApi
}

func (api *VMRecipeInstanceApi) Name() string {
	return api.name
}

// deploy creates the VM. It calls Deploy. Preview is not called, and the
// request has no simulate flag.
func (api *VMRecipeInstanceApi) deploy(ctx context.Context, data *VMRecipeInstanceResourceModel) error {
	req, err := recipeDeployRequest(ctx, data)
	if err != nil {
		return err
	}
	instance, err := api.sdk.VMRecipeInstances.Deploy(ctx, req)
	if err != nil {
		if vergeos.IsValidationError(err) {
			return err
		}
		found, getErr := api.sdk.VMRecipeInstances.GetByName(ctx, data.Name.ValueString())
		if getErr != nil || found.Recipe != data.RecipeID.ValueString() {
			return err
		}
		applyRecipeInstance(data, found)
		return err
	}
	applyRecipeInstance(data, instance)
	tflog.Debug(ctx, fmt.Sprintf("deployed recipe instance %s vm %s", data.Id.ValueString(), data.VMID.String()))
	return nil
}

func (api *VMRecipeInstanceApi) read(ctx context.Context, data *VMRecipeInstanceResourceModel) error {
	id, err := parseRecipeInstanceID(data.Id)
	if err != nil {
		return err
	}
	instance, err := api.sdk.VMRecipeInstances.Get(ctx, id)
	if err != nil {
		return err
	}
	if instance == nil {
		return &vergeos.NotFoundError{Resource: "VMRecipeInstance", ID: id}
	}
	if name := strings.TrimSpace(data.Name.ValueString()); name != "" && instance.Name != name {
		return &vergeos.NotFoundError{Resource: "VMRecipeInstance", ID: id}
	}
	if recipe := strings.TrimSpace(data.RecipeID.ValueString()); recipe != "" && instance.Recipe != recipe {
		return &vergeos.NotFoundError{Resource: "VMRecipeInstance", ID: id}
	}
	vmID := instance.VM.Int()
	if vmID <= 0 {
		return &vergeos.NotFoundError{Resource: "VM", ID: id}
	}
	if _, err := api.sdk.VMs.Get(ctx, vmID); err != nil {
		return err
	}
	answers := data.Answers
	timeouts := data.Timeouts
	applyRecipeInstance(data, instance)
	data.Answers = answers
	data.Timeouts = timeouts
	tflog.Debug(ctx, fmt.Sprintf("read recipe instance %d", id))
	return nil
}

func (api *VMRecipeInstanceApi) delete(ctx context.Context, data *VMRecipeInstanceResourceModel) error {
	id, err := parseRecipeInstanceID(data.Id)
	if err != nil {
		return err
	}
	instance, err := api.sdk.VMRecipeInstances.Get(ctx, id)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	vmID := 0
	name := data.Name.ValueString()
	if instance != nil {
		vmID = instance.VM.Int()
		if instance.Name != "" {
			name = instance.Name
		}
	}
	if vmID > 0 {
		timeout, err := recipeDeleteTimeout(data)
		if err != nil {
			return err
		}
		vmKey := strconv.Itoa(vmID)
		if err := api.vm.gracefulPowerOff(ctx, vmKey, name, timeout, true); err != nil {
			return err
		}
		if err := api.sdk.VMs.Delete(ctx, vmID); err != nil && !vergeos.IsNotFoundError(err) {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("deleted VM %d for recipe instance %d", vmID, id))
	}
	if err := deleteRecipeInstanceRow(ctx, api.client, id); err != nil {
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted recipe instance %d", id))
	return nil
}

func recipeDeployRequest(ctx context.Context, data *VMRecipeInstanceResourceModel) (*vergeos.VMRecipeDeployRequest, error) {
	if data == nil {
		return nil, fmt.Errorf("recipe instance is missing")
	}
	recipe := strings.TrimSpace(data.RecipeID.ValueString())
	name := strings.TrimSpace(data.Name.ValueString())
	if recipe == "" {
		return nil, fmt.Errorf("recipe_id is required")
	}
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	answers, err := recipeAnswerMap(ctx, data.Answers)
	if err != nil {
		return nil, err
	}
	req := &vergeos.VMRecipeDeployRequest{
		Recipe:  recipe,
		Name:    name,
		Answers: answers,
	}
	if auto := vergeio.KnownBool(data.AutoUpdate); auto != nil {
		req.AutoUpdate = auto
	}
	return req, nil
}

func recipeAnswerMap(ctx context.Context, answers types.Map) (vergeos.RecipeAnswers, error) {
	if answers.IsNull() || answers.IsUnknown() {
		return nil, nil
	}
	raw := map[string]string{}
	if diags := answers.ElementsAs(ctx, &raw, false); diags.HasError() {
		return nil, fmt.Errorf("answers: %s", diags)
	}
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(vergeos.RecipeAnswers, len(raw))
	for key, value := range raw {
		out[key] = value
	}
	return out, nil
}

func applyRecipeInstance(data *VMRecipeInstanceResourceModel, instance *vergeos.VMRecipeInstance) {
	if data == nil || instance == nil {
		return
	}
	if key := instance.Key.Int(); key > 0 {
		data.Id = types.StringValue(strconv.Itoa(key))
	}
	if instance.Name != "" {
		data.Name = types.StringValue(instance.Name)
	}
	if instance.Recipe != "" {
		data.RecipeID = types.StringValue(instance.Recipe)
	}
	data.AutoUpdate = types.BoolValue(instance.AutoUpdate)
	data.VMID = types.Int64Value(int64(instance.VM.Int()))
	data.RecipeName = types.StringValue(instance.RecipeName)
	data.Version = types.StringValue(instance.Version)
	data.Build = types.Int64Value(int64(instance.Build.Int()))
}

func recipeDeleteTimeout(data *VMRecipeInstanceResourceModel) (time.Duration, error) {
	if data == nil || data.Timeouts == nil {
		return gracefulShutdownTimeout, nil
	}
	return parseShutdownTimeout(data.Timeouts.Delete, "timeouts.delete")
}

func parseRecipeInstanceID(id types.String) (int, error) {
	if id.IsNull() || id.IsUnknown() {
		return 0, fmt.Errorf("recipe instance id is empty")
	}
	return parseRecipeInstanceIDText(id.ValueString())
}

func parseRecipeInstanceIDText(id string) (int, error) {
	text := strings.TrimSpace(id)
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("recipe instance id %q is not a positive integer", id)
	}
	return n, nil
}

// deleteRecipeInstanceRow removes the instance row. govergeos
// VMRecipeInstances has no Delete method.
func deleteRecipeInstanceRow(ctx context.Context, c *vergeio.Client, id int) error {
	if c == nil {
		return fmt.Errorf("vergeio client is nil")
	}
	resp, err := c.Delete(ctx, vergeio.ObjectPath(vergeio.RecipeInstanceEndpoint, strconv.Itoa(id)))
	if err != nil {
		var apiErr vergeio.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil
		}
		return err
	}
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
	}
	return nil
}
