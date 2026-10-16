# Agent Release Mirror Contract

A **mirror** source is any HTTPS server that serves agent releases in this layout. McHarbor
reads nothing else from it, and trusts nothing it reads until the signature verifies, so a
mirror can be a plain static file server, an artifact repository, or an S3 bucket behind a
TLS endpoint.

## Layout

```text
<base>/latest.json
<base>/<version>/SHA256SUMS
<base>/<version>/SHA256SUMS.sig
<base>/<version>/BUILD                               optional
<base>/<version>/image.txt                           optional
<base>/<version>/mcharbor-agent-linux-amd64
<base>/<version>/mcharbor-agent-linux-arm64
<base>/<version>/mcharbor-agent-darwin-amd64
<base>/<version>/mcharbor-agent-darwin-arm64
<base>/<version>/mcharbor-agent-windows-amd64.exe
```

`<base>` is the URL entered as the source, without a trailing slash. `<version>` is the bare
calendar version: `2026.09.02`, no `v`. Any subset of the binaries may be present; a
platform that is absent is simply not served from that release.

A version may also be split by build: `<base>/<version>/<build>/<file>`, one folder per
`yyyyMMddHHmm` build stamp. When the index lists a build for the version being pulled,
McHarbor reads the newest listed build's folder first and falls back to `<base>/<version>/`
when that folder does not exist, so flat mirrors keep working.

A source may instead name its index file: any URL ending in `.json`, such as the default
`https://www.mcharbor.eu/catalog/agents/index.json`. That file is read in place of
`latest.json`, with the same content, and `<base>` is the directory it sits in.

| File | Content | Verified |
|---|---|---|
| `latest.json` | `{"version":"2026.09.02","build":"202609281200","versions":[…]}`: which version is newest, and optionally every `{version, build}` the mirror carries | No. It only says what to look for; nothing is stored because of it |
| `SHA256SUMS` | `sha256sum` output: `<64 hex>  <file name>` per line (a `*` before the name is accepted) | By the signature |
| `SHA256SUMS.sig` | Base64 of the raw 64-byte ed25519 signature over `SHA256SUMS` | Against the pinned agent keys |
| `BUILD` | The `yyyyMMddHHmm` build stamp | Against its line in `SHA256SUMS` |
| `image.txt` | A digest-pinned image reference, `registry/repo@sha256:<64 hex>` | Against its line in `SHA256SUMS` |
| `mcharbor-agent-*` | The binaries | Against their lines in `SHA256SUMS` |

File names outside this set that appear in `SHA256SUMS` (an SBOM, say) are ignored.

## Building a mirror

The simplest mirror copies the official release unchanged. The signature stays valid
because nothing it covers changed:

```bash
version=2026.09.02
base=https://github.com/therealmcsparrow/mcharbor-agent/releases/download/v$version
mkdir -p "mirror/$version"
for f in SHA256SUMS SHA256SUMS.sig BUILD image.txt \
         mcharbor-agent-linux-amd64 mcharbor-agent-linux-arm64 \
         mcharbor-agent-darwin-amd64 mcharbor-agent-darwin-arm64 \
         mcharbor-agent-windows-amd64.exe; do
  curl -fsSLo "mirror/$version/$f" "$base/$f" || echo "skipped $f"
done
(cd "mirror/$version" && sha256sum -c --ignore-missing SHA256SUMS)
printf '{"version":"%s"}\n' "$version" > mirror/latest.json
```

Serve `mirror/` over HTTPS and add its URL as a **Mirror** source, above or below the GitHub
source as you want it tried.

A release built in-house (`scripts/development/build-agent.sh`) produces the same
`<version>/` folder in `agents/`, signed when the agent signing key is present. The build then
runs `scripts/development/publish-agent-catalog.mjs`, which copies it into
`agents/catalog/<version>/<build>/` and rewrites `agents/catalog/index.json` to list every
`{version, build}` folder there, newest first. Every development build gets its own folder,
so uploading `agents/catalog/` as-is to `https://www.mcharbor.eu/catalog/agents/` publishes
all of them. Old builds stay until they are deleted by hand. The flat files in `agents/`
remain the copy the local container serves.

```text
agent/catalog/index.json
agent/catalog/2026.09.01/202609272222/SHA256SUMS, SHA256SUMS.sig, BUILD, mcharbor-agent-*
agent/catalog/2026.09.01/202609281015/SHA256SUMS, SHA256SUMS.sig, BUILD, mcharbor-agent-*
agent/catalog/2026.09.02/202609300900/SHA256SUMS, SHA256SUMS.sig, BUILD, mcharbor-agent-*
``` It must be
signed with the **agent** key McHarbor pins; a release signed with anything else is refused.

## What a mirror cannot do

- Serve a binary that is not in the signed `SHA256SUMS`.
- Change a binary, `BUILD` or `image.txt` without breaking the signature.
- Point McHarbor at a moving image tag: `image.txt` must be pinned by digest.
- Reach an agent. Agents only ever download from McHarbor.
