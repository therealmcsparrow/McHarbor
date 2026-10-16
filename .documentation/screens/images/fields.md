# Images: Fields

Every column, filter, form field and detail tab on Images: what the list shows, what a
pull or import asks for, and what an open image holds, down to its layer history.

## Columns

| Column | What it is |
|---|---|
| Repository:Tag | The full reference |
| Status | In use or unused |
| Repository | Repository alone |
| Tag | Tag alone |
| ID | Image id |
| Size | On-disk size |
| Created | Build time |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Repository and tag |
| View | Table or card |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Image name | text | For a pull, e.g. `nginx:latest` |
| Registry / File | tabs | Import from a registry, or upload a `.tar` archive |

## Detail tabs

| Tab | Fields |
|---|---|
| **Overview** | ID, tags, size, created, architecture, OS, Docker version, author, digests |
| **Config** | Cmd, entrypoint, working directory, user, stop signal, exposed ports, volumes, environment |
| **Labels** | The image's label set |
| **History** | Instruction, size and created, per layer |

## Environment values

The **Config** tab's environment list, `GET /api/images/{id}` and `mcharbor image get` show
every variable's name but replace a credential-looking value in `Config.Env` with
`••••••••`, by the same rule the containers page uses: a name containing `key`, `secret`,
`token`, `passw`, `pwd`, `dsn`, `credential`, `private`, `cookie`, `session`,
`auth_header`, `bearer` or `cert_path`, or a value that is a URL carrying a password or a PEM
block. The **History** tab and `GET /api/images/{id}/history` mask the same values inside
`ENV` steps and inside the `|N NAME=value` build-argument prefix a `RUN` step records.

**Reveal values**, on either tab, fetches the same data with `?reveal=true`. It needs
`images.manage` on the environment and renders disabled, naming the key, without it. Every
reveal writes a `reveal_env` audit entry at warning severity with the entity type `image`.
A value that contains the secret, the encryption key, the database DSN or the backup key
McHarbor itself runs with stays masked even then. Revealed values are never cached in the
browser.

Images have no write path for these values, so nothing can save the marker back. The
**Export** action is not masked: `docker save` writes the image's configuration, `Config.Env`
included, into the archive, which is why it needs `images.manage`, the same key as a reveal.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Images](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
