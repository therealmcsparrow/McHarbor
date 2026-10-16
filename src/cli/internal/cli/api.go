// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/therealmcsparrow/mcharbor-cli/internal/client"
	"github.com/therealmcsparrow/mcharbor-cli/internal/permissions"
)

// API wraps the generated client with the decoding every call shares.
type API struct {
	c      *client.Client
	http   *http.Client
	target *Target
}

// envelope is the shape every McHarbor JSON response shares.
type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Message string          `json:"message"`
	Code    string          `json:"code"`
}

func newHTTPClient(t *Target) (*http.Client, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: t.Insecure}
	if t.CAFile != "" {
		pem, err := os.ReadFile(t.CAFile)
		if err != nil {
			return nil, usageError("reading CA file %s: %v", t.CAFile, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, usageError("%s contains no PEM certificate", t.CAFile)
		}
		tlsCfg.RootCAs = pool
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsCfg
	return &http.Client{Transport: transport}, nil
}

// newAPI builds a client for t. The key travels as a bearer header and is
// never logged or printed.
func newAPI(t *Target) (*API, error) {
	hc, err := newHTTPClient(t)
	if err != nil {
		return nil, err
	}
	base := strings.TrimRight(t.URL, "/")
	c, err := client.NewClient(base,
		client.WithHTTPClient(hc),
		client.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("User-Agent", "mcharbor-cli/"+Version)
			req.Header.Set("Accept", "application/json")
			if t.APIKey != "" {
				req.Header.Set("Authorization", "Bearer "+t.APIKey)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, usageError("invalid server URL %q: %v", t.URL, err)
	}
	return &API{c: c, http: hc, target: t}, nil
}

// env returns the environment selector for a scoped call, nil when unset so the
// server's default environment applies.
func (a *API) env() *string {
	if a.target.Env == "" {
		return nil
	}
	e := a.target.Env
	return &e
}

// do decodes one generated-client call. op is the operationId, used to name
// the permission a refused call needs.
func (a *API) do(op string, resp *http.Response, err error) (json.RawMessage, error) {
	if err != nil {
		return nil, transportError(op, err)
	}
	body, readErr := io.ReadAll(resp.Body)
	warnErr(resp.Body.Close())
	if readErr != nil {
		return nil, transportError(op, readErr)
	}
	if resp.StatusCode >= 400 {
		return nil, apiError(op, resp, body)
	}
	if len(bytes.TrimSpace(body)) == 0 || resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, &Error{Code: ExitRemote, Operation: op, Status: resp.StatusCode,
			Message: "the server answered with something that is not McHarbor JSON; is the URL a McHarbor instance?"}
	}
	if !env.Success && env.Data == nil && (env.Error != "" || env.Code != "") {
		return nil, apiError(op, resp, body)
	}
	return env.Data, nil
}

func apiError(op string, resp *http.Response, body []byte) *Error {
	e := &Error{Code: codeForStatus(resp.StatusCode), Operation: op, Status: resp.StatusCode,
		RequestID: resp.Header.Get("X-Request-Id")}
	var env envelope
	if json.Unmarshal(body, &env) == nil {
		e.Message = firstNonEmpty(env.Error, env.Message)
		e.ServerCode = env.Code
	}
	if e.Message == "" {
		e.Message = http.StatusText(resp.StatusCode)
	}
	if resp.StatusCode == http.StatusForbidden {
		e.Permissions = permissions.For(op)
		// A refusal that names its own key (host_self.manage on McHarbor's own containers)
		// is the one to grant; the route's static keys are ones the caller already holds.
		if key := deniedPermission(body); key != "" {
			e.Permissions = []string{key}
		}
	}
	if resp.StatusCode == http.StatusUnauthorized {
		e.Message += "; check the API key (`mcharbor login` or MCHARBOR_API_KEY)"
	}
	return e
}

func transportError(op string, err error) *Error {
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{Code: ExitTimeout, Operation: op, Message: "timed out waiting for the server"}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &Error{Code: ExitTimeout, Operation: op, Message: "timed out waiting for the server"}
	}
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) {
		return &Error{Code: ExitFailure, Operation: op,
			Message: "the server's certificate is not trusted: pass its root with --ca-file (Security → HTTPS in McHarbor), or --insecure for a lab"}
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return &Error{Code: ExitFailure, Operation: op, Message: fmt.Sprintf("cannot reach the server: %v", urlErr.Err)}
	}
	return &Error{Code: ExitFailure, Operation: op, Message: err.Error()}
}

