// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

type volumeSecrets struct {
	EncryptionKey  string
	SendEncryption bool
	CIFSPassword   string
	SendCIFS       bool
}

func (a *API) createVolume(ctx context.Context, data *volumeModel, secrets volumeSecrets) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	req, err := volumeCreateRequest(data, secrets)
	if err != nil {
		return err
	}
	created, err := a.sdk.Volumes.Create(ctx, req)
	if err != nil {
		return err
	}
	id := rowID(created.Key, created.ID)
	data.ID = types.StringValue(id)
	if err := a.readVolume(ctx, data); err != nil {
		return abandon(err, "volume", id, a.DeleteVolume(ctx, id))
	}
	return nil
}

func (a *API) readVolume(ctx context.Context, data *volumeModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(data.ID, "volume")
	if err != nil {
		return err
	}
	encVersion := data.EncryptionKeyWOVersion
	cifsVersion := data.CIFSPasswordWOVersion
	volume, err := a.sdk.Volumes.Get(ctx, id)
	if err != nil {
		return err
	}
	applyVolume(data, volume)
	data.EncryptionKeyWOVersion = encVersion
	data.CIFSPasswordWOVersion = cifsVersion
	return nil
}

func (a *API) updateVolume(ctx context.Context, plan, state *volumeModel, secrets volumeSecrets) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(state.ID, "volume")
	if err != nil {
		return err
	}
	plan.ID = state.ID
	if req := volumeUpdateRequest(plan, state, secrets); req != nil {
		if _, err := a.sdk.Volumes.Update(ctx, id, req); err != nil {
			return err
		}
		tflog.Debug(ctx, fmt.Sprintf("updated volume %s", id))
	}
	// readVolume keeps the versions already on plan, including a CIFS
	// password version that just changed. The passphrase itself is not stored.
	return a.readVolume(ctx, plan)
}

func (a *API) deleteVolume(ctx context.Context, data *volumeModel) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id, err := parseObjectID(data.ID, "volume")
	if err != nil {
		return err
	}
	return a.DeleteVolume(ctx, id)
}

// DeleteVolume disables a volume and waits until VergeOS reports it disabled,
// then deletes it. An enabled volume is refused. If shares still block the
// delete, they are removed and the delete is tried again.
func (a *API) DeleteVolume(ctx context.Context, id string) error {
	if err := a.requireSDK(); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	sharesCleared := false
	var last error
	for attempt := 0; attempt < nasPollAttempts; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, nasPollInterval); err != nil {
				return err
			}
		}
		volume, err := a.sdk.Volumes.Get(ctx, id)
		if missing(err) {
			return nil
		}
		if err != nil {
			last = err
			continue
		}
		if volume.Enabled || attempt == 0 {
			if err := a.sdk.Volumes.Disable(ctx, id); err != nil && volume.Enabled {
				last = err
				continue
			}
			volume, err = a.sdk.Volumes.Get(ctx, id)
			if missing(err) {
				return nil
			}
			if err != nil {
				last = err
				continue
			}
			if volume.Enabled {
				last = fmt.Errorf("volume %s is still enabled", id)
				tflog.Debug(ctx, last.Error())
				continue
			}
		}
		err = a.sdk.Volumes.Delete(ctx, id)
		if err == nil || missing(err) {
			tflog.Debug(ctx, fmt.Sprintf("deleted volume %s", id))
			return nil
		}
		last = err
		if sharesCleared {
			continue
		}
		if childErr := a.deleteVolumeShares(ctx, id); childErr != nil {
			return fmt.Errorf("delete volume %s: %w (shares were left in place: %v)", id, err, childErr)
		}
		sharesCleared = true
	}
	if last == nil {
		last = fmt.Errorf("volume was not deleted")
	}
	return fmt.Errorf("delete volume %s: %w", id, last)
}

