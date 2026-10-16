# McHarbor: Reference

| Setting | Default | What it does |
|---|---|---|
| **Check for new releases** | Off | Runs a background check on the schedule below and shows a banner when a newer version exists |
| **Look for a new release** | Daily 04:00 (`0 4 * * *`) | When the check runs, on the readable schedule builder; cron is read in the server's time zone. The next check is shown under it |
| **Install automatically in this window** | Daily 04:30 (`30 4 * * *`) | When the install window opens. Automatic McHarbor installs and automatic agent updates start only inside it. The next window is shown under it |
| **Keep the window open for (minutes)** | 120 | 15 to 720. How long an install waits for running work to finish before the window is skipped |
| **Notify these channels on a new release** | none | "Leave empty to only surface the in-app banner" |
| **Install McHarbor updates automatically** | Off | "Pull the image and restart into it without waiting for a confirmation" |

**Update now** confirms first: "McHarbor will pull the new image, stop the current
container, and start a new one in its place. You will be disconnected and need to reload
the page after it comes back online."

Automatic install does nothing while the checker is off, and the section says so rather than
leaving the switch looking armed: "The self-update checker is off, so nothing here runs."

A failure is reported rather than retried silently: "Automatic install of v3.1.0 failed.
Update manually or check the logs."

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| The automatic install switch does nothing | The checker is off | Enable the checker first; the section says so |
| Update now fails | No writable Docker socket | Self-update starts a container and needs one |
| No banner appears for a known release | The next scheduled check has not come yet | Use **Check now** for an immediate answer |
| An update was found but not installed | It waits for the install window, or the window was skipped while work was running | The section shows the reason; widen the window or move it to a quieter time |
| Agents broke after an update | They were left on the old version | Enable automatic agent updates, or run a sweep |

## Related

- [McHarbor](./README.md): what the section is for
- [Settings](../../README.md): the other groups
