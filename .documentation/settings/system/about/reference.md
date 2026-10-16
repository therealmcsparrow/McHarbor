# About: Reference

| Setting | Default | What it does |
|---|---|---|
| **Version** | — | The McHarbor version |
| **Platform** | — | OS and architecture |
| **Backend dependencies** | — | The Go modules this build links |
| **Frontend dependencies** | — | The npm packages this build bundles |
| **Check now** | — | Asks the release feed whether something newer exists |
| **Update now** | — | Pulls the new image, stops the current container, starts a new one |
| **Factory reset** | — | Erases all McHarbor data; requires typing a confirmation phrase |

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| "Could not check for updates" | No outbound access to the release feed | Check egress from the container; the check is not required for anything else |
| Update now does nothing | No writable Docker socket | Self-update starts a container |
| Factory reset did not remove containers | By design | It erases McHarbor’s data, not your hosts |
| Old backups will not restore after a reset | The backup key is a Docker secret, not McHarbor state | Reinstall the original key |

## Related

- [About](./README.md): what the section is for
- [Settings](../../README.md): the other groups
