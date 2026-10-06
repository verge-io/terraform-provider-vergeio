// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func (a *API) createIncoming(ctx context.Context, data *incomingModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	req, err := incomingCreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.SiteSyncsIncoming.Create(ctx, req)
	if err != nil {
		return err
	}
	data.ID = idString(created.Key.Int())
	filled, err := a.fillRegistrationCode(ctx, created)
	if err != nil {
		applyIncoming(data, created, *data)
		return err
	}
	applyIncoming(data, filled, *data)
	return nil
}

func (a *API) readIncoming(ctx context.Context, data *incomingModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseID(data.ID, "incoming sync")
	if err != nil {
		return err
	}
	got, err := a.sdk.SiteSyncsIncoming.Get(ctx, id)
	if err != nil {
		return err
	}
	applyIncoming(data, got, *data)
	return nil
}

func (a *API) updateIncoming(ctx context.Context, plan, state *incomingModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseID(state.ID, "incoming sync")
	if err != nil {
		return err
	}
	port, err := changedInt(plan.VSANPort, state.VSANPort)
	if err != nil {
		return err
	}
	minSnapshots, err := changedInt(plan.MinSnapshots, state.MinSnapshots)
	if err != nil {
		return err
	}
	req := &vergeos.SiteSyncIncomingUpdateRequest{
		Name:         vergeio.ChangedString(plan.Name, state.Name),
		Description:  vergeio.ChangedString(plan.Description, state.Description),
		Enabled:      vergeio.ChangedBool(plan.Enabled, state.Enabled),
		PublicIP:     vergeio.ChangedString(plan.PublicIP, state.PublicIP),
		ForceTier:    vergeio.ChangedString(plan.ForceTier, state.ForceTier),
		VSANHost:     vergeio.ChangedString(plan.VSANHost, state.VSANHost),
		VSANPort:     port,
		RequestURL:   vergeio.ChangedString(plan.RequestURL, state.RequestURL),
		MinSnapshots: minSnapshots,
	}
	if !incomingUpdateEmpty(req) {
		if _, err := a.sdk.SiteSyncsIncoming.Update(ctx, id, req); err != nil {
			return err
		}
	}
	plan.ID = state.ID
	return a.readIncoming(ctx, plan)
}

func (a *API) deleteIncoming(ctx context.Context, id int) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	return ignoreMissing(a.sdk.SiteSyncsIncoming.Delete(ctx, id))
}

func incomingCreateRequest(data *incomingModel) (*vergeos.SiteSyncIncomingCreateRequest, error) {
	siteID, err := parseID(data.SiteID, "site")
	if err != nil {
		return nil, err
	}
	if !setString(data.Name) {
		return nil, fmt.Errorf("name is required")
	}
	port, err := knownInt(data.VSANPort)
	if err != nil {
		return nil, err
	}
	minSnapshots, err := knownInt(data.MinSnapshots)
	if err != nil {
		return nil, err
	}
	return &vergeos.SiteSyncIncomingCreateRequest{
		Site:         siteID,
		Name:         data.Name.ValueString(),
		Description:  stringValue(data.Description),
		Enabled:      vergeio.KnownBool(data.Enabled),
		PublicIP:     vergeio.KnownString(data.PublicIP),
		ForceTier:    vergeio.KnownString(data.ForceTier),
		VSANHost:     vergeio.KnownString(data.VSANHost),
		VSANPort:     port,
		RequestURL:   vergeio.KnownString(data.RequestURL),
		MinSnapshots: minSnapshots,
	}, nil
}

func incomingUpdateEmpty(req *vergeos.SiteSyncIncomingUpdateRequest) bool {
	if req == nil {
		return true
	}
	return req.Name == nil &&
		req.Description == nil &&
		req.Enabled == nil &&
		req.PublicIP == nil &&
		req.ForceTier == nil &&
		req.VSANHost == nil &&
		req.VSANPort == nil &&
		req.RequestURL == nil &&
		req.MinSnapshots == nil
}

