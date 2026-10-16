# Plugins

McHarbor is extended through plugins: packages (signed, or unsigned and installed at the
operator's own risk) that add a Go backend, an optional
React interface, dashboard widgets, workflow nodes, database tables, translations, and,
most often, an adapter that fills one of the host's pluggable slots, such as a backup
destination, a notification channel, an identity provider or an image scanner.

This section documents the plugin runtime and **every plugin that ships with McHarbor**,
one folder each, with one exception: the six **add-ons** each have a section of their own
at the top level, because what an add-on adds is a product area and not an integration.
They are still listed below and in the [catalog](./catalog.md).

## Start here

| If you want to | Read |
|---|---|
| Find the plugin you need | [Catalog](./catalog.md) |
| Understand what a plugin is | [Overview](./overview.md) |
| Understand how the runtime loads one | [Architecture](../development/plugins/architecture.md) |
| Write one | [Developing a plugin](../development/plugins/developing.md) |
| Scaffold, check, build and sign one from a terminal | [Plugin authoring from the CLI](../development/plugins/authoring-cli/README.md) |
| Give a plugin its own `mcharbor` commands | [Plugin CLI commands](./cli-commands/README.md) |
| Configure one you already have | The family README below, then the plugin's own folder |
| Check which McHarbor versions a plugin declares it runs on | [Compatibility](./compatibility.md) |
| Let a plugin raise incidents about what it manages | [Incident sources](./incident-sources/README.md) |
| Let the reconciler watch what a plugin manages | [Desired state for plugins](./desired-state/README.md) |

## The families

| Family | Plugins | What a plugin in it does |
|---|---|---|
| [Add-ons](./addons.md) | 6 | Owns a capability end to end: routes, pages, tables, widgets, nodes. **Each has its own section** |
| [AI providers](./ai-providers/README.md) | 18 | Answers a chat completion |
| [Storage destinations](./storage/README.md) | 14 | Stores backup archives; see [Backup](../backup/README.md) for what writes to them |
| [Communication channels](./communication/README.md) | 11 | Delivers a notification |
| [Identity providers](./identity/README.md) | 5 | Signs a user in |
| [Image scanners](./scanners/README.md) | 3 | Reports what is known to be wrong with an image |
| [Audit log forwarders](./logging/README.md) | 2 | Ships the audit trail to a collector |
| [Snapshot drivers](./snapshots/README.md) | 2 | Takes a consistent point in time to back up from; see [Backup](../backup/README.md) |

61 plugins in total.

## What is in a plugin's folder

Every plugin has a folder under its family, with three pages:

| Page | What it covers |
|---|---|
| `README.md` | What it is, where it appears in the product, the permission that gates it, how it works, and the capability flags it advertises |
| `configuration.md` | Prerequisites, every configuration field as the plugin declares it, the setup order, and the host scopes the manifest asks for |
| `troubleshooting.md` | Symptom, cause and fix, plus what its **Test** actually does |

So `plugins/communication/slack/configuration.md` is the Slack channel's field reference.

The fourth page, the plugin's **developer notes** (the adapter it registers, what the host
calls and when, its Go file layout, its full manifest, and how to build, sign and fork it)
lives in the [Development](../development/plugins/README.md) section, one page per plugin under
the same family: `development/plugins/storage/sftp.md` is the SFTP destination's.

## Two conventions worth knowing before you read further

**The host owns the admin surface; the plugin owns the work.** Destinations, channels,
providers, scanners and forwarders are all created, tested and deleted through host
screens, even though the code behind them is a plugin. That is why an adapter plugin
usually declares no routes, and why "where it appears" names a host screen.

**The manifest is the source of truth.** The loader mounts only the routes, widgets, nodes
and permission keys `plugin.yaml` declares. Code that is registered but not declared is
unreachable.

## Where the facts on these pages come from

Every field table, capability table, scope list and manifest table is taken from the
plugin's own `plugin.yaml` and its `backend/*.go`: the adapter spec it registers, its
`ConfigFieldDecl` schema, its capability flags and its endpoint constants. The host-side
behaviour is taken from the loader, the installer, the RBAC catalog and the `routes.go` of
each module that owns a family's admin surface.

Where a repository guide and the code disagree, the code wins and the page says so.
