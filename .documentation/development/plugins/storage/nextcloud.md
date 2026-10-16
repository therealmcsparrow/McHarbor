# Nextcloud: Developer

How this plugin is put together, and what to change if you fork it.

## What it registers

It implements `AdapterContributor` and registers one adapter in `RegisterAdapters`:

```go
func (p *Plugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
    return reg.Storage().Register(
        plugin.StorageAdapterSpec{
            Slug:         p.manifest.Slug,
            DisplayName:  "Nextcloud",
            Icon:         "plugin.svg",
            Capabilities: plugin.StorageAdapterCapabilities{ /* … */ },
            ConfigSchema: configSchema(),
        },
        newClient(),
    )
}
```

The spec is what the host renders and dispatches on; the client is what does the work.
Every other `Register*` method on the `Plugin` interface is a no-op here.

## What the host calls, and when

| Call | When | What it returns |
|---|---|---|
| `Test` | You press **Test** on the destination | A write, change and delete round trip |
| `Upload` | A backup writes an archive | Success, or the transport error |
| `Download` | A restore reads one back | The archive stream |
| `List` | Selective restore browses the destination | What is stored there |
| `Delete` | Retention prunes | Success, or the transport error |

## Source layout

| File | Lines |
|---|---|
| `backend/main.go` | 190 |
| `backend/nextcloud_client.go` | 977 |
| `backend/nextcloud_quota.go` | 98 |

A `frontend/` bundle ships alongside the backend: a UMD build the host loads at
runtime. It supplies the `config_form` and `setup_guide` this plugin declares.

## Manifest

`plugin.yaml` is the source of truth. The loader mounts only what it declares, so code
that is registered but not declared is unreachable.

| Field | Value |
|---|---|
| `slug` | `storage-nextcloud` |
| `version` | 2026.09.01 |
| `mcharbor` | `>=2026.09.01`, the host range the installer enforces |
| `go` | `>=1.23` |
| `license` | Apache-2.0 |
| `permissions` | `storage.adapter.register`, `audit.emit` |
| `adds_permissions` | _none_ |
| `contributes.i18n_namespaces` | `storage-nextcloud` |
| `contributes.forms` | `storage_location/config_form/nextcloud`, `storage_location/setup_guide/nextcloud` |
| `migrations` | _none_ |

## Building it

There are no per-plugin Makefiles in this repository. `src/plugins/Makefile` iterates
`Plugins/*/Makefile`, which matches nothing today, so its `build` and `sign` targets are
no-ops. Build it the way the image does instead:

```bash
cd src/plugins/Plugins/storage-nextcloud
CGO_ENABLED=1 go build -trimpath -buildmode=plugin -o backend/storage-nextcloud.so ./backend
```

The `.so` has to be built against the **same** Go toolchain and module graph as the host
binary, or `plugin.Open` rejects it with `plugin was built with a different version of
package …`. That is why `docker/Dockerfile` compiles the host binary and every bundled
plugin inside one `RUN`, sharing one module cache and one build cache.

For a plugin that ships in the image, the only reliable way to get a rebuild into a
running McHarbor is to rebuild the image:

```bash
docker compose build && docker compose up -d
```

The entrypoint then copies the bundled `.so` over the installed one and re-extracts the
matching tarball payload, so the frontend bundle, i18n and icons move with it.

## Signing it

Only needed to distribute outside the image. Signing uses ed25519 through PyNaCl:

```bash
python -m venv .venv
.venv/bin/pip install -r src/plugins/scripts/requirements-signing.txt
.venv/bin/python src/plugins/scripts/sign-plugin.py \
  --tarball dist/mcharbor-plugin-storage-nextcloud-1.0.1.tar.gz \
  --key signing/plugin-signing.key
```

It writes a base64 ed25519 signature next to the tarball as `.sig`, which is exactly what
the installer verifies. Keep the private half out of the repository, and add the public
half to the host's trust list through `MCHARBOR_PLUGIN_TRUST_KEYS`.

Install the tarball from **App Store → Plugins**, or drop it into a catalog directory
the host reads.

## Forking it

1. Copy the folder to a new slug and change `slug`, `name` and `homepage` in
   `plugin.yaml`. The slug has to match the directory name.
2. Change the destination type (currently `nextcloud`) so it does not
   collide with this plugin's. Two plugins cannot claim the same key.
3. Declare any permission keys you add in `adds_permissions`, with a scope no wider than
   the resource they gate.
4. Update `ConfigSchema` for the fields your service needs, then the client that reads
   them.
5. Rename the i18n namespace (`storage-nextcloud`) and the `i18n/` folder to match.
6. Rebuild, re-sign with your own key, and add that key to the host's trust list
   (`MCHARBOR_PLUGIN_TRUST_KEYS`).

## Related

- [Nextcloud](../../../plugins/storage/nextcloud/README.md): what it is and how it works
- [Plugin architecture](../architecture.md): the loader, the ABI check, signing and trust
- [Developing a plugin](../developing.md): the SDK, the manifest reference and the developer loop