// jsonBody encodes v for a WithBody call.
func jsonBody(v any) (io.Reader, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding request: %w", err)
	}
	return bytes.NewReader(raw), nil
}

// decodeAny decodes data keeping numbers exact, into the generic shape the
// renderer walks; unknown fields from a newer server are kept, not rejected.
func decodeAny(data json.RawMessage) (any, error) {
	if len(data) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return v, nil
}

// listItems returns the rows of a list response, unwrapping the paginated
// {items,total,...} shape and tolerating a bare array.
func listItems(v any) []map[string]any {
	if m, ok := v.(map[string]any); ok {
		if items, ok := m["items"]; ok {
			v = items
		} else if only, ok := soleArray(m); ok {
			// Several routes name their array something else — `imports`,
			// `schedules`, `channels`. Taking the one array in the object renders
			// those instead of an empty table, and is unambiguous by definition:
			// the moment a response carries two arrays this does nothing.
			v = only
		}
	}
	arr, _ := v.([]any)
	rows := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	return rows
}

// listItemsPreferring is listItems with hints for the array's key, used when the
// caller knows what the rows are called.
//
// It exists because a response with two arrays is ambiguous to soleArray and
// renders nothing: `GET /api/vms/{id}/snapshot-schedules` answers
// {"schedules": […], "runs": […]}, and a command at `snapshot schedule list`
// means the first of those.
func listItemsPreferring(v any, keys ...string) []map[string]any {
	if m, ok := v.(map[string]any); ok {
		if _, hasItems := m["items"]; !hasItems {
			for _, key := range keys {
				if arr, ok := m[key].([]any); ok {
					return listItems(arr)
				}
			}
		}
	}
	return listItems(v)
}

// soleArray returns the object's only array value, when it has exactly one.
func soleArray(m map[string]any) (any, bool) {
	var found any
	count := 0
	for _, v := range m {
		if _, ok := v.([]any); ok {
			found = v
			count++
		}
	}
	return found, count == 1
}

// rawGet issues a plain GET for the routes the generated client cannot model,
// such as the SSE log stream.
func (a *API) rawGet(ctx context.Context, path string, query url.Values, headers map[string]string) (*http.Response, error) {
	u := strings.TrimRight(a.target.URL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mcharbor-cli/"+Version)
	if a.target.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.target.APIKey)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return a.http.Do(req)
}

// pollInterval is how often a followed operation is re-read.
var pollInterval = 2 * time.Second

// rawRequest issues any method against an absolute API path, for the routes the
// generated client cannot model — every plugin-contributed command, whose route
// is known only at runtime.
func (a *API) rawRequest(ctx context.Context, method, path string, query url.Values, body []byte) (*http.Response, error) {
	u := strings.TrimRight(a.target.URL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mcharbor-cli/"+Version)
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", jsonContent)
	}
	if a.target.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.target.APIKey)
	}
	return a.http.Do(req)
}

// rawUpload streams a multipart body. The reader is consumed as it is sent
// rather than buffered, so a multi-gigabyte image does not have to fit in memory.
func (a *API) rawUpload(ctx context.Context, method, path string, query url.Values, contentType string, body io.Reader) (*http.Response, error) {
	u := strings.TrimRight(a.target.URL, "/") + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "mcharbor-cli/"+Version)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", contentType)
	if a.target.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.target.APIKey)
	}
	return a.http.Do(req)
}

// deniedPermission reads the permission a 403 body names in data.permission, if any.
func deniedPermission(body []byte) string {
	var env struct {
		Data struct {
			Permission string `json:"permission"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &env) != nil {
		return ""
	}
	return strings.TrimSpace(env.Data.Permission)
}
