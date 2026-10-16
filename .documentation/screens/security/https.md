# Security: HTTPS and the local certificate authority

McHarbor serves HTTPS from first boot with no configuration. It does that by running a
**local certificate authority**: a long-lived root that signs a short-lived server
certificate. Install the root once per machine and the browser warning is gone for good,
including across every later rotation.

Uploading a publicly-trusted certificate instead is still supported and takes precedence
over everything on this page.

## What is on disk

Under `$DATA_DIR/tls/`:

| File | What it is | Mode |
|---|---|---|
| `ca.pem` | The local root certificate. Public material; this is the file you distribute | `0644` |
| `ca-key.pem` | The root's signing key. Never served, never leaves the host | `0600` |
| `cert.pem` | The served certificate, followed by the root so clients can build the chain | `0600` |
| `key.pem` | The served certificate's key | `0600` |

Both keys are ECDSA P-256. The root is valid for ten years; the server certificate for
**90 days**, reissued automatically once it is inside 30 days of expiry. Renewal is
hot-swapped by the certificate reload watcher within 30 seconds, with no restart.

## Verifying what you are trusting

A certificate you cannot verify is a certificate you are trusting blindly, so the
fingerprint is published in two places that do not share a channel:

- **Settings → Security → HTTPS** shows the SHA-256 of the served certificate, with a copy
  button, and the root's fingerprint beneath it.
- The server log prints both at startup:

  ```
  docker logs mcharbor
  ...
  msg="TLS certificate ready" cert=data/tls/cert.pem sha256=ed:89:36:...
  msg="local root CA ready; install it to trust this host" ca=data/tls/ca.pem sha256=fb:d8:f8:...
  ```

Compare either value against what your browser's certificate viewer reports. They are in
the same lowercase colon-separated form `openssl x509 -fingerprint -sha256` prints.

## Installing the root

**Download root CA** on the HTTPS section saves `mcharbor-root-ca.pem`. It needs
`settings.view`. Then, once per machine:

| Platform | In short |
|---|---|
| macOS | Add to the **System** keychain, then set it to **Always Trust** |
| Windows | Import into **Trusted Root Certification Authorities** |
| Debian / Ubuntu | Copy to `/usr/local/share/ca-certificates/` **as `.crt`**, run `update-ca-certificates` |
| RHEL / Fedora | Copy to `/etc/pki/ca-trust/source/anchors/`, run `update-ca-trust extract` |
| Firefox | Keeps its own store; import under Settings → Privacy & Security → Certificates |
| Chrome / Edge on Linux | Keep their own NSS store: `certutil -d sql:$HOME/.pki/nssdb -A -t "C,,"` |
| Android | User store, via Encryption & credentials. Browsers trust it; apps do not |
| iOS / iPadOS | Install the profile, **then** enable it under Certificate Trust Settings |

Check it took:

```bash
curl --cacert mcharbor-root-ca.pem https://<host>:<port>/api/health
```

A machine has more than one certificate store, and which one to write to is decided by the
browser rather than the operating system: a root installed system-wide on Linux is still
invisible to Chrome and to Firefox. The click paths, the commands, the phone and tablet
procedures, the non-browser clients and the removal steps are all on
[Installing the root CA](./install-root-ca.md).

## Rotating the server certificate

**Regenerate** issues a new server certificate and **keeps the root**. Machines that
already trust the root stay trusted and need no action. Only a client that pinned the old
*server* fingerprint has to be given the new one.

The button is hidden when a certificate was uploaded, and the endpoint refuses the request
as well, so a direct `POST` cannot discard an operator's own certificate.

## Remote agents

An agent verifies TLS by default, which against a privately-issued certificate means the
handshake fails, so the old workaround was `MCHARBOR_INSECURE=true`, which accepts *any*
certificate and removes the protection entirely.

**The installers no longer do that.** The agent dialog carries a **Server uses a
self-signed certificate** switch, and while it is on every install method sets
**`MCHARBOR_PIN`** to this host's **root CA** fingerprint: the one-line script, the Docker
and Compose snippets, the binary and its systemd unit, and the SSH deploy. `MCHARBOR_INSECURE`
is emitted as `false`.

The pin names the **root**, not the served certificate. That is deliberate: the served
certificate is reissued automatically every 90 days, and a pin on it would drop every agent
at each renewal. A root pin survives both renewal and **Regenerate**, and only has to be
replaced if the root itself is ever rotated.

The script and the SSH deploy additionally carry the root inline and pass it to their own
`curl` calls as `--cacert`, so the agent binary is downloaded over a verified connection
rather than with `-k`.

Turn the switch **off** when your certificate is signed by a CA the remote host already
trusts. No pin is then emitted at all: ordinary verification applies, and there is nothing
extra to rotate.

Setting the pin by hand is still possible: copy the **Root CA fingerprint** from the HTTPS
section:

```bash
MCHARBOR_PIN=fb:d8:f8:aa:1f:73:cd:20:...
```

Both the colon-separated and bare-hex forms are read. `MCHARBOR_PIN` takes precedence over
`MCHARBOR_INSECURE` when both are set, and it applies to the WebSocket control channel and
the agent's HTTP calls alike.

The agent does not merely look for the fingerprint somewhere in what the server presents;
it builds a real chain to it and checks the hostname, so appending the genuine root to a
forged chain does not pass. Passing the *server* certificate's fingerprint is rejected with
a message telling you to use the root's.

Pinning is *stronger* than a publicly-trusted certificate with no pin, because it trusts
exactly one CA rather than every public root. No pin is emitted when the host serves an
uploaded certificate: that one may be reissued by a CA McHarbor knows nothing about, and a
pin would break at their next renewal.

## Uploading your own certificate

Paste the PEM certificate and private key under **Upload Certificate**. An uploaded
certificate is stored encrypted, written to `cert.pem` / `key.pem`, and is **never touched
by renewal**; McHarbor only reissues certificates its own CA signed. The source shown in
the UI changes to "User uploaded" and the regenerate button disappears.

## A note on HSTS

`hstsMaxAge` defaults to `0`, off. Turning it on while the certificate is not publicly
trusted makes the browser interstitial **unbypassable** (there is no "Proceed anyway"),
and you can lock yourself out of the panel. Install the root first, confirm it works, and
only then consider HSTS.

## Related

- [Installing the root CA](./install-root-ca.md): every platform, browser and device, step by step
- [Security](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each control does and what gates it
- [Agent configuration](../../agents/configuration/README.md): every agent environment variable
- [Troubleshooting](./troubleshooting.md): symptom, cause and fix
