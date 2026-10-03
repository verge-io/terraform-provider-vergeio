// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

// Package tenant manages a VergeOS tenant and the compute and storage handed to it.
//
// Network blocks (vnet_cidrs) and tenant external IPs are not resources.
// The SDK can assign both. This provider does not. See the tenants guide.
package tenant
