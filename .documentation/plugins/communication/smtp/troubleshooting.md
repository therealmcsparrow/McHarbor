# SMTP: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `535 authentication failed` with the right password | The provider requires an app password | Gmail, Outlook and Yahoo all reject account passwords for SMTP; generate an app password. |
| Test hangs on port 465 | `starttls` selected for an implicit-TLS port | Port 465 needs `ssl_tls`; 587 needs `starttls`. |
| Mail is accepted but never arrives | SPF or DMARC rejects the From address downstream | Use a From address the relay is authorised to send for. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-smtp
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [SMTP](./README.md): what it is and how it works
