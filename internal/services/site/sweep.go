// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package site

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/verge-io/govergeos"
)

// SweepTestRows removes site sync periods, outgoing syncs, incoming syncs,
// and sites left by acceptance tests. Periods go first because they reference
// an outgoing sync and a snapshot profile period. A sync is removed when its
// name has the test prefix or its parent site does.
func SweepTestRows(ctx context.Context, sdk *vergeos.Client, hasPrefix func(string) bool) error {
	if sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	if hasPrefix == nil {
		hasPrefix = func(string) bool { return false }
	}
	sites, err := sdk.Sites.List(ctx)
	if listMissing(err) {
		log.Printf("[SWEEP] sites endpoint unavailable, skipping")
		return nil
	}
	if err != nil {
		return fmt.Errorf("list sites: %w", err)
	}
	ownedSites := map[int]string{}
	for _, item := range sites {
		if hasPrefix(item.Name) {
			ownedSites[item.Key.Int()] = item.Name
		}
	}

	outgoing, err := sdk.SiteSyncsOutgoing.List(ctx)
	if listMissing(err) {
		outgoing = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list outgoing syncs: %w", err)
	}
	ownedOutgoing := map[int]string{}
	for _, item := range outgoing {
		_, parent := ownedSites[item.Site.Int()]
		if hasPrefix(item.Name) || parent {
			ownedOutgoing[item.Key.Int()] = item.Name
		}
	}

	incoming, err := sdk.SiteSyncsIncoming.List(ctx)
	if listMissing(err) {
		incoming = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list incoming syncs: %w", err)
	}
	ownedIncoming := map[int]string{}
	for _, item := range incoming {
		_, parent := ownedSites[item.Site.Int()]
		if hasPrefix(item.Name) || parent {
			ownedIncoming[item.Key.Int()] = item.Name
		}
	}

	var errs []error
	periods, err := sdk.SiteSyncProfilePeriods.List(ctx)
	if listMissing(err) {
		periods = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list site sync periods: %w", err)
	}
	for _, period := range periods {
		if _, ok := ownedOutgoing[period.SiteSyncsOutgoing.Int()]; !ok {
			continue
		}
		log.Printf("[SWEEP] deleting site sync period %d", period.Key.Int())
		if err := ignoreMissing(sdk.SiteSyncProfilePeriods.Delete(ctx, period.Key.Int())); err != nil {
			errs = append(errs, fmt.Errorf("delete site sync period %d: %w", period.Key.Int(), err))
		}
	}
	for id, name := range ownedOutgoing {
		log.Printf("[SWEEP] deleting outgoing sync %d (%s)", id, name)
		if err := ignoreMissing(sdk.SiteSyncsOutgoing.Delete(ctx, id)); err != nil {
			errs = append(errs, fmt.Errorf("delete outgoing sync %s: %w", name, err))
		}
	}
	for id, name := range ownedIncoming {
		log.Printf("[SWEEP] deleting incoming sync %d (%s)", id, name)
		if err := ignoreMissing(sdk.SiteSyncsIncoming.Delete(ctx, id)); err != nil {
			errs = append(errs, fmt.Errorf("delete incoming sync %s: %w", name, err))
		}
	}
	for id, name := range ownedSites {
		log.Printf("[SWEEP] deleting site %d (%s)", id, name)
		if err := ignoreMissing(sdk.Sites.Delete(ctx, id)); err != nil {
			errs = append(errs, fmt.Errorf("delete site %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// Leftovers lists prefixed sites and syncs that are still present.
// A missing endpoint is ignored.
func Leftovers(ctx context.Context, sdk *vergeos.Client, hasPrefix func(string) bool) []string {
	if sdk == nil || hasPrefix == nil {
		return nil
	}
	var left []string
	sites, err := sdk.Sites.List(ctx)
	if err != nil && !listMissing(err) {
		return []string{fmt.Sprintf("sites: %v", err)}
	}
	for _, item := range sites {
		if hasPrefix(item.Name) {
			left = append(left, fmt.Sprintf("site %s (%d)", item.Name, item.Key.Int()))
		}
	}
	outgoing, err := sdk.SiteSyncsOutgoing.List(ctx)
	if err != nil && !listMissing(err) {
		return append(left, fmt.Sprintf("outgoing syncs: %v", err))
	}
	for _, item := range outgoing {
		if hasPrefix(item.Name) {
			left = append(left, fmt.Sprintf("outgoing sync %s (%d)", item.Name, item.Key.Int()))
		}
	}
	incoming, err := sdk.SiteSyncsIncoming.List(ctx)
	if err != nil && !listMissing(err) {
		return append(left, fmt.Sprintf("incoming syncs: %v", err))
	}
	for _, item := range incoming {
		if hasPrefix(item.Name) {
			left = append(left, fmt.Sprintf("incoming sync %s (%d)", item.Name, item.Key.Int()))
		}
	}
	return left
}
