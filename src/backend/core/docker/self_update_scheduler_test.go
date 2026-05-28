// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"strings"
	"testing"
)

func TestDataDirMountDestinationsIncludeContainerDefault(t *testing.T) {
	destinations := dataDirMountDestinations("./data")
	for _, destination := range destinations {
		if strings.ReplaceAll(destination, "\\", "/") == "/app/data" {
			return
		}
	}
	t.Fatalf("expected /app/data fallback in %#v", destinations)
}
