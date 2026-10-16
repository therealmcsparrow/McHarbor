// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"

	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/client"
	"github.com/therealmcsparrow/terraform-provider-mcharbor/internal/permissions"
)

// apiClient is what Configure hands every resource and data source.
type apiClient struct {
	api *client.Client
}

// envelope is the shape every McHarbor JSON response shares.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Message string          `json:"message"`
	Code    string          `json:"code"`
}

// APIError is a refused request, carrying the permission keys the operation
// needs so a 403 tells the operator exactly what to grant.
type APIError struct {
	// Operation is the operationId from the McHarbor OpenAPI document.
	Operation string
	// Status is the HTTP status the server answered with.
	Status int
	// Message is the server's own, localised explanation.
	Message string
	// Code is the server's i18n error code.
	Code string
	// Permissions lists the keys the operation's route enforces.
	Permissions []string
}

// Error renders the refusal as one sentence an operator can act on.
func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	out := fmt.Sprintf("McHarbor refused %s (HTTP %d): %s", e.Operation, e.Status, msg)
	if e.Code != "" {
		out += " [" + e.Code + "]"
	}
	if e.Status == http.StatusForbidden && len(e.Permissions) > 0 {
		out += ". The API key needs the permission " + strings.Join(e.Permissions, ", ")
	}
	return out
}

// isNotFound reports whether err is the server saying the object is gone,
// which Read turns into removal from state rather than an error.
func isNotFound(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound
}

// call pairs a generated client method's two results so they can be passed
// to decode alongside the operation name in one expression.
type call struct {
	resp *http.Response
	err  error
}

// apiCall captures a generated client call for decode.
func apiCall(resp *http.Response, err error) call { return call{resp: resp, err: err} }

// decode reads one response, enforces the envelope, and unmarshals data into
// out. It is the only place a response body is interpreted.
func decode(c call, operation string, out any) error {
	resp, callErr := c.resp, c.err
	if callErr != nil {
		return fmt.Errorf("calling McHarbor %s: %w", operation, callErr)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading McHarbor %s response: %w", operation, err)
	}

	var env envelope
	parseErr := json.Unmarshal(body, &env)

	if resp.StatusCode < 200 || resp.StatusCode > 299 || (parseErr == nil && !env.Success && len(body) > 0) {
		apiErr := &APIError{
			Operation:   operation,
			Status:      resp.StatusCode,
			Message:     env.Error,
			Code:        env.Code,
			Permissions: permissions.For(operation),
		}
		if apiErr.Message == "" {
			apiErr.Message = env.Message
		}
		return apiErr
	}
	if resp.StatusCode == http.StatusNoContent || len(body) == 0 || out == nil {
		return nil
	}
	if parseErr != nil {
		return fmt.Errorf("McHarbor %s answered with a body that is not the JSON envelope: %w", operation, parseErr)
	}
	if len(env.Data) == 0 || string(env.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("decoding McHarbor %s data: %w", operation, err)
	}
	return nil
}

// withQuery adds a query parameter the spec does not declare on a route, such
// as the environment selector a stack delete reads.
func withQuery(key, value string) client.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if value == "" {
			return nil
		}
		q := req.URL.Query()
		q.Set(key, value)
		req.URL.RawQuery = q.Encode()
		return nil
	}
}

// addError records err as a diagnostic under summary.
func addError(diags *diag.Diagnostics, summary string, err error) {
	diags.AddError(summary, err.Error())
}

// rawData keeps a data payload undecoded so it can be read into more than one
// struct, as the flat API key create result needs.
type rawData []byte

// UnmarshalJSON stores the payload verbatim.
func (r *rawData) UnmarshalJSON(b []byte) error {
	*r = append((*r)[:0], b...)
	return nil
}

func (r rawData) into(outs ...any) error {
	for _, out := range outs {
		if err := json.Unmarshal(r, out); err != nil {
			return err
		}
	}
	return nil
}
