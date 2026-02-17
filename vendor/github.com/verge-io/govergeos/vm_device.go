package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VMDeviceService handles VM device operations.
type VMDeviceService struct {
	client *Client
}

// List returns all devices for a VM.
func (s *VMDeviceService) List(ctx context.Context, vmID int) ([]VMDevice, error) {
	params := url.Values{}
	params.Set("fields", deviceListFields)
	params.Set("filter", fmt.Sprintf("machine eq %d", vmID))

	var devices []VMDevice
	if err := s.client.get(ctx, "/machine_devices", params, &devices); err != nil {
		return nil, err
	}

	// Load settings for each device
	for i := range devices {
		if err := s.loadSettings(ctx, &devices[i]); err != nil {
			// Non-fatal: continue without settings
		}
	}

	return devices, nil
}

// Get returns a single device by ID.
func (s *VMDeviceService) Get(ctx context.Context, deviceID int) (*VMDevice, error) {
	params := url.Values{}
	params.Set("fields", deviceListFields)

	var device VMDevice
	endpoint := fmt.Sprintf("/machine_devices/%d", deviceID)
	if err := s.client.get(ctx, endpoint, params, &device); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VMDevice", ID: deviceID}
		}
		return nil, err
	}

	// Load settings based on device type
	if err := s.loadSettings(ctx, &device); err != nil {
		// Non-fatal: return device without settings
	}

	return &device, nil
}

// loadSettings loads device-specific settings based on the device type.
func (s *VMDeviceService) loadSettings(ctx context.Context, device *VMDevice) error {
	switch device.Type {
	case DeviceTypeUSB:
		settings, err := s.getUSBSettings(ctx, device.ID.Int())
		if err != nil {
			return err
		}
		device.USBSettings = settings

	case DeviceTypeTPM:
		settings, err := s.getTPMSettings(ctx, device.ID.Int())
		if err != nil {
			return err
		}
		device.TPMSettings = settings

	case DeviceTypeVGPU:
		settings, err := s.getVGPUSettings(ctx, device.ID.Int())
		if err != nil {
			return err
		}
		device.VGPUSettings = settings
	}

	return nil
}

// getUSBSettings retrieves USB device settings.
func (s *VMDeviceService) getUSBSettings(ctx context.Context, deviceID int) (*USBDeviceSettings, error) {
	params := url.Values{}
	params.Set("fields", "most")
	params.Set("filter", fmt.Sprintf("machine_device eq %d", deviceID))

	var settings []USBDeviceSettings
	if err := s.client.get(ctx, "/machine_device_settings_usb", params, &settings); err != nil {
		return nil, err
	}

	if len(settings) == 0 {
		return nil, nil
	}

	return &settings[0], nil
}

// getTPMSettings retrieves TPM device settings.
func (s *VMDeviceService) getTPMSettings(ctx context.Context, deviceID int) (*TPMDeviceSettings, error) {
	params := url.Values{}
	params.Set("fields", "most")
	params.Set("filter", fmt.Sprintf("machine_device eq %d", deviceID))

	var settings []TPMDeviceSettings
	if err := s.client.get(ctx, "/machine_device_settings_tpm", params, &settings); err != nil {
		return nil, err
	}

	if len(settings) == 0 {
		return nil, nil
	}

	return &settings[0], nil
}

// getVGPUSettings retrieves NVIDIA vGPU device settings.
func (s *VMDeviceService) getVGPUSettings(ctx context.Context, deviceID int) (*VGPUDeviceSettings, error) {
	params := url.Values{}
	params.Set("fields", "most")
	params.Set("filter", fmt.Sprintf("machine_device eq %d", deviceID))

	var settings []VGPUDeviceSettings
	if err := s.client.get(ctx, "/machine_device_settings_nvidia_vgpu", params, &settings); err != nil {
		return nil, err
	}

	if len(settings) == 0 {
		return nil, nil
	}

	return &settings[0], nil
}

// Create creates a new device and returns the created device.
func (s *VMDeviceService) Create(ctx context.Context, vmID int, req *VMDeviceCreateRequest) (*VMDevice, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}
	if req.Type == "" {
		return nil, &ValidationError{Field: "type", Message: "type is required"}
	}

	// Set the machine ID
	req.Machine = vmID

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/machine_devices", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created device's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created device
	device, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// Update device settings if provided
	if err := s.updateSettings(ctx, device, req.USBSettings, req.TPMSettings, req.VGPUSettings); err != nil {
		// Non-fatal: device was created but settings update failed
	}

	// Read back with settings
	return s.Get(ctx, id)
}

// Update updates a device and returns the updated device.
func (s *VMDeviceService) Update(ctx context.Context, deviceID int, req *VMDeviceUpdateRequest) (*VMDevice, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/machine_devices/%d", deviceID)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VMDevice", ID: deviceID}
		}
		return nil, err
	}

	// Read back the device to get its type
	device, err := s.Get(ctx, deviceID)
	if err != nil {
		return nil, err
	}

	// Update device settings if provided
	if err := s.updateSettings(ctx, device, req.USBSettings, req.TPMSettings, req.VGPUSettings); err != nil {
		// Non-fatal: device was updated but settings update failed
	}

	// Read back with settings
	return s.Get(ctx, deviceID)
}

// updateSettings updates device-specific settings.
func (s *VMDeviceService) updateSettings(ctx context.Context, device *VMDevice, usb *USBDeviceSettings, tpm *TPMDeviceSettings, vgpu *VGPUDeviceSettings) error {
	switch device.Type {
	case DeviceTypeUSB:
		if usb != nil {
			// Get existing settings to find the ID
			existing, err := s.getUSBSettings(ctx, device.ID.Int())
			if err != nil {
				return err
			}
			if existing != nil {
				usb.MachineDevice = device.ID.Int()
				endpoint := fmt.Sprintf("/machine_device_settings_usb/%d", existing.ID)
				return s.client.put(ctx, endpoint, usb, nil)
			}
		}

	case DeviceTypeTPM:
		if tpm != nil {
			existing, err := s.getTPMSettings(ctx, device.ID.Int())
			if err != nil {
				return err
			}
			if existing != nil {
				tpm.MachineDevice = device.ID.Int()
				endpoint := fmt.Sprintf("/machine_device_settings_tpm/%d", existing.ID)
				return s.client.put(ctx, endpoint, tpm, nil)
			}
		}

	case DeviceTypeVGPU:
		if vgpu != nil {
			existing, err := s.getVGPUSettings(ctx, device.ID.Int())
			if err != nil {
				return err
			}
			if existing != nil {
				vgpu.MachineDevice = device.ID.Int()
				endpoint := fmt.Sprintf("/machine_device_settings_nvidia_vgpu/%d", existing.ID)
				return s.client.put(ctx, endpoint, vgpu, nil)
			}
		}
	}

	return nil
}

// Delete deletes a device.
func (s *VMDeviceService) Delete(ctx context.Context, deviceID int) error {
	endpoint := fmt.Sprintf("/machine_devices/%d", deviceID)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}
	return nil
}
