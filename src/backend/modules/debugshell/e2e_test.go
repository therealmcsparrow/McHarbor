// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package debugshell

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	dockerclient "github.com/moby/moby/client"
	"github.com/rs/xid"
)

// TestAgainstARealDaemon runs the whole install, attach-by-exec and removal
// against the Docker daemon in DOCKER_HOST with a shell-less pause container. It
// is skipped unless MCHARBOR_DEBUGSHELL_E2E=1 and MCHARBOR_DEBUG_BIN_DIR are set.
func TestAgainstARealDaemon(t *testing.T) {
	if os.Getenv("MCHARBOR_DEBUGSHELL_E2E") != "1" {
		t.Skip("MCHARBOR_DEBUGSHELL_E2E not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cli, err := dockerclient.New(dockerclient.FromEnv)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cli.Close(); err != nil {
			t.Errorf("close docker client: %v", err)
		}
	}()

	const pause = "registry.k8s.io/pause:3.10"
	pull, err := cli.ImagePull(ctx, pause, dockerclient.ImagePullOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, pull); err != nil {
		t.Fatal(err)
	}
	if err := pull.Close(); err != nil {
		t.Fatal(err)
	}
	created, err := cli.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{Config: &container.Config{Image: pause}, Name: "mcharbor-debugshell-e2e-" + xid.New().String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := cli.ContainerRemove(context.Background(), created.ID, dockerclient.ContainerRemoveOptions{Force: true}); err != nil {
			t.Errorf("removing the e2e container: %v", err)
		}
	})
	if _, err := cli.ContainerStart(ctx, created.ID, dockerclient.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}

	database := openDB(t)
	auditor := &recordingAuditor{}
	svc := newService(database, func(context.Context, string) (dockerAPI, error) { return cli, nil }, os.Getenv("MCHARBOR_DEBUG_BIN_DIR"), auditor, nil)

	sess, err := svc.Install(ctx, InstallRequest{EnvironmentID: "local", ContainerID: created.ID, UserID: "u1"})
	if err != nil {
		t.Fatalf("install into a pause container: %v", err)
	}
	t.Logf("installed %s (%s, %d bytes, %s)", sess.Path, sess.Arch, sess.SizeBytes, sess.BusyboxDigest)

	code, err := runDetached(ctx, cli, created.ID, "", []string{sess.Path, "sh", "-c", "PATH=" + InstallDir + " ls / >/dev/null && cat /proc/1/cmdline >/dev/null"})
	if err != nil || code != 0 {
		t.Fatalf("the uploaded busybox must run its applets without symlinks: code %d err %v", code, err)
	}

	if _, err := svc.Install(ctx, InstallRequest{EnvironmentID: "local", ContainerID: created.ID, UserID: "u1"}); err != nil {
		t.Fatalf("a second install must reuse the first: %v", err)
	}
	if _, err := svc.Close(ctx, "local", created.ID, ""); err != nil {
		t.Fatalf("closing both sessions: %v", err)
	}
	if _, err := cli.ContainerStatPath(ctx, created.ID, dockerclient.ContainerStatPathOptions{Path: InstallDir}); err == nil || !errdefs.IsNotFound(err) {
		t.Fatalf("%s must be gone after the last session ends (stat err %v)", InstallDir, err)
	}

	shellful, err := cli.ContainerCreate(ctx, dockerclient.ContainerCreateOptions{Config: &container.Config{Image: "alpine:3.22", Cmd: []string{"sleep", "120"}}, Name: "mcharbor-debugshell-e2e-sh-" + xid.New().String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := cli.ContainerRemove(context.Background(), shellful.ID, dockerclient.ContainerRemoveOptions{Force: true}); err != nil {
			t.Errorf("removing the e2e shell container: %v", err)
		}
	})
	if _, err := cli.ContainerStart(ctx, shellful.ID, dockerclient.ContainerStartOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Install(ctx, InstallRequest{EnvironmentID: "local", ContainerID: shellful.ID, UserID: "u1"}); !errors.Is(err, ErrHasShell) {
		t.Fatalf("a container with /bin/sh must be refused, got %v", err)
	}
}
