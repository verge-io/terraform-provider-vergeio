package vergeos

import (
	"context"
	"fmt"
	"net/url"
)

// StorageTierService handles storage tier read operations.
// Storage tiers provide system-wide VSAN storage information.
type StorageTierService struct {
	client *Client
}

// List returns all storage tiers, with optional filtering.
func (s *StorageTierService) List(ctx context.Context, opts ...ListOption) ([]StorageTier, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = storageTierListFields
	}

	params := options.toQueryParams()

	var tiers []StorageTier
	if err := s.client.get(ctx, "/storage_tiers", params, &tiers); err != nil {
		return nil, err
	}

	return tiers, nil
}

// Get returns a single storage tier by tier number (0-5).
func (s *StorageTierService) Get(ctx context.Context, tier int) (*StorageTier, error) {
	params := url.Values{}
	params.Set("fields", storageTierListFields)

	var storageTier StorageTier
	endpoint := fmt.Sprintf("/storage_tiers/%d", tier)
	if err := s.client.get(ctx, endpoint, params, &storageTier); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "StorageTier", ID: tier}
		}
		return nil, err
	}

	return &storageTier, nil
}

// ClusterTierService handles cluster tier read operations.
// Cluster tiers provide cluster-specific VSAN tier status and statistics.
type ClusterTierService struct {
	client *Client
}

// List returns all cluster tiers, with optional filtering.
func (s *ClusterTierService) List(ctx context.Context, opts ...ListOption) ([]ClusterTier, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = clusterTierListFields
	}

	params := options.toQueryParams()

	var tiers []ClusterTier
	if err := s.client.get(ctx, "/cluster_tiers", params, &tiers); err != nil {
		return nil, err
	}

	return tiers, nil
}

// ListByCluster returns all tiers for a specific cluster.
func (s *ClusterTierService) ListByCluster(ctx context.Context, clusterID int, opts ...ListOption) ([]ClusterTier, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("cluster eq %d", clusterID)))
	return s.List(ctx, opts...)
}

// Get returns a single cluster tier by ID.
func (s *ClusterTierService) Get(ctx context.Context, id int) (*ClusterTier, error) {
	params := url.Values{}
	params.Set("fields", clusterTierListFields)

	var tier ClusterTier
	endpoint := fmt.Sprintf("/cluster_tiers/%d", id)
	if err := s.client.get(ctx, endpoint, params, &tier); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "ClusterTier", ID: id}
		}
		return nil, err
	}

	return &tier, nil
}

// GetByClusterAndTier returns a specific tier for a cluster by tier number.
func (s *ClusterTierService) GetByClusterAndTier(ctx context.Context, clusterID int, tierNum int) (*ClusterTier, error) {
	tiers, err := s.List(ctx, WithFilter(fmt.Sprintf("cluster eq %d and tier eq %d", clusterID, tierNum)))
	if err != nil {
		return nil, err
	}

	if len(tiers) == 0 {
		return nil, &NotFoundError{Resource: "ClusterTier", ID: fmt.Sprintf("cluster=%d,tier=%d", clusterID, tierNum)}
	}

	return &tiers[0], nil
}

// MachineDrivePhysService handles machine drive physical status read operations.
// This service provides hardware-level drive information including temperature,
// wear level, and VSAN-specific metrics.
type MachineDrivePhysService struct {
	client *Client
}

// List returns all machine drive physical records, with optional filtering.
func (s *MachineDrivePhysService) List(ctx context.Context, opts ...ListOption) ([]MachineDrivePhys, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = machineDrivePhysListFields
	}

	params := options.toQueryParams()

	var drives []MachineDrivePhys
	if err := s.client.get(ctx, "/machine_drive_phys", params, &drives); err != nil {
		return nil, err
	}

	return drives, nil
}

// ListByVSANTier returns all drives in a specific VSAN tier.
func (s *MachineDrivePhysService) ListByVSANTier(ctx context.Context, tier int, opts ...ListOption) ([]MachineDrivePhys, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("vsan_tier eq %d", tier)))
	return s.List(ctx, opts...)
}

// ListSpares returns all VSAN spare drives.
func (s *MachineDrivePhysService) ListSpares(ctx context.Context, opts ...ListOption) ([]MachineDrivePhys, error) {
	opts = append(opts, WithFilter("spare eq true"))
	return s.List(ctx, opts...)
}

// ListWithWarnings returns drives with any warning flags set.
func (s *MachineDrivePhysService) ListWithWarnings(ctx context.Context, opts ...ListOption) ([]MachineDrivePhys, error) {
	opts = append(opts, WithFilter("temp_warn eq true or realloc_sectors_warn eq true or wear_level_warn eq true or hours_warn eq true or current_pending_sector_warn eq true or offline_uncorrectable_warn eq true"))
	return s.List(ctx, opts...)
}

