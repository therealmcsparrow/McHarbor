# Docker Swarm: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| The overview shows the "initialise" empty state on a working cluster | The environment is connected to a node that has left the swarm | Check `docker info` on that host; the probe reports what the daemon reports. |
| Join token routes return 403 for a user who can see the cluster | Those are `swarm.manage`, not `swarm.view` | Intended: a join token is a credential. |
| Stack deploy fails with no useful message | The Compose file uses options Swarm ignores or rejects | Swarm mode supports a subset of Compose; check `deploy:` keys. |

A feature plugin has no connection to test. What to check after installing is that its
sidebar section appears; it stays hidden until a role holds at least the `view` key it
declares.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i addon-swarm
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Docker Swarm](./README.md): what it is and how it works
