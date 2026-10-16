# Push and install

8 endpoints: the web app manifest and icons the phone install uses, and Web Push device
management. The behaviour behind them is in [On-call](../../oncall/README.md).

| Key | Covers |
|---|---|
| — | The manifest and icons are public; the device routes act only on the caller's own devices |
| `settings.manage` | Regenerate the instance VAPID key pair |

## Things worth knowing first

**API keys are refused on every `/api/push` route** with `403`. A subscription belongs to a
browser, and automation has no device to notify.

**The device routes carry no permission middleware on purpose.** Each one reads and changes
only rows owned by the caller, like the rest of your own account; another user's device id
returns `404`.

**Nothing returns the push endpoint or keys.** They are credentials, stored encrypted.

**The subscription host is allow-listed.** `endpoint` must be `https://` on the default port
at `fcm.googleapis.com`, `android.googleapis.com`, `push.services.mozilla.com`,
`push.apple.com` or `notify.windows.com`, or a name under one of them; `keys.p256dh` must decode to 65 bytes and
`keys.auth` to 16. Anything else is `400 err.invalid_body`.

## Routes

| Method | Path | Permission | What it does |
|---|---|---|---|
| `GET` | `/api/pwa/manifest.webmanifest` | public | The web app manifest, `application/manifest+json` |
| `GET` | `/api/pwa/icons/{name}` | public | `192.png`, `512.png`, `maskable-512.png` or `apple-touch-icon.png`, from the branding icon; `ETag`, `404` for any other name |
| `GET` | `/api/push/vapid-public-key` | signed in | `{publicKey}`: the application server key; generates the pair on first call |
| `GET` | `/api/push/subscriptions` | signed in | The caller's devices: `id`, `label`, `userAgent`, `lastSuccessAt`, `failureCount`, `createdAt` |
| `POST` | `/api/push/subscriptions` | signed in | Register this browser. Body `{endpoint, keys: {p256dh, auth}, label, userAgent}`. Re-registering the same endpoint updates the existing device. At most **20** devices per user: a new endpoint past that is `429` `err.rate_limit_exceeded` with `reason: too_many_devices`. `201`, audited |
| `DELETE` | `/api/push/subscriptions/{id}` | signed in | Remove one of the caller's devices. `204`, audited |
| `POST` | `/api/push/test` | signed in | Send a test notification to the caller's devices; `{sent, failed, removed}` |
| `POST` | `/api/push/vapid/regenerate` | `settings.manage` | New key pair; deletes every device on the instance. `{publicKey, removedSubscriptions}`, audited at warning severity |

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `MCHARBOR_PUSH_CONTACT` | `https://www.mcharbor.eu` | The VAPID subject the push services can use to reach the operator: an e-mail address (`mailto:` is added) or an `https://` URL |
