// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MIT

package network

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"terraform-provider-vergeio/internal/client"
	"terraform-provider-vergeio/internal/shared"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/verge-io/govergeos"
)

// DNSApi reads and writes DNS views, zones, and records through govergeos.
type DNSApi struct {
	sdk *vergeos.Client
}

func NewDNSApi(c *vergeio.Client) (*DNSApi, error) {
	sdk, err := c.NewVergeosClient()
	if err != nil {
		return nil, err
	}
	return &DNSApi{sdk: sdk}, nil
}

type dnsNotice struct {
	Summary string
	Detail  string
}

func stoppedDNSNotice(network *vergeos.Network, id int) *dnsNotice {
	return &dnsNotice{
		Summary: "DNS changes are not live yet",
		Detail: fmt.Sprintf(
			"Network %s is not running, so Terraform did not apply its DNS changes. A stopped network loads staged DNS when it starts.",
			networkLabel(network, id),
		),
	}
}

func stagedDNSNotice(network *vergeos.Network, id int) *dnsNotice {
	return &dnsNotice{
		Summary: "DNS changes were staged",
		Detail: fmt.Sprintf(
			"Network %s has staged DNS changes. apply is false, so Terraform did not apply them. need_dns_apply stays set until a later apply. VergeOS may act on a stale pending flag on its own.",
			networkLabel(network, id),
		),
	}
}

func addDNSNotice(diags *diag.Diagnostics, notice *dnsNotice) {
	if diags == nil || notice == nil || notice.Detail == "" {
		return
	}
	diags.AddWarning(notice.Summary, notice.Detail)
}

func configureDNS(providerData any) (*DNSApi, diag.Diagnostics) {
	var diags diag.Diagnostics
	if providerData == nil {
		return nil, diags
	}
	c, ok := providerData.(*vergeio.Client)
	if !ok {
		diags.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *vergeio.Client, got: %T. Please report this issue to the provider developers.", providerData),
		)
		return nil, diags
	}
	api, err := NewDNSApi(c)
	if err != nil {
		diags.AddError("Unable to Create VergeOS API Client", err.Error())
		return nil, diags
	}
	return api, diags
}

func importByPositiveID(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse, summary, detail string) {
	if strings.TrimSpace(req.ID) != "" {
		if _, err := parsePositiveID(req.ID); err != nil {
			resp.Diagnostics.AddError(summary, detail)
			return
		}
	}
	shared.ImportByID(ctx, req, resp, summary, detail)
}

// finishDNSApply refreshes DNS once when this change wrote a row or the
// network already has need_dns_apply set. The decision matches firewall
// rules: a stopped network is left alone, and apply false leaves the flag.
func (a *DNSApi) finishDNSApply(ctx context.Context, networkID int, wrote, apply bool) (*dnsNotice, error) {
	if a == nil || a.sdk == nil {
		return nil, fmt.Errorf("vergeos client is nil")
	}
	if networkID <= 0 {
		if wrote {
			return nil, fmt.Errorf("DNS was changed but the network id is missing, so Terraform did not apply it")
		}
		return nil, nil
	}
	network, err := a.sdk.Networks.Get(ctx, networkID)
	if err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil, nil
		}
		return nil, err
	}
	switch decideFirewallApply(wrote, apply, networkIsRunning(network), network.NeedDNSApply) {
	case firewallApplyNow:
		tflog.Debug(ctx, fmt.Sprintf("Applying DNS on network %d", networkID))
		if err := a.sdk.Networks.ApplyDNS(ctx, networkID); err != nil {
			return nil, err
		}
		return nil, nil
	case firewallApplySkippedStopped:
		tflog.Warn(ctx, fmt.Sprintf("Network %d is not running; skipping DNS apply", networkID))
		return stoppedDNSNotice(network, networkID), nil
	case firewallApplySkippedDisabled:
		tflog.Warn(ctx, fmt.Sprintf("Network %d DNS apply is disabled; leaving DNS staged", networkID))
		return stagedDNSNotice(network, networkID), nil
	default:
		return nil, nil
	}
}