// Get returns a single machine drive physical record by ID.
func (s *MachineDrivePhysService) Get(ctx context.Context, id int) (*MachineDrivePhys, error) {
	params := url.Values{}
	params.Set("fields", machineDrivePhysListFields)

	var drive MachineDrivePhys
	endpoint := fmt.Sprintf("/machine_drive_phys/%d", id)
	if err := s.client.get(ctx, endpoint, params, &drive); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "MachineDrivePhys", ID: id}
		}
		return nil, err
	}

	return &drive, nil
}

// GetByParentDrive returns the physical status for a specific machine drive.
func (s *MachineDrivePhysService) GetByParentDrive(ctx context.Context, driveID int) (*MachineDrivePhys, error) {
	drives, err := s.List(ctx, WithFilter(fmt.Sprintf("parent_drive eq %d", driveID)))
	if err != nil {
		return nil, err
	}

	if len(drives) == 0 {
		return nil, &NotFoundError{Resource: "MachineDrivePhys", ID: fmt.Sprintf("parent_drive=%d", driveID)}
	}

	return &drives[0], nil
}

// ClusterStatsHistoryService handles cluster statistics history read operations.
// This service provides historical metrics for cluster monitoring.
type ClusterStatsHistoryService struct {
	client *Client
}

// ListShort returns short-term historical stats for all clusters.
// Short-term history provides higher resolution data for recent timeframes.
func (s *ClusterStatsHistoryService) ListShort(ctx context.Context, opts ...ListOption) ([]ClusterStatsHistory, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = clusterStatsHistoryFields
	}

	params := options.toQueryParams()

	var stats []ClusterStatsHistory
	if err := s.client.get(ctx, "/cluster_stats_history_short", params, &stats); err != nil {
		return nil, err
	}

	return stats, nil
}

// ListShortByCluster returns short-term historical stats for a specific cluster.
func (s *ClusterStatsHistoryService) ListShortByCluster(ctx context.Context, clusterID int, opts ...ListOption) ([]ClusterStatsHistory, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("cluster eq %d", clusterID)))
	return s.ListShort(ctx, opts...)
}

// ListLong returns long-term historical stats for all clusters.
// Long-term history provides lower resolution data for longer timeframes.
func (s *ClusterStatsHistoryService) ListLong(ctx context.Context, opts ...ListOption) ([]ClusterStatsHistory, error) {
	options := applyListOptions(opts)

	if options.Fields == "most" {
		options.Fields = clusterStatsHistoryFields
	}

	params := options.toQueryParams()

	var stats []ClusterStatsHistory
	if err := s.client.get(ctx, "/cluster_stats_history_long", params, &stats); err != nil {
		return nil, err
	}

	return stats, nil
}

// ListLongByCluster returns long-term historical stats for a specific cluster.
func (s *ClusterStatsHistoryService) ListLongByCluster(ctx context.Context, clusterID int, opts ...ListOption) ([]ClusterStatsHistory, error) {
	opts = append(opts, WithFilter(fmt.Sprintf("cluster eq %d", clusterID)))
	return s.ListLong(ctx, opts...)
}

// GetLatestShort returns the most recent short-term stats record for a cluster.
func (s *ClusterStatsHistoryService) GetLatestShort(ctx context.Context, clusterID int) (*ClusterStatsHistory, error) {
	stats, err := s.ListShortByCluster(ctx, clusterID, WithSort("-timestamp"), WithLimit(1))
	if err != nil {
		return nil, err
	}

	if len(stats) == 0 {
		return nil, &NotFoundError{Resource: "ClusterStatsHistory", ID: fmt.Sprintf("cluster=%d", clusterID)}
	}

	return &stats[0], nil
}

// GetShort returns a single short-term stats record by ID.
func (s *ClusterStatsHistoryService) GetShort(ctx context.Context, id int) (*ClusterStatsHistory, error) {
	params := url.Values{}
	params.Set("fields", clusterStatsHistoryFields)

	var stats ClusterStatsHistory
	endpoint := fmt.Sprintf("/cluster_stats_history_short/%d", id)
	if err := s.client.get(ctx, endpoint, params, &stats); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "ClusterStatsHistory", ID: id}
		}
		return nil, err
	}

	return &stats, nil
}

// GetLong returns a single long-term stats record by ID.
func (s *ClusterStatsHistoryService) GetLong(ctx context.Context, id int) (*ClusterStatsHistory, error) {
	params := url.Values{}
	params.Set("fields", clusterStatsHistoryFields)

	var stats ClusterStatsHistory
	endpoint := fmt.Sprintf("/cluster_stats_history_long/%d", id)
	if err := s.client.get(ctx, endpoint, params, &stats); err != nil {
		if IsNotFoundError(err) {
			return nil, &NotFoundError{Resource: "ClusterStatsHistory", ID: id}
		}
		return nil, err
	}

	return &stats, nil
}
