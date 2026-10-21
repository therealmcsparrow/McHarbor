# Security Policy

## Reporting a vulnerability

Email **security@mcsparrow.eu**. Do not open a public GitHub issue, discussion or pull
request for a suspected vulnerability.

Please include:

- the McHarbor version and build (Settings → Updates, or the `VERSION` and `BUILD` files),
  and the agent or plugin version if one is involved;
- how McHarbor is deployed (container, native package, remote agent);
- the steps to reproduce, the permissions the attacker needs, and what they gain;
- a proof of concept, if you have one.

Never send real credentials, kubeconfigs, agent tokens or data from someone else's
installation.

## What happens next

| Step | Target |
|---|---|
| Acknowledgement | within 3 working days |
| First assessment (confirmed or not, and severity) | within 10 working days |
| Fix released and coordinated disclosure | within 90 days of the report |

We keep you informed until the issue is resolved and agree on the disclosure date with
you. If you want to be credited in the release notes, tell us the name to use.

## Supported versions

Only the latest McHarbor release receives security fixes. McHarbor uses calendar
versions (`yyyy.MM.<sequence>`). The core and the remote agent share one version, so the
fix ships for both. Plugins are versioned separately, and a fix ships as a new plugin
version in the Plugin Store.

| Version | Supported |
|---|---|
| Latest release | Yes |
| Any earlier release | No, upgrade to the latest |

## Scope

In scope: the McHarbor backend and web UI, the remote agent, the CLI, the Terraform
provider, the plugins published in the McHarbor plugin catalog, and the published
container images and native packages.

Out of scope: third-party plugins, problems that require an attacker who already holds
the admin role or the host's Docker socket, and vulnerabilities in upstream software
(Docker, Kubernetes, QEMU) that McHarbor does not make worse. Report those upstream.

Testing is allowed only against an installation you own or have permission to test.
Do not degrade other people's service, and do not access or keep data that is not yours.
