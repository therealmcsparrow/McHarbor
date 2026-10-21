# Kubernetes: Troubleshooting

| Symptom | Usual cause | Fix |
|---|---|---|
| The sidebar section does not appear | The role has none of the `kubernetes.*` keys | Navigation entries are hidden rather than disabled; grant `kubernetes.view`. |
| Cluster connects, every list is empty | The service account's RBAC does not allow listing in those namespaces | The plugin surfaces 403s as empty lists; check with `kubectl auth can-i --list` as that account. |
| Logs are refused for a user who can see pods | Missing `kubernetes.logs` | Grant it deliberately: it is a credential-adjacent read. |
| Save fails and the error names a user or cluster entry and a field | The kubeconfig names an exec plugin, auth provider, token file, certificate or key file, CA file, or proxy URL | Convert it as in [Cluster credentials](./configuration.md#cluster-credentials), then save again. |
| The dashboard shows **Credentials refused** on a cluster that worked before | The stored kubeconfig was saved before this rule and is now refused | Edit the cluster and paste a converted kubeconfig. |
| The dashboard shows **Not a Kubernetes API** | The address answers, but not with a Kubernetes `/version` document | Check the server URL and port. Point it at the API server, not a web page on that address. |
| The dashboard shows **Cluster unreachable** for a `169.254.x.x` or metadata address | Link-local and cloud metadata addresses are refused by design | Use the cluster's real API server address. |
A feature plugin has no connection to test. What to check after installing is that its
sidebar section appears; it stays hidden until a role holds at least the `view` key it
declares.

## Reason codes

The dashboard and the Kubernetes page report a cluster's state with one of these reasons.

| Reason | Dashboard title | Meaning | Fix |
|---|---|---|---|
| `no_environment` | No cluster connected | No cluster is configured yet. | Add one on the Connections page. |
| `not_kubernetes` | (the page message) | The selected environment is a Docker host, not a cluster. | Pick a cluster. |
| `unreachable` | Cluster unreachable | The connection failed, `/version` answered with an error, or the address is link-local or a metadata endpoint. | Check the server URL, the network and the credentials. |
| `credentials_refused` | Credentials refused | The stored kubeconfig names an exec plugin, auth provider, host file or proxy. | Convert the kubeconfig as in [Cluster credentials](./configuration.md#cluster-credentials). |
| `api_unverified` | Not a Kubernetes API | The address answered, but not with a Kubernetes version document. | Check the server URL. |

## Reading a failure

The host logs the real error server-side and returns an i18n code to the browser, so a
toast will tell you *that* something failed rather than which line of the response said
so. For anything that is not obvious from the table above, the container log is where
the provider's own message appears:

```bash
docker compose logs mcharbor | grep -i addon-kubernetes
```

This plugin owns tables, so a failure at install time may be a migration rather than a
configuration problem. Its migrations are `0001_init.sql`.

## Related

- [Configuration](./configuration.md): every field and what it is for
- [Kubernetes](./README.md): what it is and how it works
