package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// ClusterService handles cluster operations.
type ClusterService struct {
	client *Client
}

// List returns all clusters, with optional filtering and pagination.
func (s *ClusterService) List(ctx context.Context, opts ...ListOption) ([]Cluster, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = clusterListFields
	}

	params := options.toQueryParams()

	var clusters []Cluster
	if err := s.client.get(ctx, "/clusters", params, &clusters); err != nil {
		return nil, err
	}

	return clusters, nil
}

// Get returns a single cluster by ID.
func (s *ClusterService) Get(ctx context.Context, id int) (*Cluster, error) {
	params := url.Values{}
	params.Set("fields", clusterListFields)

	var cluster Cluster
	endpoint := fmt.Sprintf("/clusters/%d", id)
	if err := s.client.get(ctx, endpoint, params, &cluster); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Cluster", ID: id}
		}
		return nil, err
	}

	return &cluster, nil
}

// GetByName returns a cluster by name.
func (s *ClusterService) GetByName(ctx context.Context, name string) (*Cluster, error) {
	clusters, err := s.List(ctx, WithFilter(fmt.Sprintf("name eq '%s'", name)))
	if err != nil {
		return nil, err
	}

	if len(clusters) == 0 {
		return nil, &NotFoundError{Resource: "Cluster", ID: name}
	}

	return &clusters[0], nil
}

// GetStatus returns detailed status for a cluster.
func (s *ClusterService) GetStatus(ctx context.Context, id int) (*ClusterStatus, error) {
	params := url.Values{}
	params.Set("fields", "status["+clusterStatusFields+"]")

	var cluster struct {
		Status ClusterStatus `json:"status"`
	}
	endpoint := fmt.Sprintf("/clusters/%d", id)
	if err := s.client.get(ctx, endpoint, params, &cluster); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Cluster", ID: id}
		}
		return nil, err
	}

	return &cluster.Status, nil
}

// Create creates a new cluster and returns the created cluster.
func (s *ClusterService) Create(ctx context.Context, req *ClusterCreateRequest) (*Cluster, error) {
	if req == nil {
		return nil, &ValidationError{Message: "create request is required"}
	}
	if req.Name == "" {
		return nil, &ValidationError{Field: "name", Message: "name is required"}
	}

	// Set defaults
	if req.Enabled == nil {
		enabled := true
		req.Enabled = &enabled
	}

	var resp apiResponse
	if err := s.client.post(ctx, "/clusters", req, &resp); err != nil {
		return nil, err
	}

	// Extract the created cluster's ID
	id, err := getKey(resp)
	if err != nil {
		return nil, err
	}

	// Read back the created cluster
	return s.Get(ctx, id)
}

// Update updates a cluster and returns the updated cluster.
func (s *ClusterService) Update(ctx context.Context, id int, req *ClusterUpdateRequest) (*Cluster, error) {
	if req == nil {
		return nil, &ValidationError{Message: "update request is required"}
	}

	endpoint := fmt.Sprintf("/clusters/%d", id)
	if err := s.client.put(ctx, endpoint, req, nil); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "Cluster", ID: id}
		}
		return nil, err
	}

	// Read back the updated cluster
	return s.Get(ctx, id)
}

// Delete deletes a cluster.
// Note: A cluster cannot be deleted if it still has nodes or machines referencing it.
func (s *ClusterService) Delete(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("/clusters/%d", id)
	if err := s.client.delete(ctx, endpoint); err != nil {
		if IsNotFoundError(err) {
			return nil // Already deleted
		}
		return err
	}
	return nil
}
