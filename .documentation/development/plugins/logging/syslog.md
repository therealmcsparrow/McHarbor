# Syslog: Developer

How this plugin is put together, and what to change if you fork it.

## What it registers

It implements `AdapterContributor` and registers one adapter in `RegisterAdapters`:

```go
func (p *Plugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
    return reg.Logging().Register(
        plugin.LoggingAdapterSpec{
            Slug:         p.manifest.Slug,
            DisplayName:  "Syslog (RFC5424)",
            Icon:         "plugin.svg",
            Capabilities: plugin.LoggingAdapterCapabilities{ /* … */ },
            ConfigSchema: configSchema(),
        },
        newClient(),
    )
}
```

The spec is what the host renders and dispatches on; the client is what does the work.
`RegisterAdapters` also registers a report provider through `reg.Reporting()`: one type, `logging-syslog-delivery` ("Syslog forwarding delivery"), which reads the plugin's own delivery history. It is listed in the [report catalog](../../../reporting/reports/README.md). A failed report registration is logged in `Init` rather than returned, so a clash on the report shelf never stops audit forwarding from loading. The report checks `audit.view` and `settings.manage`, the same pair that guards the Logging tab's delivery history.

Every other `Register*` method on the `Plugin` interface is a no-op here.

## What the host calls, and when

| Call | When | What it returns |
|---|---|---|
| `Forward` | The forwarder sweep runs on the elected leader | Per-event delivery results |
| `History` | You open the forwarder history | Recent attempts and their outcomes |
| `Test` | You press **Test** | One real delivery |

## Source layout

| File | Lines |
|---|---|
| `backend/forwarder.go` | 589 |
| `backend/main.go` | 198 |

This plugin ships no frontend bundle. The host renders its configuration form from the
`ConfigSchema` the adapter spec declares.

## Manifest

`plugin.yaml` is the source of truth. The loader mounts only what it declares, so code
that is registered but not declared is unreachable.

| Field | Value |
|---|---|
| `slug` | `logging-syslog` |
| `version` | 2026.09.01 |
| `mcharbor` | `>=2026.09.01`, the host range the installer enforces |
| `go` | `>=1.23` |
| `license` | Apache-2.0 |
| `permissions` | `logging.adapter.register`, `db.migrations.run`, `audit.emit` |
| `adds_permissions` | _none_ |
| `consumes_permissions` | `audit.read` |
| `contributes.i18n_namespaces` | `logging-syslog` |
| `contributes.forms` | _none_ |
| `migrations` | `0001_init.sql`, `0002_forwarder_id.sql` |

## Building it

There are no per-plugin Makefiles in this repository. `src/plugins/Makefile` iterates
`Plugins/*/Makefile`, which matches nothing today, so its `build` and `sign` targets are
no-ops. Build it the way the image does instead:

```bash
cd src/plugins/Plugins/logging-syslog
CGO_ENABLED=1 go build -trimpath -buildmode=plugin -o backend/logging-syslog.so ./backend
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
  --tarball dist/mcharbor-plugin-logging-syslog-1.0.0.tar.gz \
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
2. Change the forwarder type (currently `syslog`) so it does not
   collide with this plugin's. Two plugins cannot claim the same key.
3. Declare any permission keys you add in `adds_permissions`, with a scope no wider than
   the resource they gate.
4. Update `ConfigSchema` for the fields your service needs, then the client that reads
   them.
5. Rename the i18n namespace (`logging-syslog`) and the `i18n/` folder to match.
6. Rebuild, re-sign with your own key, and add that key to the host's trust list
   (`MCHARBOR_PLUGIN_TRUST_KEYS`).

## Related

- [Syslog](../../../plugins/logging/syslog/README.md): what it is and how it works
- [Plugin architecture](../architecture.md): the loader, the ABI check, signing and trust
- [Developing a plugin](../developing.md): the SDK, the manifest reference and the developer loop