func (a *API) deleteVolumeShares(ctx context.Context, volumeID string) error {
	var errs []error
	cifs, err := a.sdk.VolumeCIFSShares.ListByVolume(ctx, volumeID)
	if err != nil && !missing(err) {
		errs = append(errs, fmt.Errorf("list CIFS shares for volume %s: %w", volumeID, err))
	}
	for _, share := range cifs {
		id := rowID(share.Key, share.ID)
		if id == "" {
			errs = append(errs, fmt.Errorf("volume %s has a CIFS share named %q with no id", volumeID, share.Name))
			continue
		}
		if err := a.sdk.VolumeCIFSShares.Delete(ctx, id); err != nil && !missing(err) {
			errs = append(errs, fmt.Errorf("delete CIFS share %s: %w", share.Name, err))
		}
	}
	nfs, err := a.sdk.VolumeNFSShares.ListByVolume(ctx, volumeID)
	if err != nil && !missing(err) {
		errs = append(errs, fmt.Errorf("list NFS shares for volume %s: %w", volumeID, err))
	}
	for _, share := range nfs {
		id := rowID(share.Key, share.ID)
		if id == "" {
			errs = append(errs, fmt.Errorf("volume %s has an NFS share named %q with no id", volumeID, share.Name))
			continue
		}
		if err := a.sdk.VolumeNFSShares.Delete(ctx, id); err != nil && !missing(err) {
			errs = append(errs, fmt.Errorf("delete NFS share %s: %w", share.Name, err))
		}
	}
	return errors.Join(errs...)
}

func volumeCreateRequest(data *volumeModel, secrets volumeSecrets) (*vergeos.VolumeCreateRequest, error) {
	name := strings.TrimSpace(stringOrEmpty(data.Name))
	if name == "" {
		return nil, fmt.Errorf("volume name is required")
	}
	serviceID, err := parsePositiveID(data.ServiceID, "NAS service")
	if err != nil {
		return nil, err
	}
	req := &vergeos.VolumeCreateRequest{
		Name:               name,
		Service:            serviceID,
		Description:        stringOrEmpty(data.Description),
		Enabled:            vergeio.KnownBool(data.Enabled),
		MaxSize:            vergeio.KnownInt64(data.MaxSize),
		PreferredTier:      vergeio.KnownString(data.PreferredTier),
		SnapshotProfile:    knownInt(data.SnapshotProfile),
		Discard:            vergeio.KnownBool(data.Discard),
		ReadOnly:           vergeio.KnownBool(data.ReadOnly),
		Optimize:           vergeio.KnownString(data.Optimize),
		OwnerUser:          vergeio.KnownString(data.OwnerUser),
		OwnerGroup:         vergeio.KnownString(data.OwnerGroup),
		AutomountSnapshots: vergeio.KnownBool(data.AutomountSnapshots),
		Encrypt:            vergeio.KnownBool(data.Encrypt),
		RemoteTarget:       vergeio.KnownString(data.RemoteTarget),
		CIFSUser:           vergeio.KnownString(data.CIFSUser),
		CIFSProtocol:       vergeio.KnownString(data.CIFSProtocol),
		NFSProtocol:        vergeio.KnownString(data.NFSProtocol),
		MountOptions:       vergeio.KnownString(data.MountOptions),
		ReadAheadKB:        vergeio.KnownString(data.ReadAheadKB),
		Note:               vergeio.KnownString(data.Note),
	}
	if secrets.SendEncryption && secrets.EncryptionKey != "" {
		key := secrets.EncryptionKey
		req.EncryptionKey = &key
	}
	if secrets.SendCIFS && secrets.CIFSPassword != "" {
		password := secrets.CIFSPassword
		req.CIFSPassword = &password
	}
	return req, nil
}