func (a *DNSApi) networkIDForView(ctx context.Context, viewID int) (int, error) {
	view, err := a.sdk.VNetDNSViews.Get(ctx, viewID)
	if err != nil {
		return 0, err
	}
	id := view.VNet.Int()
	if id <= 0 {
		return 0, fmt.Errorf("DNS view %d has no network", viewID)
	}
	return id, nil
}

func (a *DNSApi) networkIDForZone(ctx context.Context, zoneID int) (int, error) {
	zone, err := a.sdk.VNetDNSZones.Get(ctx, zoneID)
	if err != nil {
		return 0, err
	}
	return a.networkIDForView(ctx, zone.View.Int())
}

func knownNetworkID(v types.String) (int, error) {
	id, ok, err := optionalPositiveID(v)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, nil
	}
	return id, nil
}

// abandon removes a row that was created but not stored in state.
func abandon(cause error, kind string, id int, cleanupErr error) error {
	if cleanupErr != nil {
		return fmt.Errorf("%s %d was created but not stored in state: %w (it was left in place: %v)", kind, id, cause, cleanupErr)
	}
	return fmt.Errorf("%s %d was created but not stored in state, and it was removed: %w", kind, id, cause)
}

func (a *DNSApi) createView(ctx context.Context, data *dnsViewModel) (*dnsNotice, error) {
	req, err := viewCreateRequest(data)
	if err != nil {
		return nil, err
	}
	created, err := a.sdk.VNetDNSViews.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	id := created.Key.Int()
	data.ID = typesStringID(id)
	if err := a.readView(ctx, data); err != nil {
		return nil, abandon(err, "DNS view", id, deleteDNSView(ctx, a.sdk, id))
	}
	notice, err := a.finishDNSApply(ctx, req.VNet, true, applyEnabled(data.Apply))
	if err != nil {
		return nil, abandon(err, "DNS view", id, deleteDNSView(ctx, a.sdk, id))
	}
	return notice, nil
}

func (a *DNSApi) readView(ctx context.Context, data *dnsViewModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("DNS view id: %w", err)
	}
	view, err := a.sdk.VNetDNSViews.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureParentID(data.NetworkID, view.VNet.Int(), "VNetDNSView", id); err != nil {
		return err
	}
	assignView(data, view)
	return nil
}

func (a *DNSApi) updateView(ctx context.Context, plan, state *dnsViewModel) (*dnsNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(state.ID))
	if err != nil {
		return nil, fmt.Errorf("DNS view id: %w", err)
	}
	networkID, err := parsePositiveID(stringOrEmpty(plan.NetworkID))
	if err != nil {
		return nil, fmt.Errorf("network_id: %w", err)
	}
	req := viewUpdateRequest(plan, state)
	wrote := false
	if !viewUpdateEmpty(req) {
		if _, err := a.sdk.VNetDNSViews.Update(ctx, id, req); err != nil {
			return nil, err
		}
		wrote = true
	}
	plan.ID = state.ID
	if err := a.readView(ctx, plan); err != nil {
		return nil, err
	}
	return a.finishDNSApply(ctx, networkID, wrote, applyEnabled(plan.Apply))
}

func (a *DNSApi) deleteView(ctx context.Context, data *dnsViewModel) (*dnsNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return nil, fmt.Errorf("DNS view id: %w", err)
	}
	networkID, err := knownNetworkID(data.NetworkID)
	if err != nil {
		return nil, fmt.Errorf("network_id: %w", err)
	}
	if networkID == 0 {
		if looked, lookErr := a.networkIDForView(ctx, id); lookErr == nil {
			networkID = looked
		}
	}
	wrote, err := dnsRowExists(func() error {
		_, err := a.sdk.VNetDNSViews.Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if wrote {
		if err := deleteDNSView(ctx, a.sdk, id); err != nil {
			return nil, err
		}
	}
	return a.finishDNSApply(ctx, networkID, wrote, applyEnabled(data.Apply))
}

func (a *DNSApi) createZone(ctx context.Context, data *dnsZoneModel) (*dnsNotice, error) {
	req, err := zoneCreateRequest(data)
	if err != nil {
		return nil, err
	}
	created, err := a.sdk.VNetDNSZones.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	id := created.Key.Int()
	data.ID = typesStringID(id)
	if err := a.readZone(ctx, data); err != nil {
		return nil, abandon(err, "DNS zone", id, deleteDNSZone(ctx, a.sdk, id))
	}
	networkID, err := knownNetworkID(data.NetworkID)
	if err != nil {
		return nil, abandon(err, "DNS zone", id, deleteDNSZone(ctx, a.sdk, id))
	}
	notice, err := a.finishDNSApply(ctx, networkID, true, applyEnabled(data.Apply))
	if err != nil {
		return nil, abandon(err, "DNS zone", id, deleteDNSZone(ctx, a.sdk, id))
	}
	return notice, nil
}

func (a *DNSApi) readZone(ctx context.Context, data *dnsZoneModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("DNS zone id: %w", err)
	}
	zone, err := a.sdk.VNetDNSZones.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureParentID(data.ViewID, zone.View.Int(), "VNetDNSZone", id); err != nil {
		return err
	}
	networkID, err := a.networkIDForView(ctx, zone.View.Int())
	if err != nil {
		return err
	}
	if err := ensureParentID(data.NetworkID, networkID, "VNetDNSZone", id); err != nil {
		return err
	}
	assignZone(data, zone, networkID)
	return nil
}

