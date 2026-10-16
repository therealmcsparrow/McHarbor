// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"errors"
	"strings"
)

// maxDetailLength keeps a host message to something a toast can hold. The full
// text is logged either way.
const maxDetailLength = 400

// detailPrefixes are the layers a message collects on its way up from the host:
// the transport names the request it was answering, the agent's packages name
// themselves. None of that helps the person reading the toast.
var detailPrefixes = []string{
	"agent failed vm_start: ",
	"agent failed vm_stop: ",
	"agent failed vm_define: ",
	"agent failed vm_delete: ",
	"agent failed: ",
	"vm: ",
	"compute: ",
}

// hostErrorDetail turns an error from the host into the sentence to show the
// operator, or "" when there is nothing worth showing.
//
// This is deliberately not err.Error() verbatim: a message that begins with
// three package prefixes reads as a stack trace and gets ignored, and the one
// useful clause is usually at the end.
func hostErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	detail := strings.TrimSpace(err.Error())

	// Repeated because the layers nest: an agent error arrives wrapped by the
	// transport, and the agent wrapped it itself on the way out.
	for trimmed := true; trimmed; {
		trimmed = false
		for _, prefix := range detailPrefixes {
			if strings.HasPrefix(detail, prefix) {
				detail = strings.TrimSpace(strings.TrimPrefix(detail, prefix))
				trimmed = true
			}
		}
	}

	if detail == "" {
		return ""
	}
	if translated := translateKnownFailure(detail); translated != "" {
		return translated
	}
	if len(detail) > maxDetailLength {
		detail = detail[:maxDetailLength] + "…"
	}
	return strings.ToUpper(detail[:1]) + detail[1:]
}

// sentinelDetail returns the operator-facing sentence a service attached to
// sentinel with fmt.Errorf("%w: <sentence>"), or "" when err carries none.
//
// Only the part the service wrote is returned, so a wrapped transport or driver
// error further down the chain can never reach a client through this path.
func sentinelDetail(err error, sentinel error) string {
	if err == nil || !errors.Is(err, sentinel) {
		return ""
	}
	prefix := sentinel.Error() + ": "
	text := err.Error()
	index := strings.Index(text, prefix)
	if index < 0 {
		return ""
	}
	detail := strings.TrimSpace(text[index+len(prefix):])
	if detail == "" {
		return ""
	}
	if len(detail) > maxDetailLength {
		detail = detail[:maxDetailLength] + "…"
	}
	return strings.ToUpper(detail[:1]) + detail[1:]
}

// validationMarkers are phrases the service's untyped validation messages use.
var validationMarkers = []string{
	"is required", "is not a", "cannot contain", "cannot start",
	"too long", "at least", "cannot be below", "choose an image",
}

// isValidationError reports whether err is one of the service's plain-message
// validation failures, which are 400s rather than 502s. The text is only
// classified here, never sent to the client.
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, marker := range validationMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// knownFailures are QEMU messages worth replacing outright.
//
// Each of these describes a condition the operator can act on and describes it
// in terms of QEMU's internals. The image-locking one is the clearest case:
// what QEMU says is that it failed to get a write lock on a byte range, and
// what actually happened is that another guest already has the disk open.
// Turning off locking is never the answer, so the message has to name the real
// cause or the operator goes looking for the wrong fix.
var knownFailures = []struct {
	// match are substrings that must all be present, lowercased.
	match []string
	// replacement is the sentence shown instead.
	replacement string
}{
	{
		match: []string{"failed to get", "lock"},
		replacement: "This disk is already open by another guest. " +
			"Stop the guest using it, or give the disk shared access if both " +
			"guests run a cluster filesystem.",
	},
	{
		match:       []string{"is used by another process"},
		replacement: "This disk is already open by another process on the host.",
	},
	{
		match:       []string{"no space left on device"},
		replacement: "The storage pool behind this guest is full.",
	},
	{
		match: []string{"not enough", "huge"},
		replacement: "This host does not have enough free hugepages for this guest. " +
			"Reserve more on the host, or turn hugepages off for this machine.",
	},
	{
		match: []string{"could not open", "permission denied"},
		replacement: "The host is not allowed to open one of this guest's files. " +
			"Check the ownership of the pool directory.",
	},
}

// translateKnownFailure replaces a host message with a plainer one, or returns
// "" to leave it alone.
//
// Substring matching rather than error codes because these arrive as text from
// QEMU through two layers of wrapping, and QEMU's wording is the only stable
// thing about them — it has no error identifiers to match on.
func translateKnownFailure(detail string) string {
	lowered := strings.ToLower(detail)
	for _, failure := range knownFailures {
		matched := true
		for _, needle := range failure.match {
			if !strings.Contains(lowered, needle) {
				matched = false
				break
			}
		}
		if matched {
			return failure.replacement
		}
	}
	return ""
}
