# McHarbor-AddOns

Optional extensions for [McHarbor](https://github.com/therealmcsparrow/McHarbor).

This repo ships **two kinds** of add-ons side by side:

- **`Apps/`** — Compose-stack templates that McHarbor's App Store deploys
  as managed stacks. Same format the existing
  [`modules/appstore/`](https://github.com/therealmcsparrow/McHarbor/tree/main/src/backend/modules/appstore)
  already consumes.
- **`Plugins/`** — Go-native extensions that McHarbor's plugin host
  loads into the running binary as Go plugins. See
  [`Plugins/README.md`](./Plugins/README.md) for the full authoring
  guide and [`Plugins/hello-world/`](./Plugins/hello-world) for a
  minimal example.

The top-level [`index.yaml`](./index.yaml) is the signed catalog index
that McHarbor's App Store fetches and renders as the **Apps** and
**Plugins** tabs.

## Layout

```
McHarbor-AddOns/
├── index.yaml                       # signed catalog
├── Apps/
│   ├── nginx/
│   └── postgres/
└── Plugins/
    ├── README.md                    # authoring guide
    ├── hello-world/                 # minimal proof-of-the-loop
    ├── identity-oidc/               # (future) OIDC IdP
    ├── identity-saml/               # (future) SAML IdP
    ├── identity-ldap/               # (future) LDAP IdP
    ├── backup-s3/                   # (future) S3 backup destination
    ├── backup-b2/                   # (future) Backblaze B2
    ├── backup-webdav/               # (future) WebDAV
    └── podman/                      # (future) Podman orchestrator
```

## Building & signing

Each plugin has its own `Makefile`. From a plugin folder:

```bash
make build      # produces backend/<slug>.so + frontend/<slug>.umd.cjs
make sign       # produces <slug>.tar.gz + .sig (requires signing/key.pem)
make all        # build + sign
```

The resulting tarball is uploaded to McHarbor via the App Store UI
(Plugins tab) or directly to `/api/appstore/plugins/install`.

## Contributing

See [`Plugins/README.md`](./Plugins/README.md) for the plugin contract.
Compose app contributions follow the existing
[app-catalog skill](https://github.com/therealmcsparrow/McHarbor/blob/main/.claude/skills/app-catalog).

Every release of `index.yaml` is **signed** (ed25519). The public key
ships with McHarbor in `core/plugin/keys.go`; admins can add their own
keys for self-hosted catalogs. Unsigned or wrong-signature tarballs
are rejected at install time.

## License

McHarbor-AddOns inherits the McHarbor License. See [`LICENSE`](./LICENSE).