# Environment variables

`/enterprise/environment-variables`, permission `settings.view`

A searchable list of every environment variable the McHarbor server and the remote agent
read. It is the same information as [Configuration](../../getting-started/configuration/README.md),
joined with what the code actually reads, and it needs no API call: the list is built into
the frontend.

## Where the list comes from

`node scripts/reference/generate-env-reference.mjs` writes
`src/frontend/modules/enterprise/data/env-reference.json` from:

- the tables in `.documentation/getting-started/configuration/README.md` (description,
  default, section);
- `env:"…"` struct tags in `src/backend/**` (with `envDefault`), and every `os.Getenv` /
  `os.LookupEnv` call, including ones that pass a named constant;
- string literals in `src/agent/**`, to confirm the agent variables the reference lists.

Run it after adding a variable or changing the configuration page. `--check` exits non-zero
when the checked-in file is stale.

## Controls

- **Search** matches the name and the description.
- **Scope**: All, Server, Agent.
- **Sensitive only**: only variables that hold a secret.
- The copy button next to each name copies it; on plain HTTP the manual-copy dialog opens instead.

## Flags

| Flag | Meaning |
|---|---|
| Sensitive | The name marks a secret (`SECRET`, `PASSWORD`, `PASSPHRASE`, `TOKEN`, `DSN`, `LICENSE_KEY`, `ENCRYPTION_KEY`, `BACKUP_KEY`), except names ending `_FILE`, `_PATH`, `_DIR`, `_CHECK`, `_ID` or `_CONTAINER` |
| Recreate required | Every variable is read at start-up; a change applies only after the container is recreated |
| Undocumented | Read by the code but not in the configuration reference, mostly variables McHarbor sets itself for its self-update and backup-key helper containers |
| Not read by the binary | Documented, but read by the entrypoint or a Compose file (`PUID`, `PGID`, `MCHARBOR_LOCAL_CA_PATH`), or not read at all |

Descriptions are reference data from the configuration page and are shown in English.
