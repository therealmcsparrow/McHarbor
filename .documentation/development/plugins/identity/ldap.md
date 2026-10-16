# LDAP / Active Directory: Developer

How this plugin is put together, and what to change if you fork it.

## What it registers

It implements `AdapterContributor` and registers one adapter in `RegisterAdapters`:

```go
func (p *Plugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
    return reg.Identity().Register(
        plugin.IdentityAdapterSpec{
            Slug:         p.manifest.Slug,
            DisplayName:  "LDAP / Active Directory",
            Icon:         "plugin.svg",
            Capabilities: plugin.IdentityAdapterCapabilities{ /* … */ },
            ConfigSchema: configSchema(),
        },
        newClient(),
    )
}
```

The spec is what the host renders and dispatches on; the client is what does the work.
Every other `Register*` method on the `Plugin` interface is a no-op here.

## The route it registers

`RegisterRoutes` mounts `POST /api/identity-ldap/auth` with `reg.Auth`, and
`plugin.yaml` declares it `public: true`. The Auth class gives it the host's login rate
limit; `public` keeps it free of any permission check. It lives under the plugin's own
slug because the host's `/api/identity-providers` subtree answers `404` for every path
it does not serve itself, so the route's first address never reached the plugin.

The handler only talks to the directory. Everything around the bind belongs to the host,
reached through a capability the plugin declares **locally** in
`backend/external_login.go` and type-asserts on `host.Identity`:

```go
type externalLoginHost interface {
    LDAPProviderConfig(ctx, providerID) (map[string]string, error)
    BeginExternalLogin(ctx, w, r, providerID, username) error
    CheckExternalAccount(ctx, w, r, providerID, username, externalID, email) error
    FailExternalLogin(ctx, w, r, providerID, username, externalID, reason)
    CompleteExternalLogin(ctx, w, r, providerID, externalID, username, email, displayName, groups) error
}
```

The interface is not in `core/plugin`, and its signatures use only standard-library
types, so adding it changed no type the plugin ABI covers and no installed `.so` had to
be rebuilt. The host side is `ExternalLoginSource` in `core/pluginruntime`,
implemented by `modules/auth.ExternalLogin` (lockout, `login_attempts`, audit, MFA,
enrolment, session) over `modules/identity.HostProvisioner` (provider config, account
linking, provisioning, group sync). The host hands it only to the plugin that registered
the `ldap` provider type; any other plugin gets `403`.

| Step | Call | On refusal |
|---|---|---|
| 1 | `BeginExternalLogin`: per-address lock | host writes `423`; the directory is never contacted |
| 2 | `LDAPProviderConfig`: the provider's config, secrets decrypted host-side | `FailExternalLogin(…, "directory_unavailable")` → `503` |
| 3 | open the directory, service bind, search for the user | unknown user → `FailExternalLogin(…, "invalid_credentials")`; fault → `directory_unavailable` |
| 4 | `CheckExternalAccount`: linked account locked or disabled | host writes `423` / `403`; no user bind |
| 5 | bind as the user | result code 49 → `invalid_credentials` (counted); anything else → `directory_unavailable` (not counted) |
| 6 | `CompleteExternalLogin`: provision or link, then the password login's own completion | host writes the reply: session cookie, MFA challenge, or enrolment flag |

Every method except `LDAPProviderConfig` writes the HTTP response itself; a non-nil error
means it already has. The plugin never sets a cookie. On a host without the capability
the route answers `503` `host_unsupported` before any directory contact.

The directory is behind a small `ldapDirectory` / `ldapSession` interface in
`backend/directory.go`, which is what `backend/auth_flow_test.go` fakes to check the
ordering above without an LDAP server.

## What the host calls, and when

| Call | When | What it returns |
|---|---|---|
| `Begin` | A user picks this provider on the login page | The URL to redirect the browser to |
| `Callback` | The IdP redirects back | The authenticated identity |
| `Groups` | The group mapper runs, where the adapter supports it | The groups the user belongs to |
| `Metadata` | An IdP asks for SP metadata, where the adapter serves it | The metadata document |

## Source layout

| File | Lines |
|---|---|
| `backend/directory.go` | 154 |
| `backend/external_login.go` | 49 |
| `backend/ldap_client.go` | 268 |
| `backend/main.go` | 361 |

A `frontend/` bundle ships alongside the backend: a UMD build the host loads at
runtime. It supplies the `config_form` and `setup_guide` this plugin declares.

## Manifest

`plugin.yaml` is the source of truth. The loader mounts only what it declares, so code
that is registered but not declared is unreachable.

| Field | Value |
|---|---|
| `slug` | `identity-ldap` |
| `version` | 2026.09.02 |
| `mcharbor` | `>=2026.09.01`, the host range the installer enforces |
| `go` | `>=1.23` |
| `license` | Apache-2.0 |
| `permissions` | `identity.adapter.register`, `identity.read`, `identity.write`, `audit.emit` |
| `adds_permissions` | _none_ |
| `contributes.i18n_namespaces` | `identity-ldap` |
| `contributes.forms` | `identity_provider/config_form/ldap`, `identity_provider/setup_guide/ldap` |
| `migrations` | _none_ |

## Building it

There are no per-plugin Makefiles in this repository. `src/plugins/Makefile` iterates
`Plugins/*/Makefile`, which matches nothing today, so its `build` and `sign` targets are
no-ops. Build it the way the image does instead:

```bash
cd src/plugins/Plugins/identity-ldap
CGO_ENABLED=1 go build -trimpath -buildmode=plugin -o backend/identity-ldap.so ./backend
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
  --tarball dist/mcharbor-plugin-identity-ldap-1.0.0.tar.gz \
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
2. Change the provider type (currently `ldap`) so it does not
   collide with this plugin's. Two plugins cannot claim the same key.
3. Declare any permission keys you add in `adds_permissions`, with a scope no wider than
   the resource they gate.
4. Update `ConfigSchema` for the fields your service needs, then the client that reads
   them.
5. Rename the i18n namespace (`identity-ldap`) and the `i18n/` folder to match.
6. Rebuild, re-sign with your own key, and add that key to the host's trust list
   (`MCHARBOR_PLUGIN_TRUST_KEYS`).

## Related

- [LDAP / Active Directory](../../../plugins/identity/ldap/README.md): what it is and how it works
- [Plugin architecture](../architecture.md): the loader, the ABI check, signing and trust
- [Developing a plugin](../developing.md): the SDK, the manifest reference and the developer loop
