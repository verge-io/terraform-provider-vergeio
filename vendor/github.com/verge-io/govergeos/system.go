package vergeos

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// SystemService handles system information operations.
type SystemService struct {
	client *Client
}

// GetInfo returns system version information.
// This uses the /version.json endpoint which is outside the standard API path.
func (s *SystemService) GetInfo(ctx context.Context) (*SystemInfo, error) {
	// Build URL for version.json (not under /api/v4/)
	u := fmt.Sprintf("%s/version.json", s.client.baseURL)

	req, err := s.client.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("vergeos: failed to get version info: %w", err)
	}
	defer req.Body.Close()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("vergeos: failed to read version response: %w", err)
	}

	var info SystemInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, fmt.Errorf("vergeos: failed to decode version info: %w", err)
	}

	return &info, nil
}

// GetVersion returns the VergeOS version string.
func (s *SystemService) GetVersion(ctx context.Context) (string, error) {
	info, err := s.GetInfo(ctx)
	if err != nil {
		return "", err
	}
	return info.Version, nil
}
