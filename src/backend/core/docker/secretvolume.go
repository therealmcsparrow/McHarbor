// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package docker

import (
	"archive/tar"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/volume"
	sdkclient "github.com/moby/moby/client"
)

// Labels McHarbor puts on the volumes it creates for file-based secret
// injection, so they can be found, audited and cleaned up without guessing
// from the name.
const (
	SecretVolumeLabel      = "com.mcharbor.secret-volume"       //nolint:gosec // label key, not a credential
	SecretVolumeOwnerLabel = "com.mcharbor.secret-volume.owner" //nolint:gosec // label key, not a credential
)

// SecretVolumeName is the volume a container or stack's secret files live in.
// One per owner, so removing an owner removes exactly its own secrets.
func SecretVolumeName(owner string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		default:
			return '-'
		}
	}, owner)
	return "mcharbor-secrets-" + strings.Trim(safe, "-")
}

// MaterialiseSecretVolume creates the volume for an owner and writes the given
// files into it, replacing whatever was there before.
//
// **How, and why this way.** Docker has no API that puts a file into a volume,
// and it cannot pre-populate a tmpfs mount at all — it hands the container an
// empty one at start. The only way in is through a container that has the
// volume mounted.
//
// So a helper container is created from `image` — the image the target is
// about to run, therefore already pulled — with the volume mounted, the files
// copied in, and the helper removed. **The helper is never started.**
// `CopyToContainer` resolves its destination against the container's mounts,
// including volumes, on a created-but-stopped container; that is the standard
// way to seed a volume and it costs no image pull, no entrypoint wrapping and
// no privileged access.
//
// The alternative — wrapping the target's entrypoint with a shim that writes
// the files onto a tmpfs at start-up — keeps the values off disk entirely but
// breaks any image with a non-trivial entrypoint, and turns every startup
// failure into something that looks like an image bug.
//
// The honest limit: the values do sit in Docker's volume directory, root-only,
// until the owner is removed. That is a higher bar than `docker inspect`, which
// any other Docker manager on the host reads trivially — it is not the same as
// never touching disk.
func MaterialiseSecretVolume(
	ctx context.Context,
	cli *sdkclient.Client,
	owner, image string,
	files map[string]string,
) (string, error) {
	if len(files) == 0 {
		return "", nil
	}

	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	name := SecretVolumeName(owner)

	// Recreate rather than update: a rotated secret must not leave the old
	// value behind in a file nobody rewrites, and a variable that stopped
	// being a reference must not leave its file readable.
	if err := removeVolume(ctx, cli, name); err != nil {
		return "", err
	}

	if _, err := cli.VolumeCreate(ctx, sdkclient.VolumeCreateOptions{
		Name: name,
		Labels: map[string]string{
			SecretVolumeLabel:      "true",
			SecretVolumeOwnerLabel: owner,
		},
	}); err != nil {
		return "", fmt.Errorf("creating secret volume %s: %w", name, err)
	}

	helper, err := cli.ContainerCreate(ctx, sdkclient.ContainerCreateOptions{Config: &container.Config{
		Image: image,
		// Never started, so the command is irrelevant — but an explicit
		// one avoids inheriting an entrypoint that some tooling might try
		// to validate at create time.
		Entrypoint: []string{"/nonexistent-mcharbor-secret-seeder"},
		Labels: map[string]string{
			SecretVolumeLabel:      "seeder",
			SecretVolumeOwnerLabel: owner,
		},
	}, HostConfig: &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: name,
			Target: secretSeedPath,
		}},
		AutoRemove: false,
	}})
	if err != nil {
		return "", fmt.Errorf("creating secret seeder for %s: %w", owner, err)
	}
	defer func() {
		removeCtx, removeCancel := context.WithTimeout(context.WithoutCancel(ctx), CallTimeout)
		defer removeCancel()
		if _, err := cli.ContainerRemove(removeCtx, helper.ID, sdkclient.ContainerRemoveOptions{
			Force: true,
		}); err != nil {
			slog.Warn("removing secret seeder container failed", "owner", owner, "error", err)
		}
	}()

	archive, err := secretTar(files)
	if err != nil {
		return "", err
	}
	if _, err := cli.CopyToContainer(ctx, helper.ID, sdkclient.CopyToContainerOptions{DestinationPath: secretSeedPath, Content: archive}); err != nil {
		return "", fmt.Errorf("writing secret files for %s: %w", owner, err)
	}

	return name, nil
}

// SecretSignalLabel names the signal to send a container after its secret
// files have been rewritten in place. Absent means "send nothing": most images
// read a `_FILE` variable once at start-up, and a signal they do not handle
// would kill the process the refresh was trying to avoid restarting.
const SecretSignalLabel = "com.mcharbor.secret-signal" //nolint:gosec // label key, not a credential

