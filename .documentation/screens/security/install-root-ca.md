# Installing the McHarbor root CA

McHarbor signs its own server certificate with a **local root certificate authority**. Install
that root once on a machine and every browser on it opens McHarbor with no warning, and keeps
doing so after the server certificate rotates, because the root is what you trusted, not the
leaf.

This page is the *procedure*, platform by platform. [HTTPS](./https.md) is the *model*: what is
on disk, how long each certificate lives, and how rotation works.

## Get the file

**Settings → Security → HTTPS → Download root CA** saves `mcharbor-root-ca.pem`. It needs
`settings.view`, so you have to be signed in, which is the awkward part on a device that
cannot open McHarbor yet. Two ways around it:

- Download it once on a machine that *can* get in, then move the file to the others by whatever
  you normally use: a share, AirDrop, a USB stick, a message to yourself. The root is public
  material; there is nothing secret in it.
- Copy it straight off the host:

  ```bash
  docker cp mcharbor:/app/data/tls/ca.pem ./mcharbor-root-ca.pem
  ```

The served certificate also carries the root as the second element of its chain, so a machine
that can reach the port at all can extract it without signing in:

```bash
openssl s_client -showcerts -connect <host>:8705 < /dev/null 2>/dev/null \
  | awk '/BEGIN CERT/{n++} n==2' > mcharbor-root-ca.pem
```

## Verify it before you install it

Installing a root grants it the power to vouch for any name, for every program on that machine.
Do not install one you fetched over a path you have not checked.

Print the fingerprint of the file you are holding:

```bash
openssl x509 -in mcharbor-root-ca.pem -noout -fingerprint -sha256 -subject -dates
```

It must match the **Root CA fingerprint** shown under Settings → Security → HTTPS, which is
also printed in the server log at every start:

```bash
docker logs mcharbor | grep "local root CA"
msg="local root CA ready; install it to trust this host" ca=data/tls/ca.pem sha256=fb:d8:f8:...
```

The subject reads `CN = McHarbor Local CA, O = McHarbor`. That is also the name to look for
when you want to remove it again.

## The one thing that decides where it goes

A machine does not have one certificate store. It has between one and three, and which one you
have to write to depends on the browser, not on the operating system:

