# AI Providers

An AI-provider plugin gives McHarbor somewhere to send a chat completion. The host owns
the connection rows, the key encryption, the assistant, its consent model and the
workflow AI nodes; the plugin supplies the client: test the credential, list the models,
complete one request.

Install as many as you like. Each one adds a provider type; a connection picks one type,
and the assistant picks a connection.

| Plugin | Slug | Runs | What it is |
|---|---|---|---|
| [Anthropic Claude](./anthropic/README.md) | `ai-provider-anthropic` | Hosted service | Routes McHarbor AI chat completions through Anthropic /v1/messages. |
| [Cohere](./cohere/README.md) | `ai-provider-cohere` | Hosted service | Routes McHarbor AI chat completions through Cohere's OpenAI compatibility API. |
| [DeepSeek](./deepseek/README.md) | `ai-provider-deepseek` | Hosted service | Routes McHarbor AI chat completions through DeepSeek's OpenAI-compatible /v1/chat/completions endpoint. |
| [Fireworks AI](./fireworks/README.md) | `ai-provider-fireworks` | Hosted service | Routes McHarbor AI chat completions through Fireworks AI's OpenAI-compatible inference API for hosted open-weights models. |
| [Google Gemini](./gemini/README.md) | `ai-provider-gemini` | Hosted service | Routes McHarbor AI chat completions through Google Gemini's OpenAI-compatible endpoint. |
| [Groq](./groq/README.md) | `ai-provider-groq` | Hosted service | Routes McHarbor AI chat completions through Groq's OpenAI-compatible LPU inference API. |
| [llama.cpp](./llamacpp/README.md) | `ai-provider-llamacpp` | On your hardware | Routes McHarbor AI chat completions through a self-hosted llama.cpp server (llama-server) over its OpenAI-compatible /v1 endpoint. |
| [LM Studio](./lm-studio/README.md) | `ai-provider-lmstudio` | On your hardware | Routes McHarbor AI chat completions through a local LM Studio server over its OpenAI-compatible /v1 endpoint. |
| [LocalAI](./localai/README.md) | `ai-provider-localai` | On your hardware | Routes McHarbor AI chat completions through a self-hosted LocalAI instance over its OpenAI-compatible /v1 endpoint. |
| [MiniMax](./minimax/README.md) | `ai-provider-minimax` | Hosted service | Routes McHarbor AI chat completions through MiniMax's OpenAI-compatible /v1/chat/completions endpoint. |
| [Mistral](./mistral/README.md) | `ai-provider-mistral` | Hosted service | Routes McHarbor AI chat completions through Mistral's OpenAI-compatible /v1/chat/completions endpoint. |
| [Ollama](./ollama/README.md) | `ai-provider-ollama` | On your hardware | Routes McHarbor AI chat completions through a self-hosted Ollama instance using the native /api/tags and /api/chat endpoints. |
| [OpenAI](./openai/README.md) | `ai-provider-openai` | Hosted service | Routes McHarbor AI chat completions through OpenAI /v1/chat/completions. |
| [OpenRouter](./openrouter/README.md) | `ai-provider-openrouter` | Hosted service | Routes McHarbor AI chat completions through OpenRouter, a single OpenAI-compatible gateway in front of hundreds of hosted models from many vendors. |
| [Perplexity](./perplexity/README.md) | `ai-provider-perplexity` | Hosted service | Routes McHarbor AI chat completions through Perplexity's OpenAI-compatible Sonar API, which answers with live web search grounding. |
| [Together AI](./together/README.md) | `ai-provider-together` | Hosted service | Routes McHarbor AI chat completions through Together AI's OpenAI-compatible endpoint, which serves a large catalog of hosted open-weights models. |
| [vLLM](./vllm/README.md) | `ai-provider-vllm` | On your hardware | Routes McHarbor AI chat completions through a self-hosted vLLM OpenAI-compatible server. |
| [xAI Grok](./xai/README.md) | `ai-provider-xai` | Hosted service | Routes McHarbor AI chat completions through xAI's OpenAI-compatible /v1/chat/completions endpoint. |

Each plugin has a folder of its own: a README, the configuration reference, a
troubleshooting table, and a developer page covering the adapter it registers and how to
build, sign and fork it.

## What they have in common

- They register through `AdapterRegistry.AI()` and appear under
  **Settings → Integrations → AI**.
- All but two share the same three fields (`api_key`, `base_url`, `default_model`)
  because all but two speak the OpenAI `/models` and `/chat/completions` shape.
  Anthropic uses `/v1/messages`, and Ollama uses `/api/tags` and `/api/chat`.
- `base_url` is editable on every one of them. That is what makes an internal gateway or
  a proxy usable without writing a plugin.
- API keys are encrypted at rest and are never returned by the API once saved.
- **None of them stream.** `SupportsStreaming` is false everywhere, so the assistant
  renders a response when it is complete rather than token by token.
- Perplexity is the only one that reports no tool support, which limits it to answering
  rather than acting.

## On `localhost`

For a local provider, `localhost` inside the McHarbor container means the container
itself. Use the machine's LAN address, or `host.docker.internal` on Docker Desktop. This
is the single most common reason a working local model fails its first **Test**.

## Related

- [Plugin catalog](../catalog.md): every shipped plugin in one table
- [Plugins](../README.md): the section index
