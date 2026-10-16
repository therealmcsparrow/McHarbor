# Agent Releases

McHarbor keeps its **own store of agent releases**, and every agent install and every agent
update (native binary and container alike) is served from McHarbor. An agent host never
contacts GitHub or a container registry for the agent; only McHarbor does, and only to fill
its store.

This page covers where releases come from, how they are verified, where they are kept, what
McHarbor serves when, and how a container agent updates without a registry.

| Page | What it covers |
|---|---|
| [Mirror contract](./mirror-contract.md) | The directory layout an operator's mirror must serve, and how to build one |

## Where a release comes from

Settings → Updates → Agents → **Agent release downloads** → **Sources, tried in order**.

| Source type | Reads |
|---|---|
| **GitHub releases** | A repository's release assets: `<repo>/releases/download/v<version>/<file>` and `<repo>/releases/latest/download/latest.json` |
| **Mirror** | A plain HTTPS directory laid out as the [mirror contract](./mirror-contract.md) says, given either as the directory or as its index file. The default list holds exactly one: `https://www.mcharbor.eu/catalog/agents/index.json` |

A source's index may list every version it carries, and several builds of each (`versions` in
the [mirror contract](./mirror-contract.md)). **Check now** records up to the last five builds of
every version, newest first. The releases table offers each of them that is not the stored
build under **Other versions and builds** → **Pull version** (`updates.manage`), including
older builds of the latest version; the newest build of the latest version is the separate
**Download** button. `mcharbor agent releases pull <version> [--build <yyyyMMddHHmm>]` does
the same; the API takes `?build=` on `POST /api/updates/agents/releases/{version}/pull`.

### Builds

A release is a version **and** a build, and each build is stored on its own: its own row in
the table and its own folder (`<version>_<build>/`). Republishing a version with a new `BUILD`
stamp is a new release: once a check has seen the newer build, pulling that version downloads
it beside the builds already stored, never over them.

A pull you start (the **Download** button, **Pull version**, `mcharbor agent releases pull`, or
`POST …/{version}/pull`) makes the build the **target** once it has downloaded and verified.
Until then the previous target keeps being served. The previous target stays **Stored**, so you
can go back with **Make … the target** on its row. Agents running anything other than the new
target show as outdated in the agents table as soon as the target changes, without waiting for
a check, and the next sweep updates them. A pull of a build that is already stored does not
download it again and only makes it the target.

If a download cannot be fetched or fails verification, only its own row is marked, with the
error behind the row's error icon. Every other build, the target included, is left as it was.
The scheduler's **Download it** mode stores a new build without making it the target; **Download
it and make it the target** does both.

A pull that names a build fetches that build's folder only, never the flat `<version>/`
layout, and refuses a source whose signed `BUILD` file names a different build, so the store
never ends up holding a build nobody chose. Builds stored before McHarbor kept one folder per
build are still read from their old `<version>/` folder.

### Pinned builds

Pulling a build older than the newest one a check has listed for that version **pins** it: the
row's build shows **Pinned**, and neither the scheduled check nor a pull without a build moves
that version forward again, whatever the mode. Pulling the newest build of the version — the
**Download** button, or that build from the list — removes the pin and hands the version back to
the checks. Deleting the release forgets the pin too.

Sources are tried in order. A source that does not have the version (HTTP 404) is skipped
quietly. A source that serves something that **fails verification** is logged as a warning
naming the source and is also skipped, so a tampered mirror cannot block the official
release, but it can never be stored either. At most eight sources; `https://` only.

## Verification

Every release carries `SHA256SUMS` and `SHA256SUMS.sig`.

1. `SHA256SUMS.sig` is the base64 of a raw **ed25519** signature over the exact bytes of
   `SHA256SUMS`, the same scheme as plugin signatures (`pluginsigner -file` writes it).
2. It must verify against one of the **agent signing keys pinned in McHarbor**
   (`src/backend/modules/agentreleases/keys.go`). That is a list, newest first, so a key can
   be rotated by shipping the new public key in one release and dropping the old one later.
   The agent key is separate from the plugin signing key.
3. Every agent binary listed in `SHA256SUMS` is downloaded and hashed, and must match its
   line. `BUILD` and `image.txt`, when listed, are checked the same way.
4. Only when all of that passes is the release moved into the store.

Any failure (a missing signature, a signature from an unpinned key, an edited
`SHA256SUMS`, a binary whose digest does not match, a malformed line) **refuses the whole
release**. Nothing is stored or served, the row reads **Refused**, and the reason is behind
the error icon beside the status (**Why this release was refused**), with the source that
failed named in it.

