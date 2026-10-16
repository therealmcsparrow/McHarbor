# Troubleshooting

Start with `mcharbor doctor`. It checks the configuration, the key, reachability, the
clock, authentication, version compatibility, the key's permissions and the selected
environment, and exits with the code of the first failure.

| Symptom | Cause | Fix |
|---|---|---|
| `the server's certificate is not trusted` | McHarbor's self-signed root is not trusted here | `--ca-file` with the root from **Security → HTTPS**, or `context set <name> --ca-file …` |
| Exit `3`, `err.auth.required` | No key reached the server | `mcharbor login`, or set `MCHARBOR_API_KEY`; check a proxy passes `Authorization` through |
| Exit `3`, `err.api_key.revoked` / `expired` | The key was revoked or passed its expiry | Create a new key and `mcharbor login` again |
| Exit `4` naming a key | The key, or its owner, lacks that permission | Grant the named key; a key never has more than its owner |
| Exit `5` on `volume get`, `image get`, `network get` | The object does not exist in the selected environment | Check `--env`; `env list` shows which is which |
| Exit `2`: `cannot be undone: pass --confirm` | An irreversible command ran without a terminal | Add `--confirm <name>` naming the target |
| Exit `2`: `refusing to … without confirmation` | A disruptive command ran without a terminal | Add `--yes` |
| `version` says the server is older | Commands added after that server version answer not found | Use a CLI built for that server, or upgrade it |
| `doctor` warns about the clock | This machine's clock is more than two minutes off the server's | Fix time sync here or on the server |
| `doctor` warns the key holds every permission | The key is a wildcard | Give automation a narrower key |
| `logs -f` stops | The container stopped, so its log ended | Expected; start the container and follow again |

## `container exec`

- **mcharbor exits with an unexpected code after `exec -- <command>`.** With a command, the exit
  status is the command's own, as with `ssh` and `docker exec`; the frozen codes 2 to 8 apply only
  when the CLI fails before the command runs. `-o json` returns the full result, `exitCode`
  included, and exits 0.
- **"an interactive shell needs a terminal".** Without a command, `exec` opens a shell and needs
  a real terminal on stdin. In a script, put the command after `--`.
- **Git Bash turns `--shell /bin/bash` into a Windows path.** MSYS rewrites arguments that look
  like paths; prefix the command with `MSYS_NO_PATHCONV=1`, or use PowerShell.
- **A shell refused with 403 on a protected container.** McHarbor's own containers need
  `system.manage` on top of `terminal.access`, exactly as in the web terminal.
