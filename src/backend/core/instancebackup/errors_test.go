// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package instancebackup

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// TestRefusalsCarryTheirReason pins the sentinel each kind of bad input wraps.
// The HTTP layer answers with the reason alone, so a refusal that lost its
// sentinel would reach the operator as the generic "cannot be read".
func TestRefusalsCarryTheirReason(t *testing.T) {
	src := testSource(t)
	pp := Secret{Passphrase: passphrase}
	raw := export(t, src, ExportOptions{Secret: pp})

	cases := []struct {
		name string
		err  func() error
		want error
	}{
		{"not an export", func() error {
			_, err := ReadManifest(strings.NewReader("hello, world\n"))
			return err
		}, ErrNotExport},
		{"a newer format", func() error {
			_, err := ReadManifest(strings.NewReader("MCHARBOR-INSTANCE-EXPORT/9\n{}\n"))
			return err
		}, ErrUnsupportedFormat},
		{"an edited manifest", func() error {
			edited := bytes.Replace(raw, []byte(`"instanceId":"inst-1"`), []byte(`"instanceId":"inst-2"`), 1)
			_, err := Open(bytes.NewReader(edited), pp)
			return err
		}, ErrDamaged},
		{"a key file that is not one", func() error {
			_, err := parseKeyFile("not a key")
			return err
		}, ErrBadKeyFile},
	}
	for _, c := range cases {
		if err := c.err(); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want it to wrap %v", c.name, err, c.want)
		}
	}

	kf, err := GenerateKeyFile()
	if err != nil {
		t.Fatal(err)
	}
	sealed := export(t, src, ExportOptions{Secret: Secret{KeyFile: kf}})
	if _, err := Open(bytes.NewReader(sealed), pp); !errors.Is(err, ErrKeyKind) {
		t.Errorf("a passphrase against a key-file export: got %v, want ErrKeyKind", err)
	}

	for _, cut := range []int{len(raw) - 1, len(raw) / 2} {
		_, err := Verify(context.Background(), bytes.NewReader(raw[:cut]), pp)
		if !errors.Is(err, ErrIncomplete) && !errors.Is(err, ErrDamaged) {
			t.Errorf("an export cut at %d of %d: got %v, want ErrIncomplete or ErrDamaged", cut, len(raw), err)
		}
	}
}
