# Discord: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| 401 or 404 | The webhook was deleted, or the channel was removed | Recreate the webhook; deleting a Discord channel deletes its webhooks with it. |
| Rate limited under a burst of alerts | Discord webhooks are rate limited per channel | Group alerts or spread them across channels; the send is retried, not dropped. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-discord
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Discord](./README.md): what it is and how it works
