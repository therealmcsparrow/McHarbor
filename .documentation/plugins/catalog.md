# Plugin Catalog

Every plugin that ships with McHarbor, by family. Each one has a folder carrying four
pages: what it is, its configuration, its troubleshooting table, and its developer notes.

61 plugins across 8 families.

## Add-ons

Own a capability end to end: routes, pages, tables, widgets, nodes. Each has a section of its own; [Add-ons](./addons.md) is the plugin section's view of them.

| Plugin | Slug | Version | What it is |
|---|---|---|---|
| [Compute Foundation](../compute-foundation/README.md) | `addon-compute` | 1.0.0 | The shared foundation for running virtual machines and system containers on your own hosts: node capability probing, storage pools, guest networks and the image catalog. |
| [Docker Swarm](../docker-swarm/README.md) | `addon-swarm` | 1.0.0 | Manage Docker Swarm from McHarbor: services, tasks, nodes, secrets, configs, swarm stacks and overlay networks, with cluster init, join, leave, join-token and unlock-key lifecycle, service scaling, rolling restarts and rollback. |
| [Kubernetes](../kubernetes/README.md) | `addon-kubernetes` | 1.0.0 | Manage Kubernetes clusters from McHarbor: pods, deployments and other workloads, services and networking, storage, config, namespaces, nodes and events, with pod logs, deployment scaling and rolling restarts. |
| [Reporting](../reporting/README.md) | `addon-reporting` | 1.0.0 | Generate board-presentable reports on environments, workloads, incidents, security, capacity, delivery and recovery. |
| [System Containers](../system-containers/README.md) | `addon-lxc` | 1.0.0 | Run full-system LXC containers on your own hosts: a whole distribution with its own init, users and services, in a fraction of a VM's memory and starting in about a second. |
| [Virtual Machines](../virtual-machines/README.md) | `addon-kvm` | 1.0.0 | Run KVM virtual machines on your own hosts. Create from a cloud image and reach a login prompt in the browser in under two minutes, or install from an ISO. |

## AI providers

Answer a chat completion ([family overview](./ai-providers/README.md))

| Plugin | Slug | Version | What it is |
|---|---|---|---|
| [Anthropic Claude](./ai-providers/anthropic/README.md) | `ai-provider-anthropic` | 1.0.0 | Routes McHarbor AI chat completions through Anthropic /v1/messages. |
| [Cohere](./ai-providers/cohere/README.md) | `ai-provider-cohere` | 1.0.0 | Routes McHarbor AI chat completions through Cohere's OpenAI compatibility API. |
| [DeepSeek](./ai-providers/deepseek/README.md) | `ai-provider-deepseek` | 1.0.0 | Routes McHarbor AI chat completions through DeepSeek's OpenAI-compatible /v1/chat/completions endpoint. |
| [Fireworks AI](./ai-providers/fireworks/README.md) | `ai-provider-fireworks` | 1.0.0 | Routes McHarbor AI chat completions through Fireworks AI's OpenAI-compatible inference API for hosted open-weights models. |
| [Google Gemini](./ai-providers/gemini/README.md) | `ai-provider-gemini` | 1.0.0 | Routes McHarbor AI chat completions through Google Gemini's OpenAI-compatible endpoint. |
| [Groq](./ai-providers/groq/README.md) | `ai-provider-groq` | 1.0.0 | Routes McHarbor AI chat completions through Groq's OpenAI-compatible LPU inference API. |
| [llama.cpp](./ai-providers/llamacpp/README.md) | `ai-provider-llamacpp` | 1.0.0 | Routes McHarbor AI chat completions through a self-hosted llama.cpp server (llama-server) over its OpenAI-compatible /v1 endpoint. |
| [LM Studio](./ai-providers/lm-studio/README.md) | `ai-provider-lmstudio` | 1.0.0 | Routes McHarbor AI chat completions through a local LM Studio server over its OpenAI-compatible /v1 endpoint. |
| [LocalAI](./ai-providers/localai/README.md) | `ai-provider-localai` | 1.0.0 | Routes McHarbor AI chat completions through a self-hosted LocalAI instance over its OpenAI-compatible /v1 endpoint. |
| [MiniMax](./ai-providers/minimax/README.md) | `ai-provider-minimax` | 1.0.0 | Routes McHarbor AI chat completions through MiniMax's OpenAI-compatible /v1/chat/completions endpoint. |
| [Mistral](./ai-providers/mistral/README.md) | `ai-provider-mistral` | 1.0.0 | Routes McHarbor AI chat completions through Mistral's OpenAI-compatible /v1/chat/completions endpoint. |
| [Ollama](./ai-providers/ollama/README.md) | `ai-provider-ollama` | 1.0.0 | Routes McHarbor AI chat completions through a self-hosted Ollama instance using the native /api/tags and /api/chat endpoints. |
| [OpenAI](./ai-providers/openai/README.md) | `ai-provider-openai` | 1.0.0 | Routes McHarbor AI chat completions through OpenAI /v1/chat/completions. |
| [OpenRouter](./ai-providers/openrouter/README.md) | `ai-provider-openrouter` | 1.0.0 | Routes McHarbor AI chat completions through OpenRouter, a single OpenAI-compatible gateway in front of hundreds of hosted models from many vendors. |
| [Perplexity](./ai-providers/perplexity/README.md) | `ai-provider-perplexity` | 1.0.0 | Routes McHarbor AI chat completions through Perplexity's OpenAI-compatible Sonar API, which answers with live web search grounding. |
| [Together AI](./ai-providers/together/README.md) | `ai-provider-together` | 1.0.0 | Routes McHarbor AI chat completions through Together AI's OpenAI-compatible endpoint, which serves a large catalog of hosted open-weights models. |
| [vLLM](./ai-providers/vllm/README.md) | `ai-provider-vllm` | 1.0.0 | Routes McHarbor AI chat completions through a self-hosted vLLM OpenAI-compatible server. |
| [xAI Grok](./ai-providers/xai/README.md) | `ai-provider-xai` | 1.0.0 | Routes McHarbor AI chat completions through xAI's OpenAI-compatible /v1/chat/completions endpoint. |

