# SIEM: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Everything sits in history as failed | The collector answers non-2xx, often 403 on a token path | Splunk HEC wants the token in the URL or header the collector expects; test the endpoint with `curl` first. |
| Events arrive but the signature never verifies | The collector hashes a re-serialised body | Verify the HMAC over the raw request body, before any JSON round-trip. |
| Nothing is forwarded in a cluster | This node is not the leader | That is correct behaviour; check the leader node. |

**Test** delivers one real event. After that, delivery history is the thing to read: it
records every attempt and its outcome, so a forwarder that stops working is a list of
failures rather than an absence.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i logging-siem
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql`, `0002_forwarder_id.sql`.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [SIEM](./README.md): what it is and how it works
