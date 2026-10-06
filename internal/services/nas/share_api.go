// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

func (a *API) createCIFS(ctx context.Context, data *cifsModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	req, err := cifsCreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.VolumeCIFSShares.Create(ctx, req)
	if err != nil {
		return err
	}
	id := rowID(created.Key, created.ID)
	data.ID = types.StringValue(id)
	if err := a.readCIFS(ctx, data); err != nil {
		return abandon(err, "CIFS share", id, a.deleteCIFS(ctx, id))
	}
	return nil
}

func (a *API) readCIFS(ctx context.Context, data *cifsModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(data.ID, "CIFS share")
	if err != nil {
		return err
	}
	share, err := a.sdk.VolumeCIFSShares.Get(ctx, id)
	if err != nil {
		return err
	}
	applyCIFS(data, share)
	return nil
}

func (a *API) updateCIFS(ctx context.Context, plan, state *cifsModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(state.ID, "CIFS share")
	if err != nil {
		return err
	}
	plan.ID = state.ID
	if req := cifsUpdateRequest(plan, state); req != nil {
		if _, err := a.sdk.VolumeCIFSShares.Update(ctx, id, req); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("updated CIFS share %s", id))
	}
	return a.readCIFS(ctx, plan)
}

func (a *API) deleteCIFSShare(ctx context.Context, data *cifsModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(data.ID, "CIFS share")
	if err != nil {
		return err
	}
	return a.deleteCIFS(ctx, id)
}

func (a *API) deleteCIFS(ctx context.Context, id string) error {
	if err := a.sdk.VolumeCIFSShares.Delete(ctx, id); err != nil && !missing(err) {
		return err
	}
	return nil
}

func (a *API) createNFS(ctx context.Context, data *nfsModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	req, err := nfsCreateRequest(data)
	if err != nil {
		return err
	}
	created, err := a.sdk.VolumeNFSShares.Create(ctx, req)
	if err != nil {
		return err
	}
	id := rowID(created.Key, created.ID)
	data.ID = types.StringValue(id)
	if err := a.readNFS(ctx, data); err != nil {
		return abandon(err, "NFS share", id, a.deleteNFS(ctx, id))
	}
	return nil
}

func (a *API) readNFS(ctx context.Context, data *nfsModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(data.ID, "NFS share")
	if err != nil {
		return err
	}
	share, err := a.sdk.VolumeNFSShares.Get(ctx, id)
	if err != nil {
		return err
	}
	applyNFS(data, share)
	return nil
}

func (a *API) updateNFS(ctx context.Context, plan, state *nfsModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(state.ID, "NFS share")
	if err != nil {
		return err
	}
	plan.ID = state.ID
	if req := nfsUpdateRequest(plan, state); req != nil {
		if _, err := a.sdk.VolumeNFSShares.Update(ctx, id, req); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("updated NFS share %s", id))
	}
	return a.readNFS(ctx, plan)
}

func (a *API) deleteNFSShare(ctx context.Context, data *nfsModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(data.ID, "NFS share")
	if err != nil {
		return err
	}
	return a.deleteNFS(ctx, id)
}

func (a *API) deleteNFS(ctx context.Context, id string) error {
	if err := a.sdk.VolumeNFSShares.Delete(ctx, id); err != nil && !missing(err) {
		return err
	}
	return nil
}

func cifsCreateRequest(data *cifsModel) (*vergeos.VolumeCIFSShareCreateRequest, error) {
	name := strings.TrimSpace(stringOrEmpty(data.Name))
	volumeID, err := parseObjectID(data.VolumeID, "volume")
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("CIFS share name is required")
	}
	return &vergeos.VolumeCIFSShareCreateRequest{
		Name:           name,
		Volume:         volumeID,
		Description:    stringOrEmpty(data.Description),
		Enabled:        vergeio.KnownBool(data.Enabled),
		SharePath:      vergeio.KnownString(data.SharePath),
		Comment:        vergeio.KnownString(data.Comment),
		ValidUsers:     vergeio.KnownString(data.ValidUsers),
		ValidGroups:    vergeio.KnownString(data.ValidGroups),
		AdminUsers:     vergeio.KnownString(data.AdminUsers),
		AdminGroups:    vergeio.KnownString(data.AdminGroups),
		HostAllow:      vergeio.KnownString(data.HostAllow),
		HostDeny:       vergeio.KnownString(data.HostDeny),
		ForceUser:      vergeio.KnownString(data.ForceUser),
		ForceGroup:     vergeio.KnownString(data.ForceGroup),
		Browseable:     vergeio.KnownBool(data.Browseable),
		ReadOnly:       vergeio.KnownBool(data.ReadOnly),
		GuestOK:        vergeio.KnownBool(data.GuestOK),
		GuestOnly:      vergeio.KnownBool(data.GuestOnly),
		Advanced:       vergeio.KnownString(data.Advanced),
		VFSShadowCopy2: vergeio.KnownBool(data.VFSShadowCopy2),
	}, nil
}

