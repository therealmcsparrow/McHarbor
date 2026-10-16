# Grype: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| **Binary missing** on a native install | The plugin's own download is still running, or failed (no outbound access to `github.com`, or a checksum that does not match the pin) | Run **Test**: the binary step says which. Allow the download, or put `grype` on McHarbor's `PATH`; see [On a native install](../README.md#on-a-native-install) |
| Grype and Trivy disagree on counts | Different feeds and different matching rules | Expected. Compare each scanner against its own history rather than against the other. |
| Scan finds nothing on a distroless image | No package manager metadata to match against | Add `add_cpes_if_none`, and accept the extra noise. |

**Test** checks configuration and reachability without running a full scan. A scanner
that tests green and then times out on a real image is usually a timeout problem rather
than a configuration one.

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, McHarbor's log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i scanner-grype   # container
journalctl -u mcharbor | grep -i scanner-grype         # native
```

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Grype](./README.md): what it is and how it works
