// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package failreason

import (
	"errors"
	"strings"
	"testing"
)

// Compose prints its progress first and the daemon's refusal last; the operator
// needs the refusal, not "Container hass-otbr Creating".
func TestOfPicksTheDaemonsAnswerOverComposeProgress(t *testing.T) {
	err := errors.New("starting stack:  Network hass-otbr_default  Creating\n Container hass-otbr  Creating\nError response from daemon: error gathering device information while adding custom device \"/dev/ttyUSB0\": no such file or directory\n Container hass-otbr  Removed\n")
	got := Of(err)
	want := `Error response from daemon: error gathering device information while adding custom device "/dev/ttyUSB0": no such file or directory`
	if got != want {
		t.Fatalf("Of = %q, want %q", got, want)
	}
}

func TestOfKeepsASingleLineError(t *testing.T) {
	const msg = "resolving tarball: open /mnt/plugin-catalog/files/addon-kvm-2026.10.01.tar.gz: no such file or directory"
	if got := Of(errors.New(msg)); got != msg {
		t.Fatalf("Of = %q", got)
	}
}

func TestOfNeverShowsTheDockerSocketOrACredential(t *testing.T) {
	got := Of(errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running? token=abcd1234secret"))
	for _, leak := range []string{"docker.sock", "unix://", "abcd1234secret"} {
		if strings.Contains(got, leak) {
			t.Fatalf("Of leaked %q: %q", leak, got)
		}
	}
	if !strings.Contains(got, "the Docker socket") {
		t.Fatalf("Of = %q, want the socket named generically", got)
	}
}

func TestOfNil(t *testing.T) {
	if got := Of(nil); got != "" {
		t.Fatalf("Of(nil) = %q", got)
	}
}