| Client | Reads from |
|---|---|
| Safari, Chrome and Edge on macOS | The macOS keychain |
| Chrome, Edge and every Windows application | The Windows certificate store |
| Chrome and Edge on Linux | The **NSS** store in `~/.pki/nssdb`, *not* `/etc/ssl` |
| Firefox and Thunderbird, on every platform | Their **own** store, always |
| `curl`, `wget`, `git`, most daemons on Linux | `/etc/ssl/certs`, via `update-ca-certificates` |
| Node, Python, Java | Their own bundles. See [Non-browser clients](#non-browser-clients) |

So "I installed it and Firefox still warns" is not a failure, and neither is "I installed it
system-wide and Chrome on Linux still warns". Both stores need their own copy.

## macOS

**Keychain Access.** Double-click `mcharbor-root-ca.pem`, and when the import dialog asks,
choose the **System** keychain rather than **login**; a login-keychain root covers that one
account and nothing that runs outside it. Then find *McHarbor Local CA* under **System →
Certificates**, open it, expand **Trust**, and set **When using this certificate** to **Always
Trust**. Close the window; macOS asks for your password.

Or in one command:

```bash
sudo security add-trusted-cert -d -r trustRoot \
  -k /Library/Keychains/System.keychain mcharbor-root-ca.pem
```

Safari, Chrome and Edge pick it up immediately. Firefox does not. See
[Firefox and Thunderbird](#firefox-and-thunderbird).

## Windows

Run `certlm.msc` (local machine, covers every user) or `certmgr.msc` (your account only), open
**Trusted Root Certification Authorities → Certificates**, right-click, **All Tasks → Import**,
and point it at `mcharbor-root-ca.pem`. Set "Place all certificates in the following store" to
**Trusted Root Certification Authorities**; the wizard's automatic choice puts it somewhere
that has no effect.

Or in an elevated PowerShell:

```powershell
Import-Certificate -FilePath .\mcharbor-root-ca.pem -CertStoreLocation Cert:\LocalMachine\Root
```

Chrome, Edge and everything else that reads the Windows store trust it from the next new tab.
Firefox needs its own step, or `security.enterprise_roots.enabled` set to `true` in
`about:config`, which makes Firefox read the Windows store as well.

## Debian and Ubuntu

The trust updater only reads files ending in `.crt`, so **rename it**. This is the single most
common reason a correct install does nothing:

```bash
sudo cp mcharbor-root-ca.pem /usr/local/share/ca-certificates/mcharbor-root-ca.crt
sudo update-ca-certificates
```

It prints `1 added`. Restart anything long-running that already read the old bundle.

## RHEL, Fedora, CentOS Stream, Rocky and Alma

```bash
sudo cp mcharbor-root-ca.pem /etc/pki/ca-trust/source/anchors/
sudo update-ca-trust extract
```

## Alpine

```bash
sudo apk add ca-certificates
sudo cp mcharbor-root-ca.pem /usr/local/share/ca-certificates/mcharbor-root-ca.crt
sudo update-ca-certificates
```

## Chrome and Edge on Linux

These read NSS, not `/etc/ssl`, so the system install above does not reach them:

```bash
sudo apt install libnss3-tools          # nss-tools on Fedora
certutil -d sql:$HOME/.pki/nssdb -A -t "C,," \
  -n "McHarbor Local CA" -i mcharbor-root-ca.pem
```

`-t "C,,"` is the trust flag that matters: trusted to issue **server** certificates and nothing
else. Restart the browser. `certutil -d sql:$HOME/.pki/nssdb -L` lists what is there.

## Firefox and Thunderbird

Firefox ships its own root store on every platform and ignores the operating system's unless
told otherwise.

**Per profile:** **Settings → Privacy & Security → Certificates → View Certificates →
Authorities → Import**, pick `mcharbor-root-ca.pem`, and tick **Trust this CA to identify
websites**. The other two boxes are for email and software signing; leave them clear.

**Across a fleet**, a `policies.json` beside the Firefox binary is less work than visiting every
profile:

```json
{
  "policies": {
    "Certificates": {
      "ImportEnterpriseRoots": true,
      "Install": ["/usr/local/share/ca-certificates/mcharbor-root-ca.crt"]
    }
  }
}
```

`ImportEnterpriseRoots` alone is enough on Windows and macOS, where it makes Firefox read the OS
store you have already populated.

## Android

Android splits its store in two, and the difference decides how far this gets you.

**Browsers** use the user store, which you can write to:

1. Copy `mcharbor-root-ca.pem` to the device and rename it `mcharbor-root-ca.crt`.
2. **Settings → Security & privacy → More security settings → Encryption & credentials →
   Install a certificate → CA certificate**, acknowledge the warning, pick the file.
3. It appears under **Trusted credentials → User**.

Android then shows a standing "network may be monitored" notice. That is what a private root
looks like on Android; it is not an error.

**Apps are a different matter.** Since Android 7 an app trusts the user store only if its own
network-security configuration opts in, and almost none do. A browser will trust McHarbor; an
arbitrary app talking to the same host will not. There is no fix short of writing to the system
store on a rooted device, which is not worth doing for this.

## iOS and iPadOS

Two steps, and skipping the second is why it usually "does not work":

1. Open or AirDrop `mcharbor-root-ca.pem` to the device. iOS takes it as a *profile*:
   **Settings → General → VPN & Device Management → Downloaded Profile → Install**, then enter
   the passcode.
2. **Settings → General → About → Certificate Trust Settings**, and switch **McHarbor Local CA**
   on under "Enable full trust for root certificates".

Step 1 installs it. Step 2 is what makes Safari and every app on the device actually trust it.

## Non-browser clients

| Client | How |
|---|---|
| `curl` | The system store after the steps above, or `--cacert mcharbor-root-ca.pem` for a one-off |
| `wget` | `--ca-certificate=mcharbor-root-ca.pem` |
| `git` | `git config --global http.sslCAInfo /path/to/mcharbor-root-ca.pem` |
| Node.js | `NODE_EXTRA_CA_CERTS=/path/to/mcharbor-root-ca.pem`. It adds to the bundled roots rather than replacing them. Node does not read the OS store |
| Python `requests` | `REQUESTS_CA_BUNDLE=/path/to/mcharbor-root-ca.pem`, or append the file to the `certifi` bundle |
| Java | `keytool -importcert -alias mcharbor -cacerts -file mcharbor-root-ca.pem`; the default store password is `changeit` |
| A container | Mount the file in and run the image's own trust updater, or set the language variable above |
| The McHarbor agent | Trust the root on the host as normal, or pin the **server** certificate with `MCHARBOR_PIN`. See [HTTPS → Remote agents](./https.md) |

## Check it took

```bash
curl --cacert mcharbor-root-ca.pem https://<host>:8705/api/health
curl https://<host>:8705/api/health          # after a system-store install, no flags
```

A `{"success":true,...}` with no certificate complaint is the answer. In a browser the padlock
is the answer, and the certificate viewer should name *McHarbor Local CA* as the issuer.

## When the root is installed and the warning is still there

| What the browser says | What it means |
|---|---|
| `ERR_CERT_AUTHORITY_INVALID`, or "unknown issuer" | The store you wrote to is not the one this browser reads. Firefox and Chrome-on-Linux both keep their own |
| `ERR_CERT_COMMON_NAME_INVALID`, or "name does not match" | The root is trusted; the **address you typed is not in the certificate**. See below |
| `NET::ERR_CERT_DATE_INVALID` | The leaf expired, so renewal is not running. Check the server log, or press **Regenerate** |
| The warning cannot be dismissed at all | HSTS is on. Installing the root is the only way past it, by design |

**The name has to be in the certificate.** The leaf covers `localhost`, the host's own hostname,
`host.docker.internal`, the loopback addresses, and every address bound to a live non-loopback
adapter at the moment it was issued. A name you invented for it (`mcharbor.lan`, a
reverse-proxy hostname, an address the host moved to afterwards) is not in there. Add it and
reissue:

```yaml
environment:
  - SELF_SIGNED_SAN_EXTRA=mcharbor.lan,10.0.0.10
```

Then **Settings → Security → HTTPS → Regenerate**. Entries that parse as an IP address become IP
SANs, the rest become DNS names. The root is untouched, so nothing you installed has to be
installed again.

## Removing it

| Platform | How |
|---|---|
| macOS | Keychain Access → System → Certificates → delete, or `sudo security delete-certificate -c "McHarbor Local CA" /Library/Keychains/System.keychain` |
| Windows | `certlm.msc` → Trusted Root Certification Authorities → delete, or `certutil -delstore Root "McHarbor Local CA"` |
| Debian/Ubuntu | `sudo rm /usr/local/share/ca-certificates/mcharbor-root-ca.crt && sudo update-ca-certificates --fresh` |
| RHEL/Fedora | `sudo rm /etc/pki/ca-trust/source/anchors/mcharbor-root-ca.pem && sudo update-ca-trust extract` |
| NSS | `certutil -d sql:$HOME/.pki/nssdb -D -n "McHarbor Local CA"` |
| Firefox | Certificate Manager → Authorities → select → **Delete or Distrust** |
| Android | Settings → Encryption & credentials → Trusted credentials → User → remove |
| iOS | Settings → General → VPN & Device Management → the profile → Remove Profile |

Deleting `ca.pem` and `ca-key.pem` from `$DATA_DIR/tls/` makes McHarbor mint a **new** root on
the next start, which invalidates every install of the old one. That is the recovery path for a
compromised signing key, not routine maintenance.

## Related

- [HTTPS](./https.md): what the CA is, what is on disk, rotation and agent pinning
- [Actions](./actions.md): what Download root CA and Regenerate are gated on
- [Troubleshooting](./troubleshooting.md): symptom, cause and fix
- [Installation → TLS](../../getting-started/installation/README.md): the certificate at first boot
- [Configuration](../../getting-started/configuration/README.md): `SELF_SIGNED_SAN_EXTRA` and the TLS variables
