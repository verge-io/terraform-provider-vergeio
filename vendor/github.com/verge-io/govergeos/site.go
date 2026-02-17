package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// SiteService handles site operations for DR, backup, and synchronization.
type SiteService struct {
	client *Client
}

// List returns all sites, with optional filtering and pagination.
func (s *SiteService) List(ctx context.Context, opts ...ListOption) ([]Site, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = siteListFields
	}

	params := options.toQueryParams()

	var sites []Site
	if err := s.client.get(ctx, "/sites", params, &sites); err != nil {
		return nil, err
	}

	return sites, nil
}

// Get returns a single site by ID.
func (s *SiteService) Get(ctx context.Context, id int) (*Site, error) {
	params := url.Values{}
	params.Set("fields", siteGetFields)

	var site Site
	endpoint := fmt.Sprintf("/sites/%d", id)
	if err := s.client.get(ctx, endpoint, params, &site); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Site", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &site, nil
}

// GetByName returns a site by name.
func (s *SiteService) GetByName(ctx context.Context, name string) (*Site, error) {
	sites, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}
	if len(sites) == 0 {
		return nil, &NotFoundError{Resource: "Site", ID: name}
	}
	return s.Get(ctx, int(sites[0].Key))
}

// GetBySiteID returns a site by its 40-character SHA1 site ID.
func (s *SiteService) GetBySiteID(ctx context.Context, siteID string) (*Site, error) {
	sites, err := s.List(ctx, WithFilter(fmt.Sprintf("id eq '%s'", siteID)))
	if err != nil {
		return nil, err
	}
	if len(sites) == 0 {
		return nil, &NotFoundError{Resource: "Site", ID: siteID}
	}
	return s.Get(ctx, int(sites[0].Key))
}

// Create creates a new site and returns the created site.
func (s *SiteService) Create(ctx context.Context, req *SiteCreateRequest) (*Site, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.URL == "" {
		return nil, &ValidationError{Field: "url", Message: "url is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/sites", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a site and returns the updated site.
func (s *SiteService) Update(ctx context.Context, id int, req *SiteUpdateRequest) (*Site, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/sites/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Site", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a site.
func (s *SiteService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/sites/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Site", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// Refresh refreshes the site connection and settings.
func (s *SiteService) Refresh(ctx context.Context, id int) error {
	action := siteAction{
		Site:   id,
		Action: "refresh",
	}

	if err := s.client.post(ctx, "/site_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to refresh site %d: %w", id, err)
	}
	return nil
}

// RefreshSettings refreshes only the site settings.
func (s *SiteService) RefreshSettings(ctx context.Context, id int) error {
	action := siteAction{
		Site:   id,
		Action: "refresh_settings",
	}

	if err := s.client.post(ctx, "/site_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to refresh site settings %d: %w", id, err)
	}
	return nil
}

// Reauthenticate reauthenticates with the remote site.
func (s *SiteService) Reauthenticate(ctx context.Context, id int) error {
	action := siteAction{
		Site:   id,
		Action: "reauthenticate",
	}

	if err := s.client.post(ctx, "/site_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to reauthenticate site %d: %w", id, err)
	}
	return nil
}

// RunUpdates runs system updates from the remote site.
func (s *SiteService) RunUpdates(ctx context.Context, id int) error {
	action := siteAction{
		Site:   id,
		Action: "run_updates",
	}

	if err := s.client.post(ctx, "/site_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to run updates from site %d: %w", id, err)
	}
	return nil
}

// ClearSyncedLogs clears synced logs for the site.
func (s *SiteService) ClearSyncedLogs(ctx context.Context, id int) error {
	action := siteAction{
		Site:   id,
		Action: "clear_synced_logs",
	}

	if err := s.client.post(ctx, "/site_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to clear synced logs for site %d: %w", id, err)
	}
	return nil
}

// SiteSyncIncomingService handles incoming sync operations.
type SiteSyncIncomingService struct {
	client *Client
}

