package fleetdm

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
)

// Fleet's setup-experience-software endpoint controls which software titles
// get installed automatically during macOS Setup Assistant for a given
// team + platform. It is a separate concept from the policy-based
// `automatic_install` field on the Add Package endpoint:
//
//   * automatic_install (policy)  — creates a Fleet policy that installs the
//                                   software on hosts missing it; fires on
//                                   any host that fails the policy. Settable
//                                   only at Create time via Add Package /
//                                   Add Fleet Maintained App body.
//   * install_during_setup        — flags the title for installation during
//                                   the device's first-time setup flow.
//                                   Managed exclusively via the endpoints
//                                   in this file.
//
// The "install_during_setup" attribute exposed by the provider's software
// resources maps to this endpoint, NOT to the policy-based field.

// setupExperienceSoftwareTitle is one entry in the response to
// GET /setup_experience/software.
type setupExperienceSoftwareTitle struct {
	ID int `json:"id"`
}

// listSetupExperienceSoftwareResponse is the response wire shape.
type listSetupExperienceSoftwareResponse struct {
	SoftwareTitles []setupExperienceSoftwareTitle `json:"software_titles"`
}

// setSetupExperienceSoftwareRequest is the PUT body — a replace-the-whole-list
// payload. Omitting a title id removes it from the setup-experience set.
type setSetupExperienceSoftwareRequest struct {
	SoftwareTitleIDs []int `json:"software_title_ids"`
}

// GetSetupExperienceSoftware returns the set of software title IDs that are
// currently flagged for install during setup for the given team + platform.
// teamID = nil means the "Unassigned" team scope (Fleet's default when no
// team_id query param is provided).
func (c *Client) GetSetupExperienceSoftware(ctx context.Context, teamID *int, platform string) ([]int, error) {
	params := map[string]string{}
	if teamID != nil {
		params["team_id"] = strconv.Itoa(*teamID)
	}
	if platform != "" {
		params["platform"] = platform
	}

	var resp listSetupExperienceSoftwareResponse
	if err := c.Get(ctx, "/setup_experience/software", params, &resp); err != nil {
		return nil, fmt.Errorf("failed to list setup-experience software: %w", err)
	}

	ids := make([]int, 0, len(resp.SoftwareTitles))
	for _, t := range resp.SoftwareTitles {
		ids = append(ids, t.ID)
	}
	return ids, nil
}

// putSetupExperienceSoftware replaces the entire set of setup-experience
// software for the given team + platform. The caller must already hold the
// per-(team, platform) mutex; this helper is unexported and not
// concurrency-safe on its own.
func (c *Client) putSetupExperienceSoftware(ctx context.Context, teamID *int, platform string, titleIDs []int) error {
	endpoint := "/setup_experience/software"
	// Build the query string via net/url.Values so platform (HCL-supplied)
	// is properly escaped — raw string concatenation would mangle the URL
	// if platform contained `&`, `?`, `#`, spaces, or non-ASCII bytes, and
	// would invite SAST findings for "string-built URL with user input".
	q := url.Values{}
	if teamID != nil {
		q.Set("team_id", strconv.Itoa(*teamID))
	}
	if platform != "" {
		q.Set("platform", platform)
	}
	if enc := q.Encode(); enc != "" {
		endpoint += "?" + enc
	}

	body := setSetupExperienceSoftwareRequest{SoftwareTitleIDs: titleIDs}
	// Fleet's API documents PUT for this endpoint. Route through
	// doRequest to share the standard auth + error-handling rather than
	// the Patch/Get/Post convenience helpers which assume those methods.
	if err := c.doRequest(ctx, "PUT", endpoint, body, nil); err != nil {
		return fmt.Errorf("failed to put setup-experience software: %w", err)
	}
	return nil
}

