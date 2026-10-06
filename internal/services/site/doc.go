// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

// Package site holds disaster recovery building blocks: a remote site, the
// incoming and outgoing syncs paired with it, and the sync status a pipeline
// can read.
//
// Cloud snapshots are not a second profile type. VergeOS stores the cloud
// snapshot schedule in snapshot_profiles, which vergeio_snapshot_profile
// already manages. A site sync period points at one of that profile's periods.
package site