func applyIncoming(data *incomingModel, got *vergeos.SiteSyncIncoming, prior incomingModel) {
	data.ID = idString(got.Key.Int())
	data.SiteID = idString(got.Site.Int())
	data.Name = types.StringValue(got.Name)
	data.Description = stringFromAPI(got.Description, prior.Description)
	data.PublicIP = stringFromAPI(got.PublicIP, prior.PublicIP)
	data.ForceTier = stringFromAPI(got.ForceTier, prior.ForceTier)
	data.VSANHost = stringFromAPI(got.VSANHost, prior.VSANHost)
	data.VSANPort = intFromAPI(got.VSANPort, prior.VSANPort)
	data.RequestURL = stringFromAPI(got.RequestURL, prior.RequestURL)
	data.MinSnapshots = intFromAPI(got.MinSnapshots, prior.MinSnapshots)
	data.Enabled = types.BoolValue(got.Enabled)
	data.SyncID = stringFromAPI(got.SyncID, prior.SyncID)
	data.RegistrationCode = volatileStringValue(got.RegistrationCode)
	data.Status = volatileStringValue(got.Status)
	data.StatusInfo = volatileStringValue(got.StatusInfo)
	data.State = volatileStringValue(got.State)
	data.LastSync = types.Int64Value(got.LastSync)
	data.SystemCreated = types.BoolValue(got.SystemCreated)
}

func incomingForState(data *incomingModel) incomingModel {
	stored := *data
	stored.ID = knownString(data.ID)
	stored.SiteID = knownString(data.SiteID)
	stored.Name = knownString(data.Name)
	stored.Description = knownString(data.Description)
	stored.PublicIP = knownString(data.PublicIP)
	stored.ForceTier = knownString(data.ForceTier)
	stored.VSANHost = knownString(data.VSANHost)
	stored.VSANPort = knownInt64(data.VSANPort)
	stored.RequestURL = knownString(data.RequestURL)
	stored.MinSnapshots = knownInt64(data.MinSnapshots)
	stored.Enabled = knownBool(data.Enabled)
	stored.SyncID = knownString(data.SyncID)
	stored.RegistrationCode = knownString(data.RegistrationCode)
	stored.Status = knownString(data.Status)
	stored.StatusInfo = knownString(data.StatusInfo)
	stored.State = knownString(data.State)
	stored.LastSync = knownInt64(data.LastSync)
	stored.SystemCreated = knownBool(data.SystemCreated)
	return stored
}

// fillRegistrationCode waits for VergeOS to fill the incoming registration
// code. The wait is only used after create. A finished wait returns the sync
// that was read, even when the code is still empty.
func (a *API) fillRegistrationCode(ctx context.Context, sync *vergeos.SiteSyncIncoming) (*vergeos.SiteSyncIncoming, error) {
	if sync == nil || sync.RegistrationCode != "" {
		return sync, nil
	}
	attempts := a.codeAttempts
	interval := a.codeInterval
	for i := 0; i < attempts; i++ {
		if interval > 0 {
			timer := time.NewTimer(interval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return sync, ctx.Err()
			case <-timer.C:
			}
		}
		if ctx.Err() != nil {
			return sync, ctx.Err()
		}
		got, err := a.sdk.SiteSyncsIncoming.Get(ctx, sync.Key.Int())
		if err != nil || got == nil {
			return sync, nil
		}
		sync = got
		if sync.RegistrationCode != "" {
			return sync, nil
		}
	}
	return sync, nil
}

func (a *API) createOutgoing(ctx context.Context, data *outgoingModel, registrationCode string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if err := validatePeriods(data.Period); err != nil {
		return err
	}
	req, err := outgoingCreateRequest(data, registrationCode)
	if err != nil {
		return err
	}
	created, err := a.sdk.SiteSyncsOutgoing.Create(ctx, req)
	if err != nil {
		return err
	}
	data.ID = idString(created.Key.Int())
	data.SiteID = idString(created.Site.Int())
	if err := a.syncPeriods(ctx, created.Key.Int(), data.Period); err != nil {
		return err
	}
	return a.readOutgoing(ctx, data)
}

