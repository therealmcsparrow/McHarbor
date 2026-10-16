# Ollama

Routes McHarbor chat completions to a self-hosted Ollama instance. Unlike the other local providers it uses Ollama's **native** API (`/api/tags` to list models and `/api/chat` to complete) instead of the OpenAI-compatible shim.

- **Slug** `ai-provider-ollama`, **version** 2026.09.01
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → AI, as a provider type
- **Gated by** `settings.view` or `ai_assistant.manage` to read a connection, `settings.manage` or `ai_assistant.manage` to change one

## How it works

- Default base URL: `http://localhost:11434`. OpenAI-compatible `/models` and `/chat/completions`.
- The base URL is the server root (`http://host:11434`), with no `/v1` suffix.
- Both `base_url` and `default_model` are required here: Ollama will not pick a model for you, and a request without one fails rather than guessing.
- The base URL is editable, which is what makes an internal gateway or a proxy usable without writing a plugin.
- It does not stream: `SupportsStreaming` is false, so a response is rendered when it is complete.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | The API key is stored encrypted and handed to the plugin decrypted at call time. |
| `SupportsStreaming` | No | Token-by-token streaming. Not implemented by any shipped provider; the assistant renders a completed response. |
| `SupportsTools` | Yes | Tool calling, which is what lets the assistant act on McHarbor resources rather than only answer. |
| `SupportsVision` | No | Image input on models that accept it. |
| `SupportsOAuth` | No | Browser consent instead of a pasted key. No shipped provider uses it. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/ai-providers/ollama.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [AI providers](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
