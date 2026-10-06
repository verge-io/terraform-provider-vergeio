// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package platform

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/verge-io/govergeos"
)

// SweepTestRows removes certificates and webhook destinations left by
// acceptance tests. A certificate matches when its domain or description
// has the test prefix. A destination matches when its name does, and its
// delivery rows are deleted first. System settings are not swept. Destroy
// on vergeio_setting restores one key and must not be replayed against
// every prefixed name.
func SweepTestRows(ctx context.Context, sdk *vergeos.Client, hasPrefix func(string) bool) error {
	if sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	if hasPrefix == nil {
		hasPrefix = func(string) bool { return false }
	}
	var errs []error
	certs, err := sdk.Certificates.List(ctx)
	if listMissing(err) {
		log.Printf("[SWEEP] certificates endpoint unavailable, skipping")
		certs = nil
		err = nil
	}
	if err != nil {
		return fmt.Errorf("list certificates: %w", err)
	}
	for _, cert := range certs {
		if !hasPrefix(cert.Domain) && !hasPrefix(cert.Description) {
			continue
		}
		id := cert.Key.Int()
		log.Printf("[SWEEP] deleting certificate %d (%s)", id, cert.Domain)
		if err := ignoreMissing(sdk.Certificates.Delete(ctx, id)); err != nil {
			errs = append(errs, fmt.Errorf("delete certificate %s: %w", cert.Domain, err))
		}
	}

	urls, err := sdk.WebhookURLs.List(ctx)
	if listMissing(err) {
		log.Printf("[SWEEP] webhook destinations endpoint unavailable, skipping")
		return errors.Join(errs...)
	}
	if err != nil {
		return fmt.Errorf("list webhook destinations: %w", err)
	}
	for _, item := range urls {
		if !hasPrefix(item.Name) {
			continue
		}
		id := item.Key.Int()
		if err := deleteWebhookDeliveries(ctx, sdk, id); err != nil {
			errs = append(errs, err)
			continue
		}
		log.Printf("[SWEEP] deleting webhook destination %d (%s)", id, item.Name)
		if err := ignoreMissing(sdk.WebhookURLs.Delete(ctx, id)); err != nil {
			errs = append(errs, fmt.Errorf("delete webhook destination %s: %w", item.Name, err))
		}
	}
	return errors.Join(errs...)
}

func deleteWebhookDeliveries(ctx context.Context, sdk *vergeos.Client, urlID int) error {
	rows, err := sdk.Webhooks.ListByWebhookURL(ctx, urlID)
	if listMissing(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("list deliveries for webhook destination %d: %w", urlID, err)
	}
	for _, row := range rows {
		log.Printf("[SWEEP] deleting webhook delivery %d", row.Key.Int())
		if err := ignoreMissing(sdk.Webhooks.Delete(ctx, row.Key.Int())); err != nil {
			return fmt.Errorf("delete webhook delivery %d: %w", row.Key.Int(), err)
		}
	}
	return nil
}

// Leftovers lists prefixed certificates and webhook destinations that are
// still present. A missing endpoint is ignored. Settings are not listed.
func Leftovers(ctx context.Context, sdk *vergeos.Client, hasPrefix func(string) bool) []string {
	if sdk == nil || hasPrefix == nil {
		return nil
	}
	var left []string
	certs, err := sdk.Certificates.List(ctx)
	if err != nil && !listMissing(err) {
		left = append(left, fmt.Sprintf("certificates: %v", err))
	}
	for _, cert := range certs {
		if hasPrefix(cert.Domain) || hasPrefix(cert.Description) {
			left = append(left, fmt.Sprintf("certificate %s (%d)", cert.Domain, cert.Key.Int()))
		}
	}
	urls, err := sdk.WebhookURLs.List(ctx)
	if err != nil && !listMissing(err) {
		return append(left, fmt.Sprintf("webhook destinations: %v", err))
	}
	for _, item := range urls {
		if hasPrefix(item.Name) {
			left = append(left, fmt.Sprintf("webhook destination %s (%d)", item.Name, item.Key.Int()))
		}
	}
	return left
}
