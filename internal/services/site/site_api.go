// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"fmt"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/verge-io/govergeos"
)

func (a *API) createSite(ctx context.Context, data *siteModel, password string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if !setString(data.Name) {
		return fmt.Errorf("name is required")
	}
	if !setString(data.URL) {
		return fmt.Errorf("url is required")
	}
	auto := false
	interval, err := knownInt(data.StatisticsInterval)
	if err != nil {
		return err
	}
	retention, err := knownInt(data.StatisticsRetention)
	if err != nil {
		return err
	}
	req := &vergeos.SiteCreateRequest{
		Name:                     data.Name.ValueString(),
		URL:                      data.URL.ValueString(),
		Description:              stringValue(data.Description),
		Domain:                   stringValue(data.Domain),
		City:                     stringValue(data.City),
		Country:                  stringValue(data.Country),
		Timezone:                 stringValue(data.Timezone),
		Enabled:                  vergeio.KnownBool(data.Enabled),
		AllowInsecure:            vergeio.KnownBool(data.AllowInsecure),
		Latitude:                 knownFloatPtr(data.Latitude),
		Longitude:                knownFloatPtr(data.Longitude),
		ConfigCloudSnapshots:     vergeio.KnownString(data.ConfigCloudSnapshots),
		ConfigStatistics:         vergeio.KnownString(data.ConfigStatistics),
		ConfigManagement:         vergeio.KnownString(data.ConfigManagement),
		ConfigRepairServer:       vergeio.KnownString(data.ConfigRepairServer),
		StatisticsInterval:       interval,
		StatisticsRetention:      retention,
		RequestURL:               vergeio.KnownString(data.RequestURL),
		AuthUser:                 stringValue(data.AuthUser),
		AuthPassword:             password,
		AutomaticallyCreateSyncs: &auto,
	}
	created, err := a.sdk.Sites.Create(ctx, req)
	if err != nil {
		return err
	}
	applySite(data, created, *data)
	return nil
}

func (a *API) readSite(ctx context.Context, data *siteModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseID(data.ID, "site")
	if err != nil {
		return err
	}
	got, err := a.sdk.Sites.Get(ctx, id)
	if err != nil {
		return err
	}
	applySite(data, got, *data)
	return nil
}

func (a *API) updateSite(ctx context.Context, plan, state *siteModel, password string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseID(state.ID, "site")
	if err != nil {
		return err
	}
	interval, err := changedInt(plan.StatisticsInterval, state.StatisticsInterval)
	if err != nil {
		return err
	}
	retention, err := changedInt(plan.StatisticsRetention, state.StatisticsRetention)
	if err != nil {
		return err
	}
	req := &vergeos.SiteUpdateRequest{
		Name:                 vergeio.ChangedString(plan.Name, state.Name),
		Description:          vergeio.ChangedString(plan.Description, state.Description),
		Enabled:              vergeio.ChangedBool(plan.Enabled, state.Enabled),
		Domain:               vergeio.ChangedString(plan.Domain, state.Domain),
		City:                 vergeio.ChangedString(plan.City, state.City),
		Country:              vergeio.ChangedString(plan.Country, state.Country),
		Latitude:             changedFloat(plan.Latitude, state.Latitude),
		Longitude:            changedFloat(plan.Longitude, state.Longitude),
		Timezone:             vergeio.ChangedString(plan.Timezone, state.Timezone),
		URL:                  vergeio.ChangedString(plan.URL, state.URL),
		AllowInsecure:        vergeio.ChangedBool(plan.AllowInsecure, state.AllowInsecure),
		ConfigCloudSnapshots: vergeio.ChangedString(plan.ConfigCloudSnapshots, state.ConfigCloudSnapshots),
		ConfigStatistics:     vergeio.ChangedString(plan.ConfigStatistics, state.ConfigStatistics),
		ConfigManagement:     vergeio.ChangedString(plan.ConfigManagement, state.ConfigManagement),
		ConfigRepairServer:   vergeio.ChangedString(plan.ConfigRepairServer, state.ConfigRepairServer),
		StatisticsInterval:   interval,
		StatisticsRetention:  retention,
		RequestURL:           vergeio.ChangedString(plan.RequestURL, state.RequestURL),
		RemoteUser:           vergeio.ChangedString(plan.AuthUser, state.AuthUser),
	}
	if versionChanged(plan.AuthPasswordWOVersion, state.AuthPasswordWOVersion) && password != "" {
		req.RemotePassword = &password
		if req.RemoteUser == nil {
			req.RemoteUser = vergeio.KnownString(plan.AuthUser)
		}
	}
	if !siteUpdateEmpty(req) {
		if _, err := a.sdk.Sites.Update(ctx, id, req); err != nil {
			return err
		}
	}
	plan.ID = state.ID
	return a.readSite(ctx, plan)
}

