# Push notifications

Push notifications reach a device even when McHarbor is closed. They are built into McHarbor,
not a plugin, so uninstalling a plugin can never silence the on-call channel. The
communication plugins (Discord, Telegram, ntfy, Gotify and the rest) keep working beside it.

## What is sent, and through whom

A push travels through the push service of the browser vendor: Google for Chrome and
Android, Apple for Safari and iOS, Mozilla for Firefox, Microsoft for Edge on Windows. The
message is encrypted for the receiving device and the push service cannot read it, but **the
push service does learn that a notification was sent, to which device, and when.** If that is
not acceptable, do not turn push on; the communication plugins deliver the same alerts
through services you choose.

The payload is five fields and nothing else:

| Field | Content |
|---|---|
| `title` | The alert's title, or *Elevation requested* |
| `body` | The alert message, cut to 240 characters |
| `severity` | `info`, `warning`, `error` or `critical` |
| `url` | The deep link the notification opens |
| `tag` | Groups repeats of one alert into one notification |

No tokens, ids of other users, or anything beyond the text the alert already carried. Opening
the notification loads the screen over your normal, authenticated session.

`critical` and `error` notifications are sent with high urgency and stay on screen until
dismissed.

## Who receives what

| Event | Sent to |
|---|---|
| An alert fires or resolves | Every subscribed user holding `alerts.view` for the alert's environment |
| A temporary elevation is requested | Every subscribed user holding `users.manage` |

The check is made per user at the moment of delivery, so a revoked role stops notifications
immediately.

Each device gets the notification in its owner's language: the language chosen under
*Profile*, or, for someone who never chose one, the instance default (*Settings → General*),
or English. Titles and fixed sentences are translated into English, Dutch, German, Spanish,
French and Portuguese; an alert's message is sent as the rule wrote it, because that text is
yours.

## Turning it on

*Device → Push notifications*:

1. Optionally name the device (*Device name*, e.g. "On-call phone").
2. *Turn on for this device*, and allow notifications when the browser asks.
3. *Send a test notification*. It goes to every one of your devices and reports how many it
   reached, how many failed and how many were removed as dead.

*Turn off for this device* removes it from the server and unsubscribes the browser.

## Your devices

*Device → Your devices* lists every device you have turned on, with its name, the last
successful delivery and a count of consecutive failures. *Revoke* removes one after
confirmation; revoking a device that is not the one in your hand stops it receiving anything,
and it can be turned on again only from that device.

Nobody sees anybody else's devices, and the list never shows the push address or keys; those
are credentials, stored encrypted and never returned by any route.

## Delivery failures

| The push service answers | McHarbor |
|---|---|
| 2xx | Records the delivery time and clears the failure count |
| 404 or 410 | Deletes the device at once: the browser was uninstalled, its data cleared, or the subscription expired |
| Anything else, or no answer | Counts a failure and waits before trying that device again: 1 minute, doubling, up to 6 hours |

A device that has failed every attempt for **30 days** is deleted whatever the code. Only
devices on the known push services are accepted:
`fcm.googleapis.com`, `android.googleapis.com`, `push.services.mozilla.com`,
`push.apple.com` and `notify.windows.com` or a name under one of them, over HTTPS on the
default port, so a
subscription cannot make the server send requests anywhere else.

## The instance push keys

Push uses a VAPID key pair that identifies this McHarbor install to the push services. It is
generated the first time anyone opens *Device*, and the private key is stored encrypted like
every other secret.

Regenerating it (**Settings → General → Notifications → Web push → Regenerate VAPID key**, or
`POST /api/push/vapid/regenerate`, both `settings.manage`) **removes every
device on the instance**, because the push services reject the old subscriptions, and everyone has
to turn push on again. The request is audited at warning severity.

The contact address sent to the push services is `MCHARBOR_PUSH_CONTACT`
(default `https://www.mcharbor.eu`). Set it to an e-mail address (`ops@example.com`, without
`mailto:`; it is added for you) or an `https://` URL that reaches you.

## What goes wrong

| Symptom | Cause |
|---|---|
| *This browser did not start McHarbor's background worker* | The browser refused the service worker. The error icon beside the message shows the browser's own reason; *An SSL certificate error occurred…* means the device does not trust the certificate; see [Trusting McHarbor's own root on a phone](./install.md#trusting-mcharbors-own-root-on-a-phone). With no reason shown, the worker registered but did not start within 15 seconds |
| *Notifications are blocked* | Notifications were denied for this site; allow them in the browser's site settings |
| *Turn on* fails with a server error | The browser's push service is not one of the accepted hosts |
| Test says *1 failed* and nothing arrives | The push service is unreachable from the McHarbor host; it needs outbound HTTPS |
| Notifications stopped after an upgrade | The push keys were regenerated, or the browser's subscription expired; turn it on again |
