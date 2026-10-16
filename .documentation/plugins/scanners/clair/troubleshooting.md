# Clair: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Test passes, scans time out | Indexing a large image exceeds the timeout | Raise `timeout_seconds`; a warm Clair is much faster on the second pass. |
| 401 from the server | Clair has auth enabled and no token is configured | Add the bearer token. |
| Reports come back empty | The Clair instance has not loaded updaters yet | Check the Clair side: an indexer with no matcher data matches nothing. |

**Test** checks configuration and reachability without running a full scan. A scanner
that tests green and then times out on a real image is usually a timeout problem rather
than a configuration one.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i scanner-clair
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Clair](./README.md): what it is and how it works