// List returns all incoming syncs, with optional filtering and pagination.
func (s *SiteSyncIncomingService) List(ctx context.Context, opts ...ListOption) ([]SiteSyncIncoming, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = siteSyncIncomingListFields
	}

	params := options.toQueryParams()

	var syncs []SiteSyncIncoming
	if err := s.client.get(ctx, "/site_syncs_incoming", params, &syncs); err != nil {
		return nil, err
	}

	return syncs, nil
}

// ListBySite returns all incoming syncs for a specific site.
func (s *SiteSyncIncomingService) ListBySite(ctx context.Context, siteID int, opts ...ListOption) ([]SiteSyncIncoming, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("site eq %d", siteID)))
	return s.List(ctx, opts...)
}

// Get returns a single incoming sync by ID.
func (s *SiteSyncIncomingService) Get(ctx context.Context, id int) (*SiteSyncIncoming, error) {
	params := url.Values{}
	params.Set("fields", siteSyncIncomingGetFields)

	var sync SiteSyncIncoming
	endpoint := fmt.Sprintf("/site_syncs_incoming/%d", id)
	if err := s.client.get(ctx, endpoint, params, &sync); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SiteSyncIncoming", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &sync, nil
}

// GetByName returns an incoming sync by name within a site.
func (s *SiteSyncIncomingService) GetByName(ctx context.Context, siteID int, name string) (*SiteSyncIncoming, error) {
	syncs, err := s.List(ctx,
		WithFilter(fmt.Sprintf("site eq %d and name eq '%s'", siteID, name)),
	)
	if err != nil {
		return nil, err
	}
	if len(syncs) == 0 {
		return nil, &NotFoundError{Resource: "SiteSyncIncoming", ID: name}
	}
	return s.Get(ctx, int(syncs[0].Key))
}

// GetBySyncID returns an incoming sync by its 40-character sync ID.
func (s *SiteSyncIncomingService) GetBySyncID(ctx context.Context, syncID string) (*SiteSyncIncoming, error) {
	syncs, err := s.List(ctx, WithFilter(fmt.Sprintf("sync_id eq '%s'", syncID)))
	if err != nil {
		return nil, err
	}
	if len(syncs) == 0 {
		return nil, &NotFoundError{Resource: "SiteSyncIncoming", ID: syncID}
	}
	return s.Get(ctx, int(syncs[0].Key))
}

// Create creates a new incoming sync and returns it.
func (s *SiteSyncIncomingService) Create(ctx context.Context, req *SiteSyncIncomingCreateRequest) (*SiteSyncIncoming, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Site == 0 {
		return nil, &ValidationError{Field: "site", Message: "site is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/site_syncs_incoming", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates an incoming sync and returns the updated sync.
func (s *SiteSyncIncomingService) Update(ctx context.Context, id int, req *SiteSyncIncomingUpdateRequest) (*SiteSyncIncoming, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/site_syncs_incoming/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SiteSyncIncoming", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes an incoming sync.
func (s *SiteSyncIncomingService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/site_syncs_incoming/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "SiteSyncIncoming", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// Enable enables an incoming sync.
func (s *SiteSyncIncomingService) Enable(ctx context.Context, id int) error {
	action := siteSyncIncomingAction{
		SiteSyncIncoming: id,
		Action:           "enable",
	}

	if err := s.client.post(ctx, "/site_syncs_incoming_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to enable incoming sync %d: %w", id, err)
	}
	return nil
}

// Disable disables an incoming sync.
func (s *SiteSyncIncomingService) Disable(ctx context.Context, id int) error {
	action := siteSyncIncomingAction{
		SiteSyncIncoming: id,
		Action:           "disable",
	}

	if err := s.client.post(ctx, "/site_syncs_incoming_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to disable incoming sync %d: %w", id, err)
	}
	return nil
}

// SiteSyncOutgoingService handles outgoing sync operations.
type SiteSyncOutgoingService struct {
	client *Client
}

// List returns all outgoing syncs, with optional filtering and pagination.
func (s *SiteSyncOutgoingService) List(ctx context.Context, opts ...ListOption) ([]SiteSyncOutgoing, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = siteSyncOutgoingListFields
	}

	params := options.toQueryParams()

	var syncs []SiteSyncOutgoing
	if err := s.client.get(ctx, "/site_syncs_outgoing", params, &syncs); err != nil {
		return nil, err
	}

	return syncs, nil
}

// ListBySite returns all outgoing syncs for a specific site.
func (s *SiteSyncOutgoingService) ListBySite(ctx context.Context, siteID int, opts ...ListOption) ([]SiteSyncOutgoing, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("site eq %d", siteID)))
	return s.List(ctx, opts...)
}

