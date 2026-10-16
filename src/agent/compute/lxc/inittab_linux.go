// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

//go:build linux

package lxc

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/therealmcsparrow/mcharbor-agent/internal/errlog"
)

// What PrepareConsoleInit did to a rootfs's inittab.
const (
	InittabUntouched = ""
	InittabGenerated = "generated"
	InittabAdapted   = "adapted"
)

// inittabOriginalName is where an adapted image's own inittab is kept, so the
// operator can see and restore exactly what the image shipped.
const inittabOriginalName = "inittab.mcharbor-orig"

// maxInittabBytes bounds the read; a real inittab is a few hundred bytes.
const maxInittabBytes = 64 << 10

// PrepareConsoleInit gives a freshly unpacked busybox-init rootfs a shell on
// /dev/console. Called once, at create from an image, and never on a restore or
// a clone, which carry whatever the operator made of the file since.
//
// Links are never followed: an /etc or /etc/inittab that is a symlink is left
// alone, because an absolute link in an image resolves against the host here.
func PrepareConsoleInit(rootfs string, shift IDShift) (string, error) {
	if !DetectInitAt(rootfs).BusyboxInit {
		return InittabUntouched, nil
	}
	etc := filepath.Join(rootfs, "etc")
	if info, err := os.Lstat(etc); err != nil || !info.IsDir() {
		return InittabUntouched, nil //nolint:nilerr // an unreadable /etc means there is no inittab to adapt, which leaves the image untouched
	}
	uid, err := shift.UID(0)
	if err != nil {
		return InittabUntouched, err
	}
	gid, err := shift.GID(0)
	if err != nil {
		return InittabUntouched, err
	}

	path := filepath.Join(etc, "inittab")
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if err := writeOwned(path, []byte(defaultBusyboxInittab), 0o644, uid, gid); err != nil {
			return InittabUntouched, err
		}
		return InittabGenerated, nil
	case err != nil:
		return InittabUntouched, fmt.Errorf("lxc: reading the image's inittab: %w", err)
	case !info.Mode().IsRegular() || info.Size() > maxInittabBytes:
		return InittabUntouched, nil
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return InittabUntouched, fmt.Errorf("lxc: reading the image's inittab: %w", err)
	}
	adapted, changed := AdaptBusyboxInittab(string(body))
	if !changed {
		return InittabUntouched, nil
	}
	original := filepath.Join(etc, inittabOriginalName)
	if _, err := os.Lstat(original); errors.Is(err, fs.ErrNotExist) {
		if err := writeOwned(original, body, info.Mode().Perm(), uid, gid); err != nil {
			return InittabUntouched, err
		}
	}
	if err := writeOwned(path, []byte(adapted), info.Mode().Perm(), uid, gid); err != nil {
		return InittabUntouched, err
	}
	return InittabAdapted, nil
}

// writeOwned replaces path with body through a rename in the same directory,
// owned by the container's root rather than the host's.
func writeOwned(path string, body []byte, mode fs.FileMode, uid, gid int) error {
	staged := path + ".mcharbor-tmp"
	errlog.Log(os.Remove(staged))
	file, err := os.OpenFile(staged, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("lxc: staging %s: %w", filepath.Base(path), err)
	}
	if _, err := file.Write(body); err != nil {
		errlog.Close(file)
		errlog.Log(os.Remove(staged))
		return fmt.Errorf("lxc: writing %s: %w", filepath.Base(path), err)
	}
	if err := file.Close(); err != nil {
		errlog.Log(os.Remove(staged))
		return fmt.Errorf("lxc: writing %s: %w", filepath.Base(path), err)
	}
	if err := os.Lchown(staged, uid, gid); err != nil {
		errlog.Log(os.Remove(staged))
		return fmt.Errorf("lxc: handing %s to the container's root: %w", filepath.Base(path), err)
	}
	if err := os.Chmod(staged, mode); err != nil {
		errlog.Log(os.Remove(staged))
		return fmt.Errorf("lxc: setting the mode of %s: %w", filepath.Base(path), err)
	}
	if err := os.Rename(staged, path); err != nil {
		errlog.Log(os.Remove(staged))
		return fmt.Errorf("lxc: installing %s: %w", filepath.Base(path), err)
	}
	return nil
}
