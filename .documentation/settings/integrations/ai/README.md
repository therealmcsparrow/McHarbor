# AI

**Settings → Integrations → AI** · `?tab=ai`

Connect AI providers so AI-backed features have a model to call.

## Permissions

| Key | What it unlocks |
|---|---|
| `settings.view` or `ai_assistant.manage` | Read a connection |
| `settings.manage` or `ai_assistant.manage` | Add, change or delete one |
| `ai_assistant.use` / `ai_assistant.act` | Use the assistant, and let it act |

A section you lack the key for renders **disabled with a tooltip naming it**, rather than
opening onto an empty panel.

## What it governs

- One connection per provider type, each with an endpoint, a default model and a credential.
- How each connection authenticates: an API key, or a sign-in.
- Which connection is the default.
- Whether a connection is enabled at all.

## Two ways to authenticate

A connection carries **either** a pasted API key **or** an OAuth sign-in. The form asks
which on the configuration step, and the choice can be changed later from **Edit**.

**API key** is the short path: paste the key the provider issued. It is encrypted at rest
and never echoed back.

**Sign in** runs the OAuth 2.0 device flow (RFC 8628). McHarbor shows a short code, you
approve it on the provider's own site while signed in there, and the resulting token is
stored encrypted on the connection. Nothing about your provider password reaches McHarbor,
and a token can be revoked on the provider side without touching any other connection.

Sign-in needs an OAuth application **you** register with the provider. McHarbor ships no
registered client of its own, so you paste your own client ID. Providers with published
endpoints (GitHub, Google/Gemini, Azure OpenAI) have their device and token URLs filled in
for you; anything else takes both URLs by hand, which is what lets a provider type that
arrives later as a plugin use the flow too.

The sign-in is stored on the connection, so a new connection is saved first and signed in
straight afterwards; the dialog walks you there. Until it is signed in, the card reads
**Sign-in required** and **Send test** says so rather than reporting a credential failure.

## Providers are plugins

There is nothing to configure until a provider plugin is installed: "AI providers ship as
plugins. Install one from the App Store to configure a connection." Eighteen ship with
McHarbor; see [AI provider plugins](../../../plugins/ai-providers/README.md).

An installed-but-disabled plugin shows its tile greyed with the reason.

## What uses a provider

The AI assistant, AI nodes in workflows, and plugins that declare the `ai.invoke` scope and
call through the host rather than holding a key of their own.

None of them stream: no shipped provider advertises streaming, so a response appears when
it is complete.

## Local versus hosted

Prompts sent to a hosted provider leave your network. Decide that deliberately, because the
assistant’s context includes container names, image names and configuration.

## In this folder

| Page | What it covers |
|---|---|
| [Reference](./reference.md) | Every setting, its default, and what changing it costs |

## Related

- [AI provider plugins](../../../plugins/ai-providers/README.md): all 18, with their fields and defaults
- [Communications](../communications/README.md): the neighbouring integration
