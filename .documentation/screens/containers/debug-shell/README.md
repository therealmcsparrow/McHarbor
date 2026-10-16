# Containers: Debug shell

`/containers/:id` → **Terminal** tab → **Debug shell** · `mcharbor container exec <id> --debug`

A distroless or `scratch` image carries no shell, so the ordinary terminal and
`container exec` fail with the runtime's own complaint:

```text
OCI runtime exec failed: exec failed: unable to start container process:
exec: "/bin/sh": stat /bin/sh: no such file or directory
```

The debug shell puts a static **busybox** into the running container, opens a shell with it,
and takes it out again. The container's filesystem gains exactly one path while the session
lasts, and every install is audited with the digest of the bytes that were placed there.

## Permissions

| Key | Scope | What it unlocks |
|---|---|---|
| `containers.debug_shell` | environment | Install the debug shell, attach a terminal to it, and remove it early |
| `containers.view` | environment | See whether a container has an open debug session, or one whose cleanup failed |
| `containers.manage` | owner | Additionally needed by the CLI's one-shot form (`--debug -- <command>`), which runs through the exec route |

`containers.debug_shell` is deliberately separate from `terminal.access` and
`containers.manage`: placing an executable inside a running container is a stronger act than
running one that is already there. No role preset carries it; grant it explicitly, or hold
the wildcard.

In the UI the **Debug shell** button renders **disabled with a tooltip naming
`containers.debug_shell`** when you lack the key, and the confirm button in its dialog does
the same.

## What happens

1. **Install**: `POST /api/containers/{id}/debug-shell?env=<env>`. McHarbor inspects the
   container and refuses, each with its own code:

   | Refusal | Status | Code |
   |---|---|---|
   | The container has a shell (`/bin/sh -c :` exits 0) | 409 | `err.container.has_shell` |
   | The root filesystem is read-only (`HostConfig.ReadonlyRootfs`) | 409 | `err.container.readonly_rootfs` |
   | The container is not running | 409 | `err.container.not_running` |
   | The container is protected (McHarbor itself) | 403 | `err.protected_target` |
   | The daemon is neither `amd64` nor `arm64` | 400 | `err.debug_shell.unsupported_arch` |
   | This McHarbor build carries no busybox | 503 | `err.debug_shell.binary_unavailable` |

   Otherwise it reads the **daemon's** architecture from `/info` (not McHarbor's own, since
   an agent host can be a different machine) and uploads the matching busybox with a single
   `PUT /containers/{id}/archive` extracted at `/`:

   ```text
   /.mcharbor-debug/          0755  0:0
   /.mcharbor-debug/busybox   0755  0:0
   ```

   No symlinks are created. Applets run as `busybox <applet>`, and the shell starts with
   `PATH=/.mcharbor-debug:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin`;
   busybox is built with `FEATURE_PREFER_APPLETS` and `FEATURE_SH_STANDALONE`, so `ls`,
   `cat`, `ps` and the rest resolve inside busybox itself.

   The answer is the session: `sessionId`, `path`, `arch`, `busyboxDigest`, `sizeBytes`,
   `installed` (false when an existing install on the same container was reused rather than
   uploaded again) and `expiresAt` (15 minutes out).

2. **Shell**: the terminal WebSocket, `GET /api/terminal/ws?container=<id>&debugSession=<sessionId>`,
   runs `/.mcharbor-debug/busybox sh`. It is refused unless the session exists, is unexpired,
   is not removed, belongs to you, names this container, and has no other socket attached.
   The terminal's usual `shell.session.*` audit rows are written, carrying
   `debug_session=<id>`.

3. **Removal**: `/.mcharbor-debug/busybox rm -rf /.mcharbor-debug`, run as `0` with a detached
   exec. While another live session on the same container still uses the install, the
   binary stays and only the ending session is closed.

## Cleanup: four independent triggers

| Trigger | When | Recorded as |
|---|---|---|
| The terminal socket closes, for any reason | Immediately | `session_end` |
| The session expires with a shell still attached | The reaper closes the socket, then removes | `expiry` |
| The background reaper | Every minute, for any open session past its expiry | `reaper` |
| McHarbor starts | The same sweep, so a crash mid-session does not leave the binary forever | `startup` |

`DELETE /api/containers/{id}/debug-shell?env=<env>[&session=<id>]` ends sessions early; the
CLI's one-shot form calls it as soon as the command returns.

Removal is idempotent and never fatal. A container that stopped or was removed in the
meantime is closed without an exec; the directory goes with its writable layer. A removal
that fails keeps the session **open** with its `lastError`, is retried by the reaper, and is
reported by `GET /api/containers/{id}/debug-shell` as `leftBehind: true`, so a busybox left in
a production container is something McHarbor tells you rather than `docker diff`.

## Audit