// RefreshSecretFiles rewrites an owner's secret files **without stopping the
// container that is reading them**. It is the only path in this package that
// propagates a rotation with no downtime at all.
//
// Why it needs a helper container rather than copying into the target: the
// secret volume is mounted read-only into the target (SecretVolumeMount), and
// the daemon refuses to extract an archive into a read-only mount. The
// read-only flag is per-container, so a second container mounting the same
// volume read-write can write it while the first is still reading — same
// directory on the host, so the running process sees the new bytes at once.
//
// It also deliberately does **not** remove and recreate the volume the way
// MaterialiseSecretVolume does. Removal is impossible while the target holds
// the volume, so files for variables that stopped being references survive a
// refresh. That is the trade: a refresh is for rotating values in place, and a
// change to *which* variables are secrets still needs a recreate.
func RefreshSecretFiles(
	ctx context.Context,
	cli *sdkclient.Client,
	owner, image string,
	files map[string]string,
) error {
	if len(files) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	name := SecretVolumeName(owner)
	if _, err := cli.VolumeInspect(ctx, name, sdkclient.VolumeInspectOptions{}); err != nil {
		if cerrdefs.IsNotFound(err) {
			// No volume means the owner was never built in file mode, so
			// there is nothing to refresh and a recreate is the only route.
			return fmt.Errorf("no secret volume for %s", owner)
		}
		return fmt.Errorf("inspecting secret volume %s: %w", name, err)
	}

	helper, err := cli.ContainerCreate(ctx, sdkclient.ContainerCreateOptions{Config: &container.Config{
		Image:      image,
		Entrypoint: []string{"/nonexistent-mcharbor-secret-seeder"},
		Labels: map[string]string{
			SecretVolumeLabel:      "seeder",
			SecretVolumeOwnerLabel: owner,
		},
	}, HostConfig: &container.HostConfig{
		Mounts: []mount.Mount{{
			Type:   mount.TypeVolume,
			Source: name,
			Target: secretSeedPath,
		}},
		AutoRemove: false,
	}})
	if err != nil {
		return fmt.Errorf("creating secret refresher for %s: %w", owner, err)
	}
	defer func() {
		removeCtx, removeCancel := context.WithTimeout(context.WithoutCancel(ctx), CallTimeout)
		defer removeCancel()
		if _, err := cli.ContainerRemove(removeCtx, helper.ID, sdkclient.ContainerRemoveOptions{
			Force: true,
		}); err != nil {
			slog.Warn("removing secret refresher container failed", "owner", owner, "error", err)
		}
	}()

	archive, err := secretTar(files)
	if err != nil {
		return err
	}
	if _, err := cli.CopyToContainer(ctx, helper.ID, sdkclient.CopyToContainerOptions{DestinationPath: secretSeedPath, Content: archive}); err != nil {
		return fmt.Errorf("refreshing secret files for %s: %w", owner, err)
	}
	return nil
}

// SignalContainer delivers a signal to a running container, used after a live
// secret refresh so a process that re-reads its credentials on SIGHUP does so.
// The signal comes from the container's own SecretSignalLabel, because which
// signal — if any — an image honours is a property of that image.
func SignalContainer(ctx context.Context, cli *sdkclient.Client, containerID, signal string) error {
	if strings.TrimSpace(signal) == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	if _, err := cli.ContainerKill(ctx, containerID, sdkclient.ContainerKillOptions{Signal: signal}); err != nil {
		return fmt.Errorf("signalling container %s with %s: %w", containerID, signal, err)
	}
	return nil
}

// secretSeedPath is where the volume is mounted inside the seeder. It is not
// the path the target sees — that is secrets.SecretMountPath — and it is
// deliberately different so a seeder that somehow did start could not be
// mistaken for the real thing.
const secretSeedPath = "/mcharbor-secret-seed"

// secretTar renders the files as a tar stream, 0400 and owned by root.
//
// 0400 rather than 0600: nothing should be writing these, including the
// process that reads them. The volume is mounted read-only into the target as
// well, so this is belt and braces — and it means a container running as root
// that is later compromised cannot quietly swap a credential for its own.
func secretTar(files map[string]string) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Sorted so the archive is byte-identical for identical input, which makes
	// a diff of two seeding runs meaningful.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		content := files[name]
		if err := tw.WriteHeader(&tar.Header{
			Name:    name,
			Mode:    0o400,
			Size:    int64(len(content)),
			ModTime: time.Unix(0, 0),
		}); err != nil {
			return nil, fmt.Errorf("writing secret tar header for %s: %w", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			return nil, fmt.Errorf("writing secret tar body for %s: %w", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("closing secret tar: %w", err)
	}
	return &buf, nil
}

// RemoveSecretVolume deletes an owner's secret volume. Called when the
// container or stack it belongs to is removed, so a credential does not
// outlive the thing that used it.
func RemoveSecretVolume(ctx context.Context, cli *sdkclient.Client, owner string) error {
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()
	return removeVolume(ctx, cli, SecretVolumeName(owner))
}

func removeVolume(ctx context.Context, cli *sdkclient.Client, name string) error {
	if _, err := cli.VolumeRemove(ctx, name, sdkclient.VolumeRemoveOptions{Force: true}); err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		// A volume still attached to a running container cannot be removed,
		// and that is the ordinary case when a stack is redeployed without
		// being brought down first. Compose replaces the container, which
		// releases it; reporting this as a failure would fail a deploy over
		// bookkeeping.
		if strings.Contains(strings.ToLower(err.Error()), "in use") {
			return nil
		}
		return fmt.Errorf("removing secret volume %s: %w", name, err)
	}
	return nil
}

// SecretVolumeMount is the mount to add to a container that uses file-based
// injection. Read-only: the container reads its credentials, it never writes
// them.
func SecretVolumeMount(name, target string) mount.Mount {
	return mount.Mount{
		Type:     mount.TypeVolume,
		Source:   name,
		Target:   target,
		ReadOnly: true,
	}
}

// ListSecretVolumes returns every volume McHarbor created for secret
// injection, so an operator can see them and an orphan sweep can find them.
func ListSecretVolumes(ctx context.Context, cli *sdkclient.Client) ([]volume.Volume, error) {
	ctx, cancel := context.WithTimeout(ctx, CallTimeout)
	defer cancel()

	list, err := cli.VolumeList(ctx, sdkclient.VolumeListOptions{
		Filters: make(sdkclient.Filters).Add("label", SecretVolumeLabel+"=true"),
	})
	if err != nil {
		return nil, fmt.Errorf("listing secret volumes: %w", err)
	}
	out := make([]volume.Volume, 0, len(list.Items))
	out = append(out, list.Items...)
	return out, nil
}