func volumeUpdateRequest(plan, state *volumeModel, secrets volumeSecrets) *vergeos.VolumeUpdateRequest {
	req := &vergeos.VolumeUpdateRequest{
		Name:               vergeio.ChangedString(plan.Name, state.Name),
		Description:        vergeio.ChangedString(plan.Description, state.Description),
		Enabled:            vergeio.ChangedBool(plan.Enabled, state.Enabled),
		MaxSize:            vergeio.ChangedInt64(plan.MaxSize, state.MaxSize),
		PreferredTier:      vergeio.ChangedString(plan.PreferredTier, state.PreferredTier),
		SnapshotProfile:    changedInt(plan.SnapshotProfile, state.SnapshotProfile),
		Discard:            vergeio.ChangedBool(plan.Discard, state.Discard),
		ReadOnly:           vergeio.ChangedBool(plan.ReadOnly, state.ReadOnly),
		Optimize:           vergeio.ChangedString(plan.Optimize, state.Optimize),
		OwnerUser:          vergeio.ChangedString(plan.OwnerUser, state.OwnerUser),
		OwnerGroup:         vergeio.ChangedString(plan.OwnerGroup, state.OwnerGroup),
		AutomountSnapshots: vergeio.ChangedBool(plan.AutomountSnapshots, state.AutomountSnapshots),
		CIFSUser:           vergeio.ChangedString(plan.CIFSUser, state.CIFSUser),
		CIFSProtocol:       vergeio.ChangedString(plan.CIFSProtocol, state.CIFSProtocol),
		NFSProtocol:        vergeio.ChangedString(plan.NFSProtocol, state.NFSProtocol),
		MountOptions:       vergeio.ChangedString(plan.MountOptions, state.MountOptions),
		ReadAheadKB:        vergeio.ChangedString(plan.ReadAheadKB, state.ReadAheadKB),
		Note:               vergeio.ChangedString(plan.Note, state.Note),
	}
	if secrets.SendCIFS && secrets.CIFSPassword != "" {
		password := secrets.CIFSPassword
		req.CIFSPassword = &password
	}
	if req.Name == nil && req.Description == nil && req.Enabled == nil && req.MaxSize == nil &&
		req.PreferredTier == nil && req.SnapshotProfile == nil && req.Discard == nil && req.ReadOnly == nil &&
		req.Optimize == nil && req.OwnerUser == nil && req.OwnerGroup == nil && req.AutomountSnapshots == nil &&
		req.CIFSUser == nil && req.CIFSPassword == nil && req.CIFSProtocol == nil && req.NFSProtocol == nil &&
		req.MountOptions == nil && req.ReadAheadKB == nil && req.Note == nil {
		return nil
	}
	return req
}

func applyVolume(data *volumeModel, volume *vergeos.Volume) {
	data.ID = types.StringValue(rowID(volume.Key, volume.ID))
	data.ServiceID = types.StringValue(fmt.Sprintf("%d", volume.Service.Int()))
	data.Name = types.StringValue(volume.Name)
	data.Description = types.StringValue(volume.Description)
	data.Enabled = types.BoolValue(volume.Enabled)
	data.MaxSize = types.Int64Value(volume.MaxSize)
	data.PreferredTier = types.StringValue(volume.PreferredTier)
	data.SnapshotProfile = types.Int64Value(int64(volume.SnapshotProfile.Int()))
	data.Discard = types.BoolValue(volume.Discard)
	data.ReadOnly = types.BoolValue(volume.ReadOnly)
	data.Optimize = types.StringValue(volume.Optimize)
	data.OwnerUser = types.StringValue(volume.OwnerUser)
	data.OwnerGroup = types.StringValue(volume.OwnerGroup)
	data.AutomountSnapshots = types.BoolValue(volume.AutomountSnapshots)
	data.Encrypt = types.BoolValue(volume.Encrypt)
	data.RemoteTarget = types.StringValue(volume.RemoteTarget)
	data.CIFSUser = types.StringValue(volume.CIFSUser)
	data.CIFSProtocol = types.StringValue(volume.CIFSProtocol)
	data.NFSProtocol = types.StringValue(volume.NFSProtocol)
	data.MountOptions = types.StringValue(volume.MountOptions)
	data.ReadAheadKB = types.StringValue(volume.ReadAheadKB)
	data.Note = types.StringValue(volume.Note)
	data.Created = types.Int64Value(volume.Created)
	data.Modified = types.Int64Value(volume.Modified)
	data.Creator = types.StringValue(volume.Creator)
	data.Drive = types.Int64Value(int64(volume.Drive.Int()))
	data.IsSnapshot = types.BoolValue(volume.IsSnapshot)
	data.FSType = types.StringValue(volume.FSType)
	data.EncryptionKeyWO = types.StringNull()
	data.CIFSPasswordWO = types.StringNull()
}

func volumeSecretsFrom(config *volumeModel, state *volumeModel, create bool) volumeSecrets {
	secrets := volumeSecrets{}
	if password := stringOrEmpty(config.EncryptionKeyWO); password != "" && create {
		secrets.EncryptionKey = password
		secrets.SendEncryption = true
	}
	sendCIFS := false
	if password := stringOrEmpty(config.CIFSPasswordWO); password != "" {
		if create {
			sendCIFS = true
		} else if state == nil || versionChanged(map[string]types.Int64{"cifs": state.CIFSPasswordWOVersion}, "cifs", config.CIFSPasswordWOVersion) {
			sendCIFS = true
		}
		if sendCIFS {
			secrets.CIFSPassword = password
			secrets.SendCIFS = true
		}
	}
	return secrets
}
