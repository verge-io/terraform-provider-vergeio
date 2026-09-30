// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

// Package tenant manages a VergeOS tenant and the compute and storage handed to it.
//
// Network blocks (vnet_cidrs) and tenant external IPs are not implemented.
// govergeos has no service for vnet_cidrs and no helper that assigns an
// external IP to a tenant. See the tenants guide.
package tenant
