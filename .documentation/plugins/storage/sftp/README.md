# SFTP

Stores backup archives over SFTP, using `github.com/pkg/sftp` on top of `golang.org/x/crypto/ssh`. Four authentication methods are supported: password, public key, keyboard-interactive, and a local SSH agent.

- **Slug** `storage-sftp`, **version** 2026.09.01, **destination type** `sftp`
- **Host range** `>=2026.09.01`
- **Where it appears** Settings → Integrations → Storage, and in the destination picker on a backup plan
- **Gated by** the host storage screen: `settings.view` or `storage_locations.view` to read, `settings.manage` or `storage_locations.manage` to change. This family declares no permission keys of its own

## How it works

- `private_key_path` and `agent_socket` are paths **inside the McHarbor container**. Mount the key or the agent socket in; a path from your workstation will not resolve.
- OpenSSH and RSA private-key formats are accepted. PuTTY `.ppk` is not; convert it first.
- Set `host_key_fingerprint` to the server's SHA-256 fingerprint to pin it. Left blank, the host key is not verified, which is worth knowing before trusting this destination over an untrusted network.
- `keyboard_interactive` echoes the configured password back to password-style prompts; a genuine second factor cannot be satisfied this way.
- Intermediate directories under **Subdirectory** are created as needed. `connect_timeout` has a 5s floor.

## Capabilities it advertises

| Capability | Set | Meaning |
|---|---|---|
| `EncryptedCredentials` | Yes | Credentials are handed to the plugin decrypted and stored re-encrypted by the host. |
| `SupportsListing` | Yes | The host can list what is already at the destination, which is what selective restore browses. |
| `SupportsDelete` | Yes | Retention can delete old archives at the destination instead of only locally. |

## In this folder

| Page | What it covers |
|---|---|
| [Configuration](./configuration.md) | Prerequisites, every field, and the setup order |
| [Troubleshooting](./troubleshooting.md) | Symptoms, causes and fixes |
| [Developer](../../../development/plugins/storage/sftp.md) | The adapter it registers, its Go layout, and how to build, sign and fork it |

## Related

- [Storage destinations](../README.md): the family, and what the host does with it
- [Plugin catalog](../../catalog.md): every shipped plugin
