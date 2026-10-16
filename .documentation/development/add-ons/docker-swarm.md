# Docker Swarm: Developer

How this plugin is put together, and what to change if you fork it.

## What it registers

It is a feature plugin: it registers routes, menu entries, widgets and workflow nodes
of its own rather than filling an adapter slot. See its manifest table below for what it
declares.

## Source layout

| File | Lines |
|---|---|
| `backend/capability.go` | 243 |
| `backend/compose.go` | 495 |
| `backend/handler_cluster.go` | 524 |
| `backend/handler_configs.go` | 161 |
| `backend/handler_nodes.go` | 204 |
| `backend/handler_secrets.go` | 167 |
| `backend/handler_services.go` | 313 |
| `backend/handler_stacks.go` | 190 |
| `backend/handler_tasks.go` | 93 |
| `backend/main.go` | 484 |
| `backend/model.go` | 815 |
| `backend/nodes.go` | 443 |
| `backend/reports.go` | 555 |
| `backend/reports_blocks.go` | 406 |
| `backend/reports_build.go` | 426 |
| `backend/reports_tables.go` | 724 |
| `backend/service_cluster.go` | 372 |
| `backend/service_configs.go` | 172 |
| `backend/service_nodes.go` | 265 |
| `backend/service_secrets.go` | 191 |
| `backend/service_services.go` | 578 |
| `backend/service_stacks.go` | 721 |
| `backend/service_tasks.go` | 110 |
| `backend/triggers.go` | 227 |

A `frontend/` bundle ships alongside the backend: a UMD build the host loads at
runtime. It supplies the UI this plugin declares.

## Manifest

`plugin.yaml` is the source of truth. The loader mounts only what it declares, so code
that is registered but not declared is unreachable.

| Field | Value |
|---|---|
| `slug` | `addon-swarm` |
| `version` | 2026.09.01 |
| `mcharbor` | `>=2026.09.01` (the host range the installer enforces) |
| `go` | `>=1.23` |
| `license` | Apache-2.0 |
| `permissions` | `docker.socket.read`, `menu.sidebar.add`, `i18n.namespace.add:swarm`, `rbac.permission.register`, `audit.emit`, `reporting.adapter.register` |
| `adds_permissions` | `swarm.view` (env), `swarm.manage` (global), `swarm_services.view` (env), `swarm_services.manage` (env), `swarm_services.scale` (env), `swarm_services.delete` (env), `swarm_tasks.view` (env), `swarm_nodes.view` (env), `swarm_nodes.manage` (env), `swarm_nodes.delete` (env), `swarm_secrets.view` (env), `swarm_secrets.manage` (env), `swarm_secrets.delete` (env), `swarm_configs.view` (env), `swarm_configs.manage` (env), `swarm_configs.delete` (env), `swarm_stacks.view` (env), `swarm_stacks.manage` (stack), `swarm_stacks.delete` (env), `swarm_networks.view` (env) |
| `contributes.i18n_namespaces` | `swarm` |
| `contributes.forms` | _none_ |
| `contributes.widgets` | `swarm-overview`, `swarm-services`, `swarm-nodes` |
| `contributes.nodes` | `swarm-service-scale`, `swarm-service-action`, `swarm-stack-deploy`, `swarm-stack-remove`, `swarm-node-availability`, `swarm-overview`, `swarm-event` |
| `migrations` | _none_ |

## Building it

There are no per-plugin Makefiles in this repository. `src/plugins/Makefile` iterates
`Plugins/*/Makefile`, which matches nothing today, so its `build` and `sign` targets are
no-ops. Build it the way the image does instead:

```bash
cd src/plugins/Plugins/addon-swarm
CGO_ENABLED=1 go build -trimpath -buildmode=plugin -o backend/addon-swarm.so ./backend
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
  --tarball dist/mcharbor-plugin-addon-swarm-1.0.0.tar.gz \
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
5. Rename the i18n namespace (`swarm`) and the `i18n/` folder to match.
6. Rebuild, re-sign with your own key, and add that key to the host's trust list
   (`MCHARBOR_PLUGIN_TRUST_KEYS`).

## Related

- [Docker Swarm](../../docker-swarm/README.md): what it is and how it works
- [Plugin architecture](../plugins/architecture.md): the loader, the ABI check, signing and trust
- [Developing a plugin](../plugins/developing.md): the SDK, the manifest reference and the developer loop
