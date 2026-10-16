# Slack: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `invalid_payload` or 400 from Slack | The URL is a workflow-builder webhook, not an incoming-webhook URL | Both look similar; take the URL from **Incoming Webhooks** in the app settings. |
| 404 on every send | The webhook was revoked, or the app was removed from the workspace | Recreate the webhook and update the channel. |
| Messages arrive in the wrong channel | Webhooks are bound to the channel chosen at creation | Create a second webhook for the other channel and a second McHarbor channel to go with it. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-slack
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Slack](./README.md): what it is and how it works
