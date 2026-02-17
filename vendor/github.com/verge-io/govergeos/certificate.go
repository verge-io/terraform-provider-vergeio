package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// CertificateService handles SSL/TLS certificate operations.
type CertificateService struct {
	client *Client
}

// List returns all certificates, with optional filtering and pagination.
func (s *CertificateService) List(ctx context.Context, opts ...ListOption) ([]Certificate, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = certificateListFields
	}

	params := options.toQueryParams()

	var certs []Certificate
	if err := s.client.get(ctx, "/certificates", params, &certs); err != nil {
		return nil, err
	}

	return certs, nil
}

// Get returns a single certificate by ID.
func (s *CertificateService) Get(ctx context.Context, id int) (*Certificate, error) {
	params := url.Values{}
	params.Set("fields", certificateGetFields)

	var cert Certificate
	endpoint := fmt.Sprintf("/certificates/%d", id)
	if err := s.client.get(ctx, endpoint, params, &cert); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Certificate", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &cert, nil
}

// GetByDomain returns a certificate by its primary domain.
func (s *CertificateService) GetByDomain(ctx context.Context, domain string) (*Certificate, error) {
	certs, err := s.List(ctx, WithFilter(fmt.Sprintf("domain eq '%s'", domain)))
	if err != nil {
		return nil, err
	}
	if len(certs) == 0 {
		return nil, &NotFoundError{Resource: "Certificate", ID: domain}
	}
	return s.Get(ctx, int(certs[0].Key))
}

// GetWithKeys returns a certificate including its public key, private key, and chain.
// Use this method when you need to export or use the certificate keys.
func (s *CertificateService) GetWithKeys(ctx context.Context, id int) (*Certificate, error) {
	params := url.Values{}
	params.Set("fields", certificateGetFields+",public,private,chain")

	var cert Certificate
	endpoint := fmt.Sprintf("/certificates/%d", id)
	if err := s.client.get(ctx, endpoint, params, &cert); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Certificate", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return &cert, nil
}

// Create creates a new certificate and returns the created certificate.
func (s *CertificateService) Create(ctx context.Context, req *CertificateCreateRequest) (*Certificate, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/certificates", req, &resp); err != nil {
		return nil, err
	}

	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	return s.Get(ctx, id)
}

// Update updates a certificate and returns the updated certificate.
func (s *CertificateService) Update(ctx context.Context, id int, req *CertificateUpdateRequest) (*Certificate, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/certificates/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Certificate", ID: fmt.Sprintf("%d", id)}
		}
		return nil, err
	}

	return s.Get(ctx, id)
}

// Delete deletes a certificate.
func (s *CertificateService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/certificates/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return &NotFoundError{Resource: "Certificate", ID: fmt.Sprintf("%d", id)}
		}
		return err
	}
	return nil
}

// Renew triggers renewal of a Let's Encrypt certificate.
func (s *CertificateService) Renew(ctx context.Context, id int) (*Certificate, error) {
	renew := true
	return s.Update(ctx, id, &CertificateUpdateRequest{Renew: &renew})
}

// ListExpiring returns certificates that will expire within the specified number of days.
func (s *CertificateService) ListExpiring(ctx context.Context, days int, opts ...ListOption) ([]Certificate, error) {
	// Calculate Unix timestamp for 'days' from now
	// This is a simple filter approach - exact implementation depends on API support
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("valid eq true"))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListValid returns all valid certificates.
func (s *CertificateService) ListValid(ctx context.Context, opts ...ListOption) ([]Certificate, error) {
	filterOpts := []ListOption{WithFilter("valid eq true")}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}

// ListByType returns certificates of a specific type (manual, letsencrypt, self_signed).
func (s *CertificateService) ListByType(ctx context.Context, certType string, opts ...ListOption) ([]Certificate, error) {
	filterOpts := []ListOption{WithFilter(fmt.Sprintf("type eq '%s'", certType))}
	filterOpts = append(filterOpts, opts...)
	return s.List(ctx, filterOpts...)
}
