// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package lxc

import (
	"io/fs"
	"strings"
	"testing"
	"time"
)

// fakeProbe is a rootfs of named entries: a value starting with "->" is a link
// to the rest, anything else a regular file.
type fakeProbe map[string]string

type fakeInfo struct {
	name string
	mode fs.FileMode
}

func (f fakeInfo) Name() string       { return f.name }
func (f fakeInfo) Size() int64        { return 0 }
func (f fakeInfo) Mode() fs.FileMode  { return f.mode }
func (f fakeInfo) ModTime() time.Time { return time.Time{} }
func (f fakeInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeInfo) Sys() any           { return nil }

func (p fakeProbe) Lstat(rel string) (fs.FileInfo, error) {
	value, ok := p[rel]
	if !ok {
		for key := range p {
			if strings.HasPrefix(key, rel+"/") {
				return fakeInfo{name: rel, mode: fs.ModeDir | 0o755}, nil
			}
		}
		return nil, fs.ErrNotExist
	}
	if strings.HasPrefix(value, "->") {
		return fakeInfo{name: rel, mode: fs.ModeSymlink | 0o777}, nil
	}
	return fakeInfo{name: rel, mode: 0o755}, nil
}

func (p fakeProbe) Readlink(rel string) (string, error) {
	value, ok := p[rel]
	if !ok || !strings.HasPrefix(value, "->") {
		return "", fs.ErrInvalid
	}
	return strings.TrimPrefix(value, "->"), nil
}

func TestDetectInitReadsWhatPID1WillBe(t *testing.T) {
	cases := []struct {
		name  string
		probe fakeProbe
		want  InitInfo
	}{
		{"alpine minirootfs: busybox, no openrc", fakeProbe{
			"sbin/init": "->/bin/busybox", "bin/busybox": "bin"},
			InitInfo{Kind: InitKindBusybox, BusyboxInit: true}},
		{"alpine with openrc installed", fakeProbe{
			"sbin/init": "->/bin/busybox", "bin/busybox": "bin", "sbin/openrc": "bin"},
			InitInfo{Kind: InitKindOpenRC, BusyboxInit: true}},
		{"relative busybox link", fakeProbe{
			"sbin/init": "->../bin/busybox", "bin/busybox": "bin"},
			InitInfo{Kind: InitKindBusybox, BusyboxInit: true}},
		{"debian with systemd-sysv", fakeProbe{
			"sbin/init": "->/lib/systemd/systemd", "lib/systemd/systemd": "bin"},
			InitInfo{Kind: InitKindSystemd}},
		{"usrmerge: /sbin is a link too", fakeProbe{
			"sbin": "->usr/sbin", "sbin/init": "->../lib/systemd/systemd",
			"usr/sbin/init": "->../lib/systemd/systemd", "usr/lib/systemd/systemd": "bin"},
			InitInfo{Kind: InitKindSystemd}},
		{"gentoo openrc-init", fakeProbe{"sbin/init": "->openrc-init", "sbin/openrc-init": "bin"},
			InitInfo{Kind: InitKindOpenRC}},
		{"a plain sysvinit binary", fakeProbe{"sbin/init": "bin"}, InitInfo{Kind: InitKindSysVInit}},
		{"no /sbin/init at all", fakeProbe{"bin/sh": "bin"}, InitInfo{Kind: InitKindNone}},
		{"a dangling link", fakeProbe{"sbin/init": "->/lib/systemd/systemd"}, InitInfo{Kind: InitKindNone}},
		{"a link cycle", fakeProbe{"sbin/init": "->/sbin/init2", "sbin/init2": "->/sbin/init"},
			InitInfo{Kind: InitKindNone}},
		{"a link climbing out of the rootfs", fakeProbe{"sbin/init": "->../../../host/busybox"},
			InitInfo{Kind: InitKindNone}},
	}
	for _, c := range cases {
		if got := DetectInit(c.probe); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