// Get returns a single outgoing sync by ID.
func (s *SiteSyncOutgoingService) Get(ctx context.Context, id int) (*SiteSyncOutgoing, error) {
	params := url.Values{}
	params.Set("fields", siteSyncOutgoingGetFields)

	var sync SiteSyncOutgoing
	endpoint := fmt.Sprintf("/site_syncs_outgoing/%d", id)
	if err := s.client.get(ctx, endpoint, params, &sync); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SiteSyncOutgoing", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &sync, nil
}

// GetByName returns an outgoing sync by name within a site.
func (s *SiteSyncOutgoingService) GetByName(ctx context.Context, siteID int, name string) (*SiteSyncOutgoing, error) {
	syncs, err := s.List(ctx,
		WithFilter(fmt.Sprintf("site eq %d and name eq '%s'", siteID, name)),
	)
	if err != nil {
		return nil, err
	}
	if len(syncs) == 0 {
		return nil, &NotFoundError{Resource: "SiteSyncOutgoing", ID: name}
	}
	return s.Get(ctx, int(syncs[0].Key))
}

// Create creates a new outgoing sync and returns it.
func (s *SiteSyncOutgoingService) Create(ctx context.Context, req *SiteSyncOutgoingCreateRequest) (*SiteSyncOutgoing, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Site == 0 {
		return nil, &ValidationError{Field: "site", Message: "site is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/site_syncs_outgoing", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates an outgoing sync and returns the updated sync.
func (s *SiteSyncOutgoingService) Update(ctx context.Context, id int, req *SiteSyncOutgoingUpdateRequest) (*SiteSyncOutgoing, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/site_syncs_outgoing/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SiteSyncOutgoing", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes an outgoing sync.
func (s *SiteSyncOutgoingService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/site_syncs_outgoing/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "SiteSyncOutgoing", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// Enable enables an outgoing sync.
func (s *SiteSyncOutgoingService) Enable(ctx context.Context, id int) error {
	action := siteSyncOutgoingAction{
		SiteSyncOutgoing: id,
		Action:           "enable",
	}

	if err := s.client.post(ctx, "/site_syncs_outgoing_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to enable outgoing sync %d: %w", id, err)
	}
	return nil
}

// Disable disables an outgoing sync.
func (s *SiteSyncOutgoingService) Disable(ctx context.Context, id int) error {
	action := siteSyncOutgoingAction{
		SiteSyncOutgoing: id,
		Action:           "disable",
	}

	if err := s.client.post(ctx, "/site_syncs_outgoing_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to disable outgoing sync %d: %w", id, err)
	}
	return nil
}

// Throttle sets the send throttle for an outgoing sync.
// Set throttle to 0 to disable throttling.
func (s *SiteSyncOutgoingService) Throttle(ctx context.Context, id int, throttle int) error {
	action := siteSyncOutgoingAction{
		SiteSyncOutgoing: id,
		Action:           "throttle",
		Params: map[string]interface{}{
			"throttle": throttle,
		},
	}

	if err := s.client.post(ctx, "/site_syncs_outgoing_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to set throttle for outgoing sync %d: %w", id, err)
	}
	return nil
}

// DisableThrottle disables throttling for an outgoing sync.
func (s *SiteSyncOutgoingService) DisableThrottle(ctx context.Context, id int) error {
	action := siteSyncOutgoingAction{
		SiteSyncOutgoing: id,
		Action:           "throttle_disable",
	}

	if err := s.client.post(ctx, "/site_syncs_outgoing_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to disable throttle for outgoing sync %d: %w", id, err)
	}
	return nil
}

// RefreshSnapshots refreshes the list of snapshots on the destination.
func (s *SiteSyncOutgoingService) RefreshSnapshots(ctx context.Context, id int) error {
	action := siteSyncOutgoingAction{
		SiteSyncOutgoing: id,
		Action:           "refresh",
	}

	if err := s.client.post(ctx, "/site_syncs_outgoing_actions", action, nil); err != nil {
		return fmt.Errorf("vergeos: failed to refresh snapshots for outgoing sync %d: %w", id, err)
	}
	return nil
}

// SiteSyncProfilePeriodService handles site sync profile period operations.
// Profile periods configure when snapshots are synced to remote sites based on snapshot profile periods.
type SiteSyncProfilePeriodService struct {
	client *Client
}

// List returns all site sync profile periods, with optional filtering and pagination.
func (s *SiteSyncProfilePeriodService) List(ctx context.Context, opts ...ListOption) ([]SiteSyncProfilePeriod, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = siteSyncProfilePeriodListFields
	}

	params := options.toQueryParams()

	var periods []SiteSyncProfilePeriod
	if err := s.client.get(ctx, "/site_syncs_outgoing_profile_periods", params, &periods); err != nil {
		return nil, err
	}

	return periods, nil
}