## Communication channels

Deliver a notification ([family overview](./communication/README.md))

| Plugin | Slug | Version | What it is |
|---|---|---|---|
| [Discord](./communication/discord/README.md) | `comm-discord` | 1.0.0 | Discord communication channel. |
| [Exchange](./communication/exchange/README.md) | `comm-exchange` | 1.0.0 | Exchange Online communication channel. |
| [Gmail](./communication/gmail/README.md) | `comm-gmail` | 1.0.0 | Gmail communication channel. |
| [Gotify](./communication/gotify/README.md) | `comm-gotify` | 1.0.0 | Gotify communication channel. |
| [Microsoft Teams](./communication/microsoft-teams/README.md) | `comm-teams` | 1.0.0 | Microsoft Teams Incoming Webhook connector channel. |
| [ntfy](./communication/ntfy/README.md) | `comm-ntfy` | 1.0.0 | ntfy.sh or self-hosted ntfy notification channel. |
| [Signal](./communication/signal/README.md) | `comm-signal` | 1.0.0 | Signal notification channel. |
| [Slack](./communication/slack/README.md) | `comm-slack` | 1.0.0 | Slack communication channel. |
| [SMTP](./communication/smtp/README.md) | `comm-smtp` | 1.0.0 | SMTP communication channel. |
| [Telegram](./communication/telegram/README.md) | `comm-telegram` | 1.0.0 | Telegram Bot API notification channel. |
| [WhatsApp](./communication/whatsapp/README.md) | `comm-whatsapp` | 1.0.0 | WhatsApp notification channel. |
| [Alertmanager webhook](./communication/alertmanager-webhook/README.md) | `comm-alertmanager-webhook` | 2026.09.01 | Any receiver that ingests Alertmanager webhook v4: Grafana IRM, Squadcast, Zenduty, Keep. |
| [GLPI](./communication/glpi/README.md) | `comm-glpi` | 2026.09.01 | GLPI 9.5, 10 and 11 tickets through the legacy or the high-level API. |
| [iTop](./communication/itop/README.md) | `comm-itop` | 2026.09.01 | Combodo iTop incidents or user requests, following the iTop lifecycle. |
| [OTRS / Znuny / OTOBO](./communication/otrs/README.md) | `comm-otrs` | 2026.09.01 | Tickets through a GenericInterface REST web service the plugin ships. |
| [PagerDuty](./communication/pagerduty/README.md) | `comm-pagerduty` | 2026.09.01 | Incidents through Events API v2. |
| [Request Tracker](./communication/request-tracker/README.md) | `comm-request-tracker` | 2026.09.01 | RT 5 and 6 tickets through REST2. |
| [Zammad](./communication/zammad/README.md) | `comm-zammad` | 2026.09.01 | Zammad tickets through the REST API. |

## Storage destinations

Store backup archives ([family overview](./storage/README.md))

