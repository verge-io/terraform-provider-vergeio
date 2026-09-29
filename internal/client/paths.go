// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

import (
	"net/http"
	"net/url"
)

// Collection paths. Services pass these to the client instead of joining
// URL segments themselves.
const (
	VMEndpoint                       = APIEndpoint + "/vms"
	VMActionEndpoint                 = APIEndpoint + "/vm_actions"
	NICEndpoint                      = APIEndpoint + "/machine_nics"
	IPEndpoint                       = APIEndpoint + "/vnet_addresses"
	DiskEndpoint                     = APIEndpoint + "/machine_drives"
	DeviceEndpoint                   = APIEndpoint + "/machine_devices"
	DeviceUSBSettingsEndpoint        = APIEndpoint + "/machine_device_settings_usb"
	DeviceTPMSettingsEndpoint        = APIEndpoint + "/machine_device_settings_tpm"
	DeviceNvidiaVGPUSettingsEndpoint = APIEndpoint + "/machine_device_settings_nvidia_vgpu"
)

// ObjectPath joins a collection path with one object id. The id is escaped.
func ObjectPath(collection, id string) string {
	return collection + "/" + url.PathEscape(id)
}

// SuccessStatus reports whether code is 200, 201, or 204.
func SuccessStatus(code int) bool {
	return code == http.StatusOK || code == http.StatusCreated || code == http.StatusNoContent
}