// setupExperienceMutex returns (and lazily creates) the *sync.Mutex
// associated with the (teamID, platform) tuple on this Client. Calls to
// SetSetupExperienceSoftwareInclude / Exclude serialize on this mutex so
// concurrent read-modify-write operations don't lose updates against the
// replace-the-whole-list PUT semantics of Fleet's setup-experience
// endpoint.
func (c *Client) setupExperienceMutex(teamID *int, platform string) *sync.Mutex {
	var key string
	if teamID == nil {
		key = "nil|" + platform
	} else {
		key = strconv.Itoa(*teamID) + "|" + platform
	}
	if existing, ok := c.setupExperienceMu.Load(key); ok {
		return existing.(*sync.Mutex)
	}
	m := &sync.Mutex{}
	actual, _ := c.setupExperienceMu.LoadOrStore(key, m)
	return actual.(*sync.Mutex)
}

// SetSetupExperienceSoftwareInclude adds titleID to the setup-experience
// set for the given team + platform. Idempotent — calling with a titleID
// already in the set is a no-op (and skips the PUT).
//
// Read-modify-write is serialized per-(team, platform) via a mutex on the
// Client instance so two Terraform resources flipping install_during_setup
// in the same apply don't clobber each other.
//
// Before the PUT, stale ids (titles that 404 on GET /software/titles/{id})
// are filtered out of the read-modify-write payload — Fleet rejects the PUT
// with HTTP 400 ("at least one selected software title does not exist or is
// not available for setup experience") if any id in the request is unknown,
// and that can happen when a parallel apply deleted a sibling title between
// our GET and PUT.
func (c *Client) SetSetupExperienceSoftwareInclude(ctx context.Context, teamID *int, platform string, titleID int) error {
	mu := c.setupExperienceMutex(teamID, platform)
	mu.Lock()
	defer mu.Unlock()

	current, err := c.GetSetupExperienceSoftware(ctx, teamID, platform)
	if err != nil {
		return err
	}
	for _, id := range current {
		if id == titleID {
			return nil // already included
		}
	}
	live, err := c.filterStaleSetupExperienceTitleIDs(ctx, current, teamID)
	if err != nil {
		return err
	}
	return c.putSetupExperienceSoftware(ctx, teamID, platform, append(live, titleID))
}

// SetSetupExperienceSoftwareExclude removes titleID from the
// setup-experience set for the given team + platform. Idempotent — if
// titleID isn't in the set, returns nil without calling PUT.
//
// Same per-(team, platform) mutex as the Include path; concurrent
// includes/excludes serialize. Same stale-id filter as Include — see that
// method's doc comment for the rationale.
func (c *Client) SetSetupExperienceSoftwareExclude(ctx context.Context, teamID *int, platform string, titleID int) error {
	mu := c.setupExperienceMutex(teamID, platform)
	mu.Lock()
	defer mu.Unlock()

	current, err := c.GetSetupExperienceSoftware(ctx, teamID, platform)
	if err != nil {
		return err
	}
	filtered := make([]int, 0, len(current))
	found := false
	for _, id := range current {
		if id == titleID {
			found = true
			continue
		}
		filtered = append(filtered, id)
	}
	if !found {
		return nil
	}
	live, err := c.filterStaleSetupExperienceTitleIDs(ctx, filtered, teamID)
	if err != nil {
		return err
	}
	return c.putSetupExperienceSoftware(ctx, teamID, platform, live)
}

// filterStaleSetupExperienceTitleIDs returns ids unchanged in order,
// dropping any whose GET /software/titles/{id} returns 404. Any other
// error (network, 5xx, auth) propagates — we'd rather fail the apply
// than silently drop an id we can't classify.
//
// Skipping the GET when len(ids)==0 keeps this a true no-op when the
// current setup-experience set is empty, matching the previous behavior.
func (c *Client) filterStaleSetupExperienceTitleIDs(ctx context.Context, ids []int, teamID *int) ([]int, error) {
	if len(ids) == 0 {
		return ids, nil
	}
	live := make([]int, 0, len(ids))
	for _, id := range ids {
		if _, err := c.GetSoftwareTitle(ctx, id, teamID); err != nil {
			var apiErr *APIError
			if errors.As(err, &apiErr) && apiErr.StatusCode == 404 {
				continue
			}
			return nil, fmt.Errorf("validate setup-experience title %d before PUT: %w", id, err)
		}
		live = append(live, id)
	}
	return live, nil
}
