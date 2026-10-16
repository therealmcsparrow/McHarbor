# Action Nodes

Action nodes do something to the world: containers, images, networks, volumes, stacks,
backups, storage and hosts.

31 node types.

Almost every action node carries an `error` output alongside `output`. Wiring it is what
turns a failure into a branch instead of a failed run, and an unwired `error` port still
reaches an enclosing **Try Catch** guard if there is one.

| Node | Key | Ports | What it does |
|---|---|---|---|
| **Container Action** | `container-action` | in `input` · out `output`, `error` | Start, stop, restart, remove, rename or wait for a container |
| **Backup Download Info** | `container-backup-download` | in `input` · out `output`, `error` | Validate a completed backup run and return its download metadata |
| **Container Backup Manage** | `container-backup-manage` | in `input` · out `output`, `error` | List backup plans or runs, or get, cancel and delete a backup run |
| **Run Backup Plan** | `container-backup-plan-run` | in `input` · out `output`, `error` | Run a saved container backup plan |
| **Container Backup Restore** | `container-backup-restore` | in `input` · out `output`, `error` | Restore a completed container backup run into the container |
| **Container Backup** | `container-backup-run` | in `input` · out `output`, `error` | Run an encrypted backup for a container |
| **Container Create** | `container-create` | in `input` · out `output`, `error` | Create a new container from an image |
| **Container Exec** | `container-exec` | in `input` · out `output`, `error` | Execute a command inside a running container |
| **Container Logs** | `container-logs` | in `input` · out `output`, `error` | Read recent logs from a container |
| **Link Container To Stack** | `container-stack-link` | in `input` · out `output`, `error` | Create or replace the manual stack link for a container |
| **Unlink Container From Stack** | `container-stack-unlink` | in `input` · out `output`, `error` | Remove the manual stack link for a container |
| **Container Stats** | `container-stats` | in `input` · out `output`, `error` | Read one-shot runtime metrics for a container |
| **Docker Prune** | `docker-prune` | in `input` · out `output`, `error` | Remove unused Docker resources from an environment |
| **Environment Info** | `environment-info` | in `input` · out `output`, `error` | Read Docker daemon, host or reachability information for an environment |
| **Health Check** | `health-check` | in `input` · out `healthy`, `unhealthy` | Check an HTTP endpoint and branch on health |
| **Host Exec** | `host-exec` | in `input` · out `output`, `error` | Run a host command through a temporary container |
| **HTTP Request** | `http-request` | in `input` · out `output`, `error` | Send an HTTP request and store the response on the message |
| **Image Build** | `image-build` | in `input` · out `output`, `error` | Build a Docker image from a workflow-managed build context |
| **Image Manage** | `image-manage` | in `input` · out `output`, `error` | Pull, push, tag or remove a Docker image |
| **Image Vulnerability Scan** | `image-vulnerability-scan` | in `input` · out `output`, `critical`, `high`, `medium`, `low`, `clean`, `error` | Start or run a vulnerability scan for a container image |
| **Inspect** | `inspect` | in `input` · out `output`, `error` | Inspect a container, image, network, volume or stack |
| **List** | `list` | in `input` · out `output`, `error` | List containers, images, networks, volumes or stacks in an environment |
| **Network Manage** | `network-manage` | in `input` · out `output`, `error` | Create or remove a network, or connect and disconnect containers |
| **Registry Search** | `registry-search` | in `input` · out `output`, `error` | Search a container registry for images |
| **Stack Backup** | `stack-backup` | in `input` · out `output`, `error` | Export stack container metadata before risky operations |
| **Stack Manage** | `stack-manage` | in `input` · out `output`, `error` | Deploy, start, stop or remove a compose stack |
| **Get Storage Location** | `storage-location-get` | in `input` · out `output`, `error` | Read one configured storage location without secret values |
| **List Storage Locations** | `storage-location-list` | in `input` · out `output`, `error` | Return configured storage locations without secret values |
| **Storage Location Test** | `storage-location-test` | in `input` · out `output`, `error` | Run the connection test for a storage location and optionally read its capacity |
| **Storage Object** | `storage-object` | in `input` · out `output`, `error` | List, upload, download or delete objects on a storage location through its adapter |
| **Volume Manage** | `volume-manage` | in `input` · out `output`, `error` | Create, remove, back up or restore a Docker volume |

## Configuration, node by node

### Container Action (`container-action`)

