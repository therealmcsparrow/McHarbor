# Perplexity: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| **Test** returns 401 | The key was revoked, or it belongs to a different account or project | Issue a fresh key and re-save. The stored key is never echoed back, so retyping it is the only way to be sure what is saved. |
| The model dropdown is empty | The key is valid but the account has no model entitlements yet, or a proxy is blocking the list call | Open the provider console and confirm the account can list models; then check outbound access from the McHarbor container. |
| 429 during assistant use | Rate or spend limits on the account | Raise the limit at the provider, or point the connection at a smaller default model. |

**Test** exercises the credential against the real endpoint and reports what came back,
so a green test means the key works and the base URL resolves from inside the McHarbor
container.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i ai-provider-perplexity
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Perplexity](./README.md): what it is and how it works
