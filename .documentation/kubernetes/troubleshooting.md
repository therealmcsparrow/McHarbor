# Kubernetes: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| The sidebar section does not appear | The role has none of the `kubernetes.*` keys | Navigation entries are hidden rather than disabled; grant `kubernetes.view`. |
| Cluster connects, every list is empty | The service account's RBAC does not allow listing in those namespaces | The plugin surfaces 403s as empty lists; check with `kubectl auth can-i --list` as that account. |
| Logs are refused for a user who can see pods | Missing `kubernetes.logs` | Grant it deliberately: it is a credential-adjacent read. |

A feature plugin has no connection to test. What to check after installing is that its
sidebar section appears; it stays hidden until a role holds at least the `view` key it
declares.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i addon-kubernetes
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql`.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Kubernetes](./README.md): what it is and how it works
