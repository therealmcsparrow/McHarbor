// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "strings"

// maxDetailLength keeps a host message to something a toast can hold. The full
// text is logged either way.
const maxDetailLength = 400

// detailPrefixes are the layers a message collects on its way up from the host:
// the transport names the request it was answering, the agent's packages name
// themselves. None of that helps the person reading the toast.
var detailPrefixes = []string{
	"agent failed image_fetch: ",
	"agent failed probe: ",
	"agent failed: ",
	"compute: ",
	"imagefetch: ",
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
	if len(detail) > maxDetailLength {
		detail = detail[:maxDetailLength] + "…"
	}
	return strings.ToUpper(detail[:1]) + detail[1:]
}