| Action | When | Details |
|---|---|---|
| `debug_shell_install` | Every install, whether or not a shell ever opens (severity warning) | `{ environmentId, path, arch, sizeBytes, busyboxDigest, sessionId, expiresAt, uploaded }` |
| `debug_shell_remove` | Every removal attempt | `{ sessionId, trigger, succeeded, outcome, error }`; `outcome` is `removed`, `retained`, `container_gone`, `container_not_running` or `failed` |

`busyboxDigest` is the sha256 of the uploaded bytes, so "what was put into that container"
has an answer after the image that carried it is gone. An install row with no successful
remove row is the signal that something was left behind.

## Where the binary comes from

The two binaries are **built from source inside `docker/Dockerfile`**, not downloaded at
run time, because an air-gapped instance must be able to debug an outage. Nothing is fetched after
the image is built.

| | |
|---|---|
| Upstream | BusyBox **1.38.0**, `https://busybox.net/downloads/busybox-1.38.0.tar.bz2` |
| Tarball SHA-256 (pinned, `BUSYBOX_SHA256`) | `34f9ea6ff8636f2c9241153b9114eefa9e65674a45318ae1ef95bb5f31c53bb2`, the value busybox.net publishes in `busybox-1.38.0.tar.bz2.sha256` |
| Signature | `busybox-1.38.0.tar.bz2.sig`, verified with `gpg --verify` against the key committed at `signing/busybox-vda.linux.asc`; the build fails unless the `VALIDSIG` fingerprint is **`C9E9416F76E610DBD09D040F47B70C55ACC9965B`** (Denys Vlasenko, the BusyBox maintainer) |
| Toolchain | `alpine:3.22` (musl, `build-base linux-headers perl`), one stage per architecture: `--platform=linux/amd64` and `--platform=linux/arm64` (the latter under QEMU emulation on an amd64 builder) |
| Configuration | `make defconfig`, then `CONFIG_STATIC=y`, `CONFIG_FEATURE_PREFER_APPLETS=y`, `CONFIG_FEATURE_SH_STANDALONE=y`, and `CONFIG_TC` off (it does not build against current kernel headers). The build script is `docker/build-busybox.sh`; it refuses a dynamically linked result |
| Reproducibility inputs | `SOURCE_DATE_EPOCH=1778638914` (the release signature's timestamp), `KCONFIG_NOTIMESTAMP=1` |
| Installed at | `/app/debug/busybox-amd64`, `/app/debug/busybox-arm64` in the image; `debug/` beside the binary in a tarball install; overridable with `MCHARBOR_DEBUG_BIN_DIR` |

Digests of the binaries from the build that shipped with this page:

| File | Size | SHA-256 |
|---|---|---|
| `busybox-amd64` | 1,218,448 bytes | `e75e13bea27d6b8a7c62a261acc3020c0aa39d04a5fd8681150b1ea5505add82` |
| `busybox-arm64` | 1,378,064 bytes | `107f49710a93009c5a6158905990e98681a5be736c3cd94d3454daa09ad31cf5` |

These are the values the `debug_shell_install` row records as `busyboxDigest`. A rebuild
with a different toolchain image can change them; the audit row always holds the digest of
the bytes actually uploaded, so the record stays true either way.

To upgrade busybox: change `BUSYBOX_VERSION` and `BUSYBOX_SHA256` in `docker/Dockerfile`
together, confirm the new tarball verifies against the same fingerprint, and update the
tables above.

## Through a remote agent

The upload is an ordinary Docker `PUT` with a body, so it travels the agent's HTTP proxy like
any other call, unlike a non-detached exec, which is `postHijacked` and bypasses the agent.
Every exec the debug shell starts (the shell probe and the removal) is detached for that
reason, and the interactive session uses the agent's dedicated `exec_start` / `exec_input` /
`exec_output` / `exec_end` / `exec_resize` messages, exactly as the ordinary terminal does.

## What goes wrong

| Symptom | Cause | Fix |
|---|---|---|
| `err.container.has_shell` | The image has `/bin/sh` | Use the ordinary terminal |
| `err.container.readonly_rootfs` | `read_only: true` on the container | Nothing can be placed in it; debug from a sidecar sharing the PID namespace, or run it writable temporarily |
| `err.debug_shell.binary_unavailable` | A development build started with `go run`, which has no `debug/` directory | Set `MCHARBOR_DEBUG_BIN_DIR` to a directory holding `busybox-amd64` / `busybox-arm64`, or use the image |
| The socket is refused with `err.debug_shell.session_invalid` | The session expired, was removed, belongs to someone else, or already has a shell attached | Install again; each session admits one socket |
| `leftBehind: true` on the status route | A removal failed (its `lastError` says why) | The reaper retries every minute; `DELETE` the session to retry now |