func (a *API) readOutgoing(ctx context.Context, data *outgoingModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseID(data.ID, "outgoing sync")
	if err != nil {
		return err
	}
	got, err := a.sdk.SiteSyncsOutgoing.Get(ctx, id)
	if err != nil {
		return err
	}
	periods, err := a.sdk.SiteSyncProfilePeriods.ListByOutgoingSync(ctx, id)
	if listMissing(err) {
		periods = nil
		err = nil
	}
	if err != nil {
		return err
	}
	applyOutgoing(data, got, periods, *data)
	return nil
}

func (a *API) updateOutgoing(ctx context.Context, plan, state *outgoingModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if err := validatePeriods(plan.Period); err != nil {
		return err
	}
	id, err := parseID(state.ID, "outgoing sync")
	if err != nil {
		return err
	}
	req, err := outgoingUpdateRequest(plan, state)
	if err != nil {
		return err
	}
	if !outgoingUpdateEmpty(req) {
		if _, err := a.sdk.SiteSyncsOutgoing.Update(ctx, id, req); err != nil {
			return err
		}
	}
	plan.ID = state.ID
	if err := a.syncPeriods(ctx, id, plan.Period); err != nil {
		return err
	}
	return a.readOutgoing(ctx, plan)
}

func (a *API) deleteOutgoing(ctx context.Context, id int) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if err := a.deleteOutgoingPeriods(ctx, id); err != nil {
		return err
	}
	return ignoreMissing(a.sdk.SiteSyncsOutgoing.Delete(ctx, id))
}

func outgoingCreateRequest(data *outgoingModel, registrationCode string) (*vergeos.SiteSyncOutgoingCreateRequest, error) {
	siteID, err := parseID(data.SiteID, "site")
	if err != nil {
		return nil, err
	}
	if !setString(data.Name) {
		return nil, fmt.Errorf("name is required")
	}
	threads, err := knownInt(data.Threads)
	if err != nil {
		return nil, err
	}
	fileThreads, err := knownInt(data.FileThreads)
	if err != nil {
		return nil, err
	}
	throttle, err := knownInt(data.SendThrottle)
	if err != nil {
		return nil, err
	}
	retryCount, err := knownInt(data.QueueRetryCount)
	if err != nil {
		return nil, err
	}
	retryInterval, err := knownInt(data.QueueRetryIntervalSeconds)
	if err != nil {
		return nil, err
	}
	return &vergeos.SiteSyncOutgoingCreateRequest{
		Site:                   siteID,
		Name:                   data.Name.ValueString(),
		Description:            stringValue(data.Description),
		Enabled:                vergeio.KnownBool(data.Enabled),
		URL:                    vergeio.KnownString(data.URL),
		RegistrationCode:       registrationCode,
		DestinationTier:        vergeio.KnownString(data.DestinationTier),
		Threads:                threads,
		FileThreads:            fileThreads,
		Encryption:             vergeio.KnownBool(data.Encryption),
		Compression:            vergeio.KnownBool(data.Compression),
		NetInteg:               vergeio.KnownBool(data.NetInteg),
		SendThrottle:           throttle,
		QueueRetryCount:        retryCount,
		QueueRetryIntervalSec:  retryInterval,
		QueueRetryIntervalMult: vergeio.KnownBool(data.QueueRetryIntervalMultiplier),
		Note:                   vergeio.KnownString(data.Note),
	}, nil
}

