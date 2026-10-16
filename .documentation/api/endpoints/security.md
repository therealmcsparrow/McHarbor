# Scans and auto-heal

17 endpoints: vulnerability scanning of images, and the per-container auto-heal preference.

| Key | Covers |
|---|---|
| `scans.view` | List scans, read one, its vulnerabilities, the summary, available scanners |
| `scans.manage` | Start a scan, delete one |
| `settings.manage` | Purge old scans; add, edit, test and remove scanner instances |
| `scans.view` and `settings.manage` | List scanner adapters and configured instances |
| `containers.manage` or `autoheal.view` | Read a container auto-heal preference |
| `containers.manage` or `autoheal.manage` | Set it |

## Things worth knowing first

**Scanners are plugins now.** A scanner is a configured instance row, not a global boolean:
two Clair servers are expressible, and each instance carries its own enabled flag.
`GET /api/scans/scanners` returns the instances a scan can actually be started with, plus
which one is the estate default. Starting a scan with none configured returns
`err.scan.no_scanner_configured`.

**What stayed global is what is true of any engine**: how long the host will hold a scan
open, and whether it scans automatically on install and on update. Those are the three
scanner settings in [Settings](../settings/reference.md).

**Auto-heal is applied as a Docker label, in place.** Turning it on for a container does not
recreate it. The preference state is held in memory and reflected on the label, which is why
the read endpoint is described as returning the in-memory state.

## Endpoints

| Method | Path | Permission | What it does | Parameters |
|---|---|---|---|---|
| `GET` | `/api/autoheal/preference/{id}` | `containers.manage` or `autoheal.view` | Returns the in-memory preference state for a container | `{id}` |
| `POST` | `/api/autoheal/preference/{id}` | `containers.manage` or `autoheal.manage` | Enables or disables auto-heal for a container | `{id}` |
| `DELETE` | `/api/scans` | `settings.manage` | Deletes every scan (and its vulnerabilities) older than the configured retention cutoff | `?vacuum=` |
| `GET` | `/api/scans` | `scans.view` | Returns a paginated list of scans | — |
| `POST` | `/api/scans` | `scans.manage` | Starts a new vulnerability scan | — |
| `GET` | `/api/scans/adapters` | `scans.view` and `settings.manage` | Lists the installed scanner engines | — |
| `GET` | `/api/scans/by-image` | `scans.view` | Returns scans for a specific image reference | `?image=` |
| `GET` | `/api/scans/configs` | `scans.view` and `settings.manage` | Lists every configured scanner instance with its config; secret fields come back only as a stored/not-stored flag | — |
| `POST` | `/api/scans/configs` | `settings.manage` | Adds one scanner instance | — |
| `DELETE` | `/api/scans/configs/{id}` | `settings.manage` | Removes one scanner instance; its plugin stays installed and past findings are kept | `{id}` |
| `PUT` | `/api/scans/configs/{id}` | `settings.manage` | Rewrites one scanner instance | `{id}` |
| `POST` | `/api/scans/configs/{id}/test` | `settings.manage` | Runs the scanner's self-test and returns a per-step report; a failing test is still `200` | `{id}` |
| `GET` | `/api/scans/scanners` | `scans.view` | Returns the enabled scanner instances a scan can be started with, plus which one is the estate default | — |
| `GET` | `/api/scans/summary` | `scans.view` | Returns aggregated vulnerability counts, plus `needsReview`: how many images whose latest completed scan found something at critical or high | — |
| `DELETE` | `/api/scans/{id}` | `scans.manage` | Removes a scan and its vulnerabilities | `{id}` |
| `GET` | `/api/scans/{id}` | `scans.view` | Returns a single scan by ID | `{id}` |
| `GET` | `/api/scans/{id}/vulnerabilities` | `scans.view` | Returns vulnerabilities for a scan | `{id}` |


## Request bodies

**`POST` `/api/autoheal/preference/{id}`**: `enabled`

**`POST` `/api/scans`**: `imageRef`, `scanner`, `environmentId`

**`POST` `/api/scans/configs`**: `name`, `scannerType`, `enabled`, `makeDefault`, `config`

**`PUT` `/api/scans/configs/{id}`**: `name`, `scannerType`, `enabled`, `makeDefault`, `config`

## Related

- [Images](./images.md): what gets scanned
- [Containers](./containers.md): the containers auto-heal watches
