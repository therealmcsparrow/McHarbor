// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"errors"
	"fmt"
	"testing"

	core "github.com/therealmcsparrow/mcharbor/core/instancebackup"
)

// TestArtifactReasonNamesEachRefusal checks the reason key per sentinel, wrapped
// the way the artifact reader wraps them, and that a wrong key still wins over
// a damaged-file sentinel further down the chain.
func TestArtifactReasonNamesEachRefusal(t *testing.T) {
	wrap := func(sentinel error) error {
		return &core.ArtifactError{Err: fmt.Errorf("reading x: %w", fmt.Errorf("%w: detail", sentinel))}
	}
	cases := map[string]error{
		"wrong_key":    wrap(core.ErrWrongKey),
		"key_kind":     wrap(core.ErrKeyKind),
		"bad_key_file": wrap(core.ErrBadKeyFile),
		"not_export":   wrap(core.ErrNotExport),
		"format":       wrap(core.ErrUnsupportedFormat),
		"incomplete":   wrap(core.ErrIncomplete),
		"damaged":      wrap(core.ErrDamaged),
		"unreadable":   &core.ArtifactError{Err: errors.New("gzip: invalid header")},
	}
	for want, err := range cases {
		if got := artifactReason(err); got != want {
			t.Errorf("%v: got %q, want %q", err, got, want)
		}
	}
	both := &core.ArtifactError{Err: fmt.Errorf("%w: %w", core.ErrDamaged, core.ErrWrongKey)}
	if got := artifactReason(both); got != "wrong_key" {
		t.Errorf("a wrong key must outrank damage: got %q", got)
	}
}
