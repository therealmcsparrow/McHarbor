# Containers: Fields

Every column, filter, form field and detail tab on Containers: what the list shows, what
creating one asks for, and what each tab on an open container holds.

## Columns

| Column | What it is |
|---|---|
| Name | Container name |
| Image | The image it runs |
| State | Docker state, plus health where a health check exists |
| Ports | Published port mappings |
| Environment | Which host it is on |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Name and image |
| State | Running, stopped, paused, restarting, created, exited, dead, removing |
| Health | Healthy or unhealthy, where a health check exists |
| Environment | When **All Environments** is selected |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Image | text | Repository and tag. "Pulled first when it is not present locally" |
| Name | text | Leave empty for a generated name |
| Pull the image before creating | toggle | Fetch before create |
| Start the container after creating it | toggle | Run it immediately |
| Remove the container when it exits | toggle | The `--rm` behaviour |
| Restart policy | select | What Docker does when it stops |
| Ports | repeater | Host, container, protocol. Both ports must be 1 to 65535 |
| Environment | repeater | Variables passed at start. Must be `KEY=value` pairs |
| Volumes | repeater | Host path or volume name, container path, mode. Both paths required |
| Network | select | Default bridge, an existing network, or a new one created inline |
| Devices → Host Path | select | A device on the host, grouped Serial, GPU, Audio, Video, Input, Network, USB, System, Block; stores the stable `by-id` path when one exists. **Enter manually…** swaps in a text box. The list needs `host.view`. See [Devices](./devices/README.md) |
| Devices → Container Path | text | Filled with the device's node when a device is picked; kept once you edit it. Empty means the host path |
| Devices → Permissions | select | `rwm` (default), `rw`, `r` or `m`, Docker's cgroup read, write and mknod letters |
| Driver, subnet, gateway, internal, attachable | mixed | Shown when creating a network inline. Subnet and gateway are optional; Docker assigns them when left empty |
| Advanced | group | Command, entrypoint, working directory, user, hostname |

## Detail-tab fields

`environment` lists variables and labels alongside user, hostname, command, entrypoint,
working directory, stop signal, TTY and stdin. Its healthcheck section has a **Test**
button that runs the command once inside the running container, as `/bin/sh -c` the way
Docker does, and reports "Healthy" with the time it took or "Unhealthy" with the exit code,
plus the output. A run longer than the configured timeout is flagged, because Docker would
count it as a failure. `network` covers port mappings, exposed
ports, DNS servers and search domains, attached networks, hostname, network mode, IP and
gateway. `mounts` lists type, source, destination, mode, driver and propagation.
`resources` covers the restart policy and retry count, memory limit, swap and
reservation, and the CPU controls: nano CPUs, shares, period, quota. Its **Devices**
section edits the same three device fields as the create form.

## Environment values

The `environment` tab, `GET /api/containers/{id}` and `mcharbor container get` show every
variable's name but replace a credential-looking value with `••••••••`. A value is withheld
when its name contains `key`, `secret`, `token`, `passw`, `pwd`, `dsn`, `credential`,
`private`, `cookie`, `session`, `auth_header`, `bearer`, `cert_path`, `passphrase`, `salt`,
`kubeconfig`, `signing` or `connection_string`, ends in `pass`, contains `_pass_`, or is `auth`
or has `auth` as a whole `_`-separated word (except `AUTH_DISABLE` and `FORCE_SECURE_COOKIES`).
A name match alone does not withhold a value that is only a flag or a small number
(`true`, `false`, `on`, `off`, up to five digits), so `SESSION_TIMEOUT=3600` stays readable.
Whatever the name, a value is withheld when it is a URL carrying a password, a PEM block or
its base64 form, a JDBC or ADO.NET connection string with a password, a `-D…password=` flag,
a GitHub, Slack, AWS or Stripe key, a JSON Web Token, or a Slack, Discord, Teams or Zapier
webhook URL.

**Reveal values** fetches the same container with `?reveal=true` and shows the values. It needs
`containers.manage` on the environment (the key that can already exec into the container or
recreate it) and renders disabled, naming the key, without it. Every reveal writes a
`reveal_env` audit entry at warning severity, naming the container and any value that stayed
withheld. The revealed values are never cached in the browser.

Some values stay withheld from everyone, reveal or not:

- on a McHarbor container (this one, or a peer node recognised by its image, labels or name):
  `MCHARBOR_SECRET`, `ENCRYPTION_KEY`, `MCHARBOR_DB_DSN` and `MCHARBOR_BACKUP_KEY`;
- on any container, a value that contains the secret, the encryption key, the database DSN or
  the backup key McHarbor itself runs with, whatever the variable is called.

Editing keeps what you did not touch: a variable still showing `••••••••` when you save is
written back with its current value, so recreating a container from the redacted editor never
replaces a credential with the marker. Type a new value to change it. A value nobody may
reveal (a pinned key, or a McHarbor container's own keys) can be kept this way only while
the image, command, entrypoint, labels and healthcheck stay as they are: changing one of them
in the same recreate is refused with "A masked value (••••••••) was kept for a service whose
image, command, entrypoint or labels also changed", because it would hand a value you cannot
read to a program you chose.

## Files tab: protected paths

The **Files** tab (list, read, download, save, upload, rename, chmod, delete) checks every path
after normalising it, and on McHarbor containers and containers that mount McHarbor's data
also after following the symlinks in it:

| Path | Who may open it |
|---|---|
| McHarbor's key files: `.secret`, `.encryption_key`, `mcharbor_secret`, `mcharbor_encryption_key`, `mcharbor_backup_key`, `mcharbor_db_dsn`, and whatever `MCHARBOR_SECRET_FILE`, `ENCRYPTION_KEY_FILE`, `MCHARBOR_DB_DSN_FILE` and `BACKUP_ENCRYPTION_KEY_FILE` point at | Nobody, on any container: "McHarbor key files cannot be opened through the file browser" |
| On a McHarbor container: the data directory (`DATA_DIR`, normally `/app/data`; the database, its WAL and `tls/key.pem` / `tls/ca-key.pem` live there), `/run/secrets`, and the directories of the `_FILE` paths | `system.manage` (global), as for a protected container's terminal |
| On any container: a mount whose source is McHarbor's own data volume | `system.manage` |
| `/proc` on any container (it holds every process's environment) | `system.manage` |

A refusal answers `403` "This path holds McHarbor's database, keys or secrets and requires the
system.manage permission" and is audited as `<action>_denied` (for example
`secret_file_read_denied`) with the path and the reason. A permitted `system.manage` access is
audited too, at warning severity.

## Exec history

`GET /api/containers/{id}/exec/history` is `containers.view`, but a row carries the command
and its stdout and stderr, which only an exec (`containers.manage`) could have produced. A
caller with `containers.manage` on the environment sees every user's rows; anyone else sees
only their own. When the history cannot be loaded, the panel shows the server's error message with a
**Retry** button instead of an empty list.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Containers](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
