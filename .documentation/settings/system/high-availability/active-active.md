# Active/active

**Enterprise → High availability** · `MCHARBOR_HA_MODE=active-active`

Every node serves every request. There is no standby, no active lock to hand over and no
failover: the load balancer spreads traffic across all of them, and a node that stops takes
its share of traffic with it and nothing else.

This is the newer of the two multi-node modes. [Active/passive](README.md) is still the
right answer when you want one node serving and a second one ready. It needs no
node-to-node network path, and every capability works on it without exception.

## The one thing to understand first

A remote agent keeps **one** outbound WebSocket to **one** node. That has not changed and
cannot: the agent's reconnect and self-update logic is built around a single control
channel.

What changed is that a node which does not hold an agent can now reach it, over two
internal, signed hops to the node that does:

| Hop | What crosses it |
|---|---|
| `POST /api/internal/node/agent-rpc/{envId}` | Every Docker API call: containers, images, volumes, networks, stacks, Docker events, container logs, container stats |
| `GET /api/internal/node/agent-stream/{envId}` (WebSocket) | Every agent session and agent message: the container terminal (`exec`), the host terminal, the guest console (VM, LXC), the host log stream, agent-side Compose and prune, host OS info and updates, host stats, container move and backup transfers, snapshots and every compute command |
| `GET /api/internal/node/stats/{envId}` | The owning node's live container stats, which the agent pushes to that node only |
| `POST /api/internal/node/workflow-trigger/fire` | A plugin workflow trigger fired on a node that does not lead the workflow trigger, and the body-less leader probe sent before it; see [Plugin workflow triggers](#plugin-workflow-triggers) |

Nothing has to be pinned at the load balancer. A terminal, console or log stream opened on
any node works; it runs through the node holding the agent.

`GET /api/cluster/agent-routing` tells you which node holds which agent, what each hop last
proved, and the relays this node is running for other nodes.

## What you need

Everything [active/passive](README.md) needs, plus two things:

| Requirement | Why |
|---|---|
| `MCHARBOR_ADVERTISE_URL` on **every** node, reachable from the others, and `https` | It is the address a peer forwards an agent call to. A node without one can be forwarded *from* but not *to*, and its agents are unreachable from anywhere else. A plain `http://` address is refused by the other nodes unless `MCHARBOR_NODE_LINK_ALLOW_HTTP=1` is set; see [How the hops are protected](#how-the-hops-are-protected) |
| The same `MCHARBOR_SECRET` on every node | It is already required for sessions. In this mode it is also the key the node-to-node signature is derived from, so a node with a different one is refused rather than trusted |
| The same build on every node | The signed format of the hops changes between builds when it has to (the addressed node joined the signature in 2026.09.01), and a node on another build is refused, not half-trusted. The `sameVersion` gate fails while live nodes differ |

A node-to-node hop commonly runs over a certificate from a private CA the system trust store
does not hold. Point `MCHARBOR_NODE_LINK_CA_FILE` at that CA's PEM certificate on every node:
both hops then verify peers against that CA alone. `MCHARBOR_NODE_LINK_SKIP_VERIFY=1` also
makes such a certificate work, by accepting any certificate at all; see
[Streamed bodies rest on TLS](#how-the-hops-are-protected) for what that gives away. Prefer
the CA file. Certificate verification is on by default.

## Settings

| Variable | Values | Effect |
|---|---|---|
| `MCHARBOR_HA_MODE` | `active-active` | Every node serves. No standby, no active lock, no failover |
| `MCHARBOR_ADVERTISE_URL` | a URL | Where the other nodes reach **this** node. Required in this mode |
| `MCHARBOR_NODE_LINK_CA_FILE` | a path to a PEM file | Verify peer certificates on both hops against this CA only, instead of the system trust store. Wins over `MCHARBOR_NODE_LINK_SKIP_VERIFY`. A file that cannot be read or holds no certificate makes every node-to-node call from this node fail, and the `nodeLinkTransport` gate fails |
| `MCHARBOR_NODE_LINK_SKIP_VERIFY` | `1` | Accept any TLS certificate a peer presents. Anyone who can intercept the path can then stand in for a node and forge streamed bodies, agent replies and relayed sessions; the `nodeLinkTransport` gate warns. Use `MCHARBOR_NODE_LINK_CA_FILE` instead |
| `MCHARBOR_NODE_LINK_ALLOW_HTTP` | `1` | Let node-to-node calls go to a peer that does not advertise `https://`, and accept node-to-node calls that arrive without TLS. Without it such a peer is refused, such a call is answered `403`, and the `nodeLinkTransport` gate fails |
| `MCHARBOR_NODE_LINK_RATE_LIMIT` | a positive integer, default `3000` | Requests per minute, per verified peer node, that each agent hop (`agent-rpc`, `agent-stream`, `stats`) accepts. The fire route has a fixed `1200` (600 events, each a probe plus a fire). Requests that fail verification have their own fixed budget of `120` a minute per source address |

Every node must carry the same `MCHARBOR_HA_MODE`, `MCHARBOR_SECRET`, `ENCRYPTION_KEY` and
`MCHARBOR_PLUGIN_STORE`, and its own `MCHARBOR_NODE_ID` and `MCHARBOR_ADVERTISE_URL`.

## How a forwarded call works

1. A request arrives at node B for an environment whose agent is attached to node A.
2. B reads `cluster_agent_owners` and finds A's live claim on that environment.
3. B signs the request, covering the hop (`agent-rpc`), its own node id, **A's node id**, method, path, query, environment,
   the signed-in user's id and name, the caller's remaining timeout, a timestamp, a nonce, a
   digest of the body and a digest of every Docker request header it forwards (among them
   `Content-Length` and `X-Registry-Auth`), and POSTs it to
   `A/api/internal/node/agent-rpc/{envId}`.
4. A checks the call arrived over TLS, verifies the signature, checks that B is a live
   instance in the registry, rebuilds the request, applies the same timeout and runs it on
   its own agent connection.
5. A signs the daemon's status and headers for this request and streams them back with the
   body, so a `logs --follow` or an event stream arrives line by line. B refuses an answer
   A did not sign for this request.
6. B records that A executed the call. The audit row says both: `nodeId` is the node that
   served the request, `execNodeId` the node that ran the work.

A node that no longer holds the agent answers `409` rather than forwarding onwards, so a
stale claim produces one readable failure instead of a chain of hops.

## How the hops are protected

The internal routes are public (their caller is a node, not a user), so each of them
defends itself before it does any work.

**Addressed to one node and one hop.** Every signature covers the node it is addressed to,
and the receiving node checks that it is itself. A request captured on its way to node A is
refused by node B, and by A once its nonce is used. It also covers the hop it was made for
(`agent-rpc`, `agent-stream`, `stats` or `workflow-trigger`), which the receiving route
supplies itself, so a signature for one route is worthless on the others; `agent-rpc`
additionally refuses a Docker path that names another hop's claim path. The signature covers
the actor name, the timeout and every forwarded Docker header too, so none of them can be
rewritten in flight. Header values are compared with surrounding whitespace trimmed, as the
receiving HTTP server trims them, and a value HTTP cannot carry intact (a control character,
a CR or LF) is refused when signing rather than sent. A signature made by an older build,
which covered less, is refused outright, never accepted beside the new form, which is why
every node must run the same build.

**Only live nodes.** A request whose signature verifies must also name a node that is live
in the instance registry (the peer list, cached for five seconds, re-read at most once a
second for a node that has just joined). Any other node id is refused with `403` before its
body is read; a node whose registry is not loaded yet answers `503`, which the sender
retries. The shared secret alone therefore cannot invent a node, nor a fresh request budget
for each id it makes up.

**Refused before the body is read.** The receiving node checks the signature over the
headers and the body digest the request claims, in constant time, before it reads a byte of
the body. An unsigned request, one stamped more than two minutes from this node's clock, one
addressed to another node and one whose signature does not verify are all refused at that
point. Then, for a body covered by the digest, a `Content-Length` that is missing or over
1 MiB is refused; the body is read through a 1 MiB cap and must hash to the signed digest.
A route that carries no body (the stats read, the session relay and the leader probe)
accepts only the digest of an empty body. The nonce is spent only after that, so a body
damaged in flight does not burn the nonce of the request that carried it.

**Rate-limited after verification.** A verified request is charged to the node that signed
it: `MCHARBOR_NODE_LINK_RATE_LIMIT` a minute (default `3000`) per node for each agent hop,
`1200` for the fire route, which is 600 plugin events that each cost a leader probe and a
fire. A request that fails verification, or names a node that is not live, is counted
instead against its source address, `120` a minute across all four routes, with an IPv6
address counted by its `/64`; past that it is answered `429` rather than `401` or `403`.
That count protects nothing (the request is refused either way and its signature has
already been checked); it only shows a misconfigured peer it is being ignored, and junk
from an address a real peer shares never costs the peer anything. The per-node budget caps
a runaway peer; it is no bound on a stolen secret, whose holder can sign as each live node
in turn. Past a node's budget the answer is `429`. The source address
is resolved the same way as for every other rate limit, so set
`MCHARBOR_TRUSTED_PROXY_CIDRS` if a proxy sits between the nodes. The warning a refusal
logs is limited to one line per hop every ten seconds, with the number it held back, and
every caller-supplied value in it (the node name, the environment id) is cut to 64
characters.

**No redirects.** A node never follows a redirect on a hop: a `307` or `308` would re-send
the signed body and every `X-Mcharbor-Node-*` header to whatever host the peer named. The
redirect comes back as a refusal instead.

**No plain http between nodes.** In every multi-node mode (the default peer mode,
active/passive and active/active), a node refuses to send a signed call to a peer whose
`MCHARBOR_ADVERTISE_URL` is not an `https://` URL, and logs an error at boot when its own
is not. The receiving side enforces the same rule: a node-to-node call that did not arrive
over TLS is answered `403` before its signature is checked. A call that reached this node
through a TLS-terminating proxy counts as TLS only when the proxy's address is in
`MCHARBOR_TRUSTED_PROXY_CIDRS` and it sends `X-Forwarded-Proto: https`; the header from
any other address is ignored. Peer mode
forwards Docker calls, registry credentials included, and trigger fires just as
active/active does; a single node never dials a peer, so it is unaffected.
Set `MCHARBOR_NODE_LINK_ALLOW_HTTP=1` on every node only when the network between them is
already trusted; it lets the node both send and accept plain http, and the
`nodeLinkTransport` gate then warns instead of failing.

**Signed replies.** Every reply the asking node acts on is signed by the answering node,
bound to the request's nonce and both node ids. A fire's result, the leader probe and a
stats snapshot are signed over their exact body. An agent call's answer is signed over the
daemon's status, the executing node and every forwarded response header; its body streams.
An unsigned or mis-signed reply is an error (a failed delivery, a failed stats read, a
failed Docker call), never a response.

**Streamed bodies rest on TLS.** A request body over 1 MiB (an image load, a large upload),
a body of unknown length (the Docker SDK sends a `docker cp` tar and a build context that
way), the body of an agent call's answer and every frame of the session relay stream through
the hop instead of being hashed first, so their integrity comes from TLS between the nodes,
not from the signature. A known-length body is never signed in part: one that does not match
its declared length fails the call. That is why TLS is required in every multi-node mode, and
why `MCHARBOR_NODE_LINK_ALLOW_HTTP=1` and `MCHARBOR_NODE_LINK_SKIP_VERIFY=1` give away more
than confidentiality: with either, whoever sits on the path can rewrite those bodies, agent
answers and terminal, console and log frames, not only read them. Pin a private CA with
`MCHARBOR_NODE_LINK_CA_FILE` instead of skipping verification.

**No proxy.** Both hops dial the peer's advertise URL directly and ignore `HTTP_PROXY`,
`HTTPS_PROXY` and `NO_PROXY`, so a proxy set for outbound internet traffic never sees a
signed call.

## How a session is relayed

A terminal, console or log stream is not one request and one response; it is two streams
with their own lifetime. The Docker SDK's hijacked exec never passes through the HTTP hop,
so sessions take the second one.

1. Node B needs a session on an agent node A holds. It opens one WebSocket to
   `A/api/internal/node/agent-stream/{envId}`, signed like the HTTP hop, and keeps it for
   every session B opens on that agent.
2. A verifies the signature before upgrading, answers `409` when it does not hold the
   agent, and otherwise sends the agent's version, build and host details, so B applies the
   same feature checks it would locally.
3. B runs its ordinary agent transport over that socket. A relays frames unchanged in both
   directions: before writing a frame to the agent it routes the frame's id back to B, so
   the agent's first reply (a VNC banner, a shell prompt) cannot arrive ahead of its route.

The agent protocol does not change, and no agent upgrade is needed.

**Ordering.** One socket per hop and one reader per socket: keystrokes arrive in the order
typed, and a resize keeps its place among them.

**Backpressure.** No hop may stall the agent's socket, because every session on that host
shares it. A queues at most 512 frames toward B, and its writer blocks on the socket, not on
memory. Past three quarters full, terminal and console output is dropped, exactly what a
session on A itself does when its browser cannot keep up. A reply or end frame that cannot
be queued closes the link instead of being lost.

**Cancellation and close reasons.**

| What ended | What happens |
|---|---|
| The shell, console or log stream on the agent | The session ends on B as it would locally |
| The browser on B | B sends the end frame through A, so the process on the agent is freed |
| Node B, or the link | A sends the agent the end frame for every session B left open |
| The agent leaving node A (a reconnect elsewhere, a restart) | A closes the link with the reason; every session on B ends at once with *the agent for … left node A …; reopen to reach it where it reconnected* |
| Node A shutting down | The link closes with that reason, and the sessions on B end naming it |

A session never follows an agent that moved: reopening it reaches the agent on its new node.

### What stays with the node holding the agent

Messages the agent sends unasked carry no request to route them by, so they are handled
where the agent is connected: guest events and DHCP leases, container-backup progress
(the backup's result is relayed; its progress bar updates only on that node), offline
replay after a reconnect, and the stats push, which is why the other nodes read that
node's stats snapshot instead. The agent self-update and the *Test transfer* diagnostic on
the agent page also act only on agents connected to the node serving the request.


## Plugin workflow triggers

Automatic workflow triggers run on one node only: whichever holds the `workflow-trigger`
singleton lock. Plugins, though, run on every node, and a plugin fires its trigger where the
event happened: the Virtual Machines plugin fires **VM Trigger** on the node that recorded the
guest's state change, and other plugins on whichever node their watcher runs.

A fire on a node that does not lead is forwarded to the one that does:

1. The plugin's filter (its `match`) runs where the plugin runs, against the trigger placements
   in the shared database. A fire nothing matches ends there and crosses no hop.
2. The node gives the fire an id and sends it, signed like the other hops, to the node that
   accepted the last fire. Any other live node in the instance registry (read at most every
   five seconds) first gets a signed, body-less probe, and sees the fire only after it signs
   that it holds the `workflow-trigger` lock. Every node that does not hold the lock answers
   `409`, and the fire moves on to the next.
3. The leader keeps only the placements the plugin matched that still exist on active
   workflows, starts one run for each, and answers with how many started, which is what the
   plugin's own log line reports.

**Only its own keys.** A plugin fires only its own `plugin:<slug>:` trigger keys, and the host
only the keys it names (today the ITSM status trigger). The node that fired checks this, and the
leader checks it again against the source named in the fire's signed claims, answering `403` to
anything else.

**At most once.** Before starting any run, the leader records the fire id in the shared table
`workflow_trigger_fires`, whose unique key decides between two nodes. A fire retried after a
lost reply (including when the leader started the runs and died before answering, and the retry
reaches the node that took the lock over) finds its id there and starts nothing a second time.
The price is the other direction: a leader that crashes after recording the id and before
starting the runs drops that fire, and the retry is answered as a duplicate. A fire is never run
twice; in that narrow window it can be lost. An id is kept for ten minutes, well past the
one-minute retry window, and every node deletes expired ids once a minute.

**No leader.** When no node accepts (the leader has just stopped and the lock is not yet
retaken, or the path between the nodes fails), the fire is not dropped at once. The first
attempt is bounded to three seconds so the plugin is never held up; after that the fire is
retried in the background from 0.5 seconds, doubling to 8 seconds, for up to a minute. If the
asking node wins the lock itself meanwhile, it evaluates the fire locally under the same id.
Every failed attempt is logged at warning with the fire id and the action; a fire still
undelivered after a minute is logged at error as dropped. At most 256 fires wait at once; past
that a new one is dropped and logged.

A node with no `MCHARBOR_SECRET` cannot sign the hop, so a plugin trigger raised there is
logged and dropped, the same requirement as every other hop. On a single node nothing
changes, and in active/passive the active node holds every singleton lock, so a fire there
never leaves the node.

### Who owns an agent, and for how long

The node an agent connects to claims it in `cluster_agent_owners` and renews the claim
every 15 seconds against a 45-second lease. A claim cannot be taken from a node that is
still renewing it. A node that dies stops renewing, its claim expires, and the node the
agent reconnects to takes it over with no operator action, and no agent stranded behind a
claim nobody holds. A clean disconnect releases the claim at once.

## Proving it works

Configuration is not proof, so the readiness gate does not accept it as proof.

```bash
curl -fsS -X POST -H "Authorization: Bearer $MCHARBOR_API_KEY" \
  https://<node>/api/cluster/agent-routing-check
```

It sends a real Docker ping to every agent another node holds twice (once over the HTTP
hop and once through the session relay), all the way to the remote daemon and back, and
reports each result (`ok`, `streamOk`, `streamError`) with the node and the environment.
The `agentRouting` readiness gate reads what this records: it **fails** when a cross-node
Docker call failed, **warns** when nothing has crossed yet, when only the HTTP hop has been
proven, or when the session relay failed, and **passes** once a ping has come back through
both.

Requires `cluster.manage`. `GET /api/cluster/agent-routing` shows the same leases and the
last verdict, and needs only `cluster.view`.

## What is still per node

Two things, deliberately.

**Rate limits.** The auth rate limiter counts in memory on each node. A shared counter
would cost a database round trip on every request to buy a limit nobody asked to be exact.
Size it accordingly: the effective limit is the configured one multiplied by the number of
nodes.

**The console viewer cap.** A viewer is an open socket on one node. A shared count would
have to be reconciled against sockets no other node can see, and a node that died
mid-session would leak a seat forever. The cap of four applies per node.

Console **tickets** are not in this list: they live in the shared database, so a ticket
issued by one node is redeemable on any node, once, by whichever node gets there first.

## Readiness

`GET /api/cluster/readiness?target=active-active` runs the same gates as active/passive,
with three answering differently:

| Gate | In active/active |
|---|---|
| `agentRouting` | **fails** without `MCHARBOR_SECRET` or `MCHARBOR_ADVERTISE_URL`, or when the last cross-node Docker call failed. **warns** when configured and unproven, when only the HTTP hop is proven, and when the session relay failed, then naming the endpoint to let through and the paths to pin meanwhile. **passes** once both hops are proven |
| `nodeLinkTransport` | **fails** while more than one node is live and any of them advertises an address that is not `https://`, naming each one (for every target, since peer and active/passive nodes make node-to-node calls too), or while this node's `MCHARBOR_NODE_LINK_CA_FILE` cannot be loaded. **warns** instead of the http failure when `MCHARBOR_NODE_LINK_ALLOW_HTTP=1` is set, and whenever this node runs with `MCHARBOR_NODE_LINK_SKIP_VERIFY=1` and no CA file, saying that streamed bodies and relayed sessions become forgeable. **passes** when every live node advertises `https` and this node verifies their certificates, or when only one node is live |
| `perNodeState` | **fails** when the shared console ticket table does not answer from this node. Otherwise **warns**, naming the per-node rate limit and viewer cap |

A failing gate blocks the target; a warning does not. `enabled` in the response is true only
when no gate fails.

## Migrating from active/passive

1. Set `MCHARBOR_ADVERTISE_URL` on every node and confirm each one is reachable from the
   others.
2. Check `GET /api/cluster/readiness?target=active-active` on every node. Fix anything that
   fails before going further.
3. Remove the `/api/readyz` rule that sent traffic to one node only; in this mode every
   ready node takes traffic. If a proxy sits **between the nodes**, let it pass WebSocket
   upgrades (`Upgrade` and `Connection` headers) to `/api/internal/node/agent-stream/`.
4. Change `MCHARBOR_HA_MODE` to `active-active` on every node and restart them one at a
   time.
5. Run `POST /api/cluster/agent-routing-check` once every agent has reconnected, and check
   that `agentRouting` passes.

## When the session relay cannot be used

If the path between the nodes cannot carry a WebSocket, `agentRouting` warns and names the
failure. Until it is fixed, sessions work only from the node holding the agent, so pin
these paths to that node at the load balancer:

```
/api/terminal/ws
/api/system/host-terminal/ws
/api/compute/console/
/api/system/host-logs
```

Going back is the same list in reverse; nothing in the database has to change.

## Related

- [High availability](README.md): active/passive, the controls, the probes, the plugin
  package store
- [Database](../database/README.md): the shared database every mode needs
- [Compatibility](../../../getting-started/deployment/compatibility.md): which HA modes a
  version supports
