// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package vergeio

// Version-specific error messages for API endpoints
const (
	// V26 API endpoint error template - use with fmt.Sprintf to include endpoint name
	ErrEndpointV26 = "%s endpoint is not available in this version of VergeOS. This API requires VergeOS v26 or newer. Please upgrade your VergeOS system or remove the corresponding resource/data source from your configuration"
)

// Future version error messages can be added here
// Example:
// const (
//     ErrEndpointV27 = "%s endpoint requires VergeOS v27 or newer..."
// )
