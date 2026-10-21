# Kubernetes: Configuration

Everything this plugin asks for, as the plugin itself declares it.

## Before you start

- A role holding at least this plugin's `view` key, or its sidebar section stays hidden.

## Where it appears

| Path | Label key | Permission |
|---|---|---|
| `/plugins/addon-kubernetes` | `nav.overview` | `kubernetes.view` |
| `/plugins/addon-kubernetes/pods` | `nav.workloads` | `kubernetes.view` |
| `/plugins/addon-kubernetes/services` | `nav.network` | `kubernetes.view` |
| `/plugins/addon-kubernetes/storage` | `nav.storage` | `kubernetes.view` |
| `/plugins/addon-kubernetes/config` | `nav.config` | `kubernetes.view` |
| `/plugins/addon-kubernetes/nodes` | `nav.cluster` | `kubernetes.view` |
| `/plugins/addon-kubernetes/clusters` | `nav.clusters` | `kubernetes.clusters.manage` |

Sidebar entries are **hidden** when the permission is missing, unlike buttons, which
render disabled with a tooltip. A dead-end page is not informative.

## Routes

Every route is mounted under the plugin router and carries the permission declared in
the manifest. The host mounts only what the manifest declares.

| Method | Path | Permission | Audit action |
|---|---|---|---|
| `GET` | `/api/kubernetes/clusters` | `kubernetes.view` | — |
| `POST` | `/api/kubernetes/clusters` | `kubernetes.clusters.manage` | `plugin.kubernetes.cluster.created` |
| `GET` | `/api/kubernetes/clusters/{id}` | `kubernetes.clusters.manage` | — |
| `PUT` | `/api/kubernetes/clusters/{id}` | `kubernetes.clusters.manage` | `plugin.kubernetes.cluster.updated` |
| `DELETE` | `/api/kubernetes/clusters/{id}` | `kubernetes.clusters.manage` | `plugin.kubernetes.cluster.deleted` |
| `GET` | `/api/kubernetes/summary` | `kubernetes.view` | — |
| `GET` | `/api/kubernetes/pods/{namespace}/{name}` | `kubernetes.view` | — |
| `GET` | `/api/kubernetes/pods/{namespace}/{name}/logs` | `kubernetes.logs` | — |
| `DELETE` | `/api/kubernetes/pods/{namespace}/{name}` | `kubernetes.delete` | `plugin.kubernetes.pod.deleted` |
| `GET` | `/api/kubernetes/deployments/{namespace}/{name}` | `kubernetes.view` | — |
| `POST` | `/api/kubernetes/deployments/{namespace}/{name}/scale` | `kubernetes.manage` | `plugin.kubernetes.deployment.scaled` |
| `POST` | `/api/kubernetes/deployments/{namespace}/{name}/restart` | `kubernetes.manage` | `plugin.kubernetes.deployment.restarted` |
| `DELETE` | `/api/kubernetes/deployments/{namespace}/{name}` | `kubernetes.delete` | `plugin.kubernetes.deployment.deleted` |

Secrets are encrypted with the host's AES-256-GCM service before they are written, and
the API never returns them once saved. Editing an existing entry leaves a stored secret
in place unless you replace it.

## Setting it up

- Install it from **App Store → Plugins**.
- Grant its permission keys to the roles that should see it.
- Open its section in the sidebar.

## Cluster credentials

A kubeconfig can ask McHarbor to run a program, or to read a file on the McHarbor host.
McHarbor refuses both, because that program or file would run on McHarbor's machine rather
than in the cluster's own login flow. Inline data is accepted.

| Field in the kubeconfig | Accepted | Refused |
|---|---|---|
| `users[].user.token` | Yes | |
| `users[].user.client-certificate-data`, `client-key-data` | Yes | |
| `clusters[].cluster.certificate-authority-data` | Yes | |
| `users[].user.exec` | | Yes |
| `users[].user.auth-provider` | | Yes |
| `users[].user.tokenFile` | | Yes |
| `users[].user.client-certificate`, `client-key` (file paths) | | Yes |
| `clusters[].cluster.certificate-authority` (file path) | | Yes |
| `clusters[].cluster.proxy-url` | | Yes |

Every user and cluster entry is checked, not only the current context, so switching context
later cannot make a refused entry live. A kubeconfig that does not parse is refused too.

- **On save**, the request fails with HTTP 400 and nothing is stored. The error names the entry
  and the field, for example `user "admin" sets exec`.
- **When stored**, a cluster saved before this rule is not connected. The dashboard shows
  **Credentials refused** (reason `credentials_refused`) until you edit it.

The bearer-token form (server URL and token) is not affected by the kubeconfig rule. The
connection checks below apply to it.

### Convert a kubeconfig

1. Where `kubectl` already reaches the cluster, write out the current context with its files
   embedded:

   ```bash
   kubectl config view --minify --flatten --raw > cluster.kubeconfig
   ```

   `--flatten` turns certificate and key files into `-data` fields. `--raw` keeps those values
   readable.
2. Search the file for `exec:`, `auth-provider:`, `tokenFile:` and `proxy-url:`. Any match is
   refused.
3. For a `tokenFile`, replace the line with `token:` and the token. For an `exec` login (a cloud
   CLI or an OIDC plugin), use a token instead: create one with
   `kubectl create token <service-account> -n <namespace>` for a service account that holds the
   RBAC the cluster needs. A token lasts as long as the cluster allows, so plan to renew it.
4. Paste the file into the **Kubeconfig** field and save. Then delete `cluster.kubeconfig`. It
   holds credentials in plain text.

### Connection checks

- Before McHarbor reads a cluster, it sends `GET /version`. Only an answer that is a Kubernetes
  version document passes. A reachable address that answers in any other way shows **Not a
  Kubernetes API** (`api_unverified`). A connection failure, or a non-2xx answer to `/version`,
  shows **Cluster unreachable**.
- Link-local addresses (`169.254.0.0/16`, `fe80::/10`) and cloud metadata endpoints
  (`100.100.100.200`, `fd00:ec2::/32`) are refused and show **Cluster unreachable**. Loopback and
  private addresses stay allowed, for local kind and k3s clusters.
- The check is kept for five minutes per cluster. Editing or removing the cluster runs it again
  on the next request.
- Error text from the cluster is shown only after the check passes. A verified API server's own
  `Status` message, such as an RBAC denial, is kept. Any other error body is dropped.

## Host scopes it requests

These are the capabilities the manifest asks the host for. The installer refuses a
manifest asking for a scope outside the known set.

| Scope token | What it allows |
|---|---|
| `menu.sidebar.add` | Add an entry to the sidebar. |
| `i18n.namespace.add:kubernetes` | Merge the `kubernetes` translation namespace into the host bundles. |
| `rbac.permission.register` | Reserve the permission keys it declares in `adds_permissions`. |
| `db.migrations.run` | Create and migrate its own tables at install time. |
| `audit.emit` | Write entries into the tamper-evident audit trail. |
| `reporting.adapter.register` | Contribute report sections to the Reporting plugin. |
| `alerting.adapter.register` | Contribute alert conditions the host evaluates on a schedule. |

## Related

- [Kubernetes](./README.md): what it is and how it works
- [Troubleshooting](./troubleshooting.md): when a field is right and it still fails
