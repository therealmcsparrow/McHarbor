// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package pluginruntime

import (
	"errors"
	"fmt"
)

// A plugin's version is written in four places — its appstore_plugins row, its
// <slug>@<version> install directory, its plugin.yaml and its .so Manifest() — and the
// loader refuses unless all four agree. Each sentinel names which pair disagreed.
var (
	// ErrInstallDirMissing means the installed row names a version with no install directory.
	ErrInstallDirMissing = errors.New("plugin install directory missing for installed version")
	// ErrRowVersionMismatch means the directory being loaded is not the version the row installed.
	ErrRowVersionMismatch = errors.New("installed row version does not match install directory")
	// ErrManifestVersionMismatch means plugin.yaml disagrees with its install directory.
	ErrManifestVersionMismatch = errors.New("plugin.yaml version does not match install directory")
	// ErrSymbolVersionMismatch means the built .so reports a different version than plugin.yaml,
	// which is what a source-only version bump produces.
	ErrSymbolVersionMismatch = errors.New("plugin symbol version does not match plugin.yaml")
)

// versionMismatch keeps the loader's long-standing message text while letting callers
// match the kind with errors.Is.
type versionMismatch struct {
	kind error
	msg  string
}

func (e *versionMismatch) Error() string { return e.msg }

func (e *versionMismatch) Unwrap() error { return e.kind }

// checkVersionAgreement compares the four copies of a plugin's version in the order the
// loader learns them; an empty value is one not known yet and is skipped.
func checkVersionAgreement(slug, rowVersion, dirVersion, manifestVersion, symbolVersion string) error {
	if rowVersion != "" && dirVersion != "" && rowVersion != dirVersion {
		return &versionMismatch{ErrRowVersionMismatch,
			fmt.Sprintf("%s: installed version %q does not match directory %q", slug, rowVersion, dirVersion)}
	}
	if dirVersion != "" && manifestVersion != "" && manifestVersion != dirVersion {
		return &versionMismatch{ErrManifestVersionMismatch,
			fmt.Sprintf("manifest version %q does not match directory %q", manifestVersion, dirVersion)}
	}
	if manifestVersion != "" && symbolVersion != "" && symbolVersion != manifestVersion {
		return &versionMismatch{ErrSymbolVersionMismatch,
			fmt.Sprintf("manifest version %q does not match plugin symbol %q", manifestVersion, symbolVersion)}
	}
	return nil
}

// missingInstallDirError describes an installed row with no directory at its version,
// listing the versions that are on disk because that is the usual clue to the cause.
func missingInstallDirError(slug, rowVersion string, onDisk []string) error {
	msg := fmt.Sprintf("installed version %q has no directory %s@%s", rowVersion, slug, rowVersion)
	if len(onDisk) > 0 {
		msg += fmt.Sprintf("; directories on disk: %v", onDisk)
	}
	return &versionMismatch{ErrInstallDirMissing, msg}
}
