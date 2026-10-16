# Containers

**Settings → Updates → Containers** · `?tab=updates-containers`

Policies that check for and apply new container images on a schedule.

## Permissions

| Key | What it unlocks |
|---|---|
| `updates.view` | See policies |
| `updates.manage` | Create and change them |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- Which containers and images a policy matches, by selection or by glob.
- When it checks, how it decides an update exists, and whether it restarts afterwards.

## Update strategies

| Strategy | Decides an update exists when |
|---|---|
| **Latest tag** | The `:latest` tag now points at a different image |
| **Semantic versioning** | A higher semver tag is available |
| **Image digest** | The digest behind the same tag has changed |

Digest is the strict one: it catches a rebuilt `:1.2.3` that was silently republished, which
the other two miss. Semver is the predictable one: it will not move you across a major
version unless you ask.

## Auto-restart

Pulling an image changes nothing about a running container. Without **auto-restart**, the
policy leaves a newer image on disk and the old one still running. That is a legitimate choice when
you want to control the restart yourself, and a common source of "the update did not work"
when it is not.

## Where a policy fights something else

An auto-update policy and a [reconciler](../../../screens/reconciler/README.md) declaration
pinning an image will undo each other. So will a policy and a GitOps pipeline deploying a
pinned tag. Pick one owner per container.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [Agents](../agents/README.md) and [McHarbor](../mcharbor/README.md): the other two update audiences
- [Containers](../../../screens/containers/README.md): the per-container Updates tab
- [Reconciler](../../../screens/reconciler/README.md): the other thing that decides what a container runs
