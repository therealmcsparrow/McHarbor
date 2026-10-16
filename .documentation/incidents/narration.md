# Incidents: Narration

A narrative is the AI model's reading of an incident's evidence. It is off until you turn it
on for an environment, it is sent only to the model you configured, and every narrative
records exactly what it was shown.

## Turning it on

**Narration settings** on the Incidents page, for the environment selected in the header,
needs `incidents.manage`:

| Setting | Default | What it does |
|---|---|---|
| **Allow narration for this environment** | off | Without it, nothing about this environment's incidents is ever sent |
| **Narrate new incidents automatically** | off | Narrate each new incident as it opens, and again once if its signal gets worse after half an hour; summarise it when it resolves |
| **Automatic narrations per hour** | 10 | Automatic narration pauses once this many narrations, automatic or asked for, have run in the last hour |

The dialog names where the evidence would go (the connection, the provider, the model and
its address) and whether that is a self-hosted model (Ollama, LM Studio, llama.cpp,
LocalAI, vLLM) or one that leaves the network. Turning narration on asks you to acknowledge
that, and is audited as `incident.settings.updated` at warning severity. Turning it off also
turns off automatic narration.

The model is the **default AI provider connection**. When there is none, or it is disabled,
or its plugin is not loaded, nothing can be sent and the dialog says so.

## What is sent

The incident's evidence: the container's recent events and last log lines (at most 80 lines
in the prompt), image changes, audit entries and deploys touching it, alerts, drift and
metric samples, and a short instruction.

Before anything is stored or sent, the evidence is redacted:

- passwords, tokens, API keys, JWTs, agent tokens, registry credentials and any long random
  string;
- credentials in connection URLs (`postgres://user:[REDACTED]@…`);
- email addresses (masked) and the last part of IPv4 addresses;
- Docker endpoints and socket paths.

**Container environment variables are never part of the evidence**, which is where resolved
secrets live. The bundle is capped at 64 KiB; the oldest log lines and events are trimmed
first, because the failure is at the end.

## What comes back

The model is asked for a fixed shape: a summary of at most three sentences, when it started,
what changed shortly before, and up to three causes (each citing the evidence that supports
it, each with **Low**, **Medium** or **High** confidence), and an overall confidence. It is
told that "the evidence does not show a cause" is a correct answer. A cause that cites no
evidence is dropped. An answer that is not in the required shape is kept as it was written,
marked low confidence and undetermined, rather than trusted.

The narrator has no tools and no way to act: it receives text and returns text.

## Attribution

Every narrative stores the provider, connection, model, who asked (`system:narrator` for an
automatic one), when, and a SHA-256 fingerprint of the exact prompt. The prompt itself is
shown under **What the model was shown**. Each narration is audited as `incident.narrated`
with the same details; summaries as `incident.summarised`, finding explanations as
`incident.finding.explained`.

## When nothing is sent

| Message | Why |
|---|---|
| Narration is off for this environment | Not opted in |
| No AI model is configured and enabled | No default connection, or its plugin is not loaded |
| A narration is already running for this incident | One at a time per incident |
| The hourly narration limit for this environment is reached | Stops automatic narration only; **Explain this** is never refused for it |
| The model did not answer | The provider refused or timed out (90 seconds); the reason is kept on the incident behind **The last narration failed** |

## On a phone

On the [on-call](../oncall/README.md) alert inbox, a container alert that belongs to an
incident shows its narrative (the summary and its confidence) with a link to the
incident. It needs `incidents.view`; without it the inbox asks for nothing.

## Related

- [Incidents](./README.md)
- [Behaviour findings](./behaviour.md)
