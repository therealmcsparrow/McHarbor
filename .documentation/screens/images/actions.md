# Images: Actions

Everything you can do to an image (pull, import, export, tag, scan, remove and prune)
and the permission each one needs. Prune is the one to read first: it is a bulk delete of
every image no container currently references.

| Action | Gated by | What it does |
|---|---|---|
| **Pull image** | `images.manage` | Pull a name and tag, using the credentials from Docker → Registries |
| **Import** | `images.manage` | Pull from a registry, or upload a `.tar` from `docker save` |
| **Export** | `images.manage` | Save an image out as an archive; runs in the background. The archive carries the image's full configuration, environment values included, so the button renders disabled, naming the key, without it |
| **Tag** | `images.manage` | Add a repository and tag to an existing image |
| **Remove** | `images.delete` | "This will remove this image from the Docker host" |
| **Prune unused** | `images.manage` | Removes every image no container references |
| **Scan** | `scans.manage` | Runs the configured scanner; needs a scanner plugin |
| **Reveal values** | `images.manage` | Shows the credential-looking environment and build-argument values the Config and History tabs mask; audited as `reveal_env` |

## Prune is a bulk delete with a friendly name

The **unused** badge is computed from current container references. An image referenced by
nothing at all goes, tags included, even one you were about to start a container
from.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Images](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