func (a *API) deleteSite(ctx context.Context, id int) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	if err := a.deleteSiteChildren(ctx, id); err != nil {
		return err
	}
	return ignoreMissing(a.sdk.Sites.Delete(ctx, id))
}

func (a *API) deleteSiteChildren(ctx context.Context, siteID int) error {
	outgoing, err := a.sdk.SiteSyncsOutgoing.ListBySite(ctx, siteID)
	if listMissing(err) {
		outgoing = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list outgoing syncs for site %d: %w", siteID, err)
	}
	for _, sync := range outgoing {
		if err := a.deleteOutgoingPeriods(ctx, sync.Key.Int()); err != nil {
			return err
		}
		if err := ignoreMissing(a.sdk.SiteSyncsOutgoing.Delete(ctx, sync.Key.Int())); err != nil {
			return fmt.Errorf("delete outgoing sync %d: %w", sync.Key.Int(), err)
		}
	}
	incoming, err := a.sdk.SiteSyncsIncoming.ListBySite(ctx, siteID)
	if listMissing(err) {
		incoming = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list incoming syncs for site %d: %w", siteID, err)
	}
	for _, sync := range incoming {
		if err := ignoreMissing(a.sdk.SiteSyncsIncoming.Delete(ctx, sync.Key.Int())); err != nil {
			return fmt.Errorf("delete incoming sync %d: %w", sync.Key.Int(), err)
		}
	}
	return nil
}

func siteUpdateEmpty(req *vergeos.SiteUpdateRequest) bool {
	if req == nil {
		return true
	}
	return req.Name == nil &&
		req.Description == nil &&
		req.Enabled == nil &&
		req.Domain == nil &&
		req.City == nil &&
		req.Country == nil &&
		req.Latitude == nil &&
		req.Longitude == nil &&
		req.Timezone == nil &&
		req.URL == nil &&
		req.AllowInsecure == nil &&
		req.ConfigCloudSnapshots == nil &&
		req.ConfigStatistics == nil &&
		req.ConfigManagement == nil &&
		req.ConfigRepairServer == nil &&
		req.StatisticsInterval == nil &&
		req.StatisticsRetention == nil &&
		req.RequestURL == nil &&
		req.RemoteUser == nil &&
		req.RemotePassword == nil
}

func applySite(data *siteModel, got *vergeos.Site, prior siteModel) {
	data.ID = idString(got.Key.Int())
	data.Name = types.StringValue(got.Name)
	data.URL = types.StringValue(got.URL)
	data.SiteID = stringFromAPI(got.ID, prior.SiteID)
	data.Description = stringFromAPI(got.Description, prior.Description)
	data.Domain = stringFromAPI(got.Domain, prior.Domain)
	data.City = stringFromAPI(got.City, prior.City)
	data.Country = stringFromAPI(got.Country, prior.Country)
	data.Timezone = stringFromAPI(got.Timezone, prior.Timezone)
	data.AllowInsecure = boolFromAPI(got.AllowInsecure, prior.AllowInsecure)
	data.ConfigCloudSnapshots = stringFromAPI(got.ConfigCloudSnapshots, prior.ConfigCloudSnapshots)
	data.ConfigStatistics = stringFromAPI(got.ConfigStatistics, prior.ConfigStatistics)
	data.ConfigManagement = stringFromAPI(got.ConfigManagement, prior.ConfigManagement)
	data.ConfigRepairServer = stringFromAPI(got.ConfigRepairServer, prior.ConfigRepairServer)
	data.StatisticsInterval = intFromAPI(got.StatisticsInterval, prior.StatisticsInterval)
	data.StatisticsRetention = intFromAPI(got.StatisticsRetention, prior.StatisticsRetention)
	data.RequestURL = stringFromAPI(got.RequestURL, prior.RequestURL)
	data.Latitude = floatFromAPI(got.Latitude, prior.Latitude)
	data.Longitude = floatFromAPI(got.Longitude, prior.Longitude)
	data.Enabled = types.BoolValue(got.Enabled)
	data.AuthUser = knownString(prior.AuthUser)
	data.AuthPasswordWO = types.StringNull()
	data.AuthPasswordWOVersion = knownInt64(prior.AuthPasswordWOVersion)
	data.Status = volatileStringValue(got.Status)
	data.StatusInfo = volatileStringValue(got.StatusInfo)
	data.AuthenticationStatus = volatileStringValue(got.AuthenticationStatus)
	data.VSANHost = volatileStringValue(got.VSANHost)
	data.VSANPort = types.Int64Value(int64(got.VSANPort))
	data.IsTenant = types.BoolValue(got.IsTenant)
	data.IncomingSyncsEnabled = types.BoolValue(got.IncomingSyncsEnabled)
	data.OutgoingSyncsEnabled = types.BoolValue(got.OutgoingSyncsEnabled)
	data.RepairsOutgoingEnabled = types.BoolValue(got.RepairsOutgoingEnabled)
	data.IncomingStatsEnabled = types.BoolValue(got.IncomingStatsEnabled)
	data.OutgoingStatsEnabled = types.BoolValue(got.OutgoingStatsEnabled)
	data.OutgoingManagementEnabled = types.BoolValue(got.OutgoingManagementEnabled)
	data.IncomingManagementEnabled = types.BoolValue(got.IncomingManagementEnabled)
	data.RemoteUser = volatileStringValue(got.RemoteUser)
	data.LastStatUpdate = types.Int64Value(got.LastStatUpdate)
	data.Created = types.Int64Value(got.Created)
	data.Modified = types.Int64Value(got.Modified)
	data.Creator = stringFromAPI(got.Creator, prior.Creator)
}

