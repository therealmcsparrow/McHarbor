# Perplexity

Routes McHarbor chat completions to Perplexity's Sonar API, which grounds answers in live web search.

- **Slug** `ai-provider-perplexity`, **version** 2026.09.01
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → AI, as a provider type
- **Gated by** `settings.view` or `ai_assistant.manage` to read a connection, `settings.manage` or `ai_assistant.manage` to change one

## How it works

- Default base URL: `https://api.perplexity.ai`. OpenAI-compatible `/models` and `/chat/completions`.
- This is the one shipped provider that reports no tool support. The assistant can use it to answer, but not to act on McHarbor resources. Keep a tool-capable provider configured as well if you want assistant actions.
- The base URL is editable, which is what makes an internal gateway or a proxy usable without writing a plugin.
- It does not stream: `SupportsStreaming` is false, so a response is rendered when it is complete.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The API key is stored encrypted and handed to the plugin decrypted at call time. |
| `SupportsStreaming` | No | Token-by-token streaming. Not implemented by any shipped provider; the assistant renders a completed response. |
| `SupportsTools` | No | Tool calling, which is what lets the assistant act on McHarbor resources rather than only answer. |
| `SupportsVision` | No | Image input on models that accept it. |
| `SupportsOAuth` | No | Browser consent instead of a pasted key. No shipped provider uses it. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/ai-providers/perplexity.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [AI providers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