func (a *DNSApi) updateZone(ctx context.Context, plan, state *dnsZoneModel) (*dnsNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(state.ID))
	if err != nil {
		return nil, fmt.Errorf("DNS zone id: %w", err)
	}
	req := zoneUpdateRequest(plan, state)
	wrote := false
	if !zoneUpdateEmpty(req) {
		if _, err := a.sdk.VNetDNSZones.Update(ctx, id, req); err != nil {
			return nil, err
		}
		wrote = true
	}
	plan.ID = state.ID
	if err := a.readZone(ctx, plan); err != nil {
		return nil, err
	}
	networkID, err := knownNetworkID(plan.NetworkID)
	if err != nil {
		return nil, err
	}
	return a.finishDNSApply(ctx, networkID, wrote, applyEnabled(plan.Apply))
}

func (a *DNSApi) deleteZone(ctx context.Context, data *dnsZoneModel) (*dnsNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return nil, fmt.Errorf("DNS zone id: %w", err)
	}
	networkID, err := knownNetworkID(data.NetworkID)
	if err != nil {
		return nil, fmt.Errorf("network_id: %w", err)
	}
	if networkID == 0 {
		if looked, lookErr := a.networkIDForZone(ctx, id); lookErr == nil {
			networkID = looked
		}
	}
	wrote, err := dnsRowExists(func() error {
		_, err := a.sdk.VNetDNSZones.Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if wrote {
		if err := deleteDNSZone(ctx, a.sdk, id); err != nil {
			return nil, err
		}
	}
	return a.finishDNSApply(ctx, networkID, wrote, applyEnabled(data.Apply))
}

func (a *DNSApi) createRecord(ctx context.Context, data *dnsRecordModel) (*dnsNotice, error) {
	if err := a.syncRecordValue(ctx, data); err != nil {
		return nil, err
	}
	req, err := recordCreateRequest(data)
	if err != nil {
		return nil, err
	}
	created, err := a.sdk.VNetDNSRecords.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	id := created.Key.Int()
	data.ID = typesStringID(id)
	if err := a.readRecord(ctx, data); err != nil {
		return nil, abandon(err, "DNS record", id, deleteDNSRecord(ctx, a.sdk, id))
	}
	networkID, err := knownNetworkID(data.NetworkID)
	if err != nil {
		return nil, abandon(err, "DNS record", id, deleteDNSRecord(ctx, a.sdk, id))
	}
	notice, err := a.finishDNSApply(ctx, networkID, true, applyEnabled(data.Apply))
	if err != nil {
		return nil, abandon(err, "DNS record", id, deleteDNSRecord(ctx, a.sdk, id))
	}
	return notice, nil
}

func (a *DNSApi) readRecord(ctx context.Context, data *dnsRecordModel) error {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return fmt.Errorf("DNS record id: %w", err)
	}
	record, err := a.sdk.VNetDNSRecords.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := ensureParentID(data.ZoneID, record.Zone.Int(), "VNetDNSRecord", id); err != nil {
		return err
	}
	networkID, err := a.networkIDForZone(ctx, record.Zone.Int())
	if err != nil {
		return err
	}
	if err := ensureParentID(data.NetworkID, networkID, "VNetDNSRecord", id); err != nil {
		return err
	}
	assignRecord(data, record, networkID)
	return nil
}

