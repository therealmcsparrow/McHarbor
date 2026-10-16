# Reconciler: Git sources

`/reconciler/sources`

A Git source makes a directory of a repository the desired state: a commit changes
intent, the reconciler notices reality no longer matches, and the watch's response
follows. Drift can go the other way too, as a pull request that changes the intent to
match what runs.

## Documents

Every `.yaml`, `.yml` or `.json` file under the directory is read. A file holds one watch,
or several under `desiredStates:`:

```yaml
desiredStates:
  - name: web
    kind: container
    containerName: web
    imageRef: nginx:1.27
    restartPolicy: unless-stopped
    mode: propose
    config:
      env:
        MODE: prod
  - name: shop
    kind: stack
    stackName: shop
    composeFile: stacks/shop/compose.yaml
```

| Key | Meaning |
|---|---|
| `name` | Required, unique within the source |
| `kind` | `container` (default), `stack` or `environment` |
| `containerName`, `imageRef` | For a container |
| `stackName` | For a stack; `composeFile` (from the repository root) makes that file the expected services instead of the stack McHarbor holds |
| `desiredStatus` | `running` (default), `stopped` or `removed` |
| `restartPolicy`, `config` | As on the screen; `config` takes the same keys as the API |
| `mode` | `observe` (default) or `propose`. `enforce` is refused; enforcement is granted in McHarbor by someone with `reconciler.enforce`, never by a commit |

Watches land in the environment selected when the source was added.

## Syncing

- At the start of every reconciler pass, each enabled source's branch tip is read without
  cloning; the repository is read only when it moved. **Sync now** reads it regardless.
- A sync is **all or nothing**. One invalid document (a missing field, bad config, a
  duplicate name, `mode: enforce`) fails the sync, changes no watch, and puts the file,
  document number and reason behind the source's **Failing** badge.
- A document that disappears from the repository deletes its watch and closes its plans.
- A watch from Git shows a **Git** badge. On the screen only its response can change;
  edit anything else in the repository. It cannot be deleted from the screen either.
- A watch already set to **Enforce** stays so while its document says `propose`; a document
  saying `observe` lowers it.
- **Delete** on a source stops reading from it; its watches stay and become ordinary,
  editable watches.

## Accepting drift in Git

**Accept drift in Git** on a drifted Git-managed watch:

1. rewrites its document to what is running: image, restart policy, env and label values,
   desired status, memory and CPU; for an environment watch, a new file declares the
   undeclared containers (in **Observe**);
2. commits that to a new branch `mcharbor/drift-<watch>-<time>` and pushes it with the
   repository's credential;
3. opens a pull request on GitHub, GitLab or Gitea/Forgejo.

Nothing changes in McHarbor until the request is merged and the next sync reads it. Drift
that cannot be written as intent (a moved digest, a mount, a port, a network, a stack) is
listed as not expressible and stays drift. When the host is not recognised, or the
repository has no token, the branch is still pushed and the dialog says why no request was
opened.

## Related

- [Git, GitOps and the reconciler API](../../api/endpoints/gitops.md): exact routes and document rules
- [Plans and enforcement](./plans-and-enforcement.md)
- [GitOps](../gitops/README.md): deploying from a repository
