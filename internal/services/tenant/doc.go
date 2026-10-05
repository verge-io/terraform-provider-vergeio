// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

// Package tenant manages a VergeOS tenant and the compute, storage, and parent
// external IP handed to it.
//
// Network blocks (vnet_cidrs) are not resources. A layer 2 network handed to a
// tenant is not a resource either. See the tenants guide.
package tenant
