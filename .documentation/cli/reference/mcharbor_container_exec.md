## mcharbor container exec

Run a command in a container, or open an interactive shell

### Synopsis

With a command after --, exec runs it once and prints its output; mcharbor then exits with the command's own status. Without one it opens an interactive shell over the same audited session the web terminal uses, which needs a terminal. A command needs containers.manage; a shell needs terminal.access or containers.manage. --debug is for a distroless or scratch image with no shell: McHarbor uploads a static busybox to /.mcharbor-debug, runs the shell or the command with it, and removes it when the session ends (at the latest 15 minutes later). It needs containers.debug_shell, plus containers.manage with a command; every install is audited with the binary's digest.

```
mcharbor container exec <id|name> [-- command [args...]] [flags]
```

### Examples

```
  mcharbor container exec web -- cat /etc/os-release
  mcharbor container exec web --shell /bin/bash
  mcharbor container exec distroless-api --debug
  mcharbor container exec distroless-api --debug -- ls -la /app
```

### Options

```
      --debug            for a shell-less image: upload a temporary static busybox and run with it (needs containers.debug_shell)
  -h, --help             help for exec
      --shell string     shell for an interactive session (default /bin/sh, or the best one found); with a command, run it through this shell with -c
  -u, --user string      user to run as, instead of the image's USER
  -w, --workdir string   working directory inside the container
```

### Options inherited from parent commands

```
      --ca-file string     PEM root to trust for the server certificate ($MCHARBOR_CA_FILE)
      --confirm string     name the target of an irreversible change to confirm it non-interactively
      --context string     context to use instead of the current one ($MCHARBOR_CONTEXT)
      --env string         environment id for scoped commands ($MCHARBOR_ENV)
      --insecure           skip certificate verification; for labs only ($MCHARBOR_INSECURE=true)
      --json               shorthand for --output json
  -o, --output string      output format: table, json, yaml or tsv (default "table")
      --timeout duration   give up on a request after this long; 0 waits forever (default 1m0s)
      --url string         McHarbor URL, overriding the context ($MCHARBOR_URL)
      --yaml               shorthand for --output yaml
      --yes                skip the prompt for disruptive but reversible changes
```

### SEE ALSO

* [mcharbor container](mcharbor_container.md)	 - Containers in the selected environment

