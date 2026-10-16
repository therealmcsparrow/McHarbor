// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package cli

import (
	"net/http"
	"strings"
	"testing"
)

// TestExecOnADistrolessContainerExplainsItself: a shell-less image makes the
// runtime complain about stat /bin/sh, which reads as a McHarbor fault. The
// refusal has to name the real cause and what the operator can do instead, and it
// must keep the exit code the failure already had.
func TestExecOnADistrolessContainerExplainsItself(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/containers/svc/exec": func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusInternalServerError, "err.container.exec_failed",
				`OCI runtime exec failed: exec failed: unable to start container process: exec: "/bin/sh": stat /bin/sh: no such file or directory`)
		},
	})

	code, _, stderr := run(t, f, goodKey, "container", "exec", "svc", "--", "ls")
	if code != ExitRemote {
		t.Fatalf("the exit code must still be the failure's own, got %d (%q)", code, stderr)
	}
	if !strings.Contains(stderr, "has no shell to run") {
		t.Fatalf("the refusal must name the real cause: %q", stderr)
	}
	if !strings.Contains(stderr, "distroless") {
		t.Fatalf("the refusal must say what kind of image this is: %q", stderr)
	}
	if !strings.Contains(stderr, "stat /bin/sh") {
		t.Fatalf("the server's own message must survive: %q", stderr)
	}
}

// TestExecKeepsItsGuaranteesForAnOrdinaryFailure: the hint must not attach itself
// to every failure, or it becomes noise that hides the real one.
func TestExecKeepsItsGuaranteesForAnOrdinaryFailure(t *testing.T) {
	f := groupServer(t, map[string]func(http.ResponseWriter, string){
		"POST /api/containers/svc/exec": func(w http.ResponseWriter, _ string) {
			fail(w, http.StatusForbidden, "err.rbac.permission_denied", "You do not have permission to perform this action")
		},
	})
	code, _, stderr := run(t, f, goodKey, "container", "exec", "svc", "--", "ls")
	if code != ExitForbidden {
		t.Fatalf("a 403 must still exit %d, got %d", ExitForbidden, code)
	}
	if strings.Contains(stderr, "no shell to run") {
		t.Fatalf("the shell-less hint must not attach to an unrelated failure: %q", stderr)
	}
}

// TestShelllessRecognisesOnlyTheRealThing pins the matcher directly, because the
// interactive path delivers the same complaint over a WebSocket error frame and
// no HTTP test reaches it.
func TestShelllessRecognisesOnlyTheRealThing(t *testing.T) {
	yes := []string{
		`exec: "/bin/sh": stat /bin/sh: no such file or directory`,
		`unable to start container process: exec: "/bin/bash": executable file not found in $PATH`,
		`OCI runtime exec failed: starting container process caused: exec: "/bin/ash": no such file or directory`,
	}
	for _, msg := range yes {
		if !shellless(&Error{Code: ExitRemote, Message: msg}) {
			t.Fatalf("must be recognised as shell-less: %q", msg)
		}
	}
	no := []string{
		"You do not have permission to perform this action",
		"no such container: svc",
		"open /var/lib/mcharbor/x: no such file or directory",
		"",
	}
	for _, msg := range no {
		if shellless(&Error{Code: ExitRemote, Message: msg}) {
			t.Fatalf("must not be read as shell-less: %q", msg)
		}
	}
	if shellless(nil) {
		t.Fatal("nil is not a shell-less failure")
	}
}