// ListByOutgoingSync returns all profile periods for a specific outgoing sync.
func (s *SiteSyncProfilePeriodService) ListByOutgoingSync(ctx context.Context, outgoingSyncID int, opts ...ListOption) ([]SiteSyncProfilePeriod, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("site_syncs_outgoing eq %d", outgoingSyncID)))
	return s.List(ctx, opts...)
}

// Get returns a single site sync profile period by ID.
func (s *SiteSyncProfilePeriodService) Get(ctx context.Context, id int) (*SiteSyncProfilePeriod, error) {
	params := url.Values{}
	params.Set("fields", siteSyncProfilePeriodGetFields)

	var period SiteSyncProfilePeriod
	endpoint := fmt.Sprintf("/site_syncs_outgoing_profile_periods/%d", id)
	if err := s.client.get(ctx, endpoint, params, &period); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SiteSyncProfilePeriod", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &period, nil
}

// Create creates a new site sync profile period and returns it.
func (s *SiteSyncProfilePeriodService) Create(ctx context.Context, req *SiteSyncProfilePeriodCreateRequest) (*SiteSyncProfilePeriod, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.SiteSyncsOutgoing == 0 {
		return nil, &ValidationError{Field: "site_syncs_outgoing", Message: "site_syncs_outgoing is required"}
	}
	if req.ProfilePeriod == 0 {
		return nil, &ValidationError{Field: "profile_period", Message: "profile_period is required"}
	}
	if req.Retention == 0 {
		return nil, &ValidationError{Field: "retention", Message: "retention is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/site_syncs_outgoing_profile_periods", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a site sync profile period and returns the updated period.
func (s *SiteSyncProfilePeriodService) Update(ctx context.Context, id int, req *SiteSyncProfilePeriodUpdateRequest) (*SiteSyncProfilePeriod, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/site_syncs_outgoing_profile_periods/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "SiteSyncProfilePeriod", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a site sync profile period.
func (s *SiteSyncProfilePeriodService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/site_syncs_outgoing_profile_periods/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "SiteSyncProfilePeriod", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}
