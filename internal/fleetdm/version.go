package fleetdm

import (
	"context"
	"fmt"
)

// VersionInfo represents FleetDM server version information.
type VersionInfo struct {
	Version   string `json:"version"`
	Branch    string `json:"branch"`
	Revision  string `json:"revision"`
	GoVersion string `json:"go_version"`
	BuildDate string `json:"build_date"`
	BuildUser string `json:"build_user"`
}

// GetVersion retrieves the version information from the FleetDM server.
func (c *Client) GetVersion(ctx context.Context) (*VersionInfo, error) {
	var version VersionInfo
	err := c.Get(ctx, "/version", nil, &version)
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}
	return &version, nil
}
