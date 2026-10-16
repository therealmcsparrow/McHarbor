# Security: Troubleshooting

What goes wrong with access and TLS. Several are deliberate refusals rather than faults:
you cannot grant a role containing a permission you do not hold yourself, and the HTTPS
switches wait for a restart even though certificate files on disk do not.

| Symptom | Usual cause | Fix |
|---|---|---|
| Password reset emails never arrive | No email channel is selected under Password → Email delivery | Links are issued but not sent until one is picked |
| A role cannot be granted to an API key | It contains a permission you do not hold | "You do not hold every permission in this role", by design |
| A new certificate has no effect | The switches need a restart | Certificate files on disk are picked up within ~30s; the Enable and Force switches are a restart |
| The root CA is installed and the browser still warns | The store you wrote to is not the one that browser reads | Firefox and Chrome-on-Linux keep their own. See [Installing the root CA](./install-root-ca.md) |
| The warning names the **address**, not the issuer | The name you typed is not a SAN on the leaf | Add it with `SELF_SIGNED_SAN_EXTRA` and press **Regenerate** |
| A phone cannot download the root CA | The download needs a signed-in session, which needs the certificate to be trusted | Fetch it on a machine that can sign in, or `docker cp mcharbor:/app/data/tls/ca.pem` |
| A federated user lands with no access | No group mapping | Map the directory group to a McHarbor role; group fetch has to be on |
| Sessions end sooner than expected | Automatic sign-out is on | Check the inactivity window; only real input counts as activity |

## When a control is disabled

A disabled control with a tooltip naming a permission is RBAC, not a fault: the tooltip
names the exact key to ask for. A disabled control with no tooltip is usually state: the
action does not apply to the current status of the thing you selected.

## Related

- [Installing the root CA](./install-root-ca.md): per platform, browser and device
- [Security](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what each action really does
