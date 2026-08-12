package fleetdm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// doMultipartRequest builds a multipart/form-data request with a single file part
// and optional text fields, executes it, and returns the raw response body.
// Callers are responsible for unmarshalling the response.
func (c *Client) doMultipartRequest(ctx context.Context, method, endpoint, fileField, fileName string, fileContent []byte, fields map[string]string) ([]byte, error) {
	return c.doMultipartRequestMulti(ctx, method, endpoint, fileField, fileName, fileContent, singleValueFields(fields))
}

// doMultipartRequestMulti is doMultipartRequest with repeated text fields:
// each value of a key is written as its own form field. Fleet's
// configuration-profile endpoints require this shape for label targeting
// (comma-joined values are rejected as a single unknown label name).
func (c *Client) doMultipartRequestMulti(ctx context.Context, method, endpoint, fileField, fileName string, fileContent []byte, fields map[string][]string) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	// Add the file part
	part, err := writer.CreateFormFile(fileField, fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := part.Write(fileContent); err != nil {
		return nil, fmt.Errorf("failed to write file content: %w", err)
	}

	if err := writeMultiFields(writer, fields); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return c.sendMultipart(ctx, method, endpoint, &body, writer.FormDataContentType())
}

// doMultipartFormRequest builds a multipart/form-data request with only text
// fields (no file part) and returns the raw response body. Use this for
// endpoints that require multipart/form-data but where the caller has no file
// to attach — Fleet's PATCH /software/titles/{id}/package supports this shape
// for metadata-only updates (scripts, labels, flags) and rejects
// application/json bodies outright. When that same endpoint is used to
// replace the installer binary in-place, the caller switches to
// doMultipartRequest with the "software" file field instead.
func (c *Client) doMultipartFormRequest(ctx context.Context, method, endpoint string, fields map[string]string) ([]byte, error) {
	return c.doMultipartFormRequestMulti(ctx, method, endpoint, singleValueFields(fields))
}

// doMultipartFormRequestMulti is doMultipartFormRequest with repeated text
// fields; see doMultipartRequestMulti.
func (c *Client) doMultipartFormRequestMulti(ctx context.Context, method, endpoint string, fields map[string][]string) ([]byte, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	if err := writeMultiFields(writer, fields); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	return c.sendMultipart(ctx, method, endpoint, &body, writer.FormDataContentType())
}

// singleValueFields converts a single-value field map to the multi-value
// shape used by the *Multi helpers.
func singleValueFields(fields map[string]string) map[string][]string {
	if fields == nil {
		return nil
	}
	multi := make(map[string][]string, len(fields))
	for k, v := range fields {
		multi[k] = []string{v}
	}
	return multi
}

// writeMultiFields writes each value of every key as its own form field.
func writeMultiFields(writer *multipart.Writer, fields map[string][]string) error {
	for k, values := range fields {
		for _, v := range values {
			if err := writer.WriteField(k, v); err != nil {
				return fmt.Errorf("failed to write field %s: %w", k, err)
			}
		}
	}
	return nil
}

// sendMultipart executes a multipart/form-data request whose body has already
// been built and closed. Shared by doMultipartRequest and doMultipartFormRequest.
func (c *Client) sendMultipart(ctx context.Context, method, endpoint string, body io.Reader, contentType string) ([]byte, error) {
	reqURL := c.BaseURL + endpoint
	httpReq, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	httpReq.Header.Set("Content-Type", contentType)
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.UserAgent)

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readResponseBody(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, newAPIError(resp.StatusCode, respBody)
	}

	return respBody, nil
}
