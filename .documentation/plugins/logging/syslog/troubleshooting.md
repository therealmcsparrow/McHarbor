# Syslog: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| No events arrive over UDP and no errors are logged | UDP has no delivery confirmation | Switch to TCP or TLS before concluding the forwarder is broken. |
| TLS handshake fails | The collector uses a private CA | Trust the CA in the McHarbor image, or terminate TLS on a proxy the host already trusts. |
| Messages are truncated | A collector-side message-size limit | Raise `$MaxMessageSize` (rsyslog) or the equivalent. |

**Test** delivers one real event. After that, delivery history is the thing to read: it
records every attempt and its outcome, so a forwarder that stops working is a list of
failures rather than an absence.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i logging-syslog
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql`, `0002_forwarder_id.sql`.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Syslog](./README.md): what it is and how it works
