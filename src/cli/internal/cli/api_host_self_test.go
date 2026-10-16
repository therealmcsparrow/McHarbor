// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

// TestRefusalNamesTheServersPermission: a host_self refusal on McHarbor's own container is
// answered to a caller who already holds the route's keys, so the CLI must print the key the
// server names rather than the route's own.
func TestRefusalNamesTheServersPermission(t *testing.T) {
	body := []byte(`{"success":false,"error":"This container belongs to McHarbor itself. Acting on it needs the host_self.manage permission as well as the container permission","code":"err.host_self.container_required","data":{"permission":"host_self.manage","target":"app"}}`)
	e := apiError("containerStop", &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}, body)
	msg := e.Error()
	if len(e.Permissions) != 1 || e.Permissions[0] != "host_self.manage" {
		t.Fatalf("permissions = %v, want [host_self.manage]", e.Permissions)
	}
	for _, want := range []string{"belongs to McHarbor itself", "[err.host_self.container_required]", "missing permission: host_self.manage"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not carry %q", msg, want)
		}
	}
}