func outgoingUpdateRequest(plan, state *outgoingModel) (*vergeos.SiteSyncOutgoingUpdateRequest, error) {
	threads, err := changedInt(plan.Threads, state.Threads)
	if err != nil {
		return nil, err
	}
	fileThreads, err := changedInt(plan.FileThreads, state.FileThreads)
	if err != nil {
		return nil, err
	}
	throttle, err := changedInt(plan.SendThrottle, state.SendThrottle)
	if err != nil {
		return nil, err
	}
	retryCount, err := changedInt(plan.QueueRetryCount, state.QueueRetryCount)
	if err != nil {
		return nil, err
	}
	retryInterval, err := changedInt(plan.QueueRetryIntervalSeconds, state.QueueRetryIntervalSeconds)
	if err != nil {
		return nil, err
	}
	return &vergeos.SiteSyncOutgoingUpdateRequest{
		Name:                   vergeio.ChangedString(plan.Name, state.Name),
		Description:            vergeio.ChangedString(plan.Description, state.Description),
		Enabled:                vergeio.ChangedBool(plan.Enabled, state.Enabled),
		URL:                    vergeio.ChangedString(plan.URL, state.URL),
		DestinationTier:        vergeio.ChangedString(plan.DestinationTier, state.DestinationTier),
		Threads:                threads,
		FileThreads:            fileThreads,
		Encryption:             vergeio.ChangedBool(plan.Encryption, state.Encryption),
		Compression:            vergeio.ChangedBool(plan.Compression, state.Compression),
		NetInteg:               vergeio.ChangedBool(plan.NetInteg, state.NetInteg),
		SendThrottle:           throttle,
		QueueRetryCount:        retryCount,
		QueueRetryIntervalSec:  retryInterval,
		QueueRetryIntervalMult: vergeio.ChangedBool(plan.QueueRetryIntervalMultiplier, state.QueueRetryIntervalMultiplier),
		Note:                   vergeio.ChangedString(plan.Note, state.Note),
	}, nil
}

func outgoingUpdateEmpty(req *vergeos.SiteSyncOutgoingUpdateRequest) bool {
	if req == nil {
		return true
	}
	return req.Name == nil &&
		req.Description == nil &&
		req.Enabled == nil &&
		req.URL == nil &&
		req.DestinationTier == nil &&
		req.Threads == nil &&
		req.FileThreads == nil &&
		req.Encryption == nil &&
		req.Compression == nil &&
		req.NetInteg == nil &&
		req.SendThrottle == nil &&
		req.QueueRetryCount == nil &&
		req.QueueRetryIntervalSec == nil &&
		req.QueueRetryIntervalMult == nil &&
		req.Note == nil
}

func applyOutgoing(data *outgoingModel, got *vergeos.SiteSyncOutgoing, periods []vergeos.SiteSyncProfilePeriod, prior outgoingModel) {
	data.ID = idString(got.Key.Int())
	data.SiteID = idString(got.Site.Int())
	data.Name = types.StringValue(got.Name)
	data.Description = stringFromAPI(got.Description, prior.Description)
	data.URL = stringFromAPI(got.URL, prior.URL)
	data.DestinationTier = stringFromAPI(got.DestinationTier, prior.DestinationTier)
	data.Threads = intFromAPI(got.Threads, prior.Threads)
	data.FileThreads = intFromAPI(got.FileThreads, prior.FileThreads)
	data.Encryption = boolFromAPI(got.Encryption, prior.Encryption)
	data.Compression = boolFromAPI(got.Compression, prior.Compression)
	data.NetInteg = boolFromAPI(got.NetInteg, prior.NetInteg)
	data.SendThrottle = intFromAPI(got.SendThrottle, prior.SendThrottle)
	data.QueueRetryCount = intFromAPI(got.QueueRetryCount, prior.QueueRetryCount)
	data.QueueRetryIntervalSeconds = intFromAPI(got.QueueRetryIntervalSec, prior.QueueRetryIntervalSeconds)
	data.QueueRetryIntervalMultiplier = boolFromAPI(got.QueueRetryIntervalMult, prior.QueueRetryIntervalMultiplier)
	data.Note = stringFromAPI(got.Note, prior.Note)
	data.Enabled = types.BoolValue(got.Enabled)
	data.RegistrationCodeWO = types.StringNull()
	data.RegistrationCodeWOVersion = knownInt64(prior.RegistrationCodeWOVersion)
	data.Period = orderPeriods(prior.Period, periods)
	data.Status = volatileStringValue(got.Status)
	data.StatusInfo = volatileStringValue(got.StatusInfo)
	data.State = volatileStringValue(got.State)
	data.User = volatileStringValue(got.User)
	data.RemoteSiteID = volatileStringValue(got.RemoteSiteID)
	data.RemoteVSANHost = volatileStringValue(got.RemoteVSANHost)
	data.RemoteVSANPort = types.Int64Value(int64(got.RemoteVSANPort))
	data.RemoteSyncID = volatileStringValue(got.RemoteSyncID)
	data.RemoteMinSnapshots = types.Int64Value(int64(got.RemoteMinSnapshots))
	data.RemoteSnapsStatus = volatileStringValue(got.RemoteSnapsStatus)
	data.RemoteSnapsStatusInfo = volatileStringValue(got.RemoteSnapsStatusInfo)
	data.RemoteSnapsLastRefresh = types.Int64Value(got.RemoteSnapsLastRefresh)
	data.LastRun = types.Int64Value(got.LastRun)
}

