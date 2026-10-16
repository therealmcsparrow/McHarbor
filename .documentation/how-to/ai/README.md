# Add an AI provider

The assistant does not talk to any model directly. Every provider is a plugin registering an
**AI adapter**, and the host owns everything around it: the connection rows, the credential
encryption, the picker, the test button, the default selection, and the consent model on
whatever the assistant proposes.

That means adding a provider is a small, well-shaped job: one Go file and a manifest, no
frontend at all.

## What already exists

Eighteen provider plugins ship. Before writing one, check whether the provider you want is
**OpenAI-compatible**. Many are, and an existing plugin pointed at a different base URL is
often the whole answer. The local-model plugins exist for exactly this reason.

## The shape of a provider plugin

```
src/plugins/Plugins/ai-provider-<name>/
├── plugin.yaml
├── go.mod
├── backend/
│   ├── main.go            the Plugin symbol and the adapter registration
│   └── <name>_client.go   the client that talks to the provider
└── icons/plugin.svg
```

No frontend, no routes, no menu, no migrations. The manifest's contribution lists are empty.

## The manifest

```yaml
schema: "mcharbor.plugin/v1"
slug:     "ai-provider-example"
name:     "Example"
version:  "2026.09.01"
description: "Routes McHarbor AI chat completions through Example."

mcharbor: ">=2026.09.01"
go:       ">=1.25"

permissions:
  - ai.adapter.register
  - encryption.handle.read
  - audit.emit

capabilities:
  outbound_http: true

config_schema:
  - key:      "api_key"
    type:     "secret"
    label:    "API Key"
    required: true
    secret:   true
    help:     "Stored encrypted at rest and sent only as a Bearer token."
  - key:      "base_url"
    type:     "string"
    label:    "Base URL"
    required: true
    default:  "https://api.example.com/v1"
    help:     "Absolute HTTP or HTTPS API base URL."
  - key:      "default_model"
    type:     "string"
    label:    "Default Model"
    required: false
    default:  "example-small"

contributes:
  routes: []
  menu: []
  widgets: []
  nodes: []
```

Three scope tokens, one capability flag, and a config schema. The host renders that schema as
the connection form, so the labels and help text are what an operator reads while configuring
it, so write them for that reader.

A provider plugin declares **no permission keys**. The host's existing AI keys already gate
every surface it appears on.

## Registering the adapter

```go
func (p *examplePlugin) RegisterAdapters(reg plugin.AdapterRegistry) error {
    err := reg.AI().Register(
        plugin.AIAdapterSpec{
            ProviderType: "example",
            Slug:         p.manifest.Slug,
            Icon:         "plugin.svg",
            DisplayName:  "Example",
            Description:  p.manifest.Desc,
            DefaultModel: "example-small",
            Capabilities: plugin.AIAdapterCapabilities{
                EncryptedCredentials: true,
                SupportsStreaming:    false,
                SupportsTools:        true,
                SupportsVision:       false,
                ListModelsDynamic:    true,
                SupportsOAuth:        false,
            },
            ConfigSchema: exampleConfigSchema(),
        },
        newExampleClient(),
    )
    if err != nil {
        return fmt.Errorf("ai-provider-example: register adapter: %w", err)
    }
    return nil
}
```

`ProviderType` is the key the host dispatches on, and it is what gets stored on every
connection row that uses this plugin. Pick it once; changing it later orphans existing
connections.

### Capabilities are promises

The host reads these to decide how to treat the adapter. Declaring one you do not honour
produces failures that look like provider bugs:

| Flag | Declaring it true means |
|---|---|
| `EncryptedCredentials` | The adapter takes credential fields the host must encrypt at rest. Leave it false for a provider that needs only a URL |
| `SupportsStreaming` | The host may choose the streaming chat path |
| `SupportsTools` | The adapter handles tool calls in the request. Adapters without it ignore the tools field |
| `SupportsVision` | The adapter accepts image content as data URLs. Adapters without it must reject image content rather than silently dropping it |
| `ListModelsDynamic` | The adapter fetches a live model catalog, so the dropdown stays in sync. When false, the host uses the static list in the spec |
| `SupportsOAuth` | The host renders an OAuth setup wizard instead of a credentials form |

## The client

Three methods. The host calls all three from parallel goroutines, so implementations **must be
safe for concurrent use**.

```go
type AIAdapterClient interface {
    Test(ctx, config)            (*AITestResult, error)
    Complete(ctx, config, req)   (*LLMResponse, error)
    Models(ctx, config)          ([]ModelDecl, error)
}
```

Each receives the connection's configuration as a plain map keyed by the config schema's keys.
The host does not pre-validate it; the adapter decides what it needs.

**`Test`** is the connection test button. Validate every field the adapter will use at
runtime, and on failure return a result carrying a descriptive message rather than an error,
so the dialog can show which step broke. A successful test may include a one-line reply, which
is what lets the UI say the provider actually answered.

Remember that a failed test is a **200** with a failing status, not a 4xx. Only a missing
connection is a 404. See [AI endpoints](../../api/endpoints/ai.md).

**`Complete`** translates the host's request into the provider's native shape and returns the
assembled reply. An adapter that declared streaming may stream internally; the host still
receives one response on return.

**`Models`** returns the live catalog when the adapter declared dynamic listing, and may
return an empty slice or the static catalog otherwise.

### Rules

- **Bound every call.** A hanging provider must become a timeout, not a stuck request.
- **Never log the credential.** It is encrypted at rest and redacted in responses; keep it out
  of logs and audit payloads.
- **Never return raw upstream error text** to the client. Log it, return a message the
  operator can act on.
- **Do not keep per-request state on the client struct.** Concurrent calls share it.

## Connections, not providers

A *provider* is the plugin. A *connection* is a configured instance of it: a name, a base
URL, a default model, a credential. An install can have several connections of the same
provider type, and exactly one default. The endpoints are in
[AI assistant and providers](../../api/endpoints/ai.md).

Reads are gated by the settings view key or the assistant management key; writes by the
settings manage key or the assistant management key. The credential goes in and never comes
back out.

## The consent model

Worth understanding before enabling the assistant, because it is the part that decides how
much damage a confused model can do.

The assistant's permissions are deliberately three, not one:

| Key | Allows |
|---|---|
| `ai_assistant.use` | Sending prompts and reading your own action history |
| `ai_assistant.act` | Consenting to, and executing, a proposed action |
| `ai_assistant.manage` | Configuring providers, connections and OAuth |

An account with only `use` can hold a conversation in which the assistant suggests restarting
a container, and **cannot restart it**. That separation is the safety property; do not collapse
it by granting `act` alongside `use` as a matter of course.

Executing a proposed action is two steps, in order:

1. The assistant records an action.
2. A user with `act` approves or denies it, with a reason.
3. Only then does the execute call run it, and it refuses with a 400 if the action is not in
   the approved state.

## After the change

```bash
cd src/plugins/Plugins/ai-provider-example
make build
cd ../../../..
docker compose build && docker compose up -d
```

Then reinstall the plugin from the store and add a connection in settings. See
[Packaging](../plugin/packaging.md).

## Related

- [AI endpoints](../../api/endpoints/ai.md): the assistant and provider API
- [AI provider plugins](../../plugins/ai-providers/README.md): the eighteen that ship
- [Build a plugin](../plugin/README.md)
- [Contract permissions](../permissions/README.md): why use and act are separate