Start, stop, restart, remove, rename or wait for a container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required
  - Action (`action`): select, required, default `start`, one of `start`, `stop`, `restart`, `remove`, `rename`, `wait`
  - Timeout (seconds) (`timeout`): number, default `10`; shown when `{"action":["stop","restart","wait"]}`
  - New Name (`new_name`): text; shown when `{"action":"rename"}`
  - Unlock Protected Container (`unlock_protected`): toggle, default `false`; shown when `{"action":"rename"}`
  - Condition (`condition`): select, default `not-running`, one of `not-running`, `next-exit`, `removed`; shown when `{"action":"wait"}`

### Backup Download Info (`container-backup-download`)

Validate a completed backup run and return its download metadata

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (filter) (`container`): container-select
  - Backup Run (`run_id`): backup-run-select, required

### Container Backup Manage (`container-backup-manage`)

List backup plans or runs, or get, cancel and delete a backup run

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Action (`action`): select, required, default `list_runs`, one of `list_plans`, `list_runs`, `get_run`, `cancel_run`, `delete_run`
  - Container (filter) (`container`): container-select; shown when `{"action":["list_plans","list_runs"]}`
  - Status (`status`): select, default `all`, one of `all`, `completed`, `running`, `failed`, `cancelled`; shown when `{"action":"list_runs"}`
  - Limit (`limit`): number, default `25`; shown when `{"action":"list_runs"}`
  - Backup Run (`run_id`): backup-run-select; shown when `{"action":["get_run","cancel_run","delete_run"]}`

### Run Backup Plan (`container-backup-plan-run`)

Run a saved container backup plan

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (filter) (`container`): container-select
  - Backup Plan (`plan_id`): backup-plan-select, required

### Container Backup Restore (`container-backup-restore`)

Restore a completed container backup run into the container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (filter) (`container`): container-select
  - Backup Run (`run_id`): backup-run-select, required
  - Restore Items (`restore_items`): backup-restore-item-multiselect
  - Secret Key (`secret_key`): text

### Container Backup (`container-backup-run`)

Run an encrypted backup for a container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required
  - Storage Location (`storage_location_id`): storage-location-select
  - Include Logs (`include_logs`): toggle, default `false`
  - Include Filesystem (`include_filesystem`): toggle, default `false`
  - Include Image (`include_image`): toggle, default `false`
  - Selected Mounts (`selected_mounts`): container-mount-multiselect

### Container Create (`container-create`)

Create a new container from an image

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Image (`image`): image-select, required
  - Name (`name`): text
  - Ports (`ports`): multiselect
  - Environment Variables (`env_vars`): key-value
  - Restart Policy (`restart_policy`): select, default `no`, one of `no`, `always`, `on-failure`, `unless-stopped`
  - Auto Start (`auto_start`): toggle, default `true`

### Container Exec (`container-exec`)

Execute a command inside a running container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required
  - Command (`command`): textarea, required
  - Working Directory (`workdir`): text

### Container Logs (`container-logs`)

Read recent logs from a container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required
  - Tail Lines (`tail`): number, default `100`
  - Since (`since`): select, one of `5m`, `15m`, `1h`, `6h`, `24h`, `168h`

### Link Container To Stack (`container-stack-link`)

Create or replace the manual stack link for a container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required
  - Stack Name (`stack_name`): stack-select, required
  - Service Name (`service_name`): stack-service-select

### Unlink Container From Stack (`container-stack-unlink`)

Remove the manual stack link for a container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required

### Container Stats (`container-stats`)

Read one-shot runtime metrics for a container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Container (`container`): container-select, required

### Docker Prune (`docker-prune`)

Remove unused Docker resources from an environment

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Resource (`resource`): select, default `all`, one of `all`, `containers`, `images`, `volumes`, `networks`

### Environment Info (`environment-info`)

Read Docker daemon, host or reachability information for an environment

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Information (`scope`): select, required, default `docker`, one of `docker`, `host`, `status`

### Health Check (`health-check`)

Check an HTTP endpoint and branch on health

- **Ports**: in: `input`; out: `healthy`, `unhealthy`
- **Fields**
  - URL (`url`): text, required
  - Method (`method`): select, default `GET`, one of `GET`, `POST`, `PUT`, `PATCH`, `DELETE`
  - Expected Status (`expected_status`): number, default `200`
  - Timeout (seconds) (`timeout`): number, default `10`
  - Retries (`retries`): number, default `3`

### Host Exec (`host-exec`)

Run a host command through a temporary container

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Command (`command`): textarea, required
  - Timeout (seconds) (`timeout`): number, default `30`

### HTTP Request (`http-request`)

