# Compute Foundation: Developer

How this plugin is put together, and what to change if you fork it.

## What it registers

It is a feature plugin: it registers routes, menu entries, widgets and workflow nodes
of its own rather than filling an adapter slot. See its manifest table below for what it
declares.

## Source layout

| File | Lines |
|---|---|
| `backend/alerts.go` | 282 |
| `backend/handler.go` | 592 |
| `backend/main.go` | 347 |
| `backend/model.go` | 266 |
| `backend/nodes.go` | 375 |
| `backend/reports.go` | 885 |
| `backend/reports_hygiene.go` | 667 |
| `backend/scan.go` | 187 |
| `backend/service.go` | 1165 |

A `frontend/` bundle ships alongside the backend: a UMD build the host loads at
runtime. It supplies the UI this plugin declares.

## Manifest

`plugin.yaml` is the source of truth. The loader mounts only what it declares, so code
that is registered but not declared is unreachable.

| Field | Value |
|---|---|
| `slug` | `addon-compute` |
| `version` | 2026.09.01 |
| `mcharbor` | `>=2026.09.01` (the host range the installer enforces) |
| `go` | `>=1.25` |
| `license` | Apache-2.0 |
| `installed_as` | `dependency` |
| `required_by` | `addon-kvm`, `addon-lxc` |
| `permissions` | `menu.sidebar.add`, `i18n.namespace.add:compute`, `rbac.permission.register`, `db.migrations.run`, `audit.emit`, `agent.rpc`, `reporting.adapter.register`, `alerting.adapter.register`, `ai.invoke` |
| `adds_permissions` | `compute.view` (env), `compute.manage` (env), `compute.delete` (env) |
| `contributes.i18n_namespaces` | `compute` |
| `contributes.forms` | _none_ |
| `contributes.widgets` | `compute-capacity`, `compute-hosts` |
| `contributes.nodes` | `compute-host-capability`, `compute-pool-capacity`, `compute-ensure-image` |
| `migrations` | `0001_init.sql` |

## Building it

There are no per-plugin Makefiles in this repository. `src/plugins/Makefile` iterates
`Plugins/*/Makefile`, which matches nothing today, so its `build` and `sign` targets are
no-ops. Build it the way the image does instead:

```bash
cd src/plugins/Plugins/addon-compute
CGO_ENABLED=1 go build -trimpath -buildmode=plugin -o backend/addon-compute.so ./backend
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
  --tarball dist/mcharbor-plugin-addon-compute-1.0.0.tar.gz \
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
2. Change the route prefix and the sidebar section id so it does not
   collide with this plugin's. Two plugins cannot claim the same key.
3. Rename the permission keys in `adds_permissions`. A plugin cannot shadow another
   plugin's key, and the host skips a duplicate.
4. Update `ConfigSchema` for the fields your service needs, then the client that reads
   them.
5. Rename the i18n namespace (`compute`) and the `i18n/` folder to match.
6. Rebuild, re-sign with your own key, and add that key to the host's trust list
   (`MCHARBOR_PLUGIN_TRUST_KEYS`).

## Related

- [Compute Foundation](../../compute-foundation/README.md): what it is and how it works
- [Plugin architecture](../plugins/architecture.md): the loader, the ABI check, signing and trust
- [Developing a plugin](../plugins/developing.md): the SDK, the manifest reference and the developer loop
