# Reporting: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| A report section is missing | The plugin that contributes it is not installed | Sections come from other plugins; install the Kubernetes or Swarm add-on and the section appears. |
| Scheduled reports are generated but never arrive | No delivery channel is configured | Configure a communication channel and select it on the schedule. |
| Exports are slow on a large estate | PDF and DOCX rendering is done in-process | Schedule them outside working hours rather than generating on demand. |

A feature plugin has no connection to test. What to check after installing is that its
sidebar section appears. It stays hidden until a role holds at least the `view` key it
declares.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i addon-reporting
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql`, `0002_adhoc.sql`, `0003_ai_connection.sql`, `0004_schedules.sql`, `0005_schedule_target_path.sql`, `0006_schedule_target_recipient.sql`.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Reporting](./README.md): what it is and how it works