Send an HTTP request and store the response on the message

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - URL (`url`): text, required
  - Method (`method`): select, default `GET`, one of `GET`, `POST`, `PUT`, `PATCH`, `DELETE`
  - Headers (`headers`): key-value
  - Body Source (`body_source`): expression, default `payload`

### Image Build (`image-build`)

Build a Docker image from a workflow-managed build context

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Tag (`tag`): text, required
  - Dockerfile (`dockerfile`): text, default `Dockerfile`
  - Context Path (`context_path`): text, default `.`
  - Target Stage (`target`): text
  - Build Args (`build_args`): key-value
  - No Cache (`no_cache`): toggle

### Image Manage (`image-manage`)

Pull, push, tag or remove a Docker image

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Action (`action`): select, required, default `pull`, one of `pull`, `push`, `tag`, `remove`
  - Registry (`registry_mode`): select, default `custom`, one of `configured`, `custom`; shown when `{"action":["pull","push"]}`
  - Registry (`registry_id`): registry-select; shown when `{"action":["pull","push"],"registry_mode":"configured"}`
  - Registry URL (`registry_url`): text; shown when `{"action":["pull","push"],"registry_mode":"custom"}`
  - Registry Username (`registry_username`): text; shown when `{"action":["pull","push"],"registry_mode":"custom"}`
  - Registry Password (`registry_password`): text; shown when `{"action":["pull","push"],"registry_mode":"custom"}`
  - Image (`image`): image-select; shown when `{"action":"pull"}`
  - Image (`image`): image-select; shown when `{"action":["push","remove"]}`
  - Source Image (`source`): image-select; shown when `{"action":"tag"}`
  - Target Image (`target`): text; shown when `{"action":"tag"}`
  - Force (`force`): toggle, default `false`; shown when `{"action":"remove"}`
  - Prune Children (`prune_children`): toggle, default `true`; shown when `{"action":"remove"}`

### Image Vulnerability Scan (`image-vulnerability-scan`)

Start or run a vulnerability scan for a container image

- **Ports**: in: `input`; out: `output`, `critical`, `high`, `medium`, `low`, `clean`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Image (`image`): image-select, required
  - Scanner (`scanner`): image-scanner-select; empty means the scanner marked default under Security → Scanners
  - Wait For Result (`wait`): toggle, default `false`

The scanner list is whatever the installed scanner plugins registered and an operator configured
under **Security → Scanners**: the field reads `GET /api/scans/scanners` (`scans.view`) and offers
each enabled instance whose plugin is installed and runnable. The value it stores is the instance
id. A workflow saved before the picker carries an engine type instead (`trivy`, `grype`,
`clair`); that still runs, on the oldest enabled instance of that type, and the field shows it as
that instance. With no scanner plugin installed and configured, the field is disabled and reads
"No scanner plugin is installed and configured. Add one under Security → Scanners."; a run then
takes the `error` output with the scan service's own reason.

### Inspect (`inspect`)

Inspect a container, image, network, volume or stack

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Resource (`resource`): select, required, default `container`, one of `container`, `image`, `network`, `volume`, `stack`
  - Container (`container`): container-select; shown when `{"resource":"container"}`
  - Image (`image`): image-select; shown when `{"resource":"image"}`
  - Network (`network`): network-select; shown when `{"resource":"network"}`
  - Volume (`volume`): volume-select; shown when `{"resource":"volume"}`
  - Stack Name (`stack_name`): stack-select; shown when `{"resource":"stack"}`

### List (`list`)

List containers, images, networks, volumes or stacks in an environment

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Resource (`resource`): select, required, default `container`, one of `container`, `image`, `network`, `volume`, `stack`
  - Show All (`show_all`): toggle, default `true`; shown when `{"resource":["container","image"]}`
  - Name Filter (`name_filter`): text; shown when `{"resource":"container"}`
  - Only Dangling (`filter_dangling`): toggle, default `false`; shown when `{"resource":["image","volume"]}`

### Network Manage (`network-manage`)

Create or remove a network, or connect and disconnect containers

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Action (`action`): select, required, default `create`, one of `create`, `remove`, `connect`, `disconnect`
  - Name (`name`): text; shown when `{"action":"create"}`
  - Driver (`driver`): select, default `bridge`, one of `bridge`, `overlay`, `macvlan`, `ipvlan`, `host`, `none`; shown when `{"action":"create"}`
  - Internal (`internal`): toggle, default `false`; shown when `{"action":"create"}`
  - Subnet (`subnet`): text; shown when `{"action":"create"}`
  - Network (`network`): network-select; shown when `{"action":["remove","connect","disconnect"]}`
  - Container (`container`): container-select; shown when `{"action":["connect","disconnect"]}`

