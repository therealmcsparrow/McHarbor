# ntfy: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Nothing arrives, no error | The subscriber is on a different topic or server | Compare both exactly: a typo in the topic name is a valid topic, just an unread one. |
| 403 on a self-hosted server | The server requires auth for publishing | Set an access token, or Basic credentials with publish rights on the topic. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-ntfy
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [ntfy](./README.md): what it is and how it works
