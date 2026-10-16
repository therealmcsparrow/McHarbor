# Logging: Reference

| Setting | Default | What it does |
|---|---|---|
| **Forwarder type** | — | One tile per installed logging plugin |
| **Name** | — | Your label, such as "Central log stack" |
| **Enable forwarding** | Off | Whether this forwarder runs |
| **Per-forwarder fields** | — | Endpoint, credential, retries and timeout, as the plugin declares them |

Secrets already saved show as "Stored — leave empty to keep", so editing a forwarder does
not mean retyping its credential.

**Test** delivers a real test log. After that, **delivery history** is the thing to read:
it records every attempt and its outcome, so a forwarder that stops working is a list of
failures rather than an absence.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| No forwarder types are offered | No logging plugin installed | Install one from the App Store |
| Everything sits in history as failed | The collector answers non-2xx | Test the endpoint with `curl` first; the history shows the status |
| Nothing is forwarded in a cluster | This node is not the leader | Correct behaviour; check the leader node |
| The forwarder list is refused | Missing `audit.view` or `settings.manage` | Listing needs both keys |

## Related

- [Logging](./README.md): what the section is for
- [Settings](../../README.md): the other groups
