# Scripting and CI

## Exit codes

These are an API and do not change. A script can branch on them.

| Code | Meaning | Typical cause |
|---|---|---|
| `0` | Success | |
| `1` | Generic failure | Server unreachable, certificate not trusted, a response that is not McHarbor |
| `2` | Usage | Unknown command or flag, bad value, an irreversible change without `--confirm`, a disruptive one without `--yes` off a terminal, HTTP 400/422 |
| `3` | Authentication | No key, or a revoked, expired or unknown one (HTTP 401) |
| `4` | Permission denied | HTTP 403; the message names the missing key |
| `5` | Not found | HTTP 404 |
| `6` | Conflict | HTTP 409 |
| `7` | Failed on the server | HTTP 5xx, or a followed backup or restore that ended in failure |
| `8` | Timeout | `--timeout` passed, HTTP 408/504, or stopped following a still-running operation |

## Output

| Flag | Output |
|---|---|
| (default) `-o table` | Aligned columns for people |
| `-o json` / `--json` | The server's data, indented. Lists are arrays even when the server paginates |
| `-o yaml` / `--yaml` | The same as YAML |
| `-o tsv` | The table's columns, tab-separated, no header, for `cut` and `while read` |

Unknown fields from a newer server are passed through in JSON and YAML, never rejected.
Progress, prompts and warnings go to **stderr**; stdout carries only the result, so a pipe
never sees anything else.

With `--json`, a failure is also JSON, on stderr:

```json
{
  "exitCode": 4,
  "message": "You do not have permission to perform this action",
  "operation": "volumesCreate",
  "status": 403,
  "code": "err.rbac.permission_denied",
  "permissions": ["volumes.manage"]
}
```

## Confirmation rules

| Kind | Commands | At a terminal | Without a terminal |
|---|---|---|---|
| Reversible | `start`, `unpause`, `pull`, `create`, `stack up`, `backup plan run`, `env test` | Runs | Runs |
| Disruptive | `stop`, `restart`, `pause`, `kill`, `stack stop/restart/down`, `backup run cancel`, `gitops pipeline promote` | Asks `[y/N]`; `--yes` skips | Needs `--yes`, or exits `2` |
| Irreversible | `remove`, `env delete`, `stack remove`, `backup restore` | Asks you to type the target's name | Needs `--confirm <name>`; `--yes` is refused |

`--yes` and `--confirm` only skip the local question. The server's permission checks,
approvals and audit apply either way.

## Long-running operations

`backup plan run` and `backup restore` return a run. By default the CLI follows it,
printing each change of stage to stderr, and prints the finished run on stdout. A run that
ends in failure exits `7` with the server's reason. `--no-follow` prints the run id and
returns at once; `mcharbor backup run get <id>` reads it later. `--timeout` bounds the
follow; the run keeps going on the server when the CLI stops watching.

`container logs -f` follows the log stream. The server closes each stream after five
minutes or a size limit; the CLI reconnects from the last line it printed, so nothing is
lost or repeated, and stops when the container's log ends.

## A CI example

```bash
export MCHARBOR_URL=https://mcharbor.example:8705
export MCHARBOR_API_KEY="$MCHARBOR_CI_KEY"
export MCHARBOR_CA_FILE=./mcharbor-root.pem
export MCHARBOR_ENV=d0a8c1…

mcharbor doctor
mcharbor stack up web
mcharbor backup plan run "$PLAN_ID" --timeout 30m
mcharbor --json container list | jq -r '.[] | select(.State != "running") | .Names[0]'
```
