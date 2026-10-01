// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package snapshot

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

var _ vergeio.IClient = &SnapshotProfileApi{}

func NewSnapshotProfileApi(c *vergeio.Client) (*SnapshotProfileApi, error) {
	// Share the govergeos client with the VM resource in the same provider
	// connection. A setup error is returned so Configure can report it.
	sdk, err := c.CachedVergeosClient()
	if err != nil {
		return nil, err
	}
	return &SnapshotProfileApi{
		name: "Snapshot Profile Api",
		sdk:  sdk,
	}, nil
}

// SnapshotProfileApi is the govergeos snapshot profile and period client.
type SnapshotProfileApi struct {
	name string
	sdk  *vergeos.Client
}

func (api *SnapshotProfileApi) Name() string {
	return api.name
}

// periodInput is one planned period. Pointer fields are omitted from the
// API request when the configuration left them unset. Retention is never a
// pointer: the period API stores a 24 hour expiry when the field is left
// out, and govergeos rejects a non-positive retention before the request.
type periodInput struct {
	Name         string
	Frequency    string
	Hour         *int
	Minute       *int
	DayOfWeek    *string
	DayOfMonth   *int
	Month        *int
	Retention    int
	Quiesce      *bool
	MaxTier      *string
	MinSnapshots *int
	Immutable    *bool
}

func (api *SnapshotProfileApi) createProfile(ctx context.Context, data *SnapshotProfileResourceModel) error {
	desired, err := periodInputs(data.Period)
	if err != nil {
		return err
	}
	created, err := api.sdk.SnapshotProfiles.Create(ctx, profileCreateRequest(data))
	if err != nil {
		return err
	}
	applyProfile(data, created)
	tflog.Debug(ctx, fmt.Sprintf("created snapshot profile %s", data.Id.ValueString()))
	if err := api.syncPeriods(ctx, created.Key.Int(), desired); err != nil {
		return err
	}
	return nil
}

func (api *SnapshotProfileApi) readProfile(ctx context.Context, data *SnapshotProfileResourceModel) error {
	id, err := parseID(data.Id, "snapshot profile")
	if err != nil {
		return err
	}
	profile, err := api.sdk.SnapshotProfiles.Get(ctx, id)
	if err != nil {
		return err
	}
	periods, err := api.sdk.SnapshotProfilePeriods.ListByProfile(ctx, id)
	if err != nil {
		return err
	}
	applyProfile(data, profile)
	ordered, err := orderedPeriods(id, data.Period, periods)
	if err != nil {
		return err
	}
	data.Period = ordered
	tflog.Debug(ctx, fmt.Sprintf("read snapshot profile %d", id))
	return nil
}

func (api *SnapshotProfileApi) updateProfile(ctx context.Context, plan, state *SnapshotProfileResourceModel) error {
	id, err := parseID(state.Id, "snapshot profile")
	if err != nil {
		return err
	}
	desired, err := periodInputs(plan.Period)
	if err != nil {
		return err
	}
	plan.Id = state.Id
	if req := profileUpdateRequest(plan, state); req != nil {
		updated, err := api.sdk.SnapshotProfiles.Update(ctx, id, req)
		if err != nil {
			return err
		}
		applyProfile(plan, updated)
		tflog.Debug(ctx, fmt.Sprintf("updated snapshot profile %d", id))
	}
	if err := api.syncPeriods(ctx, id, desired); err != nil {
		return err
	}
	return nil
}

func (api *SnapshotProfileApi) deleteProfile(ctx context.Context, data *SnapshotProfileResourceModel) error {
	id, err := parseID(data.Id, "snapshot profile")
	if err != nil {
		return err
	}
	periods, err := api.sdk.SnapshotProfilePeriods.ListByProfile(ctx, id)
	if err != nil && !vergeos.IsNotFoundError(err) {
		return err
	}
	for _, period := range periods {
		periodID := period.Key.Int()
		if periodID <= 0 {
			return fmt.Errorf("snapshot profile period %q has no key", period.Name)
		}
		if err := api.sdk.SnapshotProfilePeriods.Delete(ctx, periodID); err != nil {
			if vergeos.IsNotFoundError(err) {
				continue
			}
			return fmt.Errorf("delete period %q: %w", period.Name, err)
		}
		tflog.Debug(ctx, fmt.Sprintf("deleted snapshot profile period %d (%s)", periodID, period.Name))
	}
	if err := api.sdk.SnapshotProfiles.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return err
	}
	tflog.Debug(ctx, fmt.Sprintf("deleted snapshot profile %d", id))
	return nil
}

