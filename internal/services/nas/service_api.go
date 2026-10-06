// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

type userSecret struct {
	Password string
	Send     bool
}

func (a *API) createService(ctx context.Context, data *serviceModel, secrets map[string]userSecret) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	req, err := serviceCreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.NASServices.Create(ctx, req)
	if err != nil {
		return err
	}
	id := created.Key.Int()
	data.ID = types.StringValue(fmt.Sprintf("%d", id))
	if err := a.syncUsers(ctx, id, data.Users, secrets); err != nil {
		return abandon(err, "NAS service", data.ID.ValueString(), a.DeleteService(ctx, id))
	}
	if err := a.readService(ctx, data); err != nil {
		return abandon(err, "NAS service", data.ID.ValueString(), a.DeleteService(ctx, id))
	}
	return nil
}

func (a *API) readService(ctx context.Context, data *serviceModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parsePositiveID(data.ID, "NAS service")
	if err != nil {
		return err
	}
	service, err := a.sdk.NASServices.Get(ctx, id)
	if err != nil {
		return err
	}
	users, err := a.sdk.NASServiceUsers.ListByService(ctx, id)
	if err != nil {
		return err
	}
	merged, err := mergeUsers(data.Users, users)
	if err != nil {
		return err
	}
	applyService(data, service)
	data.Users = merged
	return nil
}

func (a *API) updateService(ctx context.Context, plan, state *serviceModel, secrets map[string]userSecret) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parsePositiveID(state.ID, "NAS service")
	if err != nil {
		return err
	}
	plan.ID = state.ID
	if req := serviceUpdateRequest(plan, state); req != nil {
		if _, err := a.sdk.NASServices.Update(ctx, id, req); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("updated NAS service %d", id))
	}
	if err := a.syncUsers(ctx, id, plan.Users, secrets); err != nil {
		return err
	}
	return a.readService(ctx, plan)
}

func (a *API) deleteService(ctx context.Context, data *serviceModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parsePositiveID(data.ID, "NAS service")
	if err != nil {
		return err
	}
	return a.DeleteService(ctx, id)
}

// DeleteService removes a NAS service. Users on the service are removed first.
// VergeOS is asked to delete the service before any volume is touched. A
// refused delete is retried after volumes are disabled and their shares are
// gone, so a partial create cannot leave a row that blocks destroy.
func (a *API) DeleteService(ctx context.Context, id int) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if id <= 0 {
		return nil
	}
	if err := a.deleteServiceUsers(ctx, id); err != nil {
		return err
	}
	err := a.sdk.NASServices.Delete(ctx, id)
	if err == nil || missing(err) {
		return nil
	}
	if childErr := a.deleteServiceVolumes(ctx, id); childErr != nil {
		return fmt.Errorf("delete NAS service %d: %w (volumes were left in place: %v)", id, err, childErr)
	}
	if userErr := a.deleteServiceUsers(ctx, id); userErr != nil {
		return fmt.Errorf("delete NAS service %d: %w (users were left in place: %v)", id, err, userErr)
	}
	retry := a.sdk.NASServices.Delete(ctx, id)
	if retry == nil || missing(retry) {
		return nil
	}
	return fmt.Errorf("delete NAS service %d: %w", id, retry)
}

func (a *API) deleteServiceUsers(ctx context.Context, serviceID int) error {
	users, err := a.sdk.NASServiceUsers.ListByService(ctx, serviceID)
	if err != nil {
		if missing(err) {
			return nil
		}
		return fmt.Errorf("list users for NAS service %d: %w", serviceID, err)
	}
	var errs []error
	for _, user := range users {
		id := firstNonEmpty(user.ID, user.Key)
		if id == "" {
			errs = append(errs, fmt.Errorf("NAS service %d has a user named %q with no id", serviceID, user.Name))
			continue
		}
		if err := a.sdk.NASServiceUsers.Delete(ctx, id); err != nil && !missing(err) {
			errs = append(errs, fmt.Errorf("delete NAS user %s: %w", user.Name, err))
		}
	}
	return errors.Join(errs...)
}

