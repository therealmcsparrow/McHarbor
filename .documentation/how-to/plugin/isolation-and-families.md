# Isolation and plugin families

Every plugin that ships with McHarbor is a self-contained Go module. None imports another,
and there is no shared plugin SDK. That choice produces a lot of deliberate duplication, so
the duplication is written down and watched instead of refactored away.

This page covers the rules a contributor follows when adding a plugin or changing one that
belongs to a family. They apply to every new plugin, and to every change inside the
`addon-*`, `ai-provider-*`, `comm-*`, `identity-*`, `logging-*`, `scanner-*`, `snapshot-*`
and `storage-*` families.

## Why the rules exist

A body-similarity sweep of the repository found about 21,900 redundant lines, 97% of them in
`src/plugins`. One 425-line client had spread to fourteen plugins, and two clients differed
by two lines out of 711. No check had noticed any of it, because nothing was looking.

Most of that duplication stays. The rules below make it visible and keep it from drifting.

## 1. Scaffold a plugin, never copy a sibling

```bash
mcharbor plugin dev new <slug>
```

The template is embedded in the CLI binary. The command rewrites the slug everywhere the
loader compares it: the manifest, the Go module path, the exported `Version` symbol, the
frontend bundle name and the npm package name.

Copying a sibling brings its vendor quirks and bugs along, plus whatever pattern was current
when it was written. That is how one client ended up in fourteen plugins. Reading a shipped
plugin as a worked example is fine. Start the new one from the command.

The slug must begin with a family prefix the packager accepts: `addon-`, `ai-provider-`,
`comm-`, `identity-`, `logging-`, `scanner-`, `snapshot-` or `storage-`.
`plugin dev new` refuses anything else before writing a file.

See [`mcharbor plugin dev new`](../../cli/reference/mcharbor_plugin_dev_new.md) for the
command reference.

## 2. Plugins do not import each other, and there is no plugin SDK

Each plugin's `go.mod` has exactly one `replace` directive, pointing at
`../../../backend`. No code is shared between plugins.

The reason is the release cycle. A package linked into several `.so` files couples them:
editing it invalidates every consumer. Each consumer then needs a version bump delivered
through the Plugin Store's **Update**, so that four values still agree: the database row,
the install directory `<slug>@<version>`, `plugin.yaml` and the `.so` `Version` symbol (see
[ABI and libc](./abi-and-libc.md) and [Packaging](./packaging.md)). A single typo in a
shared client would mean rebuilding and reinstalling a dozen plugins.

Third parties also use the bundled plugins as templates. A self-contained plugin works as a
template. One that imports an internal SDK does not.

A shared extraction such as `plugin-sdk/openaicompat`, `plugin-sdk/webdav` or
`plugin-sdk/msgraph` looks obvious, but it was considered and **withdrawn**. It is not a
pending task.

**Enforced by** `scripts/sanity/plugin-isolation/check.ps1`:

| Check | Fails on |
|---|---|
| 1 | A Go import from one plugin into another |
| 2 | A plugin `go.mod` with a second `replace`, or one that does not point at `../../../backend` |
| 3 | Any `plugin-sdk` import path |
| 4 | Nothing. It reports the host packages the plugins import, because that is the one shared surface that cannot be removed |

The baseline is clean, so any hit is a regression, not an old backlog item.

## 3. `addon-*` is absolute

The six add-ons (`addon-compute`, `addon-kubernetes`, `addon-kvm`, `addon-lxc`,
`addon-reporting`, `addon-swarm`) never depend on another plugin or on shared plugin code,
now or after any future refactor. Their duplication clusters (`widgets/mount.tsx`,
`backend/cli_test.go`, and the frontend helpers and hooks) are accepted **permanently** and
will not be extracted later.

When an add-on imports its own packages, that is a self-reference and is correct. Examples
are `addon-swarm` importing `internal/{response,i18n}` and `addon-reporting` importing its
own `charts`, `pdf`, `fontkit` and `docx`.

## 4. A family is a documented set

The accepted families are listed in `scripts/sanity/duplication/accepted.json`. Each entry
has a family name, a decision reference, a reason, a set of path patterns and the recorded
similarity baselines. The decisions it cites:

| Decision | Meaning |
|---|---|
| D-1 | No shared library code between plugins. Each is its own Go module, and each frontend carries its own copy of `mountGuard.tsx` rather than importing one. |
| D-2 | The six `addon-*` plugins are absolute: permanent, never a refactor target. |
| D-4 | Cross-module mirrors. `src/agent`, `src/cli` and the Terraform provider are separate Go modules and cannot import the host, so a shared contract exists as a copy on each side. |

When a plugin joins a family, add its path to that family's entry **in the same change**,
with a one-line reason. Then record the baseline:

```bash
node scripts/sanity/duplication/detect.mjs --update-baseline
```

If a new member is missing from its family's entry, the duplication check fails with
"Accepted family gained an undeclared member" and names the path. The fix is to add the
path to the entry. Nothing needs refactoring.

**Do not widen a pattern to silence the check.** Patterns are scoped to a family on purpose.
`ai-provider-*/backend/*_client.go` accepts a fifteenth OpenAI-compatible client, but a new
family of clients under a different prefix still fails. A widened pattern would hide the next
undocumented clone family.

## 5. A fix to one family member is checked against the others

Because plugins share no code (rule 2), a bug fixed in one family member may exist in up to
thirteen others. Whoever fixes one names every sibling and says whether each needs the same
fix:

```text
Family: fixed the retry-after parse in ai-provider-groq. Checked the other 13
OpenAI-compatible clients: together, fireworks, cohere, deepseek, openrouter, xai
carry the identical code and need it (done); llamacpp, lmstudio, localai, vllm
have no retry path; gemini, mistral, perplexity use a different client shape.
```

"The others may be affected" does not meet this rule. Name each sibling and give an answer
for each.

After the fix, run the duplication check again and re-baseline. The recorded similarity then
reflects the fix, and it is not reported as drift.

## 6. Drift is the finding that matters

Check 4 of `scripts/sanity/duplication/check.ps1` fails when a member's similarity to its
family reference moves more than **0.03** from the recorded baseline. That means one copy
changed and its siblings did not. Accepting duplication makes this failure possible, and no
other check in the repository can see it.

`-UpdateBaseline` is for a family whose members have all changed together. Running it just
to clear a check-4 finding throws that signal away.

## Checklist

- [ ] A new plugin was scaffolded with `mcharbor plugin dev new`, not copied.
- [ ] No plugin imports another, and nothing imports a `plugin-sdk` path.
- [ ] `go.mod` has exactly one `replace`, pointing at `../../../backend`.
- [ ] A plugin joining a family is listed in that family's `accepted.json` entry, and its
      baseline is recorded in the same change.
- [ ] A fix inside a family names every sibling, with an answer for each.
- [ ] `scripts/sanity/plugin-isolation/check.ps1` passes.
- [ ] `scripts/sanity/duplication/check.ps1` shows no new failing cluster and no
      unexplained drift.

## Related

- [Build a plugin](./README.md): the end-to-end path
- [ABI and libc](./abi-and-libc.md): why shared code between plugins is expensive
- [Packaging](./packaging.md): the version agreement and the Store **Update** path
- [Plugin authoring CLI](../../development/plugins/authoring-cli/README.md): `mcharbor plugin dev`
- [Catalog](../../plugins/catalog.md): every shipped plugin, by family