func siteForState(data *siteModel) siteModel {
	stored := *data
	stored.ID = knownString(data.ID)
	stored.Name = knownString(data.Name)
	stored.URL = knownString(data.URL)
	stored.Description = knownString(data.Description)
	stored.Domain = knownString(data.Domain)
	stored.City = knownString(data.City)
	stored.Country = knownString(data.Country)
	stored.Timezone = knownString(data.Timezone)
	stored.AllowInsecure = knownBool(data.AllowInsecure)
	stored.ConfigCloudSnapshots = knownString(data.ConfigCloudSnapshots)
	stored.ConfigStatistics = knownString(data.ConfigStatistics)
	stored.ConfigManagement = knownString(data.ConfigManagement)
	stored.ConfigRepairServer = knownString(data.ConfigRepairServer)
	stored.StatisticsInterval = knownInt64(data.StatisticsInterval)
	stored.StatisticsRetention = knownInt64(data.StatisticsRetention)
	stored.RequestURL = knownString(data.RequestURL)
	stored.Latitude = knownFloat(data.Latitude)
	stored.Longitude = knownFloat(data.Longitude)
	stored.Enabled = knownBool(data.Enabled)
	stored.AuthUser = knownString(data.AuthUser)
	stored.AuthPasswordWO = types.StringNull()
	stored.AuthPasswordWOVersion = knownInt64(data.AuthPasswordWOVersion)
	stored.SiteID = knownString(data.SiteID)
	stored.Status = knownString(data.Status)
	stored.StatusInfo = knownString(data.StatusInfo)
	stored.AuthenticationStatus = knownString(data.AuthenticationStatus)
	stored.VSANHost = knownString(data.VSANHost)
	stored.VSANPort = knownInt64(data.VSANPort)
	stored.IsTenant = knownBool(data.IsTenant)
	stored.IncomingSyncsEnabled = knownBool(data.IncomingSyncsEnabled)
	stored.OutgoingSyncsEnabled = knownBool(data.OutgoingSyncsEnabled)
	stored.RepairsOutgoingEnabled = knownBool(data.RepairsOutgoingEnabled)
	stored.IncomingStatsEnabled = knownBool(data.IncomingStatsEnabled)
	stored.OutgoingStatsEnabled = knownBool(data.OutgoingStatsEnabled)
	stored.OutgoingManagementEnabled = knownBool(data.OutgoingManagementEnabled)
	stored.IncomingManagementEnabled = knownBool(data.IncomingManagementEnabled)
	stored.RemoteUser = knownString(data.RemoteUser)
	stored.LastStatUpdate = knownInt64(data.LastStatUpdate)
	stored.Created = knownInt64(data.Created)
	stored.Modified = knownInt64(data.Modified)
	stored.Creator = knownString(data.Creator)
	return stored
}
