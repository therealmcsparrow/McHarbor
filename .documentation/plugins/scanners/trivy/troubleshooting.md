# Trivy: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| **Binary missing** on a native install | The plugin's own download is still running, or failed (no outbound access to `github.com`, or a checksum that does not match the pin) | Run **Test**: the binary step says which. Allow the download, or put `trivy` on McHarbor's `PATH`; see [On a native install](../README.md#on-a-native-install) |
| First scan is very slow | The vulnerability database is being downloaded | Expected once; later scans reuse it. |
| `unable to initialize a scanner` behind a proxy | No outbound access to the database mirror | Set the proxy variables on the McHarbor container, or in `/etc/mcharbor/mcharbor.env` on a native install. |
| Findings disappear after enabling `ignore_unfixed` | Working as configured | Turn it off for risk review; keep it on for patch triage. |

**Test** checks configuration and reachability without running a full scan. A scanner
that tests green and then times out on a real image is usually a timeout problem rather
than a configuration one.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, McHarbor's log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i scanner-trivy   # container
journalctl -u mcharbor | grep -i scanner-trivy         # native
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Trivy](./README.md): what it is and how it works
