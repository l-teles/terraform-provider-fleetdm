package fleetdm

import (
	"context"
	"fmt"
	"strconv"
)

// patchDeadlineSettingsPath serves both GET and PATCH (Fleet Premium 4.95+).
const patchDeadlineSettingsPath = "/software/patch_deadline_settings"

// PatchDeadlineDays lists the weekdays Fleet accepts, in the week order it
// returns them.
var PatchDeadlineDays = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}

// Fleet's defaults for a fleet that has never had its settings changed.
const (
	DefaultPatchDeadlineDay  = "tuesday"
	DefaultPatchDeadlineTime = "17:00"
)

// PatchDeadlineSettings holds the weekdays and host-local HH:MM time when a
// fleet's patch deadlines fall. FleetID 0 is Unassigned.
type PatchDeadlineSettings struct {
	// FleetID has no omitempty: Fleet requires the key, and 0 is a valid value.
	FleetID int64    `json:"fleet_id"`
	Days    []string `json:"days"`
	Time    string   `json:"time"`
}

type patchDeadlineSettingsResponse struct {
	PatchDeadlineSettings *PatchDeadlineSettings `json:"patch_deadline_settings"`
}

// DefaultPatchDeadlineSettings returns Fleet's default settings for a fleet.
func DefaultPatchDeadlineSettings(fleetID int64) PatchDeadlineSettings {
	return PatchDeadlineSettings{FleetID: fleetID, Days: []string{DefaultPatchDeadlineDay}, Time: DefaultPatchDeadlineTime}
}

// GetPatchDeadlineSettings returns a fleet's patch deadline settings. Fleet
// answers 404 when the fleet does not exist.
func (c *Client) GetPatchDeadlineSettings(ctx context.Context, fleetID int64) (*PatchDeadlineSettings, error) {
	var response patchDeadlineSettingsResponse
	params := map[string]string{"fleet_id": strconv.FormatInt(fleetID, 10)}
	if err := c.Get(ctx, patchDeadlineSettingsPath, params, &response); err != nil {
		return nil, fmt.Errorf("failed to get patch deadline settings for fleet %d: %w", fleetID, err)
	}
	if response.PatchDeadlineSettings == nil {
		return nil, fmt.Errorf("failed to get patch deadline settings for fleet %d: response contained no settings", fleetID)
	}
	return response.PatchDeadlineSettings, nil
}

// UpdatePatchDeadlineSettings replaces a fleet's patch deadline settings and
// returns them as Fleet stored them (days in week order).
func (c *Client) UpdatePatchDeadlineSettings(ctx context.Context, settings PatchDeadlineSettings) (*PatchDeadlineSettings, error) {
	var response patchDeadlineSettingsResponse
	if err := c.Patch(ctx, patchDeadlineSettingsPath, settings, &response); err != nil {
		return nil, fmt.Errorf("failed to update patch deadline settings for fleet %d: %w", settings.FleetID, err)
	}
	if response.PatchDeadlineSettings == nil {
		return nil, fmt.Errorf("failed to update patch deadline settings for fleet %d: response contained no settings", settings.FleetID)
	}
	return response.PatchDeadlineSettings, nil
}
