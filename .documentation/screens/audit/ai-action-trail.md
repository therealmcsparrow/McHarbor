# Audit: AI action trail

`/audit?view=ai`

What the AI assistant was asked to change, who consented, and what happened, with one card per
action rather than scattered `ai.action.*` rows.

## Consent-gated actions

Each card is one action id. Its steps, in order:

| Step | Comes from |
|---|---|
| **Preview** | The action record the assistant wrote when it ran the tool in dry-run mode |
| **Approved** / **Denied** / **Consent failed** | `ai.action.approved`, `ai.action.denied`, `ai.action.consent_failed` audit rows |
| **Executed** / **Failed** / **Execution failed** | `ai.action.executed`, `ai.action.failed`, `ai.action.execute_failed` audit rows |

The badge beside the tool name is the action's current status: **Pending**, **Approved**,
**Executed**, **Failed**, **Denied** or **Expired**.

**Redacted payloads** expands the arguments, the plan, the result and any error. Every one
passes through the assistant's standard redaction before it leaves the server: passwords,
tokens, API keys, kubeconfigs, encrypted blobs, high-entropy strings and `KEY=value` secrets
are replaced with `[REDACTED]`, e-mail addresses and IPv4 addresses are masked. Each is
cut at 4,000 characters. The raw payload is never sent to the browser.

When a conversation is deleted its action records go with it. The card then says "The
action record is gone (its conversation was deleted); only the audit events remain": the
consent and execution steps are still there, the preview and the payloads are not.

## Tool calls

The second section lists `ai.tool.call`, `ai.tool.denied` and `ai.tool.sanitized` events:
read-only tool calls, tool calls the assistant was refused, and outputs replaced because
they contained a prompt-injection phrase. Their details are redacted the same way.

## Filters

| Filter | API parameter | What it narrows |
|---|---|---|
| Date range | `from`, `to` | RFC 3339 bounds |
| Actor | `actor` | A username or user id |

## API

Both gated by `audit.view`, both paginated (`page`, `per_page`):

- `GET /api/audit/ai-actions`: the grouped actions, newest activity first
- `GET /api/audit/ai-tools`: the tool events

## Related

- [Audit](./README.md): the screen as a whole