func (a *DNSApi) updateRecord(ctx context.Context, plan, state *dnsRecordModel) (*dnsNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(state.ID))
	if err != nil {
		return nil, fmt.Errorf("DNS record id: %w", err)
	}
	// Keep the NIC reference from the plan. readRecord does not see it on the API.
	nicID := plan.VMNICID
	if err := a.syncRecordValue(ctx, plan); err != nil {
		return nil, err
	}
	req := recordUpdateRequest(plan, state)
	wrote := false
	if !recordUpdateEmpty(req) {
		if _, err := a.sdk.VNetDNSRecords.Update(ctx, id, req); err != nil {
			return nil, err
		}
		wrote = true
	}
	plan.ID = state.ID
	plan.VMNICID = nicID
	if err := a.readRecord(ctx, plan); err != nil {
		return nil, err
	}
	plan.VMNICID = nicID
	networkID, err := knownNetworkID(plan.NetworkID)
	if err != nil {
		return nil, err
	}
	return a.finishDNSApply(ctx, networkID, wrote, applyEnabled(plan.Apply))
}

func (a *DNSApi) deleteRecord(ctx context.Context, data *dnsRecordModel) (*dnsNotice, error) {
	id, err := parsePositiveID(stringOrEmpty(data.ID))
	if err != nil {
		return nil, fmt.Errorf("DNS record id: %w", err)
	}
	networkID, err := knownNetworkID(data.NetworkID)
	if err != nil {
		return nil, fmt.Errorf("network_id: %w", err)
	}
	if networkID == 0 {
		if looked, lookErr := a.networkIDForZone(ctx, positiveOrZero(data.ZoneID)); lookErr == nil {
			networkID = looked
		}
	}
	wrote, err := dnsRowExists(func() error {
		_, err := a.sdk.VNetDNSRecords.Get(ctx, id)
		return err
	})
	if err != nil {
		return nil, err
	}
	if wrote {
		if err := deleteDNSRecord(ctx, a.sdk, id); err != nil {
			return nil, err
		}
	}
	return a.finishDNSApply(ctx, networkID, wrote, applyEnabled(data.Apply))
}

func positiveOrZero(v types.String) int {
	id, ok, err := optionalPositiveID(v)
	if err != nil || !ok {
		return 0
	}
	return id
}

// dnsRowExists reports whether get found the row. Not found is false, nil.
func dnsRowExists(get func() error) (bool, error) {
	err := get()
	if err == nil {
		return true, nil
	}
	if vergeos.IsNotFoundError(err) {
		return false, nil
	}
	return false, err
}

// syncRecordValue copies a NIC address into value when vm_nic_id is set.
// A known plan value that does not match the NIC is refused so apply does
// not store a different address than the plan.
func (a *DNSApi) syncRecordValue(ctx context.Context, data *dnsRecordModel) error {
	nicID, nicSet, err := optionalPositiveID(data.VMNICID)
	if err != nil {
		return fmt.Errorf("vm_nic_id: %w", err)
	}
	value := strings.TrimSpace(stringOrEmpty(data.Value))
	valueSet := !data.Value.IsNull() && !data.Value.IsUnknown() && value != ""
	if !nicSet {
		if !valueSet {
			return fmt.Errorf("value is required when vm_nic_id is unset")
		}
		return nil
	}
	ip, err := a.nicAddress(ctx, nicID)
	if err != nil {
		return err
	}
	recType := strings.TrimSpace(stringOrEmpty(data.Type))
	if recType != vergeos.DNSRecordTypeA && recType != vergeos.DNSRecordTypeAAAA {
		return fmt.Errorf("vm_nic_id is only valid for an A or AAAA record")
	}
	if valueSet && value != ip {
		return fmt.Errorf("NIC %d address is %s, and the plan has %s", nicID, ip, value)
	}
	data.Value = types.StringValue(ip)
	return nil
}

