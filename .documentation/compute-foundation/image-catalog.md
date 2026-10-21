# The image catalog

The list of downloadable guest images (cloud images, rootfs tarballs and
installer ISOs) is read from two places, on every boot and on **Reload catalog**
on **Compute → Images**:

| Source | What it is |
|---|---|
| **McHarbor VM Images** | The official catalog, `https://www.mcharbor.eu/catalog/images/index.json`, listed under **Settings → Catalogs** once the compute plugin is installed. It starts **disabled**, like every source of bootable content; switch it on there. |
| **A mounted directory** | Optional JSON files you own, one per entry, read from `/mnt/compute-catalog` (below). An id defined here wins over the same id from a remote source. |

Editing either one is the only way to change the built-in list. The list a fresh install
starts with is seeded by the plugin's install migration, and a plugin's
migrations run **once, at install**, so editing that SQL, or adding another
migration, changes nothing on an instance that already has the plugin.

## Where the files live

`docker-compose.yml` bind-mounts the directory read-only:

```yaml
- ${MCHARBOR_LOCAL_COMPUTE_CATALOG_PATH:-./mnt/compute-catalog}:/mnt/compute-catalog:ro
```

Point `MCHARBOR_LOCAL_COMPUTE_CATALOG_PATH` somewhere else if you keep the
catalog outside the repository. `MCHARBOR_COMPUTE_CATALOG_DIR` overrides the
path *inside* the container, which matters only if you are not using the
supplied compose file.

Do not confuse it with
[the shared image library](./configuration.md#the-shared-image-library): the
catalog holds *descriptions* of images McHarbor can fetch, the library holds
*bytes* the hosts can already see.

## One entry

```json
{
  "id": "img-debian-13-amd64",
  "kind": "cloud_image",
  "distro": "debian",
  "release": "13",
  "arch": "x86_64",
  "displayName": "Debian 13 (Trixie)",
  "sourceUrl": "https://cloud.debian.org/images/cloud/trixie/latest/debian-13-genericcloud-amd64.qcow2",
  "checksumUrl": "https://cloud.debian.org/images/cloud/trixie/latest/SHA512SUMS",
  "checksumAlgo": "sha512",
  "initSystem": "systemd",
  "netConfigFormat": "interfaces"
}
```

| Field | Required | What it is |
|---|---|---|
| `id` | yes | Stable identity. Lowercase letters, digits, `-`, `_`, `.`. Changing it creates a second entry rather than renaming one. |
| `kind` | yes | `cloud_image` (a VM boots it), `rootfs` (a system container unpacks it), or `iso` (an installer disc). |
| `distro`, `release`, `arch` | yes | What it is, for grouping and for the create dialog's filters. `arch` is the guest's, e.g. `x86_64`. |
| `displayName` | yes | What a person sees. |
| `sourceUrl` | yes | An `https` URL the **host** fetches, not McHarbor. Nothing crosses the agent transport. |
| `checksumUrl` | one of | The distribution's own digest file. The agent fetches it and finds this artifact's line inside it. |
| `checksum` | one of | A literal digest, when the publisher has no digest file. |
| `checksumAlgo` | no | `sha256` (default) or `sha512`. |
| `signatureUrl` | no | A detached signature, verified against the bundled distribution keyring where one is known. |
| `sizeBytes` | no | Shown before a download starts. Omit it rather than guess. |
| `variant` | no | Free text, e.g. `minimal`. |
| `initSystem` | no | `systemd` or `openrc`. Picks the container profile for a `rootfs`. |
| `netConfigFormat` | no | `systemd-networkd` or `interfaces`. Picks the writer that gives a guest a static address. |

**A checksum is not optional in practice.** A built-in entry's verification
cannot be waived at download time, so an entry with neither `checksumUrl` nor
`checksum` is refused when the file is read; it would be permanently
undownloadable. Prefer `checksumUrl`: a point release moves the artifact, and a
pinned digest then fails verification with nothing to say why.

## What happens when the catalog is read

On every boot, and on **Reload catalog**:

- Each file's `id` is upserted as a built-in entry.
- A built-in entry that is **no longer in the directory** is removed, unless a
  host has already downloaded it, in which case it is kept and the fact is
  logged. The images table cascades into the per-host cache, so removing it
  would discard the record of a file still sitting on that host's disk.
- Images added through the UI are never touched.
- A file that fails validation is skipped and named in the log. One bad file
  never costs you the rest of the catalog.

Rejected outright: unknown JSON fields (a typo in a field name is a mistake, not
an extension), non-`https` URLs, an unknown `kind` or `checksumAlgo`, symlinks,
files over 64 KiB, more than 500 entries, and two files claiming one `id`.

## How the catalog and the downloads are fetched

Both fetches treat the URL as untrusted, because an operator or a catalog entry supplies it.

- **Reading a remote catalog.** The plugin connects only after checking the address it resolved.
  Loopback, link-local (`169.254.0.0/16`, `fe80::/10`), unspecified and multicast addresses and
  the cloud metadata endpoints are refused, including IPv6 forms that carry one (NAT64, 6to4,
  Teredo, ISATAP). Private ranges (RFC 1918 and ULA) stay allowed, so a catalog on your LAN works.
  A redirect is re-checked and may not leave `https`. `HTTP_PROXY` and `HTTPS_PROXY` are ignored.
  An absolute directory path as a source is still read from disk as before.
- **Downloading an image.** The agent applies the same rules to `sourceUrl`, `checksumUrl` and
  `signatureUrl`, with one difference: loopback is refused too, because the agent runs as root
  beside the Docker API and the hypervisor. A mirror on the host itself needs an address other
  than `127.0.0.1`. Redirects are followed up to ten hops, each re-checked and each over `https`.
  This needs an agent built with the change; update native agents from **Settings → Updates → Agents**.

A refused address shows as a failed catalog load or a failed download, with the reason in the
host's log.

## Publishing the official catalog

The official catalog is prepared in the repository's `mnt/compute-catalog/` folder, one JSON
file per entry, and that folder is uploaded as-is to
`https://www.mcharbor.eu/catalog/images/`. Before uploading, build its index:

```bash
node scripts/compute/build-image-index.mjs            # writes mnt/compute-catalog/index.json
node scripts/compute/build-image-index.mjs --check    # fails when index.json is out of date
```

The script applies the same checks as the plugin — known fields only, `https` URLs, a valid
`kind`, `checksumAlgo` and `id`, no duplicate ids, the 500-entry and 4 MiB limits — so a
mistake fails the build instead of turning into a skipped line in someone's reload result. It
names every entry that carries no checksum, and it skips `index.json` itself when it reads the
folder. The index is `{"version": "1.0.0", "images": [ … ]}`; a remote source also accepts a bare
array, or an object with an `entries` array.

## Reload

| Method | Path | Permission | Audit action |
|---|---|---|---|
| `POST` | `/api/compute/images/catalog/reload` | `compute.manage` | `plugin.compute.catalog.reloaded` |

It answers with what it did: how many entries were loaded and removed, and the
reason each skipped file was skipped.

## Related

- [Configuration](./configuration.md): every field, and the shared image library
- [Getting started](./getting-started.md): nothing to a running guest
- [Virtual Machines](../virtual-machines/README.md): what consumes these images