A release row records which pinned key signed it (**Signature** column, the first 16 hex
digits of the key's SHA-256).

## Where it is kept

| What | Where |
|---|---|
| Files | `$DATA_DIR/agent-releases/<version>/`: the binaries, `SHA256SUMS`, `SHA256SUMS.sig`, and one `image-<os>-<arch>.tar` per stored image |
| Record | The `agent_releases` table (migration `118_agent_releases.sql`): version, build, source, status, signature status and key, per-asset SHA-256 and size, image reference and error, when and by whom it was downloaded |
| Settings | The `settings` table, category `agent_releases` |

A download is staged in `$DATA_DIR/agent-releases/.staging-<version>-<id>/` and only renamed
into place once verified, so a half-written release is never visible.

## What McHarbor serves

`GET /api/agent/binary/{os}/{arch}` answers, in this order:

1. **The target release**: the stored release an operator made the target, when it
   carries a binary for that platform.
2. **`agent-local/`**: what `scripts/development/build-agent.sh` last built into the
   repository's `agents/` folder, mounted into the container.
3. **`agent-bin/`**: the binary baked into the McHarbor image.

The target wins because choosing it is an explicit act. The last two are **trusted as
built-in**: they came from this McHarbor's own build, not from the network, and are served
without a signature check.

With no target, McHarbor behaves as before this store existed and serves the local or
bundled agent. With nothing for a platform at all, an install or update **fails with
instructions** instead of reaching for GitHub.

The binary route stays public, as it was: agents that predate this change download their
replacement without credentials, and the bytes are a signed release or a built-in binary.
It sends the file's digest as `X-Checksum-Sha256`, and the install script checks it.

`GET /api/agent/image/{os}/{arch}` serves the target release's image tarball. It requires the
agent's own token as `Authorization: Bearer <agent token>` and answers 401 without one. There
is no built-in fallback for images.

## Container agents

McHarbor pulls the agent image for each configured platform (**Container agent images to
store**, default `linux/amd64`) on the Docker daemon it runs on, and keeps it as a
`docker save` tarball tagged `mcharbor-agent:<version>`:

- When the release lists a signed `image.txt`, the image is pulled **by that digest**, so the
  signature covers it.
- Otherwise it is pulled from **Image repository** (default
  `ghcr.io/therealmcsparrow/mcharbor-agent`) at `:<version>`.

A failed image pull does not refuse the release (the binaries are verified and stored), but
the **Images** column shows the error behind its icon, and container agents on that
platform cannot be updated until it is fixed.

An update then runs entirely through McHarbor: the agent downloads the tarball with its
token, checks the digest McHarbor sent, loads it through its own local Docker API, and
starts a replacement container from `mcharbor-agent:<version>` exactly as before. It never
pulls.

This needs an agent at **`2026.09.01` build `202609271330`** or later
(`AgentImageLoadMinVersion` / `AgentImageLoadMinBuild`). An older container agent is sent
the update request it always understood and still pulls its image from a registry itself;
it cannot be made to do otherwise. Update it once by reinstalling from McHarbor.

A native agent refuses any download URL that does not point at the McHarbor it is connected
to, and refuses an empty one with the fix spelled out.

## Settings

Settings → Updates → Agents → **Agent release downloads**. Gated by `updates.manage`.

| Setting | Default | What it does |
|---|---|---|
| **Check for new agent releases** | On | Runs the check on the schedule below |
| **Check schedule** | `0 5 * * *` (daily 05:00) | When to ask the sources which version is newest. **Next check** is shown under it. A check that fails runs again 15 minutes later instead of waiting for the next scheduled time, and McHarbor checks at start-up when the last check failed or never ran |
| **When a new release is found** | Notify only | **Notify only**: one notification per version and build. **Download it**: download and verify into the store. **Download it and make it the target**: the same, then select it; the agent auto-update and its install window roll it out |
| **Releases to keep** | 3 | After each download, older stored builds are removed, never the target and never a build a connected agent runs. Each build counts as one release |
| **Sources, tried in order** | Mirror `https://www.mcharbor.eu/catalog/agents/index.json` | See above |
| **Container agent images to store** | `linux/amd64` | Also `linux/arm64` |
| **Image repository** | `ghcr.io/therealmcsparrow/mcharbor-agent` | Used when a release does not sign an image digest |

On a high-availability cluster only the elected leader runs the scheduled check.

## The table

**Agent releases** lists every stored or attempted release: version with **Target** and
**In use** badges, build, status (**Downloading**, **Stored**, **Refused**), signing key,
binary and image platforms, size, source and who downloaded it when. Every stored build has a
row of its own, so a version can appear more than once; the row's actions name the build, as in
**Make 2026.10.01 (202610050818) the target**. A build that is downloading has its own
**Downloading** row, while the builds already stored keep being served.

| Action | Needs | Notes |
|---|---|---|
| **Check now** | `updates.manage` | Asks the sources; downloads nothing |
| **Download &lt;version&gt; (&lt;build&gt;)** | `updates.manage` | Offered when the last check found a version that is not stored, or a newer build of a stored one. The label names the build. Once verified, the build becomes the target; the builds already stored stay |
| **Make &lt;version&gt; (&lt;build&gt;) the target** | `updates.manage` | Only a stored build. Does not restart any agent by itself; the agents table re-checks which agents are outdated straight away |
| **Delete &lt;version&gt; (&lt;build&gt;)** | `updates.manage` | Confirmed first. Removes that build only. Refused for the target and for any build a connected agent runs |

Rolling a target out is still **Update all** or the automatic sweep, gated by
`environments.manage` as before.

## Routes

| Route | Permission |
|---|---|
| `GET /api/updates/agents/releases` | `updates.view` |
| `GET /api/updates/agents/releases/settings` | `updates.view` |
| `PUT /api/updates/agents/releases/settings` | `updates.manage` |
| `POST /api/updates/agents/releases/check` | `updates.manage` |
| `POST /api/updates/agents/releases/{version}/pull` (`?build=`); makes the verified build the target | `updates.manage` |
| `PUT /api/updates/agents/releases/target` (`{"version", "build"}`; no `build` means the newest stored build of the version) | `updates.manage` |
| `DELETE /api/updates/agents/releases/{version}` (`?build=`; required when the version has more than one stored build) | `updates.manage` |
| `GET /api/agent/binary/{os}/{arch}` | none (public, as before) |
| `GET /api/agent/image/{os}/{arch}` | the agent's token |

Every change is audited: `agent_release_check`, `agent_release_pull`,
`agent_release_delete`, `agent_release_target`, `agent_release_settings`.

## CLI

```bash
mcharbor agent releases list
mcharbor agent releases check
mcharbor agent releases pull 2026.09.02
mcharbor agent releases target 2026.09.02 --yes
mcharbor agent releases target 2026.10.01 --build 202610050818 --yes
mcharbor agent releases target --clear --yes
mcharbor agent releases delete 2026.09.01 --confirm 2026.09.01
mcharbor agent releases delete 2026.10.01 --build 202610050818 --confirm 2026.10.01
```

## Publishing a release

`.github/workflows/release.yml` → **Publish signed agent release** builds the five binaries,
writes `BUILD`, `image.txt` (the pushed agent image's digest), `SHA256SUMS` and
`latest.json`, and signs `SHA256SUMS` with the **`AGENT_SIGNING_KEY`** repository secret:
the base64 of the 32-byte seed in `signing/agent-signing.key`. It fails rather than publish
an unsigned release. With the `AGENT_RELEASE_TOKEN` secret set it also creates the
`v<version>` release on `therealmcsparrow/mcharbor-agent`; without it the signed set is the
workflow artifact `mcharbor-agent-release`.

Locally, `scripts/development/build-agent.sh` writes `SHA256SUMS` beside the binaries in
`agents/` and signs it when `signing/agent-signing.key` (or `$AGENT_SIGNING_KEY_FILE`) exists.

## When something does not behave

| Symptom | Cause | Fix |
|---|---|---|
| A release reads **Refused**: "signature does not verify" | Signed with a key McHarbor does not pin, or `SHA256SUMS` was edited after signing | Re-sign with the agent key; never the plugin key |
| **Refused**: "has no SHA256SUMS.sig" | The source published the sums unsigned | Publish `SHA256SUMS.sig` beside it |
| **Refused**: "checksum does not match" | A binary differs from its line: a corrupted mirror copy or a tampered file | Re-copy the release to the mirror |
| The catalog carries a version, but there is nothing to download | The last check failed (an error icon beside **checked &lt;date&gt;** at the top of the card), or the table already holds that version at the same or a newer build | **Check now**; a failed check also retries on its own after 15 minutes |
| A build's row reads **Refused** or shows an error, and the target did not move | That build could not be fetched or failed verification | Read the error; the previous target is still the one served and no other build was touched. Fix the source and download again |
| A new build is the target, and you want the previous one back | A download makes the new build the target | **Make … the target** on the previous build's row; it is still stored |
| `delete <version>` or `DELETE …/{version}` answers "That is not a valid agent version" for a version that is stored | The version has several stored builds and the request did not name one | Use the row's **Delete** (it names the build), or `--build` / `?build=` |
| An install says "this McHarbor has no agent for linux/arm64" | No target release carries that platform and nothing was built locally for it | Download a release that does, make it the target, or build with `--linux-arm64` |
| A container agent update fails: "McHarbor offered no agent image" | The target has no image for that platform | Add the platform under **Container agent images to store**, move the target off the release, delete it, download it again and make it the target |
| **Images** shows an error | McHarbor could not pull the image on its own Docker daemon | Check registry access from the McHarbor host; the binaries are unaffected |
| An old container agent still pulls from ghcr.io | It predates the image-load floor | Reinstall it once from McHarbor |
