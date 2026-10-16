# Environments: Fields

Every column, filter and form field on Environments, across all five connection types
(socket, TCP, TLS, SSH and agent), and the Kubernetes platform path, which is not an
environment at all.

## Columns

| Column | What it is |
|---|---|
| Name | Your label |
| Platform | Docker or another platform |
| Connection | Socket, TCP, TLS, SSH or Agent. Agent rows and cards carry a shield: green when the agent's control channel is encrypted, orange when it is plaintext or has certificate verification turned off. Hover it for the exact mode. |
| Endpoint | Where it connects |
| Version | Docker version reported |
| Status | Connected or disconnected |
| Type | How the environment is managed |

## Filters and facets

| Filter | What it narrows |
|---|---|
| Search | Environment name |
| View | Table or card |

## Form fields

| Field | Type | What it is |
|---|---|---|
| Name | text | Your label, e.g. "My Docker Host" |
| Connection type | select | Unix Socket, TCP, TLS, SSH or Remote Agent |
| Socket path | text | For a local socket |
| Host, Port | text | For TCP, TLS and SSH |
| CA certificate, Client certificate, Client key | secret | For TLS |
| SSH user, SSH key or password, Host fingerprint | mixed | For SSH and for SSH agent deployment |
| Deployment method | select | Manual (copy commands) or Automated via SSH |
| Deploy as | select | Docker container or system binary |
| Use existing agent token | toggle + text | Paste `agt_` + 64 hex characters instead of generating one |
| Kubeconfig, Bearer token, Server URL, Default namespace, CA certificate | mixed | The Kubernetes platform path |

Kubernetes clusters are **not** environments; they are managed by the Kubernetes plugin
and keep their own connections. The kubeconfig and bearer-token fields belong to that
platform path.

## Shared list behaviour

Every list screen inherits the same controls: free-text search, a filter mode of
**Contains**, **Exact** or **Regex** (an invalid expression says so rather than silently
matching nothing), saved filters and searches, selection with a bulk bar, and pagination
showing "X to Y of Z" with a rows-per-page control.

## Related

- [Environments](./README.md): what the screen is and what it shows
- [Actions](./actions.md): what you can do with these fields