func outgoingForState(data *outgoingModel) outgoingModel {
	stored := *data
	stored.ID = knownString(data.ID)
	stored.SiteID = knownString(data.SiteID)
	stored.Name = knownString(data.Name)
	stored.Description = knownString(data.Description)
	stored.URL = knownString(data.URL)
	stored.DestinationTier = knownString(data.DestinationTier)
	stored.Threads = knownInt64(data.Threads)
	stored.FileThreads = knownInt64(data.FileThreads)
	stored.Encryption = knownBool(data.Encryption)
	stored.Compression = knownBool(data.Compression)
	stored.NetInteg = knownBool(data.NetInteg)
	stored.SendThrottle = knownInt64(data.SendThrottle)
	stored.QueueRetryCount = knownInt64(data.QueueRetryCount)
	stored.QueueRetryIntervalSeconds = knownInt64(data.QueueRetryIntervalSeconds)
	stored.QueueRetryIntervalMultiplier = knownBool(data.QueueRetryIntervalMultiplier)
	stored.Note = knownString(data.Note)
	stored.Enabled = knownBool(data.Enabled)
	stored.RegistrationCodeWO = types.StringNull()
	stored.RegistrationCodeWOVersion = knownInt64(data.RegistrationCodeWOVersion)
	stored.Status = knownString(data.Status)
	stored.StatusInfo = knownString(data.StatusInfo)
	stored.State = knownString(data.State)
	stored.User = knownString(data.User)
	stored.RemoteSiteID = knownString(data.RemoteSiteID)
	stored.RemoteVSANHost = knownString(data.RemoteVSANHost)
	stored.RemoteVSANPort = knownInt64(data.RemoteVSANPort)
	stored.RemoteSyncID = knownString(data.RemoteSyncID)
	stored.RemoteMinSnapshots = knownInt64(data.RemoteMinSnapshots)
	stored.RemoteSnapsStatus = knownString(data.RemoteSnapsStatus)
	stored.RemoteSnapsStatusInfo = knownString(data.RemoteSnapsStatusInfo)
	stored.RemoteSnapsLastRefresh = knownInt64(data.RemoteSnapsLastRefresh)
	stored.LastRun = knownInt64(data.LastRun)
	if data.Period == nil {
		stored.Period = nil
		return stored
	}
	periods := make([]syncPeriodModel, len(data.Period))
	for i, period := range data.Period {
		periods[i] = periodForState(period)
	}
	stored.Period = periods
	return stored
}

func periodForState(period syncPeriodModel) syncPeriodModel {
	return syncPeriodModel{
		ProfilePeriod:     knownString(period.ProfilePeriod),
		Retention:         knownInt64(period.Retention),
		Priority:          knownInt64(period.Priority),
		DoNotExpire:       knownBool(period.DoNotExpire),
		DestinationPrefix: knownString(period.DestinationPrefix),
		Key:               knownString(period.Key),
		ScheduleTask:      knownInt64(period.ScheduleTask),
		Task:              knownInt64(period.Task),
	}
}