func (a *DNSApi) nicAddress(ctx context.Context, nicID int) (string, error) {
	nic, err := a.sdk.VMNICs.Get(ctx, nicID)
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(nic.IPAddress)
	if ip == "" {
		return "", fmt.Errorf("NIC %d has no IP address yet", nicID)
	}
	return ip, nil
}

// ensureParentID returns NotFound when state names a parent and the API row
// now belongs to a different one. VergeOS reuses keys. Import leaves the
// parent unset, so the check is skipped until the next read.
func ensureParentID(stored types.String, got int, resource string, id int) error {
	want, ok, err := optionalPositiveID(stored)
	if err != nil {
		return err
	}
	if ok && got != want {
		return &vergeos.NotFoundError{Resource: resource, ID: id}
	}
	return nil
}

func knownIntPtr(v types.Int64) *int {
	if v.IsNull() || v.IsUnknown() {
		return nil
	}
	n := int(v.ValueInt64())
	return &n
}

func changedIntPtr(plan, state types.Int64) *int {
	if plan.IsNull() || plan.IsUnknown() {
		return nil
	}
	if !state.IsNull() && !state.IsUnknown() && state.ValueInt64() == plan.ValueInt64() {
		return nil
	}
	n := int(plan.ValueInt64())
	return &n
}

func viewCreateRequest(data *dnsViewModel) (*vergeos.VNetDNSViewCreateRequest, error) {
	networkID, err := parsePositiveID(stringOrEmpty(data.NetworkID))
	if err != nil {
		return nil, fmt.Errorf("network_id: %w", err)
	}
	name := strings.TrimSpace(stringOrEmpty(data.Name))
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	return &vergeos.VNetDNSViewCreateRequest{
		VNet:              networkID,
		Name:              name,
		Recursion:         vergeio.KnownBool(data.Recursion),
		MatchClients:      vergeio.KnownString(data.MatchClients),
		MatchDestinations: vergeio.KnownString(data.MatchDestinations),
		MaxCacheSize:      vergeio.KnownInt64(data.MaxCacheSize),
		OrderID:           knownIntPtr(data.OrderID),
		QuerySource:       knownIntPtr(data.QuerySource),
	}, nil
}

func viewUpdateRequest(plan, state *dnsViewModel) *vergeos.VNetDNSViewUpdateRequest {
	return &vergeos.VNetDNSViewUpdateRequest{
		Name:              vergeio.ChangedString(plan.Name, state.Name),
		Recursion:         vergeio.ChangedBool(plan.Recursion, state.Recursion),
		MatchClients:      vergeio.ChangedString(plan.MatchClients, state.MatchClients),
		MatchDestinations: vergeio.ChangedString(plan.MatchDestinations, state.MatchDestinations),
		MaxCacheSize:      vergeio.ChangedInt64(plan.MaxCacheSize, state.MaxCacheSize),
		OrderID:           changedIntPtr(plan.OrderID, state.OrderID),
		QuerySource:       changedIntPtr(plan.QuerySource, state.QuerySource),
	}
}

func viewUpdateEmpty(req *vergeos.VNetDNSViewUpdateRequest) bool {
	return req == nil || (req.Name == nil && req.Recursion == nil && req.MatchClients == nil &&
		req.MatchDestinations == nil && req.MaxCacheSize == nil && req.OrderID == nil && req.QuerySource == nil)
}

func assignView(data *dnsViewModel, view *vergeos.VNetDNSView) {
	apply := normalizeApply(data.Apply)
	data.ID = typesStringID(view.Key.Int())
	if id := view.VNet.Int(); id > 0 {
		data.NetworkID = typesStringID(id)
	}
	data.Name = types.StringValue(view.Name)
	data.Recursion = types.BoolValue(view.Recursion)
	data.MatchClients = types.StringValue(view.MatchClients)
	data.MatchDestinations = types.StringValue(view.MatchDestinations)
	data.MaxCacheSize = types.Int64Value(view.MaxCacheSize)
	data.OrderID = types.Int64Value(int64(view.OrderID))
	data.QuerySource = types.Int64Value(int64(view.QuerySource.Int()))
	data.Modified = types.Int64Value(view.Modified)
	data.Apply = apply
}

