# WhatsApp: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| Cloud API returns 131047 | Outside the 24-hour customer service window | Use an approved template, or a transport without that restriction. |
| Token expires after ~24 hours | A temporary access token was used | Issue a permanent system-user token in Meta Business Manager. |
| SaaS provider rejects the sender | `provider_from` is not the identifier the provider expects | Twilio wants the WhatsApp-enabled number; other providers want an originator id. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-whatsapp
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [WhatsApp](./README.md): what it is and how it works
