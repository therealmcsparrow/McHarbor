## mcharbor

Operate McHarbor from a terminal or a CI job

### Synopsis

mcharbor is the command-line client for McHarbor. It is an API client with the same permissions and audit trail as the web interface: it authenticates with an API key and every change it makes is recorded against that key.

### Options

```
      --ca-file string     PEM root to trust for the server certificate ($MCHARBOR_CA_FILE)
      --confirm string     name the target of an irreversible change to confirm it non-interactively
      --context string     context to use instead of the current one ($MCHARBOR_CONTEXT)
      --env string         environment id for scoped commands ($MCHARBOR_ENV)
  -h, --help               help for mcharbor
      --insecure           skip certificate verification; for labs only ($MCHARBOR_INSECURE=true)
      --json               shorthand for --output json
  -o, --output string      output format: table, json, yaml or tsv (default "table")
      --timeout duration   give up on a request after this long; 0 waits forever (default 1m0s)
      --url string         McHarbor URL, overriding the context ($MCHARBOR_URL)
      --yaml               shorthand for --output yaml
      --yes                skip the prompt for disruptive but reversible changes
```

### SEE ALSO

* [mcharbor agent](mcharbor_agent.md)	 - Remote agents
* [mcharbor ai](mcharbor_ai.md)	 - The AI assistant's providers and proposed actions
* [mcharbor api-key](mcharbor_api-key.md)	 - API keys
* [mcharbor app](mcharbor_app.md)	 - The Docker app catalog
* [mcharbor audit](mcharbor_audit.md)	 - The audit log
* [mcharbor backup](mcharbor_backup.md)	 - Container backup plans, runs and restores
* [mcharbor catalog](mcharbor_catalog.md)	 - App and plugin catalog sources
* [mcharbor cluster](mcharbor_cluster.md)	 - Inspect a multi-node installation
* [mcharbor completion](mcharbor_completion.md)	 - Generate the autocompletion script for the specified shell
* [mcharbor container](mcharbor_container.md)	 - Containers in the selected environment
* [mcharbor context](mcharbor_context.md)	 - Manage the McHarbor servers this CLI knows
* [mcharbor doctor](mcharbor_doctor.md)	 - Diagnose configuration, connectivity, TLS, credentials, version and clock
* [mcharbor elevation](mcharbor_elevation.md)	 - Temporary permission elevation
* [mcharbor env](mcharbor_env.md)	 - Environments: the Docker hosts McHarbor manages
* [mcharbor gitops](mcharbor_gitops.md)	 - GitOps pipelines and promotions
* [mcharbor group](mcharbor_group.md)	 - User groups
* [mcharbor identity](mcharbor_identity.md)	 - Identity providers
* [mcharbor image](mcharbor_image.md)	 - Images in the selected environment
* [mcharbor login](mcharbor_login.md)	 - Save a server and an API key as a context, after checking the key works
* [mcharbor logout](mcharbor_logout.md)	 - Forget a context's API key; the context itself is kept
* [mcharbor network](mcharbor_network.md)	 - Networks in the selected environment
* [mcharbor plugin](mcharbor_plugin.md)	 - Installed plugins and the plugin store
* [mcharbor policy](mcharbor_policy.md)	 - Attribute-based access policies
* [mcharbor restore](mcharbor_restore.md)	 - Restore an instance export on this host, for an instance whose interface will not start
* [mcharbor role](mcharbor_role.md)	 - Roles and the permission catalog
* [mcharbor scim](mcharbor_scim.md)	 - SCIM provisioning connections
* [mcharbor stack](mcharbor_stack.md)	 - Compose stacks in the selected environment
* [mcharbor status](mcharbor_status.md)	 - Check the server: health, version, identity, cluster and the selected environment
* [mcharbor support](mcharbor_support.md)	 - Diagnostics, inventory, pre-upgrade checks and support bundles
* [mcharbor tui](mcharbor_tui.md)	 - Browse and operate the current environment's containers on one screen
* [mcharbor user](mcharbor_user.md)	 - User accounts
* [mcharbor version](mcharbor_version.md)	 - Show the CLI version, the server version, and whether they are compatible
* [mcharbor volume](mcharbor_volume.md)	 - Volumes in the selected environment
* [mcharbor whoami](mcharbor_whoami.md)	 - Show who the API key acts as and, with --permissions, what it may do

