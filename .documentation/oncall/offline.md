# Offline and stale data

The rule: **nothing stale renders as live.** Whenever a screen is showing a saved response
rather than a fresh one, it says so above the content, with the time the data was fetched.

## What the service worker keeps

| Kind | Policy |
|---|---|
| The application itself (`/index.html`, the build's `/assets/*`, the fonts) | Stored at install and replaced by the next build. Opens instantly, and offline. |
| The reads the on-call screens use | Always fetched from the server first. The saved copy is used **only when the network fails**, and carries the time it was fetched. |

The saved reads are the session and permission lookups, environments, containers and one
container's details, alert history, the in-app inbox, pending elevations, your push devices,
and the runbooks that apply to an alert, steps included, in one response, so a whole
[runbook](./runbooks/README.md) is readable in a basement. Each saved copy is stamped `X-Mcharbor-Cached-At`, and the screen shows
*Showing saved data from <time>. It may be out of date.*, or *Offline. Showing what was
known at <time>.* when the device reports no connection.

The server is always asked first, rather than showing the saved copy while refreshing it,
because a saved copy shown first would render as live before an expired session had been
detected.

## What is never kept

- Anything that is not a `GET`: no action is ever answered from a cache or replayed later.
- Log streams, event streams, terminals and WebSockets.
- Any response other than a `200` with a JSON body, so a `401` or `403` is never saved.
- Any route not on the list above.

While offline, *Restart*, *Approve* and *Deny* are disabled, and so is every runbook control
that records something: completing a step, skipping one, running a diagnostic or an action,
and finishing a run. A queued step completion would be an action whose permission check
happened at an unknown time, so it is refused rather than deferred.

## Sessions that expire in the background

The usual real-world case: the app is opened from a notification hours after the session
expired.

1. The first request returns `401`. The service worker deletes every saved read.
2. The app goes to the login page with the screen it was opening in `next`
   (`/login?next=%2Foncall%2Fcontainers%2F…`).
3. After signing in, it returns to that screen. `next` is honoured only for a path inside
   McHarbor; anything else lands on the start page.

Signing out also deletes every saved read on that device.