func zoneCreateRequest(data *dnsZoneModel) (*vergeos.VNetDNSZoneCreateRequest, error) {
	viewID, err := parsePositiveID(stringOrEmpty(data.ViewID))
	if err != nil {
		return nil, fmt.Errorf("view_id: %w", err)
	}
	domain := strings.TrimSpace(stringOrEmpty(data.Domain))
	if domain == "" {
		return nil, fmt.Errorf("domain is required")
	}
	return &vergeos.VNetDNSZoneCreateRequest{
		View:            viewID,
		Domain:          domain,
		Type:            vergeio.KnownString(data.Type),
		Nameserver:      vergeio.KnownString(data.Nameserver),
		Email:           vergeio.KnownString(data.Email),
		Notify:          vergeio.KnownString(data.Notify),
		AllowNotify:     vergeio.KnownString(data.AllowNotify),
		AlsoNotify:      vergeio.KnownString(data.AlsoNotify),
		Masters:         vergeio.KnownString(data.Masters),
		AllowTransfer:   vergeio.KnownString(data.AllowTransfer),
		DefaultTTL:      vergeio.KnownString(data.DefaultTTL),
		RefreshInterval: vergeio.KnownString(data.RefreshInterval),
		RetryInterval:   vergeio.KnownString(data.RetryInterval),
		ExpiryPeriod:    vergeio.KnownString(data.ExpiryPeriod),
		NegativeTTL:     vergeio.KnownString(data.NegativeTTL),
		Forwarders:      vergeio.KnownString(data.Forwarders),
	}, nil
}

func zoneUpdateRequest(plan, state *dnsZoneModel) *vergeos.VNetDNSZoneUpdateRequest {
	return &vergeos.VNetDNSZoneUpdateRequest{
		Domain:          vergeio.ChangedString(plan.Domain, state.Domain),
		Type:            vergeio.ChangedString(plan.Type, state.Type),
		Nameserver:      vergeio.ChangedString(plan.Nameserver, state.Nameserver),
		Email:           vergeio.ChangedString(plan.Email, state.Email),
		Notify:          vergeio.ChangedString(plan.Notify, state.Notify),
		AllowNotify:     vergeio.ChangedString(plan.AllowNotify, state.AllowNotify),
		AlsoNotify:      vergeio.ChangedString(plan.AlsoNotify, state.AlsoNotify),
		Masters:         vergeio.ChangedString(plan.Masters, state.Masters),
		AllowTransfer:   vergeio.ChangedString(plan.AllowTransfer, state.AllowTransfer),
		DefaultTTL:      vergeio.ChangedString(plan.DefaultTTL, state.DefaultTTL),
		RefreshInterval: vergeio.ChangedString(plan.RefreshInterval, state.RefreshInterval),
		RetryInterval:   vergeio.ChangedString(plan.RetryInterval, state.RetryInterval),
		ExpiryPeriod:    vergeio.ChangedString(plan.ExpiryPeriod, state.ExpiryPeriod),
		NegativeTTL:     vergeio.ChangedString(plan.NegativeTTL, state.NegativeTTL),
		Forwarders:      vergeio.ChangedString(plan.Forwarders, state.Forwarders),
	}
}

func zoneUpdateEmpty(req *vergeos.VNetDNSZoneUpdateRequest) bool {
	return req == nil || (req.Domain == nil && req.Type == nil && req.Nameserver == nil && req.Email == nil &&
		req.Notify == nil && req.AllowNotify == nil && req.AlsoNotify == nil && req.Masters == nil &&
		req.AllowTransfer == nil && req.DefaultTTL == nil && req.RefreshInterval == nil &&
		req.RetryInterval == nil && req.ExpiryPeriod == nil && req.NegativeTTL == nil && req.Forwarders == nil)
}

