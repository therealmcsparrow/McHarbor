# Plugin Development

Everything about building, changing and debugging a plugin. It covers the host's runtime, the
author's toolchain, and one developer page for every plugin that ships with McHarbor. What a
plugin *does* for an operator, including its configuration fields and troubleshooting, stays
in the [Plugins](../../plugins/README.md) section. This section covers how it is put together.

## Pages

| Page | Covers |
|---|---|
| [Developing a plugin](./developing.md) | Writing one from scratch: the folder, the manifest, the Go side, the frontend bundle, the loop you work in |
| [Architecture](./architecture.md) | What a plugin is at runtime, what it may import, where its files live, how it is trusted, what the host mounts, and why a plugin refuses to load |
| [Plugin authoring from the CLI](./authoring-cli/README.md) | `mcharbor plugin dev`: scaffold, validate, build, package and sign, and the traps each check exists for |

The task-shaped guide lives in [How-to → Build a plugin](../../how-to/plugin/README.md). It
covers the manifest, backend, frontend, packaging, ABI and libc, and isolation and families.

## Quick start

1. Scaffold with `mcharbor plugin dev new`. Never copy a sibling plugin. See
   [isolation and families](../../how-to/plugin/isolation-and-families.md).
2. Read the developer page below for the shipped plugin closest to what you are building.
3. Build and deliver through `scripts/development/catalog.sh build <slug>`, then update the
   plugin from the Plugin Store. A rebuilt `.so` alone fails the version-agreement check. See
   [Architecture](./architecture.md).

## Per-plugin developer pages

Each page names the adapter the plugin registers, what the host calls and when, its Go file
layout and its full manifest, and how to build, sign and fork it. The operator-facing pages for
the same plugin are linked from the top of each one.

The six add-ons have their developer pages under [Add-ons](../add-ons/README.md).

### AI providers

| Plugin |
|---|
| [Anthropic Claude](./ai-providers/anthropic.md) |
| [Cohere](./ai-providers/cohere.md) |
| [DeepSeek](./ai-providers/deepseek.md) |
| [Fireworks AI](./ai-providers/fireworks.md) |
| [Google Gemini](./ai-providers/gemini.md) |
| [Groq](./ai-providers/groq.md) |
| [llama.cpp](./ai-providers/llamacpp.md) |
| [LM Studio](./ai-providers/lm-studio.md) |
| [LocalAI](./ai-providers/localai.md) |
| [MiniMax](./ai-providers/minimax.md) |
| [Mistral](./ai-providers/mistral.md) |
| [Ollama](./ai-providers/ollama.md) |
| [OpenAI](./ai-providers/openai.md) |
| [OpenRouter](./ai-providers/openrouter.md) |
| [Perplexity](./ai-providers/perplexity.md) |
| [Together AI](./ai-providers/together.md) |
| [vLLM](./ai-providers/vllm.md) |
| [xAI Grok](./ai-providers/xai.md) |

### Communication channels

| Plugin |
|---|
| [Discord](./communication/discord.md) |
| [Exchange](./communication/exchange.md) |
| [Gmail](./communication/gmail.md) |
| [Gotify](./communication/gotify.md) |
| [Microsoft Teams](./communication/microsoft-teams.md) |
| [ntfy](./communication/ntfy.md) |
| [Signal](./communication/signal.md) |
| [Slack](./communication/slack.md) |
| [SMTP](./communication/smtp.md) |
| [Telegram](./communication/telegram.md) |
| [WhatsApp](./communication/whatsapp.md) |

### Identity providers

| Plugin |
|---|
| [Microsoft Entra ID](./identity/entra-id.md) |
| [Generic OpenID Connect](./identity/generic-oidc.md) |
| [Google](./identity/google.md) |
| [LDAP / Active Directory](./identity/ldap.md) |
| [SAML 2.0](./identity/saml.md) |

### Audit log forwarders

| Plugin |
|---|
| [SIEM](./logging/siem.md) |
| [Syslog](./logging/syslog.md) |

### Image scanners

| Plugin |
|---|
| [Clair](./scanners/clair.md) |
| [Grype](./scanners/grype.md) |
| [Trivy](./scanners/trivy.md) |

### Snapshot drivers

| Plugin |
|---|
| [Synology DSM](./snapshots/synology.md) |
| [TrueNAS](./snapshots/truenas.md) |

### Storage destinations

| Plugin |
|---|
| [Box](./storage/box.md) |
| [Dropbox](./storage/dropbox.md) |
| [FTP/FTPS](./storage/ftp.md) |
| [Google Drive](./storage/google-drive.md) |
| [Nextcloud](./storage/nextcloud.md) |
| [NFS](./storage/nfs.md) |
| [OneDrive for Business](./storage/onedrive-business.md) |
| [OneDrive (Personal)](./storage/onedrive-personal.md) |
| [OpenCloud](./storage/opencloud.md) |
| [ownCloud](./storage/owncloud.md) |
| [Samba](./storage/samba.md) |
| [SFTP](./storage/sftp.md) |
| [SharePoint Online](./storage/sharepoint-online.md) |
| [TrueNAS](./storage/truenas.md) |

## What holds for every plugin

- **Scaffold, never copy.** Start with `mcharbor plugin dev new`. Plugins share no library
  code: no plugin imports another plugin or a shared plugin SDK, and nothing imports an
  `addon-*` plugin. A fix to one plugin in a family is checked against every sibling.
- **The host knows no plugin by name.** Nothing under `src/backend/core`,
  `src/backend/modules`, `src/frontend/core`, `src/frontend/modules` or
  `src/frontend/resources` names a plugin slug, route or menu label. It asks the registries
  in [Hooks](../hooks/README.md) instead.
- **Permissions come from the manifest.** A plugin declares its keys in `adds_permissions` in
  `plugin.yaml`, and never adds to the host catalog in `src/backend/core/rbac/`.
- **A version bump moves four values together**: the database row, the install directory, the
  manifest and the `.so` symbol. It goes through the Plugin Store's **Update**, not a text
  edit. See [Contract tests → Plugins](../contracts/tests.md#plugins).
