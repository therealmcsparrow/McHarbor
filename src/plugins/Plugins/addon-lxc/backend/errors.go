// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "strings"

// maxDetailLength keeps a host message to something a toast can hold. The full
// text is logged either way.
const maxDetailLength = 400

// hostErrorDetail turns an error into the sentence to show the operator, or ""
// when there is nothing worth showing.
//
// Deliberately not the raw error text: a message that arrives wrapped by the
// transport and again by the agent's own packages reads as a stack trace and gets
// ignored, and the one useful clause is at the end.
func hostErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	detail := strings.TrimSpace(err.Error())
	for trimmed := true; trimmed; {
		trimmed = false
		for _, prefix := range []string{
			"agent failed ct_backup_run: ", "agent failed ct_backup_restore: ",
			"agent failed: ", "compute: ", "lxc: ",
		} {
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