func cifsUpdateRequest(plan, state *cifsModel) *vergeos.VolumeCIFSShareUpdateRequest {
	req := &vergeos.VolumeCIFSShareUpdateRequest{
		Name:           vergeio.ChangedString(plan.Name, state.Name),
		Description:    vergeio.ChangedString(plan.Description, state.Description),
		Enabled:        vergeio.ChangedBool(plan.Enabled, state.Enabled),
		SharePath:      vergeio.ChangedString(plan.SharePath, state.SharePath),
		Comment:        vergeio.ChangedString(plan.Comment, state.Comment),
		ValidUsers:     vergeio.ChangedString(plan.ValidUsers, state.ValidUsers),
		ValidGroups:    vergeio.ChangedString(plan.ValidGroups, state.ValidGroups),
		AdminUsers:     vergeio.ChangedString(plan.AdminUsers, state.AdminUsers),
		AdminGroups:    vergeio.ChangedString(plan.AdminGroups, state.AdminGroups),
		HostAllow:      vergeio.ChangedString(plan.HostAllow, state.HostAllow),
		HostDeny:       vergeio.ChangedString(plan.HostDeny, state.HostDeny),
		ForceUser:      vergeio.ChangedString(plan.ForceUser, state.ForceUser),
		ForceGroup:     vergeio.ChangedString(plan.ForceGroup, state.ForceGroup),
		Browseable:     vergeio.ChangedBool(plan.Browseable, state.Browseable),
		ReadOnly:       vergeio.ChangedBool(plan.ReadOnly, state.ReadOnly),
		GuestOK:        vergeio.ChangedBool(plan.GuestOK, state.GuestOK),
		GuestOnly:      vergeio.ChangedBool(plan.GuestOnly, state.GuestOnly),
		Advanced:       vergeio.ChangedString(plan.Advanced, state.Advanced),
		VFSShadowCopy2: vergeio.ChangedBool(plan.VFSShadowCopy2, state.VFSShadowCopy2),
	}
	if req.Name == nil && req.Description == nil && req.Enabled == nil && req.SharePath == nil &&
		req.Comment == nil && req.ValidUsers == nil && req.ValidGroups == nil && req.AdminUsers == nil &&
		req.AdminGroups == nil && req.HostAllow == nil && req.HostDeny == nil && req.ForceUser == nil &&
		req.ForceGroup == nil && req.Browseable == nil && req.ReadOnly == nil && req.GuestOK == nil &&
		req.GuestOnly == nil && req.Advanced == nil && req.VFSShadowCopy2 == nil {
		return nil
	}
	return req
}

func applyCIFS(data *cifsModel, share *vergeos.VolumeCIFSShare) {
	data.ID = types.StringValue(rowID(share.Key, share.ID))
	data.VolumeID = types.StringValue(share.Volume)
	data.Name = types.StringValue(share.Name)
	data.Description = types.StringValue(share.Description)
	data.Enabled = types.BoolValue(share.Enabled)
	data.SharePath = types.StringValue(share.SharePath)
	data.Comment = types.StringValue(share.Comment)
	data.ValidUsers = types.StringValue(share.ValidUsers)
	data.ValidGroups = types.StringValue(share.ValidGroups)
	data.AdminUsers = types.StringValue(share.AdminUsers)
	data.AdminGroups = types.StringValue(share.AdminGroups)
	data.HostAllow = types.StringValue(share.HostAllow)
	data.HostDeny = types.StringValue(share.HostDeny)
	data.ForceUser = types.StringValue(share.ForceUser)
	data.ForceGroup = types.StringValue(share.ForceGroup)
	data.Browseable = types.BoolValue(share.Browseable)
	data.ReadOnly = types.BoolValue(share.ReadOnly)
	data.GuestOK = types.BoolValue(share.GuestOK)
	data.GuestOnly = types.BoolValue(share.GuestOnly)
	data.Advanced = types.StringValue(share.Advanced)
	data.VFSShadowCopy2 = types.BoolValue(share.VFSShadowCopy2)
	data.Created = types.Int64Value(share.Created)
	data.Modified = types.Int64Value(share.Modified)
	data.Status = types.Int64Value(int64(share.Status.Int()))
}

