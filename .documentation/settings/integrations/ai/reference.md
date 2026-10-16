# AI: Reference

| Setting | Default | What it does |
|---|---|---|
| **Name** | — | Your label for the connection |
| **API base URL** | the provider’s default | Editable, which is what makes an internal gateway or proxy usable without writing a plugin |
| **Default model** | — | Used when a request does not pick one |
| **How this connection authenticates** | API key | `API key` or `Sign in` (OAuth device flow) |
| **API key** | — | Encrypted at rest. A saved key shows only "API key set" |
| **Client ID** | — | Sign-in only. Your own OAuth application, registered with the provider |
| **Client secret** | — | Sign-in only. Encrypted at rest; leave blank for a public client, or to keep the stored one |
| **Device authorization URL** | prefilled for known providers | Sign-in only. Where the short code is requested |
| **Token URL** | prefilled for known providers | Sign-in only. Where the approved code is exchanged for a token |
| **Scopes** | prefilled for known providers | Sign-in only. Space-separated |
| **Enabled** | On | Whether this connection may be used |
| **Use as default provider** | Off | One connection is the default |

**Send test** exercises the credential against the real provider and reports what came back.
For a sign-in connection that is what exercises the token, including a refresh when the
stored one has expired.

**Sign in** starts the device flow, **Disconnect** drops the stored token but keeps the
endpoints, so signing in again takes one click.

Deleting a connection is blunt and the dialog says so: "Features pointed at this connection
stop working until another provider is selected."

## When something does not behave

| Symptom | Usual cause | Fix |
|---|---|---|
| The provider list is empty | No AI provider plugin installed | Install one from the App Store |
| A local provider will not connect | `localhost` inside the container is the container | Use the LAN address, or `host.docker.internal` on Docker Desktop |
| **Test** returns 401 | Revoked key, or a key from another account or project | Issue a fresh key; the stored one is never echoed back |
| **Test** says the connection is not signed in | A sign-in connection whose device flow was never completed | Open **Edit** and press **Sign in** |
| The code expires before you approve it | The provider's window is short, often 15 minutes | Press **Sign in** again for a fresh code |
| **Sign in** reports a missing configuration | No client ID, or an endpoint that is not an absolute URL | Fill in the client ID and both URLs from your OAuth application |
| The model dropdown is empty | No entitlements on the account, or the list call is blocked | Check the provider console, then outbound access from the container |
| The assistant answers but will not act | Missing `ai_assistant.act`, or a provider with no tool support | Perplexity reports no tool support; the others do |

## Related

- [AI](./README.md): what the section is for
- [Settings](../../README.md): the other groups
