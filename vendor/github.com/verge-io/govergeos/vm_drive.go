package vergeos

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

const (
	// Drive hotplug action
	vmActionHotplugDrive = "hotplugdrive"

	// Drive power state polling
	driveUnplugMaxRetries   = 5
	driveUnplugPollInterval = 5 * time.Second

	// Import status polling
	importMaxRetries   = 10
	importPollInterval = 5 * time.Second
)

// VMDriveService handles VM drive operations.
type VMDriveService struct {
	client *Client
}

// List returns all drives for a VM.
func (s *VMDriveService) List(ctx context.Context, vmID int) ([]VMDrive, error) {
	params := url.Values{}
	params.Set("fields", driveListFields)
	params.Set("filter", fmt.Sprintf("machine eq %d", vmID))

	var drives []VMDrive
	if err := s.client.get(ctx, "/machine_drives", params, &drives); err != nil {
		return nil, err
	}

	// Convert bytes to GB
	for i := range drives {
		drives[i].SizeGB = drives[i].SizeBytes / bytesPerGB
	}

	return drives, nil
}

// Get returns a single drive by ID.
func (s *VMDriveService) Get(ctx context.Context, driveID int) (*VMDrive, error) {
	params := url.Values{}
	params.Set("fields", driveGetFields)

	var drive VMDrive
	endpoint := fmt.Sprintf("/machine_drives/%d", driveID)
	if err := s.client.get(ctx, endpoint, params, &drive); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VMDrive", ID: driveID}
		}
		return nil, err
	}

	// Convert bytes to GB
	drive.SizeGB = drive.SizeBytes / bytesPerGB

	return &drive, nil
}

// Create creates a new drive and returns the created drive.
// For import media, this method waits for the import to complete.
func (s *VMDriveService) Create(ctx context.Context, vmID int, req *VMDriveCreateRequest) (*VMDrive, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	// Set the machine ID
	req.Machine = vmID

	// Convert GB to bytes
	if req.SizeGB > 0 {
		req.SizeBytes = req.SizeGB * bytesPerGB
	}

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/machine_drives", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created drive's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created drive
	drive, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// If this is an import, wait for it to complete
	if req.Media == "import" {
		drive, err = s.waitForImport(ctx, id, req.SizeGB)
		if err != nil {
			return nil, err
		}
	}

	return drive, nil
}

// waitForImport waits for a drive import to complete and handles resizing if needed.
func (s *VMDriveService) waitForImport(ctx context.Context, driveID int, targetSizeGB int64) (*VMDrive, error) {
	// Initial wait
	time.Sleep(5 * time.Second)

	for i := 0; i < importMaxRetries; i++ {
		drive, err := s.Get(ctx, driveID)
		if err != nil {
			return nil, err
		}

		// Check if import is complete
		if drive.Status != "importing" {
			// If target size differs from actual size, resize the drive
			if targetSizeGB > 0 && drive.SizeGB != targetSizeGB {
				targetSizeBytes := targetSizeGB * bytesPerGB
				updateReq := &VMDriveUpdateRequest{
					SizeBytes: &targetSizeBytes,
				}
				return s.Update(ctx, driveID, updateReq)
			}
			return drive, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(importPollInterval):
		}
	}

	return nil, fmt.Errorf("vergeos: timeout waiting for drive %d import to complete", driveID)
}

// Update updates a drive and returns the updated drive.
func (s *VMDriveService) Update(ctx context.Context, driveID int, req *VMDriveUpdateRequest) (*VMDrive, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	// Convert GB to bytes if specified
	if req.SizeGB != nil && *req.SizeGB > 0 {
		sizeBytes := *req.SizeGB * bytesPerGB
		req.SizeBytes = &sizeBytes
	}

	endpoint := fmt.Sprintf("/machine_drives/%d", driveID)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VMDrive", ID: driveID}
		}
		return nil, err
	}

	// Read back the updated drive
	return s.Get(ctx, driveID)
}

// Delete deletes a drive.
// If the drive is currently attached (VM running), it will be hot-unplugged first.
func (s *VMDriveService) Delete(ctx context.Context, driveID int) error {
	// Get drive to check power state and get VM ID
	drive, err := s.Get(ctx, driveID)
	if err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}

	// If drive is online, hot-unplug it first
	if drive.PowerState != "" && drive.PowerState != "offline" {
		if err := s.hotUnplug(ctx, drive.Machine, driveID); err != nil {
			return fmt.Errorf("vergeos: failed to unplug drive before deletion: %w", err)
		}
	}

	endpoint := fmt.Sprintf("/machine_drives/%d", driveID)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}

	return nil
}

// hotUnplug hot-unplugs a drive from a running VM.
func (s *VMDriveService) hotUnplug(ctx context.Context, vmID, driveID int) error {
	action := vmAction{
		VM:     vmID,
		Action: vmActionHotplugDrive,
		Params: vmActionParams{
			Device: fmt.Sprintf("%d", driveID),
			Unplug: true,
		},
	}

	if err := s.client.post(ctx, "/vm_actions", action, nil); err != nil {
		return err
	}

	// Wait for drive to be unplugged
	for i := 0; i < driveUnplugMaxRetries; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(driveUnplugPollInterval):
		}

		drive, err := s.Get(ctx, driveID)
		if err != nil {
			if IsNotFoundError(err) {
				return nil // Drive was removed
			}
			return err
		}

		if drive.PowerState == "offline" || drive.PowerState == "" {
			return nil
		}
	}

	return fmt.Errorf("vergeos: timeout waiting for drive %d to unplug", driveID)
}
