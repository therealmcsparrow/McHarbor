# Gotify: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| 403 from the server | A client token was used instead of an application token | Applications post messages; clients read them. Take the token from **Apps**. |
| Test cannot connect | The server URL is not reachable from inside the container | Use an address the container can resolve, not `localhost`. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-gotify
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Gotify](./README.md): what it is and how it works
