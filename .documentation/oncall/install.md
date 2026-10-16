# Installing on a phone

## What the browser is given

| Item | Where | Notes |
|---|---|---|
| Web app manifest | `/api/pwa/manifest.webmanifest` | Generated per request. Name *McHarbor*, `start_url` and `id` `/oncall`, `display: standalone`, portrait. The title-bar colour follows *Settings → Appearance → Accent*. |
| Icons | `/api/pwa/icons/192.png`, `512.png`, `maskable-512.png`, `apple-touch-icon.png` | Rendered from the uploaded branding icon (*Settings → Branding*), or the McHarbor mark when none is uploaded. The maskable and Apple icons sit on an opaque background inside the platform's safe zone; the other two keep transparency. |
| Service worker | `/sw.js` | Served with `Cache-Control: no-cache`, scope `/`. Registered only by a production build over HTTPS. |

The manifest and icons are public: the browser fetches them without a session, and the
branding they show is already on the login page. They carry an `ETag`, so an icon changes as
soon as the branding does; an already-installed app picks the new icon up whenever the
platform next refreshes it, which it does on its own schedule.

## The certificate comes first

Service workers and push both require a **trusted** HTTPS connection. McHarbor serves HTTPS
on port 8705 with a certificate signed by its own root. A desktop browser can be taught to
trust that root (*Settings → Security → TLS*); on a phone it is harder, and some mobile
browsers will not register a service worker on a certificate they do not trust even after
you click through the warning. When that happens the app still opens, but it will not install,
will not work offline and cannot receive push, and *Device* says so.

**The supported configuration for phone use is a publicly trusted certificate** (the bundled
ACME companion, or a certificate you upload) on a name the phone can reach.

### Trusting McHarbor's own root on a phone

On a home or lab network without a public name, install McHarbor's root on the phone instead.
Download it from *Security → HTTPS → Download root CA* (`settings.view`); it is
`mcharbor-root-ca.pem`. Open the address the phone uses and check it is listed in the
certificate's names on the same tab. A host name or IP that is not listed stays untrusted
even with the root installed.

- **Android:** *Settings → Security → More security settings → Encryption & credentials →
  Install a certificate → CA certificate*, and pick the file. (Menu names vary by maker.)
  Chrome, Edge and Samsung Internet trust user-installed roots for websites; Firefox does
  not until *Use third-party CA certificates* is turned on in its secret settings.
- **iPhone and iPad:** send the file to the device and open it, install the profile under
  *Settings → General → VPN & Device Management*, then turn it on under *Settings → General →
  About → Certificate Trust Settings*. Installing the profile alone does not trust it.

Then close the browser or the installed app completely and open McHarbor again; the warning
page must be gone before the service worker can register.

## Android (Chrome, Edge, Samsung Internet, Firefox)

1. Open McHarbor and sign in.
2. Use the browser's *Install app* or *Add to Home screen*.
3. Open it from the home screen. It starts on *Alerts*.

## iPhone and iPad

iOS gives push notifications **only to an app added to the Home Screen**, and the permission
prompt must follow a tap. So the order matters:

1. Open McHarbor in **Safari** and sign in.
2. Tap *Share*, then *Add to Home Screen*.
3. Open McHarbor **from the Home Screen icon**, not from Safari.
4. Go to *Device* and tap *Turn on for this device*; allow notifications when asked.

Until the app is opened from the Home Screen, *Device* shows the install instruction instead
of the push controls. Push needs iOS 16.4 or later.

## Updates

A new McHarbor build installs a new service worker, which takes over at once and discards
the previous build's cached files. An open app then shows *A new version of McHarbor is
ready.* with a *Reload* button. The worker is re-checked every time the app comes back to
the foreground, so a phone that stays installed for months still follows the server it talks
to.

## What goes wrong

| Symptom | Cause |
|---|---|
| No install option in the browser menu | The certificate is not trusted, or the page is plain HTTP |
| The installed icon is the McHarbor mark, not yours | No branding icon was uploaded, or it was uploaded after install and the platform has not refreshed it yet |
| *Device* shows the iPhone instruction on an installed app | The app was opened from Safari rather than from the Home Screen icon |
