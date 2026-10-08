// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

// Package tenant manages a VergeOS tenant and the compute, storage, parent
// external IP, and routed network block handed to it, plus snapshots of
// that tenant.
//
// vergeio_tenant_snapshot keeps a snapshot in state. The action of the same
// name takes a snapshot and does not. A layer 2 network handed to a tenant
// is not a resource. See the tenants guide.
package tenant
