// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package imagefetch

import (
	"embed"
	"fmt"
	"path"
	"sort"
)

// bundledKeyFiles are the distribution signing keys compiled into the agent.
//
//go:embed keys/*.asc
var bundledKeyFiles embed.FS

// PinnedFingerprints names every bundled key file and the primary-key fingerprint
// it must carry. A key file swapped for another key fails the test that reads this,
// so replacing a key is a reviewed change to this table, never a silent file drop.
var PinnedFingerprints = map[string]string{
	"ubuntu-cloud-images.asc": "D2EB44626FDDC30B513D5BB71A5D6C4C7DB87C81",
	"ubuntu-cdimage-2012.asc": "843938DF228D22F7B3742BC0D94AA3F0EFE21092",
	"fedora-41.asc":           "466CF2D8B60BC3057AA9453ED0622462E99D6AD1",
	"fedora-44.asc":           "36F612DCF27F7D1A48A835E4DBFCF71C6D9F90A6",
	"alpine-ncopa.asc":        "0482D84022F52DF1C4E7CD43293ACD0907D9495A",
	"alpine-cloud.asc":        "F26ADFADBAE702EF7AF637459DA7EF23BFFCDF22",
}

// BundledKeys returns the armored keys in a stable order. A file the pin table does
// not name is refused, because a key nobody reviewed must not be trusted by default.
func BundledKeys() ([][]byte, error) {
	entries, err := bundledKeyFiles.ReadDir("keys")
	if err != nil {
		return nil, fmt.Errorf("imagefetch: read bundled keys: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if _, pinned := PinnedFingerprints[entry.Name()]; !pinned {
			return nil, fmt.Errorf("imagefetch: bundled key %s has no pinned fingerprint", entry.Name())
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	keys := make([][]byte, 0, len(names))
	for _, name := range names {
		data, err := bundledKeyFiles.ReadFile(path.Join("keys", name))
		if err != nil {
			return nil, fmt.Errorf("imagefetch: read bundled key %s: %w", name, err)
		}
		keys = append(keys, data)
	}
	return keys, nil
}
