package fleetdm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CreateWebAppRequest is the payload for creating an Android web app (web clip).
//
// Title and URL are both required by Fleet's decoder; URL must be an absolute
// URL. Icon is optional and, when present, must be a square PNG of at least
// 512x512px — Fleet rejects anything else with a 400.
type CreateWebAppRequest struct {
	Title string
	URL   string

	// IconName is the multipart filename for the icon part. Only used when
	// Icon is non-empty; defaults to "icon.png".
	IconName string

	// Icon is the raw PNG bytes. Leave empty to create the web app without an
	// icon, in which case the request carries no file part at all.
	Icon []byte
}

// createWebAppResponse is the API response for creating an Android web app.
// Fleet returns only the generated package name, e.g.
// "com.google.enterprise.webapp.0123456789abcdef".
type createWebAppResponse struct {
	AppStoreID string `json:"app_store_id"`
}

// CreateWebApp creates an Android web app and returns its app store ID
// (package name).
//
// POST /api/v1/fleet/software/web_apps is a thin wrapper over Google's Android
// Management API: it registers the web app inside the Android enterprise and
// creates nothing in Fleet itself. Consequently there is no GET (Fleet 4.90
// answers 405), no list, and no DELETE for this path — the returned ID is meant
// to be fed to AddAppStoreApp with platform "android" to actually make the app
// installable on a team.
//
// The endpoint sits behind Fleet's VerifyAndroidMDM middleware, so it returns
// 400 "Android MDM isn't turned on." unless Android MDM is enabled and
// configured. It is also Fleet Premium only.
func (c *Client) CreateWebApp(ctx context.Context, req *CreateWebAppRequest) (string, error) {
	fields := map[string]string{
		"title": req.Title,
		"url":   req.URL,
	}

	var (
		body []byte
		err  error
	)
	if len(req.Icon) > 0 {
		iconName := req.IconName
		if iconName == "" {
			iconName = "icon.png"
		}
		body, err = c.doMultipartRequest(ctx, http.MethodPost, "/software/web_apps", "icon", iconName, req.Icon, fields)
	} else {
		// No icon: send a multipart body with text fields only. Fleet's
		// decoder treats a missing "icon" file part as "no icon".
		body, err = c.doMultipartFormRequest(ctx, http.MethodPost, "/software/web_apps", fields)
	}
	if err != nil {
		return "", fmt.Errorf("failed to create web app: %w", err)
	}

	var resp createWebAppResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse create web app response: %w", err)
	}
	if resp.AppStoreID == "" {
		return "", fmt.Errorf("create web app succeeded but app_store_id is empty")
	}

	return resp.AppStoreID, nil
}