func validatePeriods(periods []syncPeriodModel) error {
	seen := map[string]struct{}{}
	for _, period := range periods {
		if !setString(period.ProfilePeriod) {
			return fmt.Errorf("profile_period is required")
		}
		id := period.ProfilePeriod.ValueString()
		if _, ok := seen[id]; ok {
			return fmt.Errorf("profile_period %s is listed more than once", id)
		}
		seen[id] = struct{}{}
		if period.Retention.IsNull() || period.Retention.IsUnknown() || period.Retention.ValueInt64() < 1 {
			return fmt.Errorf("retention for profile_period %s must be at least 1 second", id)
		}
	}
	return nil
}

// syncPeriods makes the outgoing sync's periods match desired. Periods are
// matched by profile_period. Updates are sent before deletes, and deletes
// before creates.
func (a *API) syncPeriods(ctx context.Context, syncID int, desired []syncPeriodModel) error {
	if err := validatePeriods(desired); err != nil {
		return err
	}
	existing, err := a.sdk.SiteSyncProfilePeriods.ListByOutgoingSync(ctx, syncID)
	if err != nil {
		return err
	}
	byPeriod := map[int]vergeos.SiteSyncProfilePeriod{}
	for _, period := range existing {
		id := period.ProfilePeriod.Int()
		if _, ok := byPeriod[id]; ok {
			return fmt.Errorf("outgoing sync %d has more than one period for profile_period %d", syncID, id)
		}
		byPeriod[id] = period
	}
	wanted := map[int]syncPeriodModel{}
	for _, period := range desired {
		id, err := parseID(period.ProfilePeriod, "profile period")
		if err != nil {
			return err
		}
		wanted[id] = period
		current, ok := byPeriod[id]
		if !ok {
			continue
		}
		if err := a.updateSyncPeriod(ctx, current, period); err != nil {
			return err
		}
	}
	for id, period := range byPeriod {
		if _, ok := wanted[id]; ok {
			continue
		}
		if err := ignoreMissing(a.sdk.SiteSyncProfilePeriods.Delete(ctx, period.Key.Int())); err != nil {
			return fmt.Errorf("delete site sync period %d: %w", period.Key.Int(), err)
		}
	}
	for _, period := range desired {
		id, err := parseID(period.ProfilePeriod, "profile period")
		if err != nil {
			return err
		}
		if _, ok := byPeriod[id]; ok {
			continue
		}
		if err := a.createSyncPeriod(ctx, syncID, id, period); err != nil {
			return err
		}
	}
	return nil
}

func (a *API) createSyncPeriod(ctx context.Context, syncID, profilePeriod int, period syncPeriodModel) error {
	retention, err := intPtr(period.Retention.ValueInt64())
	if err != nil {
		return err
	}
	if *retention < 1 {
		return fmt.Errorf("retention for profile_period %d must be at least 1 second", profilePeriod)
	}
	priority, err := knownInt(period.Priority)
	if err != nil {
		return err
	}
	_, err = a.sdk.SiteSyncProfilePeriods.Create(ctx, &vergeos.SiteSyncProfilePeriodCreateRequest{
		SiteSyncsOutgoing: syncID,
		ProfilePeriod:     profilePeriod,
		Retention:         *retention,
		Priority:          priority,
		DoNotExpire:       vergeio.KnownBool(period.DoNotExpire),
		DestinationPrefix: vergeio.KnownString(period.DestinationPrefix),
	})
	if err != nil {
		return fmt.Errorf("create site sync period for profile_period %d: %w", profilePeriod, err)
	}
	return nil
}

