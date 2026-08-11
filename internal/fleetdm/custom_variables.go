package fleetdm

import (
	"context"
	"fmt"
	"strconv"
)

// customVariablesEndpoint is the CRUD collection endpoint for custom variables
// (Fleet's "secret variables"). Verified against Fleet v4.90.0:
//
//	POST   /custom_variables       create, body {name, value} -> {id, name}
//	GET    /custom_variables       list, entries omit the value
//	DELETE /custom_variables/{id}  delete by numeric id
//
// There is no GET-by-id and no PATCH/PUT on this path (all return 405).
const customVariablesEndpoint = "/custom_variables"

// customVariablesSpecEndpoint is the spec (upsert) endpoint. `PUT` with a list
// of {name, value} pairs upserts *only* the listed names and leaves every other
// custom variable untouched — verified on Fleet v4.90.0 — so it is safe to use
// as a single-resource in-place update path. Unlike the CRUD create endpoint it
// strips a leading "FLEET_SECRET_" from the supplied name.
const customVariablesSpecEndpoint = "/spec/secret_variables"

// customVariablesPageSize is the page size used when listing. Fleet v4.90.0
// returns every custom variable when no pagination parameters are supplied, but
// the endpoint does support page/per_page, so listing pages explicitly keeps the
// client correct if a future Fleet release introduces a default page cap.
const customVariablesPageSize = 500

// customVariablesMaxPages bounds the pagination loop so a server that always
// reports "has next results" cannot spin forever.
const customVariablesMaxPages = 1000

// CustomVariable represents a Fleet custom variable (internally a "secret
// variable"). Fleet never returns the variable's value from any endpoint, so
// there is deliberately no Value field here.
type CustomVariable struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// ListCustomVariablesResponse represents the response from the list custom
// variables endpoint.
type ListCustomVariablesResponse struct {
	CustomVariables []CustomVariable `json:"custom_variables"`
	Meta            *PaginationMeta  `json:"meta,omitempty"`
	Count           int              `json:"count,omitempty"`
}

// CreateCustomVariableRequest represents the request body for creating a custom
// variable.
type CreateCustomVariableRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// CreateCustomVariableResponse represents the response from the create custom
// variable endpoint. Fleet echoes only the new id and the name — never the
// value.
type CreateCustomVariableResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// CustomVariableSpec is a single name/value pair in an upsert request.
type CustomVariableSpec struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// UpsertCustomVariablesRequest represents the request body for the custom
// variables spec endpoint.
type UpsertCustomVariablesRequest struct {
	Secrets []CustomVariableSpec `json:"secrets"`
}

// CreateCustomVariable creates a custom variable and returns its server-assigned
// id and name. The value is write-only: Fleet stores it encrypted with the
// server private key and never returns it again.
//
// Fleet responds 409 if the name already exists, and 422 if the name does not
// match Fleet's format rules or the value is empty.
func (c *Client) CreateCustomVariable(ctx context.Context, name, value string) (*CustomVariable, error) {
	var resp CreateCustomVariableResponse
	err := c.Post(ctx, customVariablesEndpoint, CreateCustomVariableRequest{
		Name:  name,
		Value: value,
	}, &resp)
	if err != nil {
		return nil, fmt.Errorf("failed to create custom variable %q: %w", name, err)
	}
	return &CustomVariable{ID: resp.ID, Name: resp.Name}, nil
}

// ListCustomVariables retrieves every custom variable. Entries carry the id,
// name and timestamps only — Fleet does not return values.
func (c *Client) ListCustomVariables(ctx context.Context) ([]CustomVariable, error) {
	var all []CustomVariable

	for page := 0; page < customVariablesMaxPages; page++ {
		var resp ListCustomVariablesResponse
		params := map[string]string{
			"page":     strconv.Itoa(page),
			"per_page": strconv.Itoa(customVariablesPageSize),
		}
		if err := c.Get(ctx, customVariablesEndpoint, params, &resp); err != nil {
			return nil, fmt.Errorf("failed to list custom variables: %w", err)
		}

		all = append(all, resp.CustomVariables...)

		// Stop on the last page, on an empty page (defensive: a server that
		// reports "has next" forever would otherwise loop), or when the server
		// omits pagination metadata entirely.
		if len(resp.CustomVariables) == 0 || resp.Meta == nil || !resp.Meta.HasNextResults {
			return all, nil
		}
	}

	return nil, fmt.Errorf("failed to list custom variables: pagination did not terminate after %d pages", customVariablesMaxPages)
}

// FindCustomVariableByName looks a custom variable up by name. Fleet exposes no
// GET-by-id or GET-by-name route for custom variables, so the only way to read
// one back is to list and match. Returns (nil, nil) when no variable with that
// name exists.
func (c *Client) FindCustomVariableByName(ctx context.Context, name string) (*CustomVariable, error) {
	vars, err := c.ListCustomVariables(ctx)
	if err != nil {
		return nil, err
	}
	for i := range vars {
		if vars[i].Name == name {
			return &vars[i], nil
		}
	}
	return nil, nil
}

// UpsertCustomVariable sets the value of a custom variable, creating it if it
// does not exist. Only the named variable is affected; other custom variables
// are left untouched.
//
// Fleet has no PATCH/PUT on /custom_variables/{id}, so this is the only
// in-place value-rotation path. It matters because Fleet refuses (409) to
// delete a custom variable that is still referenced by a script or
// configuration profile, which rules out destroy-and-recreate for any variable
// actually in use.
func (c *Client) UpsertCustomVariable(ctx context.Context, name, value string) error {
	body := UpsertCustomVariablesRequest{
		Secrets: []CustomVariableSpec{{Name: name, Value: value}},
	}
	if err := c.Put(ctx, customVariablesSpecEndpoint, body, nil); err != nil {
		return fmt.Errorf("failed to update custom variable %q: %w", name, err)
	}
	return nil
}

// DeleteCustomVariable deletes a custom variable by id.
//
// Fleet responds 409 when the variable is still referenced by a script, an
// Apple profile, an Apple declaration or a Windows profile; the error message
// names the referencing entity.
func (c *Client) DeleteCustomVariable(ctx context.Context, id int) error {
	endpoint := fmt.Sprintf("%s/%d", customVariablesEndpoint, id)
	if err := c.Delete(ctx, endpoint, nil, nil); err != nil {
		return fmt.Errorf("failed to delete custom variable %d: %w", id, err)
	}
	return nil
}
