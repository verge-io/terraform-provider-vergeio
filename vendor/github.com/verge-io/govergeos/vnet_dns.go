package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// VNetDNSViewService handles network DNS view operations.
type VNetDNSViewService struct {
	client *Client
}

// List returns all DNS views, with optional filtering and pagination.
func (s *VNetDNSViewService) List(ctx context.Context, opts ...ListOption) ([]VNetDNSView, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetDNSViewListFields
	}

	params := options.toQueryParams()

	var views []VNetDNSView
	if err := s.client.get(ctx, "/vnet_dns_views", params, &views); err != nil {
		return nil, err
	}

	return views, nil
}

// ListByNetwork returns all DNS views for a specific network.
func (s *VNetDNSViewService) ListByNetwork(ctx context.Context, vnetID int, opts ...ListOption) ([]VNetDNSView, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("vnet eq %d", vnetID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single DNS view by ID.
func (s *VNetDNSViewService) Get(ctx context.Context, id int) (*VNetDNSView, error) {
	params := url.Values{}
	params.Set("fields", vnetDNSViewGetFields)

	var view VNetDNSView
	endpoint := fmt.Sprintf("/vnet_dns_views/%d", id)
	if err := s.client.get(ctx, endpoint, params, &view); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetDNSView", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &view, nil
}

// GetByName returns a DNS view by name within a specific network.
func (s *VNetDNSViewService) GetByName(ctx context.Context, vnetID int, name string) (*VNetDNSView, error) {
	views, err := s.List(ctx, WithFilter(fmt.Sprintf("vnet eq %d and name eq '%s'", vnetID, name)))
	if err != nil {
		return nil, err
	}
	if len(views) == 0 {
		return nil, &NotFoundError{Resource: "VNetDNSView", ID: name}
	}
	return s.Get(ctx, int(views[0].Key))
}

// Create creates a new DNS view and returns the created view.
func (s *VNetDNSViewService) Create(ctx context.Context, req *VNetDNSViewCreateRequest) (*VNetDNSView, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.VNet <= 0 {
		return nil, &ValidationError{Field: "vnet", Message: "vnet is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_dns_views", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a DNS view and returns the updated view.
func (s *VNetDNSViewService) Update(ctx context.Context, id int, req *VNetDNSViewUpdateRequest) (*VNetDNSView, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_dns_views/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetDNSView", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a DNS view.
func (s *VNetDNSViewService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_dns_views/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetDNSView", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// VNetDNSZoneService handles network DNS zone operations.
type VNetDNSZoneService struct {
	client *Client
}

// List returns all DNS zones, with optional filtering and pagination.
func (s *VNetDNSZoneService) List(ctx context.Context, opts ...ListOption) ([]VNetDNSZone, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetDNSZoneListFields
	}

	params := options.toQueryParams()

	var zones []VNetDNSZone
	if err := s.client.get(ctx, "/vnet_dns_zones", params, &zones); err != nil {
		return nil, err
	}

	return zones, nil
}

// ListByView returns all DNS zones for a specific view.
func (s *VNetDNSZoneService) ListByView(ctx context.Context, viewID int, opts ...ListOption) ([]VNetDNSZone, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("view eq %d", viewID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single DNS zone by ID.
func (s *VNetDNSZoneService) Get(ctx context.Context, id int) (*VNetDNSZone, error) {
	params := url.Values{}
	params.Set("fields", vnetDNSZoneGetFields)

	var zone VNetDNSZone
	endpoint := fmt.Sprintf("/vnet_dns_zones/%d", id)
	if err := s.client.get(ctx, endpoint, params, &zone); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetDNSZone", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &zone, nil
}

// GetByDomain returns a DNS zone by domain name within a specific view.
func (s *VNetDNSZoneService) GetByDomain(ctx context.Context, viewID int, domain string) (*VNetDNSZone, error) {
	zones, err := s.List(ctx, WithFilter(fmt.Sprintf("view eq %d and domain eq '%s'", viewID, domain)))
	if err != nil {
		return nil, err
	}
	if len(zones) == 0 {
		return nil, &NotFoundError{Resource: "VNetDNSZone", ID: domain}
	}
	return s.Get(ctx, int(zones[0].Key))
}

// Create creates a new DNS zone and returns the created zone.
func (s *VNetDNSZoneService) Create(ctx context.Context, req *VNetDNSZoneCreateRequest) (*VNetDNSZone, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.View <= 0 {
		return nil, &ValidationError{Field: "view", Message: "view is required"}
	}
	if req.Domain == "" {
		return nil, &ValidationError{Field: "domain", Message: "domain is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_dns_zones", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a DNS zone and returns the updated zone.
func (s *VNetDNSZoneService) Update(ctx context.Context, id int, req *VNetDNSZoneUpdateRequest) (*VNetDNSZone, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_dns_zones/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetDNSZone", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a DNS zone.
func (s *VNetDNSZoneService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_dns_zones/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetDNSZone", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// VNetDNSRecordService handles network DNS record operations.
type VNetDNSRecordService struct {
	client *Client
}

// List returns all DNS records, with optional filtering and pagination.
func (s *VNetDNSRecordService) List(ctx context.Context, opts ...ListOption) ([]VNetDNSRecord, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = vnetDNSRecordListFields
	}

	params := options.toQueryParams()

	var records []VNetDNSRecord
	if err := s.client.get(ctx, "/vnet_dns_zone_records", params, &records); err != nil {
		return nil, err
	}

	return records, nil
}

// ListByZone returns all DNS records for a specific zone.
func (s *VNetDNSRecordService) ListByZone(ctx context.Context, zoneID int, opts ...ListOption) ([]VNetDNSRecord, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("zone eq %d", zoneID))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByType returns all DNS records of a specific type within a zone.
func (s *VNetDNSRecordService) ListByType(ctx context.Context, zoneID int, recordType string, opts ...ListOption) ([]VNetDNSRecord, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("zone eq %d and type eq '%s'", zoneID, recordType))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// Get returns a single DNS record by ID.
func (s *VNetDNSRecordService) Get(ctx context.Context, id int) (*VNetDNSRecord, error) {
	params := url.Values{}
	params.Set("fields", vnetDNSRecordGetFields)

	var record VNetDNSRecord
	endpoint := fmt.Sprintf("/vnet_dns_zone_records/%d", id)
	if err := s.client.get(ctx, endpoint, params, &record); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetDNSRecord", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &record, nil
}

// GetByHostAndType returns a DNS record by host and type within a zone.
func (s *VNetDNSRecordService) GetByHostAndType(ctx context.Context, zoneID int, host, recordType string) (*VNetDNSRecord, error) {
	records, err := s.List(ctx, WithFilter(fmt.Sprintf("zone eq %d and host eq '%s' and type eq '%s'", zoneID, host, recordType)))
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, &NotFoundError{Resource: "VNetDNSRecord", ID: fmt.Sprintf("%s (%s)", host, recordType)}
	}
	return s.Get(ctx, int(records[0].Key))
}

// Create creates a new DNS record and returns the created record.
func (s *VNetDNSRecordService) Create(ctx context.Context, req *VNetDNSRecordCreateRequest) (*VNetDNSRecord, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Zone <= 0 {
		return nil, &ValidationError{Field: "zone", Message: "zone is required"}
	}
	if req.Type == "" {
		return nil, &ValidationError{Field: "type", Message: "type is required"}
	}
	if req.Value == "" {
		return nil, &ValidationError{Field: "value", Message: "value is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/vnet_dns_zone_records", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a DNS record and returns the updated record.
func (s *VNetDNSRecordService) Update(ctx context.Context, id int, req *VNetDNSRecordUpdateRequest) (*VNetDNSRecord, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/vnet_dns_zone_records/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "VNetDNSRecord", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a DNS record.
func (s *VNetDNSRecordService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/vnet_dns_zone_records/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "VNetDNSRecord", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}
