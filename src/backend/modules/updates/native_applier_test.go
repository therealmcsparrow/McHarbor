// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package updates

import (
	"context"
	"errors"
	"testing"

	"github.com/therealmcsparrow/mcharbor/core/nativeupdate"
	"github.com/therealmcsparrow/mcharbor/core/updatechannel"
)

type fakeNative struct {
	versions []string
	err      error
}

func (f *fakeNative) Apply(_ context.Context, version string) (nativeupdate.Result, error) {
	f.versions = append(f.versions, version)
	return nativeupdate.Result{Version: version}, f.err
}

// TestNativeApplierNamesTheVersion: a prerelease auto-install asks for that tag, never latest,
// which on a prerelease host is the prior stable.
func TestNativeApplierNamesTheVersion(t *testing.T) {
	native := &fakeNative{}
	f := newInstallFixture(t, "", updatechannel.Patch{McHarborPrereleases: tr(), McHarborAutoPrereleases: tr()})
	f.c.SetApplier(nativeApplier(native), nil)
	if ok, _ := f.run(t, "2026.11.02-rc1"); !ok {
		t.Fatal("the prerelease was not installed")
	}
	if len(native.versions) != 1 || native.versions[0] != "2026.11.02-rc1" {
		t.Fatalf("native install asked for %v, want exactly 2026.11.02-rc1", native.versions)
	}
}

// TestNativeApplierReportsTheFailure: the error reaches AutoInstall, which records it.
func TestNativeApplierReportsTheFailure(t *testing.T) {
	boom := errors.New("boom")
	got := nativeApplier(&fakeNative{err: boom})(context.Background(), "2026.11.02")
	if !errors.Is(got, boom) {
		t.Fatalf("applier returned %v, want the install error", got)
	}
}