### Registry Search (`registry-search`)

Search a container registry for images

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Registry (`registry_mode`): select, required, default `custom`, one of `configured`, `custom`
  - Registry (`registry_id`): registry-select; shown when `{"registry_mode":"configured"}`
  - Registry URL (`registry_url`): text; shown when `{"registry_mode":"custom"}`
  - Registry Username (`registry_username`): text; shown when `{"registry_mode":"custom"}`
  - Registry Password (`registry_password`): text; shown when `{"registry_mode":"custom"}`
  - Search Term (`term`): text, required
  - Limit (`limit`): number, default `25`

### Stack Backup (`stack-backup`)

Export stack container metadata before risky operations

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Stack Name (`stack_name`): stack-select, required

### Stack Manage (`stack-manage`)

Deploy, start, stop or remove a compose stack

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Action (`action`): select, required, default `deploy`, one of `deploy`, `up`, `down`, `remove`
  - Stack Name (`stack_name`): stack-select, required
  - Compose Source (`compose_source`): select, default `message`, one of `message`, `inline`, `file`; shown when `{"action":"deploy"}`
  - Compose Content (`compose_content`): textarea; shown when `{"action":"deploy","compose_source":"inline"}`
  - Compose Path (`compose_path`): text; shown when `{"action":"deploy","compose_source":"file"}`
  - Remove Volumes (`remove_volumes`): toggle, default `false`; shown when `{"action":["down","remove"]}`

### Get Storage Location (`storage-location-get`)

Read one configured storage location without secret values

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Storage Location (`storage_location_id`): storage-location-select, required

### List Storage Locations (`storage-location-list`)

Return configured storage locations without secret values

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Enabled Only (`enabled_only`): toggle, default `true`

### Storage Location Test (`storage-location-test`)

Run the connection test for a storage location and optionally read its capacity

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Storage Location (`storage_location_id`): storage-location-select, required
  - Include Usage (`include_usage`): toggle, default `false`
  - Fail On Warning (`fail_on_warn`): toggle, default `false`

### Storage Object (`storage-object`)

List, upload, download or delete objects on a storage location through its adapter

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Storage Location (`storage_location_id`): storage-location-select, required
  - Action (`action`): select, required, default `list`, one of `list`, `upload`, `download`, `delete`
  - Prefix (`prefix`): text; shown when `{"action":"list"}`
  - Object Key (`key`): text; shown when `{"action":["upload","download","delete"]}`
  - Source (`source_mode`): select, default `property`, one of `property`, `text`; shown when `{"action":"upload"}`
  - Source Property (`property`): expression, default `payload`; shown when `{"action":"upload","source_mode":"property"}`
  - Source Encoding (`content_encoding`): select, default `auto`, one of `auto`, `base64`; shown when `{"action":"upload","source_mode":"property"}`
  - Content (`content`): textarea; shown when `{"action":"upload","source_mode":"text"}`
  - Output Encoding (`output_encoding`): select, default `text`, one of `text`, `json`, `base64`; shown when `{"action":"download"}`
  - Output Property (`output_property`): expression, default `payload`; shown when `{"action":"download"}`

### Volume Manage (`volume-manage`)

Create, remove, back up or restore a Docker volume

- **Ports**: in: `input`; out: `output`, `error`
- **Fields**
  - Environment (`environment`): environment-select, required
  - Action (`action`): select, required, default `create`, one of `create`, `remove`, `backup`, `restore`
  - Name (`name`): text; shown when `{"action":"create"}`
  - Driver (`driver`): select, default `local`, one of `local`; shown when `{"action":"create"}`
  - Labels (`labels`): key-value; shown when `{"action":"create"}`
  - Force (`force`): toggle, default `false`; shown when `{"action":"remove"}`
  - Volume (`volume`): volume-select; shown when `{"action":["remove","backup","restore"]}`
  - Backup File Path (`backup_path`): text; shown when `{"action":["backup","restore"]}`
  - Overwrite Existing Files (`overwrite`): toggle, default `false`; shown when `{"action":"restore"}`
  - Helper Image (`helper_image`): image-select, default `alpine:3.20`; shown when `{"action":["backup","restore"]}`

## Related

- [Node catalog](../README.md): all 90 node types, and how to read these pages
- [Execution model](../../execution/README.md): ports, emissions and error routing
- [The message contract](../../messages/README.md): what flows between these nodes
