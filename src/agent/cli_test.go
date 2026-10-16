// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestCLIArgsDispatchesOnlyTheCLI(t *testing.T) {
	cases := []struct {
		argv []string
		want []string
		ok   bool
	}{
		{[]string{"/usr/local/bin/mcharbor", "container", "list"}, []string{"container", "list"}, true},
		{[]string{filepath.Join("tools", "MCHARBOR.EXE"), "version"}, []string{"version"}, true},
		{[]string{"/usr/local/bin/mcharbor-agent", "cli", "login", "https://h"}, []string{"login", "https://h"}, true},
		{[]string{"/usr/local/bin/mcharbor-agent", "cli"}, []string{}, true},
		{[]string{"/usr/local/bin/mcharbor-agent"}, nil, false},
		{[]string{"/usr/local/bin/mcharbor-agent", "--help"}, nil, false},
		{[]string{"/proc/self/exe", "init"}, nil, false},
		{nil, nil, false},
	}
	for _, c := range cases {
		got, ok := cliArgs(c.argv)
		if ok != c.ok || (ok && !reflect.DeepEqual(got, c.want)) {
			t.Errorf("cliArgs(%q) = %q, %v; want %q, %v", c.argv, got, ok, c.want, c.ok)
		}
	}
}

func TestCLIServerURLFromTheAgentsURL(t *testing.T) {
	for in, want := range map[string]string{
		"wss://harbor.lan:8705/api/agent/ws": "https://harbor.lan:8705",
		"ws://10.0.0.2:8705":                 "http://10.0.0.2:8705",
		"https://harbor.lan:8705":            "https://harbor.lan:8705",
		"http://10.0.0.2:8705/":              "http://10.0.0.2:8705/",
	} {
		if got := cliServerURL(in); got != want {
			t.Errorf("cliServerURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestEnsureCLILinkNeverReplacesAnotherMcharbor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the link is a Unix install convention")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "mcharbor-agent")
	if err := os.WriteFile(exe, []byte("agent"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "mcharbor")

	if err := ensureCLILink(exe); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != "mcharbor-agent" {
		t.Fatalf("a fresh install linked %q, %v; want mcharbor-agent", got, err)
	}
	if err := ensureCLILink(exe); err != nil {
		t.Fatalf("an update over an existing link: %v", err)
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("standalone"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensureCLILink(exe); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(link); err != nil || string(b) != "standalone" {
		t.Fatalf("a separately installed mcharbor was replaced: %q, %v", b, err)
	}

	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/opt/other/mcharbor", link); err != nil {
		t.Fatal(err)
	}
	if err := ensureCLILink(exe); err != nil {
		t.Fatal(err)
	}
	if got, err := os.Readlink(link); err != nil || got != "/opt/other/mcharbor" {
		t.Fatalf("a link to another mcharbor was rewritten to %q, %v", got, err)
	}
}
