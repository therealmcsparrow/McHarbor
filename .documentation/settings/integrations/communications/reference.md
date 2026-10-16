# Communications: Reference

| Setting | Default | What it does |
|---|---|---|
| **Channel type** | — | One tile per installed communication plugin |
| **Connection method** | varies | For channels with several transports: Cloud API, self-hosted gateway, business platform, third-party SaaS, REST API, bot server, signald, simple setup |
| **Enabled** | On | Whether the channel may be used |
| **Set as default** | Off | The channel used by anything that does not name one |
| **Per-channel fields** | — | Declared by the plugin; see the plugin’s own configuration page |

**Send test notification** delivers a **real** message. There is no dry run. If the test
succeeds, someone received it.

Secrets are encrypted at rest. Microsoft Teams is the exception: its connector URL is held
as plain configuration, and should be treated as sensitive anyway.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| No channel types are offered | No communication plugin installed | Install one from the App Store |
| A channel tests green but alerts never arrive | The rule does not name it as a destination | Destinations are per alert rule |
| Password reset emails are never sent | No email channel selected under Security → Password | Configuring the channel here is only half the job |
| Deleting a channel broke an alert | The rule still references it | "N destinations no longer exist and are ignored"; repoint the rule |

## Related

- [Communications](./README.md): what the section is for
- [Settings](../../README.md): the other groups