func profileCreateRequest(data *SnapshotProfileResourceModel) *vergeos.SnapshotProfileCreateRequest {
	req := &vergeos.SnapshotProfileCreateRequest{
		Name: strings.TrimSpace(data.Name.ValueString()),
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	req.IgnoreWarnings = vergeio.KnownBool(data.IgnoreWarnings)
	return req
}

func profileUpdateRequest(plan, state *SnapshotProfileResourceModel) *vergeos.SnapshotProfileUpdateRequest {
	req := &vergeos.SnapshotProfileUpdateRequest{
		Name:           vergeio.ChangedString(plan.Name, state.Name),
		Description:    vergeio.ChangedString(plan.Description, state.Description),
		IgnoreWarnings: vergeio.ChangedBool(plan.IgnoreWarnings, state.IgnoreWarnings),
	}
	if req.Name == nil && req.Description == nil && req.IgnoreWarnings == nil {
		return nil
	}
	return req
}

func applyProfile(data *SnapshotProfileResourceModel, profile *vergeos.SnapshotProfile) {
	data.Id = idString(profile.Key.Int())
	data.Name = types.StringValue(profile.Name)
	data.Description = types.StringValue(profile.Description)
	data.IgnoreWarnings = types.BoolValue(profile.IgnoreWarnings)
}

// syncPeriods makes the profile's periods match desired. Periods are matched
// by name. A renamed period is deleted and created. Updates are sent before
// deletes, and deletes before creates, so a name can move from one period to
// another without a unique-name conflict.
func (api *SnapshotProfileApi) syncPeriods(ctx context.Context, profileID int, desired []periodInput) error {
	existing, err := api.sdk.SnapshotProfilePeriods.ListByProfile(ctx, profileID)
	if err != nil {
		return err
	}
	byName, err := periodsByName(profileID, existing)
	if err != nil {
		return err
	}
	wanted := make(map[string]periodInput, len(desired))
	for _, period := range desired {
		wanted[period.Name] = period
		current, ok := byName[period.Name]
		if !ok {
			continue
		}
		if err := api.updatePeriod(ctx, current, period); err != nil {
			return err
		}
	}
	for name, period := range byName {
		if _, ok := wanted[name]; ok {
			continue
		}
		periodID := period.Key.Int()
		if err := api.sdk.SnapshotProfilePeriods.Delete(ctx, periodID); err != nil {
			if vergeos.IsNotFoundError(err) {
				continue
			}
			return fmt.Errorf("delete period %q: %w", name, err)
		}
		tflog.Debug(ctx, fmt.Sprintf("deleted snapshot profile period %d (%s)", periodID, name))
	}
	for _, period := range desired {
		if _, ok := byName[period.Name]; ok {
			continue
		}
		if err := api.createPeriod(ctx, profileID, period); err != nil {
			return err
		}
	}
	return nil
}

func (api *SnapshotProfileApi) createPeriod(ctx context.Context, profileID int, period periodInput) error {
	created, err := api.sdk.SnapshotProfilePeriods.Create(ctx, &vergeos.SnapshotProfilePeriodCreateRequest{
		Profile:      profileID,
		Name:         period.Name,
		Frequency:    period.Frequency,
		Hour:         period.Hour,
		Minute:       period.Minute,
		DayOfWeek:    period.DayOfWeek,
		DayOfMonth:   period.DayOfMonth,
		Month:        period.Month,
		Retention:    period.Retention,
		MaxTier:      period.MaxTier,
		Quiesce:      period.Quiesce,
		MinSnapshots: period.MinSnapshots,
		Immutable:    period.Immutable,
	})
	if err != nil {
		return fmt.Errorf("create period %q: %w", period.Name, err)
	}
	tflog.Debug(ctx, fmt.Sprintf("created snapshot profile period %d (%s) retention %d", created.Key.Int(), period.Name, period.Retention))
	return nil
}

func (api *SnapshotProfileApi) updatePeriod(ctx context.Context, current vergeos.SnapshotProfilePeriod, period periodInput) error {
	req := periodUpdateRequest(current, period)
	if req == nil {
		tflog.Debug(ctx, fmt.Sprintf("snapshot profile period %d (%s) is unchanged", current.Key.Int(), period.Name))
		return nil
	}
	if _, err := api.sdk.SnapshotProfilePeriods.Update(ctx, current.Key.Int(), req); err != nil {
		return fmt.Errorf("update period %q: %w", period.Name, err)
	}
	tflog.Debug(ctx, fmt.Sprintf("updated snapshot profile period %d (%s)", current.Key.Int(), period.Name))
	return nil
}

func periodUpdateRequest(current vergeos.SnapshotProfilePeriod, period periodInput) *vergeos.SnapshotProfilePeriodUpdateRequest {
	req := &vergeos.SnapshotProfilePeriodUpdateRequest{}
	changed := false
	if period.Frequency != current.Frequency {
		frequency := period.Frequency
		req.Frequency = &frequency
		changed = true
	}
	if hour := changedInt(period.Hour, current.Hour); hour != nil {
		req.Hour = hour
		changed = true
	}
	if minute := changedInt(period.Minute, current.Minute); minute != nil {
		req.Minute = minute
		changed = true
	}
	if day := changedString(period.DayOfWeek, current.DayOfWeek); day != nil {
		req.DayOfWeek = day
		changed = true
	}
	if day := changedInt(period.DayOfMonth, current.DayOfMonth); day != nil {
		req.DayOfMonth = day
		changed = true
	}
	if month := changedInt(period.Month, current.Month); month != nil {
		req.Month = month
		changed = true
	}
	if period.Retention != current.Retention {
		retention := period.Retention
		req.Retention = &retention
		changed = true
	}
	if tier := changedString(period.MaxTier, current.MaxTier); tier != nil {
		req.MaxTier = tier
		changed = true
	}
	if quiesce := changedBool(period.Quiesce, current.Quiesce); quiesce != nil {
		req.Quiesce = quiesce
		changed = true
	}
	if minSnapshots := changedInt(period.MinSnapshots, current.MinSnapshots); minSnapshots != nil {
		req.MinSnapshots = minSnapshots
		changed = true
	}
	if immutable := changedBool(period.Immutable, current.Immutable); immutable != nil {
		req.Immutable = immutable
		changed = true
	}
	if !changed {
		return nil
	}
	return req
}

func changedInt(want *int, have int) *int {
	if want == nil || *want == have {
		return nil
	}
	return want
}

func changedString(want *string, have string) *string {
	if want == nil || *want == have {
		return nil
	}
	return want
}

func changedBool(want *bool, have bool) *bool {
	if want == nil || *want == have {
		return nil
	}
	return want
}

func periodsByName(profileID int, periods []vergeos.SnapshotProfilePeriod) (map[string]vergeos.SnapshotProfilePeriod, error) {
	byName := make(map[string]vergeos.SnapshotProfilePeriod, len(periods))
	for _, period := range periods {
		if strings.TrimSpace(period.Name) == "" {
			return nil, fmt.Errorf("snapshot profile %d has a period with no name", profileID)
		}
		if _, ok := byName[period.Name]; ok {
			return nil, fmt.Errorf("snapshot profile %d has more than one period named %q", profileID, period.Name)
		}
		byName[period.Name] = period
	}
	return byName, nil
}

// orderedPeriods keeps the prior configuration order and appends periods
// Terraform has not seen before, sorted by name. A refresh then stays stable
// when the API list order changes.
func orderedPeriods(profileID int, prior []periodModel, periods []vergeos.SnapshotProfilePeriod) ([]periodModel, error) {
	if len(periods) == 0 {
		return nil, nil
	}
	byName, err := periodsByName(profileID, periods)
	if err != nil {
		return nil, err
	}
	ordered := make([]vergeos.SnapshotProfilePeriod, 0, len(periods))
	seen := make(map[string]bool, len(periods))
	for _, previous := range prior {
		if previous.Name.IsNull() || previous.Name.IsUnknown() {
			continue
		}
		period, ok := byName[previous.Name.ValueString()]
		if !ok {
			continue
		}
		ordered = append(ordered, period)
		seen[period.Name] = true
	}
	rest := make([]vergeos.SnapshotProfilePeriod, 0, len(periods))
	for _, period := range periods {
		if seen[period.Name] {
			continue
		}
		rest = append(rest, period)
	}
	sort.Slice(rest, func(i, j int) bool {
		return rest[i].Name < rest[j].Name
	})
	ordered = append(ordered, rest...)
	models := make([]periodModel, len(ordered))
	for i, period := range ordered {
		models[i] = periodModelFromAPI(period)
	}
	return models, nil
}

func periodModelFromAPI(period vergeos.SnapshotProfilePeriod) periodModel {
	return periodModel{
		Key:          idString(period.Key.Int()),
		Name:         types.StringValue(period.Name),
		Frequency:    types.StringValue(period.Frequency),
		Hour:         types.Int32Value(int32(period.Hour)),
		Minute:       types.Int32Value(int32(period.Minute)),
		DayOfWeek:    stringValueOrNull(period.DayOfWeek),
		DayOfMonth:   types.Int32Value(int32(period.DayOfMonth)),
		Month:        types.Int32Value(int32(period.Month)),
		Retention:    types.Int64Value(int64(period.Retention)),
		Quiesce:      types.BoolValue(period.Quiesce),
		MaxTier:      stringValueOrNull(period.MaxTier),
		MinSnapshots: types.Int32Value(int32(period.MinSnapshots)),
		Immutable:    types.BoolValue(period.Immutable),
	}
}

// stringValueOrNull stores a blank API string as null. An empty max_tier or
// day_of_week would fail the OneOf validator on the next plan.
func stringValueOrNull(value string) types.String {
	if value == "" {
		return types.StringNull()
	}
	return types.StringValue(value)
}

func periodInputs(periods []periodModel) ([]periodInput, error) {
	if len(periods) == 0 {
		return nil, nil
	}
	out := make([]periodInput, 0, len(periods))
	seen := make(map[string]struct{}, len(periods))
	for _, period := range periods {
		name := strings.TrimSpace(period.Name.ValueString())
		if name == "" {
			return nil, fmt.Errorf("period name is required")
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("period name %q is used more than once", name)
		}
		seen[name] = struct{}{}
		frequency := strings.TrimSpace(period.Frequency.ValueString())
		if frequency == "" {
			return nil, fmt.Errorf("period %q frequency is required", name)
		}
		retention, err := requiredRetention(period.Retention)
		if err != nil {
			return nil, fmt.Errorf("period %q: %w", name, err)
		}
		out = append(out, periodInput{
			Name:         name,
			Frequency:    frequency,
			Hour:         optionalIntValue(period.Hour),
			Minute:       optionalIntValue(period.Minute),
			DayOfWeek:    optionalStringValue(period.DayOfWeek),
			DayOfMonth:   optionalIntValue(period.DayOfMonth),
			Month:        optionalIntValue(period.Month),
			Retention:    retention,
			Quiesce:      vergeio.KnownBool(period.Quiesce),
			MaxTier:      optionalStringValue(period.MaxTier),
			MinSnapshots: optionalIntValue(period.MinSnapshots),
			Immutable:    vergeio.KnownBool(period.Immutable),
		})
	}
	return out, nil
}

// requiredRetention rejects a missing or non-positive retention. The period
// API does not mean "keep forever" when the field is absent, and govergeos
// refuses to send a retention that is not positive.
func requiredRetention(value types.Int64) (int, error) {
	if value.IsNull() || value.IsUnknown() {
		return 0, fmt.Errorf("retention is required and is a positive number of seconds; there is no default")
	}
	seconds := value.ValueInt64()
	if seconds < 1 {
		return 0, fmt.Errorf("retention must be at least 1 second, got %d", seconds)
	}
	if seconds > int64(math.MaxInt) {
		return 0, fmt.Errorf("retention %d is too large", seconds)
	}
	return int(seconds), nil
}

func optionalIntValue(value types.Int32) *int {
	known := vergeio.KnownInt32(value)
	if known == nil {
		return nil
	}
	n := int(*known)
	return &n
}

func optionalStringValue(value types.String) *string {
	known := vergeio.KnownString(value)
	if known == nil {
		return nil
	}
	text := strings.TrimSpace(*known)
	return &text
}