func assignZone(data *dnsZoneModel, zone *vergeos.VNetDNSZone, networkID int) {
	apply := normalizeApply(data.Apply)
	data.ID = typesStringID(zone.Key.Int())
	if id := zone.View.Int(); id > 0 {
		data.ViewID = typesStringID(id)
	}
	if networkID > 0 {
		data.NetworkID = typesStringID(networkID)
	}
	data.Domain = types.StringValue(zone.Domain)
	data.Type = types.StringValue(zone.Type)
	data.Nameserver = types.StringValue(zone.Nameserver)
	data.Email = types.StringValue(zone.Email)
	data.Notify = types.StringValue(zone.Notify)
	data.AllowNotify = types.StringValue(zone.AllowNotify)
	data.AlsoNotify = types.StringValue(zone.AlsoNotify)
	data.Masters = types.StringValue(zone.Masters)
	data.AllowTransfer = types.StringValue(zone.AllowTransfer)
	data.SerialNumber = types.Int64Value(zone.SerialNumber)
	data.DefaultTTL = types.StringValue(zone.DefaultTTL)
	data.RefreshInterval = types.StringValue(zone.RefreshInterval)
	data.RetryInterval = types.StringValue(zone.RetryInterval)
	data.ExpiryPeriod = types.StringValue(zone.ExpiryPeriod)
	data.NegativeTTL = types.StringValue(zone.NegativeTTL)
	data.Forwarders = types.StringValue(zone.Forwarders)
	data.Modified = types.Int64Value(zone.Modified)
	data.Apply = apply
}

func recordCreateRequest(data *dnsRecordModel) (*vergeos.VNetDNSRecordCreateRequest, error) {
	zoneID, err := parsePositiveID(stringOrEmpty(data.ZoneID))
	if err != nil {
		return nil, fmt.Errorf("zone_id: %w", err)
	}
	recType := strings.TrimSpace(stringOrEmpty(data.Type))
	if recType == "" {
		return nil, fmt.Errorf("type is required")
	}
	value := strings.TrimSpace(stringOrEmpty(data.Value))
	if value == "" {
		return nil, fmt.Errorf("value is required")
	}
	req := &vergeos.VNetDNSRecordCreateRequest{
		Zone:          zoneID,
		Type:          recType,
		Value:         value,
		TTL:           vergeio.KnownString(data.TTL),
		MXPreference:  knownIntPtr(data.MXPreference),
		Weight:        knownIntPtr(data.Weight),
		Port:          knownIntPtr(data.Port),
		IssueWildcard: vergeio.KnownBool(data.IssueWildcard),
		OrderID:       knownIntPtr(data.OrderID),
	}
	if host := vergeio.KnownString(data.Host); host != nil {
		req.Host = *host
	}
	if description := vergeio.KnownString(data.Description); description != nil {
		req.Description = *description
	}
	return req, nil
}

func recordUpdateRequest(plan, state *dnsRecordModel) *vergeos.VNetDNSRecordUpdateRequest {
	return &vergeos.VNetDNSRecordUpdateRequest{
		Host:          vergeio.ChangedString(plan.Host, state.Host),
		TTL:           vergeio.ChangedString(plan.TTL, state.TTL),
		Type:          vergeio.ChangedString(plan.Type, state.Type),
		Value:         vergeio.ChangedString(plan.Value, state.Value),
		Description:   vergeio.ChangedString(plan.Description, state.Description),
		MXPreference:  changedIntPtr(plan.MXPreference, state.MXPreference),
		Weight:        changedIntPtr(plan.Weight, state.Weight),
		Port:          changedIntPtr(plan.Port, state.Port),
		IssueWildcard: vergeio.ChangedBool(plan.IssueWildcard, state.IssueWildcard),
		OrderID:       changedIntPtr(plan.OrderID, state.OrderID),
	}
}

func recordUpdateEmpty(req *vergeos.VNetDNSRecordUpdateRequest) bool {
	return req == nil || (req.Host == nil && req.TTL == nil && req.Type == nil && req.Value == nil &&
		req.Description == nil && req.MXPreference == nil && req.Weight == nil && req.Port == nil &&
		req.IssueWildcard == nil && req.OrderID == nil)
}

