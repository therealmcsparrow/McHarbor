# Exchange: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| `ErrorAccessDenied` on send | Delegated `Mail.Send` was consented instead of the application permission | Client-credentials tokens carry only application permissions; add and consent the application one. |
| Works for one mailbox, fails for another | An application access policy restricts which mailboxes the app may use | That is the intended state. Add the mailbox to the policy's group if it should be allowed. |

**Send test notification** delivers a real message. There is no dry run: if the test
succeeds, someone received it.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i comm-exchange
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Exchange](./README.md): what it is and how it works
