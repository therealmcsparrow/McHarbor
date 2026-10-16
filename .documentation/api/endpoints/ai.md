# AI assistant and providers

22 endpoints: the assistant conversation, the actions it proposes, and the provider
connections it runs on.

| Key | Covers |
|---|---|
| `ai_assistant.use` | Send a prompt, read your own action history |
| `ai_assistant.act` | Consent to, and execute, a proposed action |
| `ai_assistant.manage` | OAuth connections, the provider catalog |
| `settings.view` or `ai_assistant.manage` | Read provider connections |
| `settings.manage` or `ai_assistant.manage` | Create, edit, delete, test, set default |

## Things worth knowing first

**Using and acting are separate permissions, deliberately.** `ai_assistant.use` lets someone
ask; `ai_assistant.act` lets them approve what the assistant proposes. An account with only
`use` can hold a conversation that suggests restarting a container and cannot restart it.

**Proposed actions are a two-step consent.** The assistant records an action;
`POST /api/ai-assistant/actions/{id}/consent` approves or denies it with a reason; only then
does `POST /api/ai-assistant/actions/{id}/execute` run it, and it returns 400 if the action
is not in the approved state.

**The assistant can author automation, and only what you could author yourself.** Beyond reading the estate it can propose creating a workflow, an alert rule, a container update policy or a scheduled automation. Each of those tools is filtered out of the model's catalog unless the account holds `ai_assistant.act` **and** the owning module's manage key - `workflows.manage`, `alerts.manage`, `updates.manage`, `schedules.manage` respectively - and the same pair is checked again when the approved action executes, so revoking a role between proposal and approval stops the write. Workflows are always created as drafts. An alert rule with no destination channels delivers in-app only, and `alerts.conditions` (read-only, `alerts.view`) is what the assistant calls first to find a valid source and condition to build one on.

**Chat streams.** `POST /api/ai-assistant/chat` answers as SSE: a `message` frame per text
delta and a `done` frame at the end.

**Providers are plugins.** A connection names a `providerType` an installed plugin
contributes, with a base URL, a default model and an API key. The key is encrypted at rest
and never returned.

**OAuth uses the device flow.** Begin returns a user code and a verification URI, poll checks
the upstream token endpoint and persists the token on success, disconnect removes it. All
three are `ai_assistant.manage`.