| Plugin | Slug | Version | What it is |
|---|---|---|---|
| [Box](./storage/box/README.md) | `storage-box` | 1.0.1 | Backup destination backed by Box (box.com). |
| [Dropbox](./storage/dropbox/README.md) | `storage-dropbox` | 1.0.1 | Backup destination backed by Dropbox (Dropbox API v2). |
| [FTP/FTPS](./storage/ftp/README.md) | `storage-ftp` | 1.0.1 | Backup destination backed by an FTP (RFC 959) or FTPS (RFC 4217) server. |
| [Google Drive](./storage/google-drive/README.md) | `storage-google-drive` | 1.0.1 | Google Drive backup destination. |
| [Nextcloud](./storage/nextcloud/README.md) | `storage-nextcloud` | 1.0.1 | Back up containers to a Nextcloud instance via WebDAV (RFC 4918). |
| [NFS](./storage/nfs/README.md) | `storage-nfs` | 1.0.1 | Backup destination backed by an NFS (RFC 7530) export. |
| [OneDrive (Personal)](./storage/onedrive-personal/README.md) | `storage-onedrive-personal` | 1.0.5 | OneDrive Personal (consumer endpoint) backup destination. |
| [OneDrive for Business](./storage/onedrive-business/README.md) | `storage-onedrive-business` | 1.0.1 | OneDrive for Business backup destination. |
| [OpenCloud](./storage/opencloud/README.md) | `storage-opencloud` | 1.0.1 | Back up containers to an OpenCloud instance via WebDAV (RFC 4918). |
| [ownCloud](./storage/owncloud/README.md) | `storage-owncloud` | 1.0.1 | Back up containers to an ownCloud (Classic) instance via WebDAV (RFC 4918). |
| [Samba](./storage/samba/README.md) | `storage-samba` | 1.0.1 | Backup destination backed by an SMB/CIFS share (samba.org). |
| [SFTP](./storage/sftp/README.md) | `storage-sftp` | 1.0.1 | SFTP (SSH File Transfer Protocol) backup destination. |
| [SharePoint Online](./storage/sharepoint-online/README.md) | `storage-sharepoint-online` | 1.0.1 | SharePoint document library backup destination. |
| [TrueNAS](./storage/truenas/README.md) | `storage-truenas` | 1.0.1 | Back up containers to a TrueNAS instance via WebDAV (RFC 4918). |

## Identity providers

Sign a user in ([family overview](./identity/README.md))

| Plugin | Slug | Version | What it is |
|---|---|---|---|
| [Generic OpenID Connect](./identity/generic-oidc/README.md) | `identity-generic-oidc` | 1.0.0 | Generic OpenID Connect identity provider. |
| [Google](./identity/google/README.md) | `identity-google` | 1.0.0 | Google identity provider. |
| [LDAP / Active Directory](./identity/ldap/README.md) | `identity-ldap` | 1.0.0 | LDAP / Active Directory identity provider. |
| [Microsoft Entra ID](./identity/entra-id/README.md) | `identity-entra-id` | 1.0.0 | Microsoft Entra ID identity provider. |
| [SAML 2.0](./identity/saml/README.md) | `identity-saml` | 1.0.0 | SAML 2.0 identity provider. |

## Image scanners

Report what is known to be wrong with an image ([family overview](./scanners/README.md))

| Plugin | Slug | Version | What it is |
|---|---|---|---|
| [Clair](./scanners/clair/README.md) | `scanner-clair` | 1.0.0 | Scan container images against a Clair v4 server. |
| [Grype](./scanners/grype/README.md) | `scanner-grype` | 1.0.0 | Scan container images for vulnerabilities with Anchore's Grype. |
| [Trivy](./scanners/trivy/README.md) | `scanner-trivy` | 1.0.0 | Scan container images for vulnerabilities with Aqua Security's Trivy. |

## Audit log forwarders

Ship the audit trail to a collector ([family overview](./logging/README.md))

| Plugin | Slug | Version | What it is |
|---|---|---|---|
| [SIEM](./logging/siem/README.md) | `logging-siem` | 1.0.0 | Forward McHarbor audit events to an HTTP SIEM collector (Splunk HEC, Microsoft Sentinel, Elastic, Datadog). |
| [Syslog](./logging/syslog/README.md) | `logging-syslog` | 1.0.0 | Forward McHarbor audit events to a syslog collector (rsyslog, syslog-ng, QRadar, ArcSight) as RFC5424 messages over UDP, TCP, or TCP+TLS. |

## Snapshot drivers

Take a consistent point in time to back up from ([family overview](./snapshots/README.md))

| Plugin | Slug | Version | What it is |
|---|---|---|---|
| [Synology DSM](./snapshots/synology/README.md) | `snapshot-synology` | 1.0.0 | Synology DSM snapshot repository via the DSM 7.x REST API. |
| [TrueNAS](./snapshots/truenas/README.md) | `snapshot-truenas` | 1.0.0 | TrueNAS SCALE / CORE snapshot repository via the middleware API. |

## Host compatibility

Every manifest declares the host range it supports, and the installer refuses a plugin
whose range does not match the running McHarbor.

| Host range | Plugins |
|---|---|
| `>=2026.09.01` | 5: ai-provider-anthropic, ai-provider-minimax, ai-provider-mistral, ai-provider-ollama, … |
| `>=2026.09.01` | 51: storage-box, storage-dropbox, storage-ftp, storage-google-drive, … |
| `>=2026.09.01` | 2: snapshot-synology, snapshot-truenas |
| `>=2026.09.01` | 3: addon-compute, addon-lxc, addon-kvm |

## Not in this list

- **Theme packs** are catalog entries rather than plugins; they ship under
  `src/plugins/ThemePacks` and carry no Go code.
- **Apps** in the App Store are Compose definitions, not plugins.
- **The new-plugin template** is not in the tree at all. It is embedded in the CLI binary
  and written by `mcharbor plugin dev new <slug>`. See
  [Developing a plugin](../development/plugins/developing.md).

## Related

- [Overview](./overview.md): what a plugin is, the types, and the lifecycle
- [Architecture](../development/plugins/architecture.md): the runtime internals
- [Developing a plugin](../development/plugins/developing.md): the SDK, the manifest, and the developer loop
