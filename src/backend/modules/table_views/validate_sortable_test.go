// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package table_views

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLayoutKeepsAColumnsSortableChoice(t *testing.T) {
	raw := json.RawMessage(`{"v":1,"columns":[{"id":"name","visible":true,"size":"auto","sortable":false},{"id":"image","visible":true,"size":"auto"}],"density":"comfortable","pageSize":25,"autoRefreshMs":0}`)
	clean, err := validateLayout(raw)
	if err != nil {
		t.Fatalf("validateLayout refused a sortable choice: %v", err)
	}
	if !strings.Contains(string(clean), `"sortable":false`) {
		t.Fatalf("the sortable choice was dropped: %s", clean)
	}
	if strings.Count(string(clean), "sortable") != 1 {
		t.Fatalf("a column without a choice gained one: %s", clean)
	}
}