**A failed provider test is a 200** with a per-step report: load, decrypt, plugin lookup,
send. The failure of the plugin itself is captured as the send step detail, not as a 4xx.
Only a missing connection is a 404.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/ai-assistant/actions` | `ai_assistant.use` | Returns the user's recent actions, newest first | `?limit=` |
| `POST` | `/api/ai-assistant/actions/{id}/consent` | `ai_assistant.act` | Applies the user's approve/deny decision | — |
| `POST` | `/api/ai-assistant/actions/{id}/execute` | `ai_assistant.act` | Runs an approved action | — |
| `POST` | `/api/ai-assistant/chat` | `ai_assistant.use` | Handles one user turn | — |
| `POST` | `/api/ai-assistant/oauth/begin` | `ai_assistant.manage` | Starts the OAuth Device Flow against the given provider | — |
| `POST` | `/api/ai-assistant/oauth/disconnect` | `ai_assistant.manage` | Removes the stored OAuth token for a provider | — |
| `POST` | `/api/ai-assistant/oauth/poll` | `ai_assistant.manage` | Checks the device flow against the upstream token endpoint and persists the token on success | — |
| `GET` | `/api/ai-assistant/oauth/status` | `ai_assistant.manage` | Returns the current OAuth connection state | — |
| `GET` | `/api/ai-assistant/providers` | `ai_assistant.manage` | Returns the catalog of supported LLM providers and their models | — |
| `GET` | `/api/ai-providers` | `settings.view` or `ai_assistant.manage` | Returns all AI provider connections | — |
| `POST` | `/api/ai-providers` | `settings.manage` or `ai_assistant.manage` | Creates a new AI provider connection | — |
| `GET` | `/api/ai-providers/default` | `settings.view` or `ai_assistant.manage` | Returns the connection marked as default (or empty when none is set) | — |
| `DELETE` | `/api/ai-providers/{id}` | `settings.manage` or `ai_assistant.manage` | Removes an AI provider connection | `{id}` |
| `GET` | `/api/ai-providers/{id}` | `settings.view` or `ai_assistant.manage` | Returns a single AI provider connection | `{id}` |
| `PUT` | `/api/ai-providers/{id}` | `settings.manage` or `ai_assistant.manage` | Applies partial updates to an AI provider connection | `{id}` |
| `POST` | `/api/ai-providers/{id}/default` | `settings.manage` or `ai_assistant.manage` | Marks the given connection as the default, clearing any prior default within the same transaction | `{id}` |
| `POST` | `/api/ai-providers/{id}/test` | `settings.manage` or `ai_assistant.manage` | Sends a test completion through the configured plugin for the connection and returns a per-step report (load → decrypt → plugin lookup → send) | `{id}` |
| `GET` | `/api/ai-providers/oauth-defaults` | any authenticated user | Returns device-flow prefills keyed by provider type: the host's published endpoints, overlaid with any client ID + endpoints a plugin declared via `AIAdapterSpec.OAuthDefaults`. Each entry carries `pluginProvided: true` when the merged config is complete (client ID + both endpoints); the create form then offers one-click sign-in and hides the config fields. Never includes a client secret. | — |
| `GET` | `/api/ai-providers/{id}/oauth/status` | `settings.view` or `ai_assistant.manage` | Returns the connection's OAuth sign-in state | `{id}` |
| `POST` | `/api/ai-providers/{id}/oauth/begin` | `settings.manage` or `ai_assistant.manage` | Starts the device flow (RFC 8628) for the connection; returns a user code + verification URI | `{id}` |
| `POST` | `/api/ai-providers/{id}/oauth/poll` | `settings.manage` or `ai_assistant.manage` | Trades the pending device code for a token; still-pending returns `pending: true`, not an error | `{id}` |
| `POST` | `/api/ai-providers/{id}/oauth/disconnect` | `settings.manage` or `ai_assistant.manage` | Drops the stored OAuth token for the connection | `{id}` |

**Plugin-declared OAuth ("zero-config sign-in").** A connection whose provider type has a
complete `OAuthDefaults` may be created with no OAuth config at all: the operator picks the
provider, chooses **Sign in**, and the backend fills the client ID and endpoints from the
plugin at sign-in time (and picks up later plugin updates, because nothing plugin-owned is
copied into the connection row). An operator-entered value always wins over the plugin's.
The client ID and endpoints are not secrets; a confidential client's secret, if any, lives
in `plugin_config`, never in the bundle or an API response.


## Request bodies

**`POST` `/api/ai-assistant/actions/{id}/consent`**: `approve`, `reason`?

**`POST` `/api/ai-assistant/chat`**: `conversationId`?, `environmentId`?, `containerId`?, `message`, `connectionId`?, `cursor`?

**`POST` `/api/ai-providers`**: `name`, `providerType`, `isDefault`, `enabled`?, `baseUrl`, `defaultModel`, `apiKey`, `authMethod`? (`api_key` | `oauth`), `oauth`? (`clientId`, `clientSecret`, `deviceUrl`, `tokenUrl`, `scopes`; may be omitted entirely when the provider's plugin ships a complete `OAuthDefaults`)

**`PUT` `/api/ai-providers/{id}`**: `name`?, `isDefault`?, `enabled`?, `baseUrl`?, `defaultModel`?, `apiKey`?, `authMethod`?, `oauth`?

## Related

- [Permissions](../conventions/permissions.md): use versus act
- [Streaming](../conventions/streaming.md): the chat stream
