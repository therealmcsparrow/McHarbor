# SAML 2.0: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `Destination` or audience mismatch | The IdP's idea of the ACS URL or entity id differs from McHarbor's | Re-import the SP metadata at the IdP rather than typing the URLs by hand. |
| Assertions fail to validate after a rollover | The IdP metadata is cached at a stale URL | Point `metadata_url` at the live metadata document, not a saved copy. |
| Clock-skew errors | The McHarbor host clock has drifted | SAML assertion windows are minutes wide; run NTP. |

Test a provider by signing in from a private browser window rather than by trusting the
form. A configuration that saves cleanly can still fail at the identity provider, and the
redirect is where that shows up.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i identity-saml
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [SAML 2.0](./README.md): what it is and how it works
