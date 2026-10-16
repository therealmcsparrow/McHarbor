# Environments: Actions

Everything you can do to an environment, and the permission each one needs, including the
two longer paths: deploying an agent over SSH, which asks for the host fingerprint first,
and opening a shell on the host itself.

| Action | Gated by | What it does |
|---|---|---|
| **Add environment** | `environments.manage` | Pick a connection type and fill in what it needs |
| **Test connection** | `environments.manage` | A real round trip against the endpoint |
| **Regenerate token** | `environments.manage` | Issues a new agent token and invalidates the old one |
| **Deploy over SSH** | `environments.manage` | Installs the agent on the remote host, after asking for its SSH host fingerprint |
| **Set default** | `environments.manage` | The environment a new session starts on |
| **Migrate connection** | `environments.manage` | Changes how McHarbor reaches this host, in place; the environment keeps its id, and the agent is installed for you |
| **Merge duplicate** | `environments.manage` | Folds another environment into this one and deletes it |
| **Remove** | `environments.manage` | Removes McHarbor’s record. Containers, images and volumes on the host are untouched |

## Migrating a connection

**Migrate connection**, on the Overview tab, moves one environment between connection
types without creating a second one. The environment keeps its id, so its stacks, metrics,
activity, audit trail, schedules and compute pools come with it. A second environment for
the same host counts it twice in reporting and polls it twice.

Going **to an agent** issues a token and **installs the agent for you**. McHarbor deploys
the agent container onto that host through the Docker connection the environment already
has, which is why no SSH credentials are asked for: a socket, TCP or SSH environment is by
definition a daemon McHarbor can already create containers on. The image is pulled, any
previous `mcharbor-agent` container is replaced, and the new one starts with the token, the
address you reached McHarbor on, and the same daemon the environment points at.

The environment keeps using its old endpoint throughout and stays fully working. The moment
the agent authenticates, McHarbor switches it over and clears the old endpoint. Until then
the Overview tab shows a pending banner with a **Cancel migration** button.

Two switches worth knowing:

- **Install the agent for me** is on by default. Turn it off to get a token and nothing
  else, for a host you want to install by hand.
- **Server uses a self-signed certificate** is on by default when you are on HTTPS, because
  a stock McHarbor signs its own. It tells the deployed agent not to verify the server
  certificate. Turn it off when yours is signed by a CA the host trusts.

**Switch immediately** converts the row as soon as the agent is deployed rather than waiting
for it to connect. The old endpoint is still kept in reserve for the failsafe below.

## When a migration fails or stalls

A migration never leaves an environment stranded. All three failure paths put it back
exactly as it was: previous connection type restored, agent token revoked, agent metadata
cleared:

| What went wrong | What happens |
|---|---|
| The agent could not be installed | Reverted before the request returns. The dialog shows the reason from Docker. |
| You pressed **Cancel migration** | Reverted immediately, including an already-converted immediate migration. |
| The agent was installed but never connected | Reverted automatically **15 minutes** after the migration started, and the agent container McHarbor deployed is removed so it stops retrying a token that no longer works. |

The automatic revert re-checks the deadline as it writes, so an agent that connects while
the sweep is running wins: a migration that succeeded is never undone.

Nothing is lost in a revert; the environment keeps its id throughout, so no stack, metric
or schedule is touched by either the migration or the rollback.

Going **away from an agent** is immediate and takes the endpoint you name. The agent token
is revoked and the connected agent is disconnected; it cannot reconnect without a new token.

Paste an existing `agt_` token instead of taking a fresh one when the agent is already
deployed and holds it.

## Merging duplicate environments

When two environments report the same Docker daemon id, both of their Overview tabs carry a
banner saying so. **Merge duplicate** folds one into the other and deletes it.

The dialog shows the plan before anything happens: which tables would move and how many rows
in each. Every table holding an environment id is discovered from the live schema rather
than from a fixed list, so a table added by a new feature, or by a plugin, is covered
without anyone remembering to update the merge.

Two rules worth knowing before you confirm:

- **The kept environment wins collisions.** Where the merge would create a duplicate the
  schema forbids (a storage pool of the same name, one settings row per environment, the
  same role grant), the row from the environment being deleted is discarded rather than
  overwriting what the kept one has.
- **Secrets move with their scope digest recomputed**, so a moved secret still resolves. A
  secret whose name already exists in the kept environment's scope is left behind rather
  than silently replacing the one in use.

The default-environment badge moves across if the environment being deleted carried it.
A merge cannot be undone.

## Deploying over SSH

Host, port, user, and either a private key or a password. It asks for the **SSH host
fingerprint** first and tells you how to get it:
`ssh-keyscan <host> | ssh-keygen -lf - -E sha256`. Password authentication carries a
warning of its own: it is offered, and it is the weaker option. Deploy as a Docker
container or as a system binary.

## Reusing a token from another McHarbor

**Use existing agent token** pastes a token already issued by a different McHarbor that
this agent connects back to, in the form `agt_` plus 64 hex characters, instead of
generating a fresh one.

## Opening a host shell

The **Console → Terminal** tab takes the same three choices the container terminal does,
and they apply when the shell starts; they are locked while a session is live.

| Control | Default | What it does |
|---|---|---|
| Shell | Auto-detect | `/bin/sh`, `/bin/bash`, `/bin/ash`, `/bin/dash` or `/bin/zsh`. A shell the host does not have falls back to one it does, so a choice costs you a different shell, never a failed session |
| User | Host default (root) | `root`, `nobody`, or **Custom…** for any account on the host. The switch uses `runuser`, falling back to `su` on BusyBox hosts |
| Working directory | wherever the shell lands | An absolute path to start in |

Custom account names are checked against POSIX rules before they reach the host, and a
path that is not an absolute, traversal-free directory is refused outright.

Over an agent, the three choices need **agent 1.8.0 or newer for the terminal itself** and
a recent agent to honour the choices. An older one connects you to its detected shell as
root, exactly as it did before these controls existed.

The button at the end of the row takes the terminal **fullscreen**, and `Esc` brings it
back. The same button is on the container terminal and on the sidebar Terminal page. The
shell is resized to match, so the extra rows and columns are real ones the program on the
other end knows about, not a stretched picture of a smaller terminal.

Sessions advertise `TERM=xterm-256color` and `COLORTERM=truecolor`, and McHarbor maps the
sixteen ANSI colours onto the theme you are using, so colour follows the rest of the UI in
both light and dark. `TERM` is what lets a program *choose* colour, not a guarantee of it:
expect it from anything that decides for itself (`git`, `grep`, `htop`, `systemctl`,
package managers, most TUIs), while a plain `ls` stays plain on an image that does not
alias `ls --color`.

## Confirmation

Destructive actions confirm first and say how many items they will affect. A confirmation
that names a consequence ("this cannot be undone", "all its data") is quoting the
product, not padding.

## Related

- [Environments](./README.md): what the screen is and what it shows
- [Fields](./fields.md): the inputs these actions read
- [Troubleshooting](./troubleshooting.md): when an action does not do what you expect