func (a *API) deleteServiceVolumes(ctx context.Context, serviceID int) error {
	volumes, err := a.sdk.Volumes.ListByService(ctx, serviceID)
	if err != nil {
		if missing(err) {
			return nil
		}
		return fmt.Errorf("list volumes for NAS service %d: %w", serviceID, err)
	}
	var errs []error
	for _, volume := range volumes {
		id := rowID(volume.Key, volume.ID)
		if id == "" {
			errs = append(errs, fmt.Errorf("NAS service %d has a volume named %q with no id", serviceID, volume.Name))
			continue
		}
		if err := a.DeleteVolume(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (a *API) syncUsers(ctx context.Context, serviceID int, desired []userModel, secrets map[string]userSecret) error {
	if err := uniqueUserNames(desired); err != nil {
		return err
	}
	existing, err := a.sdk.NASServiceUsers.ListByService(ctx, serviceID)
	if err != nil {
		return err
	}
	byName, err := usersByName(serviceID, existing)
	if err != nil {
		return err
	}
	wanted := map[string]userModel{}
	for _, user := range desired {
		name := strings.TrimSpace(user.Name.ValueString())
		wanted[name] = user
		current, ok := byName[name]
		if !ok {
			continue
		}
		if err := a.updateUser(ctx, current, user, secrets[name]); err != nil {
			return err
		}
	}
	for name, current := range byName {
		if _, keep := wanted[name]; keep {
			continue
		}
		id := firstNonEmpty(current.ID, current.Key)
		if err := a.sdk.NASServiceUsers.Delete(ctx, id); err != nil && !missing(err) {
			return fmt.Errorf("delete NAS user %s: %w", name, err)
		}
		tflog.Debug(ctx, fmt.Sprintf("deleted NAS user %s", name))
	}
	for _, user := range desired {
		name := strings.TrimSpace(user.Name.ValueString())
		if _, ok := byName[name]; ok {
			continue
		}
		secret := secrets[name]
		if !secret.Send || secret.Password == "" {
			return fmt.Errorf("user %s needs password_wo", name)
		}
		if _, err := a.sdk.NASServiceUsers.Create(ctx, userCreateRequest(serviceID, user, secret.Password)); err != nil {
			return fmt.Errorf("create NAS user %s: %w", name, err)
		}
		tflog.Debug(ctx, fmt.Sprintf("created NAS user %s", name))
	}
	return nil
}

func (a *API) updateUser(ctx context.Context, current vergeos.NASServiceUser, plan userModel, secret userSecret) error {
	req := userUpdateRequest(current, plan, secret)
	if req == nil {
		return nil
	}
	id := firstNonEmpty(current.ID, current.Key)
	if _, err := a.sdk.NASServiceUsers.Update(ctx, id, req); err != nil {
		return fmt.Errorf("update NAS user %s: %w", current.Name, err)
	}
	tflog.Debug(ctx, fmt.Sprintf("updated NAS user %s", current.Name))
	return nil
}

func serviceCreateRequest(data *serviceModel) (*vergeos.NASServiceCreateRequest, error) {
	vmID, err := parsePositiveID(data.VMID, "virtual machine")
	if err != nil {
		return nil, err
	}
	return &vergeos.NASServiceCreateRequest{
		VM:                 vmID,
		MaxImports:         knownInt(data.MaxImports),
		MaxSyncs:           knownInt(data.MaxSyncs),
		DisableSwap:        vergeio.KnownBool(data.DisableSwap),
		ReadAheadKBDefault: vergeio.KnownString(data.ReadAheadKBDefault),
	}, nil
}

func serviceUpdateRequest(plan, state *serviceModel) *vergeos.NASServiceUpdateRequest {
	req := &vergeos.NASServiceUpdateRequest{
		MaxImports:         changedInt(plan.MaxImports, state.MaxImports),
		MaxSyncs:           changedInt(plan.MaxSyncs, state.MaxSyncs),
		DisableSwap:        vergeio.ChangedBool(plan.DisableSwap, state.DisableSwap),
		ReadAheadKBDefault: vergeio.ChangedString(plan.ReadAheadKBDefault, state.ReadAheadKBDefault),
	}
	if req.MaxImports == nil && req.MaxSyncs == nil && req.DisableSwap == nil && req.ReadAheadKBDefault == nil {
		return nil
	}
	return req
}

func applyService(data *serviceModel, service *vergeos.NASService) {
	data.ID = types.StringValue(fmt.Sprintf("%d", service.Key.Int()))
	data.VMID = types.StringValue(fmt.Sprintf("%d", service.VM.Int()))
	data.Name = types.StringValue(service.Name)
	data.MaxImports = types.Int64Value(int64(service.MaxImports))
	data.MaxSyncs = types.Int64Value(int64(service.MaxSyncs))
	data.DisableSwap = types.BoolValue(service.DisableSwap)
	data.ReadAheadKBDefault = types.StringValue(service.ReadAheadKBDefault)
	data.CIFSID = types.Int64Value(int64(service.CIFS.Int()))
	data.NFSID = types.Int64Value(int64(service.NFS.Int()))
	data.AntivirusID = types.Int64Value(int64(service.Antivirus.Int()))
}

func userCreateRequest(serviceID int, user userModel, password string) *vergeos.NASServiceUserCreateRequest {
	req := &vergeos.NASServiceUserCreateRequest{
		Service:     serviceID,
		Name:        strings.TrimSpace(user.Name.ValueString()),
		Password:    password,
		DisplayName: stringOrEmpty(user.DisplayName),
		Description: stringOrEmpty(user.Description),
		Enabled:     vergeio.KnownBool(user.Enabled),
		HomeShare:   knownInt(user.HomeShare),
	}
	if drive := vergeio.KnownString(user.HomeDrive); drive != nil {
		req.HomeDrive = drive
	}
	return req
}

func userUpdateRequest(current vergeos.NASServiceUser, plan userModel, secret userSecret) *vergeos.NASServiceUserUpdateRequest {
	req := &vergeos.NASServiceUserUpdateRequest{}
	changed := false
	if secret.Send && secret.Password != "" {
		password := secret.Password
		req.Password = &password
		changed = true
	}
	if display := changedFrom(plan.DisplayName, current.DisplayName); display != nil {
		req.DisplayName = display
		changed = true
	}
	if description := changedFrom(plan.Description, current.Description); description != nil {
		req.Description = description
		changed = true
	}
	if enabled := vergeio.ChangedBool(plan.Enabled, types.BoolValue(current.Enabled)); enabled != nil {
		req.Enabled = enabled
		changed = true
	}
	if home := changedInt(plan.HomeShare, types.Int64Value(int64(current.HomeShare.Int()))); home != nil {
		req.HomeShare = home
		changed = true
	}
	if drive := changedFrom(plan.HomeDrive, current.HomeDrive); drive != nil {
		req.HomeDrive = drive
		changed = true
	}
	if !changed {
		return nil
	}
	return req
}

func changedFrom(plan types.String, current string) *string {
	if plan.IsNull() || plan.IsUnknown() {
		return nil
	}
	if plan.ValueString() == current {
		return nil
	}
	value := plan.ValueString()
	return &value
}

func uniqueUserNames(users []userModel) error {
	seen := map[string]struct{}{}
	for _, user := range users {
		name := strings.TrimSpace(user.Name.ValueString())
		if name == "" {
			return fmt.Errorf("NAS user name is empty")
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("NAS user name %q is repeated", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func usersByName(serviceID int, users []vergeos.NASServiceUser) (map[string]vergeos.NASServiceUser, error) {
	byName := make(map[string]vergeos.NASServiceUser, len(users))
	for _, user := range users {
		name := strings.TrimSpace(user.Name)
		if name == "" {
			return nil, fmt.Errorf("NAS service %d has a user with no name", serviceID)
		}
		if _, ok := byName[name]; ok {
			return nil, fmt.Errorf("NAS service %d has more than one user named %q", serviceID, name)
		}
		byName[name] = user
	}
	return byName, nil
}

// keepNamedUsers copies unknown user values from the state user of the same
// name. Index based copy would attach one user's values to another when the
// configuration reorders the blocks.
func keepNamedUsers(plan, state []userModel) []userModel {
	byName := map[string]userModel{}
	for _, user := range state {
		name := strings.TrimSpace(stringOrEmpty(user.Name))
		if name == "" {
			continue
		}
		byName[name] = user
	}
	for i := range plan {
		name := strings.TrimSpace(stringOrEmpty(plan[i].Name))
		prior, ok := byName[name]
		if !ok {
			continue
		}
		plan[i].ID = keepString(plan[i].ID, prior.ID)
		plan[i].DisplayName = keepString(plan[i].DisplayName, prior.DisplayName)
		plan[i].Description = keepString(plan[i].Description, prior.Description)
		plan[i].HomeDrive = keepString(plan[i].HomeDrive, prior.HomeDrive)
		plan[i].HomeShare = keepInt(plan[i].HomeShare, prior.HomeShare)
		plan[i].Created = keepInt(plan[i].Created, prior.Created)
		plan[i].PasswordWOVersion = keepInt(plan[i].PasswordWOVersion, prior.PasswordWOVersion)
	}
	return plan
}

func keepString(plan, state types.String) types.String {
	if plan.IsUnknown() && !state.IsNull() && !state.IsUnknown() {
		return state
	}
	return plan
}

func keepInt(plan, state types.Int64) types.Int64 {
	if plan.IsUnknown() && !state.IsNull() && !state.IsUnknown() {
		return state
	}
	return plan
}

// mergeUsers keeps configuration order and appends users Terraform has not
// seen, sorted by name. password_wo_version stays with the user of that name.
func mergeUsers(prior []userModel, rows []vergeos.NASServiceUser) ([]userModel, error) {
	byName, err := usersByName(0, rows)
	if err != nil {
		return nil, err
	}
	var out []userModel
	seen := map[string]struct{}{}
	for _, old := range prior {
		name := strings.TrimSpace(stringOrEmpty(old.Name))
		if name == "" {
			continue
		}
		row, ok := byName[name]
		if !ok {
			continue
		}
		out = append(out, userFromAPI(row, old.PasswordWOVersion))
		seen[name] = struct{}{}
	}
	var rest []string
	for name := range byName {
		if _, ok := seen[name]; ok {
			continue
		}
		rest = append(rest, name)
	}
	sort.Strings(rest)
	for _, name := range rest {
		out = append(out, userFromAPI(byName[name], types.Int64Null()))
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

func userFromAPI(row vergeos.NASServiceUser, version types.Int64) userModel {
	return userModel{
		ID:                types.StringValue(firstNonEmpty(row.ID, row.Key)),
		Name:              types.StringValue(row.Name),
		PasswordWO:        types.StringNull(),
		PasswordWOVersion: version,
		DisplayName:       types.StringValue(row.DisplayName),
		Description:       types.StringValue(row.Description),
		Enabled:           types.BoolValue(row.Enabled),
		HomeShare:         types.Int64Value(int64(row.HomeShare.Int())),
		HomeDrive:         types.StringValue(row.HomeDrive),
		Created:           types.Int64Value(row.Created),
	}
}

// userSecrets decides which passwords to send. Create sends every configured
// password. Update sends a password when the user is new or the version changed.
func userSecrets(config, state []userModel, create bool) map[string]userSecret {
	prior := map[string]types.Int64{}
	for _, user := range state {
		name := strings.TrimSpace(stringOrEmpty(user.Name))
		if name == "" {
			continue
		}
		prior[name] = user.PasswordWOVersion
	}
	out := map[string]userSecret{}
	for _, user := range config {
		name := strings.TrimSpace(stringOrEmpty(user.Name))
		if name == "" {
			continue
		}
		password := stringOrEmpty(user.PasswordWO)
		send := password != "" && (create || versionChanged(prior, name, user.PasswordWOVersion))
		out[name] = userSecret{Password: password, Send: send}
	}
	return out
}

func versionChanged(prior map[string]types.Int64, name string, version types.Int64) bool {
	old, ok := prior[name]
	if !ok {
		return true
	}
	if version.IsNull() || version.IsUnknown() {
		return false
	}
	if old.IsNull() || old.IsUnknown() {
		return true
	}
	return old.ValueInt64() != version.ValueInt64()
}
