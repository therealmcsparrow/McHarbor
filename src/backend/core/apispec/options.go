// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package apispec

import (
	"strings"
	"unicode"
)

// Summary sets the one-line description shown against the operation.
func Summary(s string) Option { return func(op *Operation) { op.Summary = s } }

// Describe sets the longer prose shown in the rendered reference.
func Describe(s string) Option { return func(op *Operation) { op.Description = s } }

// Tags overrides the module-derived grouping for operations that belong
// somewhere other than the directory their handler lives in.
func Tags(tags ...string) Option { return func(op *Operation) { op.Tags = tags } }

// Request declares the decoded request body by example, so the schema is
// reflected from a type that already exists rather than written out again.
func Request(v any) Option { return func(op *Operation) { op.Request = v } }

// Response declares the payload carried inside the success envelope.
func Response(v any) Option { return func(op *Operation) { op.Response = v } }

// Query declares a query parameter the handler reads.
func Query(name, description string) Option {
	return func(op *Operation) {
		op.Query = append(op.Query, Param{Name: name, Description: description})
	}
}

// RequiredQuery declares a query parameter the handler rejects the request without.
func RequiredQuery(name, description string) Option {
	return func(op *Operation) {
		op.Query = append(op.Query, Param{Name: name, Description: description, Required: true})
	}
}

// Paginated declares the shared page and limit parameters that every list
// endpoint honours through response.ParsePagination.
func Paginated() Option {
	return func(op *Operation) {
		op.Query = append(op.Query,
			Param{Name: "page", Description: "1-based page number.", Example: "1"},
			Param{Name: "per_page", Description: "Rows per page; default 25, maximum 1000.", Example: "25"},
		)
	}
}

// Scoped declares the environment selector that scoped routes read via
// response.ParseEnvID, which is also what an environment-scoped permission
// resolves against.
func Scoped() Option {
	return func(op *Operation) {
		op.Query = append(op.Query, Param{Name: "env", Description: "Environment id the request applies to."})
	}
}

// SSE marks an operation that streams text/event-stream until the client leaves.
func SSE() Option { return func(op *Operation) { op.Stream = StreamSSE } }

// WebSocket marks an operation that upgrades; the spec describes the handshake
// only, because OpenAPI cannot express what follows it.
func WebSocket() Option { return func(op *Operation) { op.Stream = StreamWebSocket } }

// Binary marks an operation whose body is an open-ended byte stream.
func Binary() Option { return func(op *Operation) { op.Stream = StreamBinary } }

// Text marks an operation that answers text/plain instead of the JSON envelope.
func Text() Option { return func(op *Operation) { op.Stream = StreamText } }

// Deprecated marks an operation that still works and should not be adopted.
func Deprecated() Option { return func(op *Operation) { op.Deprecated = true } }

// Exempt records why a mutating route carries no permission gate, which is the
// difference between a deliberate decision and the omission that looks the same.
func Exempt(reason string) Option { return func(op *Operation) { op.Exempt = reason } }

// summaryFromHandler turns HandleListContainers into "List containers", so an
// un-annotated route still reads as prose in the generated reference.
func summaryFromHandler(handler string) string {
	name := handler
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	name = trimHandlePrefix(name)
	if name == "" {
		return ""
	}

	var words []string
	var cur strings.Builder
	for i, r := range name {
		if unicode.IsUpper(r) && i > 0 && cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}
	if len(words) == 0 {
		return ""
	}

	for i := 1; i < len(words); i++ {
		if !isAcronym(words[i]) {
			words[i] = strings.ToLower(words[i])
		}
	}
	return strings.Join(words, " ")
}

// isAcronym keeps ID, WS, SSE, URL and friends upright in a generated summary.
func isAcronym(w string) bool {
	if len(w) < 2 {
		return false
	}
	for _, r := range w {
		if !unicode.IsUpper(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