func assignRecord(data *dnsRecordModel, record *vergeos.VNetDNSRecord, networkID int) {
	apply := normalizeApply(data.Apply)
	nicID := data.VMNICID
	data.ID = typesStringID(record.Key.Int())
	if id := record.Zone.Int(); id > 0 {
		data.ZoneID = typesStringID(id)
	}
	if networkID > 0 {
		data.NetworkID = typesStringID(networkID)
	}
	data.Host = types.StringValue(record.Host)
	data.TTL = types.StringValue(record.TTL)
	data.Type = types.StringValue(record.Type)
	data.Value = types.StringValue(record.Value)
	data.VMNICID = nicID
	data.Description = types.StringValue(record.Description)
	data.MXPreference = types.Int64Value(int64(record.MXPreference))
	data.Weight = types.Int64Value(int64(record.Weight))
	data.Port = types.Int64Value(int64(record.Port))
	data.IssueWildcard = types.BoolValue(record.IssueWildcard)
	data.OrderID = types.Int64Value(int64(record.OrderID))
	data.Modified = types.Int64Value(record.Modified)
	data.Apply = apply
}

// DeleteNetworkDNSRows removes DNS views on a network before the network
// itself is deleted. VergeOS deletes the zones and records with the view.
// A view delete that is refused while a zone remains is retried after those
// rows are removed, so a partial create cannot leave a row that blocks the
// network delete.
func DeleteNetworkDNSRows(ctx context.Context, sdk *vergeos.Client, networkID int) error {
	if sdk == nil {
		return fmt.Errorf("vergeos client is nil")
	}
	if networkID <= 0 {
		return fmt.Errorf("network id must be a positive integer")
	}
	views, err := sdk.VNetDNSViews.ListByNetwork(ctx, networkID)
	if err != nil {
		return fmt.Errorf("list DNS views for network %d: %w", networkID, err)
	}
	var errs []error
	for _, view := range views {
		if err := deleteDNSView(ctx, sdk, view.Key.Int()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteDNSView removes one view. VergeOS also removes its zones and records.
// If that delete is refused, the child rows are removed and the view delete
// is tried again.
func deleteDNSView(ctx context.Context, sdk *vergeos.Client, id int) error {
	if id <= 0 {
		return nil
	}
	err := sdk.VNetDNSViews.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if childErr := deleteViewChildren(ctx, sdk, id); childErr != nil {
		return fmt.Errorf("delete DNS view %d: %w (zones were left in place: %v)", id, err, childErr)
	}
	retry := sdk.VNetDNSViews.Delete(ctx, id)
	if retry == nil || vergeos.IsNotFoundError(retry) {
		return nil
	}
	return fmt.Errorf("delete DNS view %d: %w", id, retry)
}

func deleteViewChildren(ctx context.Context, sdk *vergeos.Client, viewID int) error {
	zones, err := sdk.VNetDNSZones.ListByView(ctx, viewID)
	if err != nil {
		return fmt.Errorf("list DNS zones for view %d: %w", viewID, err)
	}
	var errs []error
	for _, zone := range zones {
		if err := deleteDNSZone(ctx, sdk, zone.Key.Int()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// deleteDNSZone removes one zone. VergeOS also removes its records. If that
// delete is refused, the records are removed and the zone delete is tried again.
func deleteDNSZone(ctx context.Context, sdk *vergeos.Client, id int) error {
	if id <= 0 {
		return nil
	}
	err := sdk.VNetDNSZones.Delete(ctx, id)
	if err == nil || vergeos.IsNotFoundError(err) {
		return nil
	}
	if childErr := deleteZoneRecords(ctx, sdk, id); childErr != nil {
		return fmt.Errorf("delete DNS zone %d: %w (records were left in place: %v)", id, err, childErr)
	}
	retry := sdk.VNetDNSZones.Delete(ctx, id)
	if retry == nil || vergeos.IsNotFoundError(retry) {
		return nil
	}
	return fmt.Errorf("delete DNS zone %d: %w", id, retry)
}

func deleteZoneRecords(ctx context.Context, sdk *vergeos.Client, zoneID int) error {
	records, err := sdk.VNetDNSRecords.ListByZone(ctx, zoneID)
	if err != nil {
		return fmt.Errorf("list DNS records for zone %d: %w", zoneID, err)
	}
	var errs []error
	for _, record := range records {
		if err := deleteDNSRecord(ctx, sdk, record.Key.Int()); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func deleteDNSRecord(ctx context.Context, sdk *vergeos.Client, id int) error {
	if id <= 0 {
		return nil
	}
	if err := sdk.VNetDNSRecords.Delete(ctx, id); err != nil {
		if vergeos.IsNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("delete DNS record %d: %w", id, err)
	}
	return nil
}
