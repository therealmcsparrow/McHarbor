# Containers: Reference

| Setting | Default | What it does |
|---|---|---|
| **Policy name** | — | Your label, such as "Nightly updates" |
| **Schedule (cron)** | — | When to check, read in the configured time zone |
| **Containers** | — | A picked list, a glob like `nginx*`, or **All containers** |
| **Image match** | `*` | A glob over image names, such as `nginx:*` |
| **Update strategy** | Latest tag | Latest tag, Semantic versioning, or Image digest |
| **Auto-restart containers after update** | Off | Restart once the image is pulled |
| **Status** | Enabled | Enabled or Disabled |

**Apply to all containers** is described precisely: "The policy follows the host:
containers added later are included automatically."

A pattern that currently matches nothing is badged **Not found** rather than silently
accepted, so a typo is visible at the point you make it.

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| The image updated but nothing changed | Auto-restart is off | The container still runs the old image until it is recreated |
| A container keeps reverting | A reconciler declaration pins the image | Pick one owner per container |
| A pattern matches nothing | Typo, or the containers are on another environment | The **Not found** badge shows it at the point you type it |
| Updates run at the wrong time | The configured time zone | Cron is read in the time zone from Settings → General |

## Related

- [Containers](./README.md): what the section is for
- [Settings](../../README.md): the other groups
