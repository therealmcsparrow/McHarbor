// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureRootfsCountsAllocatedSpaceAndHardLinksOnce(t *testing.T) {
	root := t.TempDir()
	must(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
	body := make([]byte, 256<<10)
	for i := range body {
		body[i] = byte(i)
	}
	must(t, os.WriteFile(filepath.Join(root, "etc", "data"), body, 0o644))
	empty, err := measureRootfs(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if empty < uint64(len(body)) {
		t.Fatalf("a rootfs holding %d bytes measured %d", len(body), empty)
	}

	must(t, os.Link(filepath.Join(root, "etc", "data"), filepath.Join(root, "etc", "data-link")))
	linked, err := measureRootfs(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if linked != empty {
		t.Fatalf("a hard link was counted twice: %d, then %d", empty, linked)
	}

	must(t, os.WriteFile(filepath.Join(root, "sparse"), nil, 0o644))
	must(t, os.Truncate(filepath.Join(root, "sparse"), 1<<30))
	sparse, err := measureRootfs(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if sparse >= 1<<30 {
		t.Fatalf("a sparse file was charged its apparent size: %d", sparse)
	}
}
