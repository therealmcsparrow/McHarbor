# Content-Security-Policy

McHarbor sends a `Content-Security-Policy` header on everything its own server serves: the
UI shell, its assets, plugin bundles and every `/api` response. The policy limits what script
running in the McHarbor origin can load and where it can send data. That matters here more
than in most web apps, because the origin also runs **plugin UMD bundles**: third-party code
executing next to the session cookie and the Docker control plane.

## The two variables

| Variable | Default | What it does |
|---|---|---|
| `CONTENT_SECURITY_POLICY` | the policy below | The policy, verbatim. **Set it to an empty value to send no CSP at all**. This is the escape hatch when a plugin breaks under the policy. Unset means the default; set-but-empty means disabled |
| `CONTENT_SECURITY_POLICY_REPORT_ONLY` | `false` | `true` sends the same policy as `Content-Security-Policy-Report-Only`: the browser reports each violation in its developer console and blocks nothing |

Both are read once at start; change them and restart.

```bash
# Trial a policy without enforcing it
CONTENT_SECURITY_POLICY_REPORT_ONLY=true

# Disable the header entirely (compose: an explicit empty string)
CONTENT_SECURITY_POLICY=
```

## The default policy

```
default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'; object-src 'none'
```

| Directive | Why it is what it is |
|---|---|
| `default-src 'self'` | Anything not named below (frames, media, manifests, workers) comes from McHarbor itself or not at all |
| `script-src 'self'` | The built UI has no inline `<script>`, and plugin bundles are served same-origin from `/api/plugins/loader/bundle`, so no allowlist and no `'unsafe-inline'` / `'unsafe-eval'` are needed. The service worker (`/sw.js`) is same-origin too |
| `style-src 'self' 'unsafe-inline'` | **Known relaxation.** Radix, Recharts, xterm.js and sonner inject `<style>` elements at runtime. Cannot be tightened while those libraries are in use |
| `img-src 'self' data: blob: https:` | **Known relaxation.** `https:` covers remote app-catalog icons and plugin icons (the compute plugin draws distribution logos from public icon CDNs). `data:` covers inline SVG in the stylesheet and cursor images; `blob:` covers brand-mark rasterisation in the browser |
| `font-src 'self'` | Fonts are self-hosted under `/fonts`, deliberately, for air-gapped installs |
| `connect-src 'self'` | Every API call, server-sent event stream and WebSocket (terminal, host terminal, VM and container consoles, agent and system sockets) is same-origin. In CSP Level 3 browsers `'self'` also matches `wss://` on the same host |
| `frame-ancestors 'none'` | McHarbor cannot be framed. Duplicates `X-Frame-Options: DENY`, which is still sent for older browsers |
| `base-uri 'self'` | An injected `<base>` cannot re-point relative URLs |
| `form-action 'self'` | Forms can only post back to McHarbor |
| `object-src 'none'` | No plugins-in-the-browser-sense (`<object>`, `<embed>`) |

## The one response with its own policy

The SAML **HTTP-POST binding** page served by `GET /api/identity-providers/{id}/authorize` is a
self-submitting form: an inline `onload` handler posts the request to the identity provider's
own origin, which `script-src 'self'` and `form-action 'self'` would both block. That single
response replaces the policy with
`default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; form-action *; frame-ancestors 'none'; base-uri 'none'`,
keeping report-only mode if that is in force, and sending nothing if the header is disabled.
Nothing else is ever served under it.

## Rolling it out

The sixty-odd in-tree plugins were checked for remote scripts, fonts and API calls from the
browser, and none makes one. A **third-party** plugin that loads a remote font, image host or
API endpoint will break visibly. The safe order is:

1. Start with `CONTENT_SECURITY_POLICY_REPORT_ONLY=true`.
2. Walk the surface you actually use (dashboard, terminal, a VM or container console,
   workflows, each plugin page, plugin form and plugin widget) with the browser console open.
3. Every `[Report Only] Refused to …` line names the directive and the blocked URL. Either
   widen that one directive in your own `CONTENT_SECURITY_POLICY`, or remove the plugin.
4. Unset `CONTENT_SECURITY_POLICY_REPORT_ONLY` and restart.

When you override the policy, write the whole policy: the variable replaces the default, it
does not merge with it.

## What it does not cover

- **The Vite dev server** (`npm run dev` on `:8173`) serves its own HTML with inline HMR
  scripts; the header is only on what the Go server serves, so development is unaffected.
- **A reverse proxy that sets its own CSP**: the browser enforces both, so the effective
  policy is the stricter intersection. Set it in one place.

## Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| A plugin page is blank; console says `Refused to load the script` or `Refused to connect` | The plugin loads something from another origin | Report-only first, then widen the named directive, or disable the header with `CONTENT_SECURITY_POLICY=` |
| App-store or plugin icons missing on an `http://` icon URL | `img-src` allows `https:` only | Serve the icon over HTTPS, or add `http:` to `img-src` in your own policy |
| SAML sign-in stops at a blank "Redirecting…" page | A proxy in front adds its own `form-action` or `script-src` | Remove the proxy's CSP for `/api/identity-providers/*/authorize` |

## Related

- [Security posture](../security-posture/README.md): the other settings an internet-facing host needs
- [Configuration](../../configuration/README.md): every environment variable