func (a *API) updateSyncPeriod(ctx context.Context, current vergeos.SiteSyncProfilePeriod, period syncPeriodModel) error {
	req := &vergeos.SiteSyncProfilePeriodUpdateRequest{}
	changed := false
	if !period.Retention.IsNull() && !period.Retention.IsUnknown() && int(period.Retention.ValueInt64()) != current.Retention {
		retention, err := intPtr(period.Retention.ValueInt64())
		if err != nil {
			return err
		}
		req.Retention = retention
		changed = true
	}
	if !period.Priority.IsNull() && !period.Priority.IsUnknown() && int(period.Priority.ValueInt64()) != current.Priority {
		priority, err := intPtr(period.Priority.ValueInt64())
		if err != nil {
			return err
		}
		req.Priority = priority
		changed = true
	}
	if !period.DoNotExpire.IsNull() && !period.DoNotExpire.IsUnknown() && period.DoNotExpire.ValueBool() != current.DoNotExpire {
		value := period.DoNotExpire.ValueBool()
		req.DoNotExpire = &value
		changed = true
	}
	if !period.DestinationPrefix.IsNull() && !period.DestinationPrefix.IsUnknown() && period.DestinationPrefix.ValueString() != current.DestinationPrefix {
		value := period.DestinationPrefix.ValueString()
		req.DestinationPrefix = &value
		changed = true
	}
	if !changed {
		return nil
	}
	if _, err := a.sdk.SiteSyncProfilePeriods.Update(ctx, current.Key.Int(), req); err != nil {
		return fmt.Errorf("update site sync period %d: %w", current.Key.Int(), err)
	}
	return nil
}

func (a *API) deleteOutgoingPeriods(ctx context.Context, syncID int) error {
	periods, err := a.sdk.SiteSyncProfilePeriods.ListByOutgoingSync(ctx, syncID)
	if listMissing(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list site sync periods for outgoing sync %d: %w", syncID, err)
	}
	for _, period := range periods {
		if err := ignoreMissing(a.sdk.SiteSyncProfilePeriods.Delete(ctx, period.Key.Int())); err != nil {
			return fmt.Errorf("delete site sync period %d: %w", period.Key.Int(), err)
		}
	}
	return nil
}

func orderPeriods(prior []syncPeriodModel, api []vergeos.SiteSyncProfilePeriod) []syncPeriodModel {
	byProfile := map[string]vergeos.SiteSyncProfilePeriod{}
	for _, period := range api {
		byProfile[strconv.Itoa(period.ProfilePeriod.Int())] = period
	}
	var ordered []syncPeriodModel
	seen := map[string]bool{}
	for _, period := range prior {
		if !setString(period.ProfilePeriod) {
			continue
		}
		id := period.ProfilePeriod.ValueString()
		got, ok := byProfile[id]
		if !ok {
			continue
		}
		ordered = append(ordered, periodFromAPI(got, period))
		seen[id] = true
	}
	var extras []vergeos.SiteSyncProfilePeriod
	for _, period := range api {
		id := strconv.Itoa(period.ProfilePeriod.Int())
		if seen[id] {
			continue
		}
		extras = append(extras, period)
	}
	sort.Slice(extras, func(i, j int) bool {
		return extras[i].ProfilePeriod.Int() < extras[j].ProfilePeriod.Int()
	})
	for _, period := range extras {
		ordered = append(ordered, periodFromAPI(period, syncPeriodModel{}))
	}
	if len(ordered) == 0 && prior == nil {
		return nil
	}
	return ordered
}

func periodFromAPI(got vergeos.SiteSyncProfilePeriod, prior syncPeriodModel) syncPeriodModel {
	return syncPeriodModel{
		ProfilePeriod:     types.StringValue(strconv.Itoa(got.ProfilePeriod.Int())),
		Retention:         types.Int64Value(int64(got.Retention)),
		Priority:          intFromAPI(got.Priority, prior.Priority),
		DoNotExpire:       boolFromAPI(got.DoNotExpire, prior.DoNotExpire),
		DestinationPrefix: stringFromAPI(got.DestinationPrefix, prior.DestinationPrefix),
		Key:               idString(got.Key.Int()),
		ScheduleTask:      intFromAPI(got.ScheduleTask.Int(), prior.ScheduleTask),
		Task:              intFromAPI(got.Task.Int(), prior.Task),
	}
}