func nfsCreateRequest(data *nfsModel) (*vergeos.VolumeNFSShareCreateRequest, error) {
	name := strings.TrimSpace(stringOrEmpty(data.Name))
	volumeID, err := parseObjectID(data.VolumeID, "volume")
	if err != nil {
		return nil, err
	}
	if name == "" {
		return nil, fmt.Errorf("NFS share name is required")
	}
	return &vergeos.VolumeNFSShareCreateRequest{
		Name:         name,
		Volume:       volumeID,
		Description:  stringOrEmpty(data.Description),
		Enabled:      vergeio.KnownBool(data.Enabled),
		SharePath:    vergeio.KnownString(data.SharePath),
		AllowedHosts: vergeio.KnownString(data.AllowedHosts),
		AllowAll:     vergeio.KnownBool(data.AllowAll),
		FSID:         vergeio.KnownString(data.FSID),
		AnonUID:      vergeio.KnownString(data.AnonUID),
		AnonGID:      vergeio.KnownString(data.AnonGID),
		NoACL:        vergeio.KnownBool(data.NoACL),
		Insecure:     vergeio.KnownBool(data.Insecure),
		Async:        vergeio.KnownBool(data.Async),
		Squash:       vergeio.KnownString(data.Squash),
		DataAccess:   vergeio.KnownString(data.DataAccess),
	}, nil
}

func nfsUpdateRequest(plan, state *nfsModel) *vergeos.VolumeNFSShareUpdateRequest {
	req := &vergeos.VolumeNFSShareUpdateRequest{
		Name:         vergeio.ChangedString(plan.Name, state.Name),
		Description:  vergeio.ChangedString(plan.Description, state.Description),
		Enabled:      vergeio.ChangedBool(plan.Enabled, state.Enabled),
		SharePath:    vergeio.ChangedString(plan.SharePath, state.SharePath),
		AllowedHosts: vergeio.ChangedString(plan.AllowedHosts, state.AllowedHosts),
		AllowAll:     vergeio.ChangedBool(plan.AllowAll, state.AllowAll),
		FSID:         vergeio.ChangedString(plan.FSID, state.FSID),
		AnonUID:      vergeio.ChangedString(plan.AnonUID, state.AnonUID),
		AnonGID:      vergeio.ChangedString(plan.AnonGID, state.AnonGID),
		NoACL:        vergeio.ChangedBool(plan.NoACL, state.NoACL),
		Insecure:     vergeio.ChangedBool(plan.Insecure, state.Insecure),
		Async:        vergeio.ChangedBool(plan.Async, state.Async),
		Squash:       vergeio.ChangedString(plan.Squash, state.Squash),
		DataAccess:   vergeio.ChangedString(plan.DataAccess, state.DataAccess),
	}
	if req.Name == nil && req.Description == nil && req.Enabled == nil && req.SharePath == nil &&
		req.AllowedHosts == nil && req.AllowAll == nil && req.FSID == nil && req.AnonUID == nil &&
		req.AnonGID == nil && req.NoACL == nil && req.Insecure == nil && req.Async == nil &&
		req.Squash == nil && req.DataAccess == nil {
		return nil
	}
	return req
}

func applyNFS(data *nfsModel, share *vergeos.VolumeNFSShare) {
	data.ID = types.StringValue(rowID(share.Key, share.ID))
	data.VolumeID = types.StringValue(share.Volume)
	data.Name = types.StringValue(share.Name)
	data.Description = types.StringValue(share.Description)
	data.Enabled = types.BoolValue(share.Enabled)
	data.SharePath = types.StringValue(share.SharePath)
	data.AllowedHosts = types.StringValue(share.AllowedHosts)
	data.AllowAll = types.BoolValue(share.AllowAll)
	data.FSID = types.StringValue(share.FSID)
	data.AnonUID = types.StringValue(share.AnonUID)
	data.AnonGID = types.StringValue(share.AnonGID)
	data.NoACL = types.BoolValue(share.NoACL)
	data.Insecure = types.BoolValue(share.Insecure)
	data.Async = types.BoolValue(share.Async)
	data.Squash = types.StringValue(share.Squash)
	data.DataAccess = types.StringValue(share.DataAccess)
	data.Created = types.Int64Value(share.Created)
	data.Modified = types.Int64Value(share.Modified)
	data.Status = types.Int64Value(int64(share.Status.Int()))
}
