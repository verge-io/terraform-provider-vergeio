// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package nas

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/verge-io/govergeos"
)

// SweepTestRows removes NAS shares, volumes, users, and services left by
// acceptance tests. Shares go first. Each volume is disabled and only then
// deleted. Services go last. Deleting a service also removes the virtual
// machine the Services recipe created for it.
// Snapshot volumes are skipped. hasPrefix selects names the sweeper owns.
// vmIDs are virtual machines about to be deleted. A service on one of those
// machines is removed even when its name has no prefix, because the name
// comes from the machine.
func SweepTestRows(ctx context.Context, sdk *vergeos.Client, vmIDs map[int]string, hasPrefix func(string) bool) error {
	if sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	if hasPrefix == nil {
		hasPrefix = func(string) bool { return false }
	}
	if vmIDs == nil {
		vmIDs = map[int]string{}
	}
	services, err := sdk.NASServices.List(ctx)
	if listMissing(err) {
		log.Printf("[SWEEP] NAS services endpoint unavailable, skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("list NAS services: %w", err)
	}
	ownedServices := map[int]string{}
	for _, service := range services {
		if hasPrefix(service.Name) || vmOwned(vmIDs, service.VM.Int()) {
			ownedServices[service.Key.Int()] = service.Name
		}
	}

	volumes, err := sdk.Volumes.List(ctx)
	if listMissing(err) {
		volumes = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list volumes: %w", err)
	}
	ownedVolumes := map[string]string{}
	for _, volume := range volumes {
		if volume.IsSnapshot {
			continue
		}
		id := firstNonEmpty(volume.ID, volume.Key)
		if id == "" {
			continue
		}
		_, parent := ownedServices[volume.Service.Int()]
		if hasPrefix(volume.Name) || parent {
			ownedVolumes[id] = volume.Name
		}
	}

	api := &API{sdk: sdk}
	var errs []error
	errs = append(errs, deleteOwnedShares(ctx, sdk, ownedVolumes, hasPrefix)...)
	for id, name := range ownedVolumes {
		log.Printf("[SWEEP] deleting volume %s (%s)", id, name)
		if err := api.DeleteVolume(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	for id, name := range ownedServices {
		log.Printf("[SWEEP] deleting NAS service %d (%s)", id, name)
		if err := api.DeleteService(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func deleteOwnedShares(ctx context.Context, sdk *vergeos.Client, volumes map[string]string, hasPrefix func(string) bool) []error {
	var errs []error
	cifs, err := sdk.VolumeCIFSShares.List(ctx)
	if listMissing(err) {
		cifs = nil
		err = nil
	}
	if err != nil {
		return []error{fmt.Errorf("list CIFS shares: %w", err)}
	}
	for _, share := range cifs {
		if _, ok := volumes[share.Volume]; !ok && !hasPrefix(share.Name) {
			continue
		}
		id := firstNonEmpty(share.ID, share.Key)
		if id == "" {
			errs = append(errs, fmt.Errorf("CIFS share %q has no id", share.Name))
			continue
		}
		log.Printf("[SWEEP] deleting CIFS share %s (%s)", id, share.Name)
		if err := sdk.VolumeCIFSShares.Delete(ctx, id); err != nil && !missing(err) {
			errs = append(errs, fmt.Errorf("delete CIFS share %s: %w", share.Name, err))
		}
	}
	nfs, err := sdk.VolumeNFSShares.List(ctx)
	if listMissing(err) {
		nfs = nil
		err = nil
	}
	if err != nil {
		return append(errs, fmt.Errorf("list NFS shares: %w", err))
	}
	for _, share := range nfs {
		if _, ok := volumes[share.Volume]; !ok && !hasPrefix(share.Name) {
			continue
		}
		id := firstNonEmpty(share.ID, share.Key)
		if id == "" {
			errs = append(errs, fmt.Errorf("NFS share %q has no id", share.Name))
			continue
		}
		log.Printf("[SWEEP] deleting NFS share %s (%s)", id, share.Name)
		if err := sdk.VolumeNFSShares.Delete(ctx, id); err != nil && !missing(err) {
			errs = append(errs, fmt.Errorf("delete NFS share %s: %w", share.Name, err))
		}
	}
	return errs
}

func vmOwned(vmIDs map[int]string, id int) bool {
	_, ok := vmIDs[id]
	return ok
}

func listMissing(err error) bool {
	if err == nil {
		return false
	}
	if vergeos.IsNotFoundError(err) {
		return true
	}
	var apiErr *vergeos.APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 404
}