func (a *API) readIncomingStatus(ctx context.Context, data *statusModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if err := validateStatusLookup(data); err != nil {
		return err
	}
	var got *vergeos.SiteSyncIncoming
	var err error
	if setString(data.ID) {
		var id int
		id, err = parseID(data.ID, "incoming sync")
		if err != nil {
			return err
		}
		got, err = a.sdk.SiteSyncsIncoming.Get(ctx, id)
	} else {
		var siteID int
		siteID, err = parseID(data.SiteID, "site")
		if err != nil {
			return err
		}
		got, err = a.sdk.SiteSyncsIncoming.GetByName(ctx, siteID, data.Name.ValueString())
	}
	if err != nil {
		return err
	}
	return a.fillStatus(data, "incoming", got.Name, got.Key.Int(), got.Site.Int(), got.Enabled, got.Status, got.StatusInfo, got.State, got.LastSync)
}

func (a *API) readOutgoingStatus(ctx context.Context, data *statusModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if err := validateStatusLookup(data); err != nil {
		return err
	}
	var got *vergeos.SiteSyncOutgoing
	var err error
	if setString(data.ID) {
		var id int
		id, err = parseID(data.ID, "outgoing sync")
		if err != nil {
			return err
		}
		got, err = a.sdk.SiteSyncsOutgoing.Get(ctx, id)
	} else {
		var siteID int
		siteID, err = parseID(data.SiteID, "site")
		if err != nil {
			return err
		}
		got, err = a.sdk.SiteSyncsOutgoing.GetByName(ctx, siteID, data.Name.ValueString())
	}
	if err != nil {
		return err
	}
	return a.fillStatus(data, "outgoing", got.Name, got.Key.Int(), got.Site.Int(), got.Enabled, got.Status, got.StatusInfo, got.State, got.LastRun)
}

func (a *API) fillStatus(data *statusModel, kind, name string, id, siteID int, enabled bool, status, statusInfo, state string, last int64) error {
	data.ID = idString(id)
	data.SiteID = idString(siteID)
	data.Name = types.StringValue(name)
	data.Enabled = types.BoolValue(enabled)
	data.Status = volatileStringValue(status)
	data.StatusInfo = volatileStringValue(statusInfo)
	data.State = volatileStringValue(state)
	data.LastActivity = types.Int64Value(last)
	label := name
	if label == "" {
		label = strconv.Itoa(id)
	}
	lag, never, err := syncLag(kind, label, last, a.now(), data.MaxLagSeconds)
	data.LagSeconds = lag
	data.NeverSynced = never
	return err
}

func validateStatusLookup(data *statusModel) error {
	hasID := setString(data.ID)
	hasSite := setString(data.SiteID)
	hasName := setString(data.Name)
	if hasID && !hasSite && !hasName {
		return nil
	}
	if !hasID && hasSite && hasName {
		return nil
	}
	return fmt.Errorf("set id, or both site_id and name")
}

// syncLag reports how far last is behind now. maxLag fails the read when the
// sync has not run, or when the gap is larger than that many seconds.
func syncLag(kind, label string, last, now int64, maxLag types.Int64) (types.Int64, types.Bool, error) {
	hasMax := !maxLag.IsNull() && !maxLag.IsUnknown()
	var max int64
	if hasMax {
		max = maxLag.ValueInt64()
	}
	if last <= 0 {
		if hasMax {
			return types.Int64Null(), types.BoolValue(true), fmt.Errorf("%s sync %s has not run", kind, label)
		}
		return types.Int64Null(), types.BoolValue(true), nil
	}
	seconds := now - last
	if seconds < 0 {
		seconds = 0
	}
	if hasMax && seconds > max {
		return types.Int64Value(seconds), types.BoolValue(false), fmt.Errorf("%s sync %s last ran %d seconds ago, past max_lag_seconds %d", kind, label, seconds, max)
	}
	return types.Int64Value(seconds), types.BoolValue(false), nil
}
