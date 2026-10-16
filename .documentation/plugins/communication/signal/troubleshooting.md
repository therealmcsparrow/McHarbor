# Signal: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `unregistered user` | A recipient number has no Signal account | Signal will not deliver to a number that is not registered. |
| The gateway rejects the sender | The sending account is not registered on that gateway | Register and verify the account on the gateway first; McHarbor cannot do registration. |
| Messages stop after a gateway restart | `signal-cli` state was not persisted | Mount the gateway's data directory so the registration survives restarts. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-signal
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Signal](./README.md): what it is and how it works
