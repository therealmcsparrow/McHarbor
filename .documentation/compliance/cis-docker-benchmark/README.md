# Evidence for the CIS Docker Benchmark

This page is the mapping: one row per control, what it requires, which McHarbor evidence
answers it, and whether that answer is **checked mechanically**, **evidenced from recorded
data**, or **not covered**.

It is evidence for the benchmark. It is not a claim that an install meets the benchmark, and a
passing check means **the check passed** (a proxy you chose to trust), not that the control is
met. The benchmark's own numbering and section names are used throughout; "CIS Docker
Benchmark" is a proper noun and is never translated.

## Coverage at a glance

| | Controls |
|---|---|
| Checked mechanically | 31 |
| Evidenced from recorded data | 2 |
| Not covered | 5 |
| **Total in the pack** | **38** |

The benchmark has more controls than 38. The ones absent from this pack are absent because
they are host-filesystem or organisational controls indistinguishable from the five below, and
listing forty identical *not covered* rows would bury the ones that matter. The five listed are
the ones an auditor most often asks McHarbor about first.

## What McHarbor cannot see, and why

Three whole benchmark sections are outside what McHarbor can read, and every row in them says
so rather than guessing:

- **Section 1, host configuration.** McHarbor manages Docker through the daemon API. It has no
  view of the host's partitions, mount table, or `auditd` rules. Its own audit log is a
  different record and does **not** substitute for host auditing (see
  [Evidence integrity](../evidence-integrity/README.md)).
- **Section 3, daemon configuration files.** File ownership and permissions on
  `/etc/docker/daemon.json`, the socket and the systemd units are host filesystem state.
- **Section 6, security operations.** Process controls. McHarbor holds the inventory an
  auditor would review (images, containers, scan results, the audit log) and names where it
  is, but does not judge whether the process happened.

Content trust (4.5) is a fourth kind of gap worth stating separately: `DOCKER_CONTENT_TRUST`
is a client-side environment variable on whatever machine pulled the image. A daemon cannot
report it after the fact, so no amount of API access recovers the answer.

## How the container controls aggregate

Every control in sections 4 and 5 is per container, and one control row covers all of them.
Each result carries:

| Fact | Means |
|---|---|
| `containersChecked` | How many containers were inspected |
| `containersViolating` | How many broke the control; always exact |
| `violations` | Up to ten container names, as a sample |
| `inspectFailures` | How many inspects failed, when any did |

A control whose container list loaded but whose individual inspects partly failed reports
`unknown`, not `pass`: "no violations found" is only true of what was actually read. An
environment with no containers reports `info`, not `pass`.

## The controls


### Section 1: Host configuration

| Control | What it requires | Evidence | Verdict |
|---|---|---|---|
| **1.1.1** A separate partition for containers has been created | /var/lib/docker is on its own partition or logical volume. | none | **Not covered** |
| **1.1.4** Auditing is configured for Docker files and directories | auditd rules cover /var/lib/docker, /etc/docker and the daemon unit files. | none | **Not covered** |
| **1.2.1** The Docker Engine version is recorded | The engine is kept up to date; the running version is known. | `GET /info` | Checked mechanically |

### Section 2: Docker daemon configuration

| Control | What it requires | Evidence | Verdict |
|---|---|---|---|
| **2.1** Network traffic between containers on the default bridge is restricted | The default bridge sets com.docker.network.bridge.enable_icc to false. | `GET /networks/bridge` | Checked mechanically |
| **2.2** The daemon logging level is info | The daemon does not run with debug logging, which records request detail indefinitely. | `GET /info` | Checked mechanically |
| **2.5** A supported storage driver is in use | The daemon does not use aufs or the legacy devicemapper loopback driver. | `GET /info` | Checked mechanically |
| **2.6** The daemon API is not reachable without authentication | A daemon exposed over TCP uses TLS client authentication; a local socket is not exposed at all. | `environments.connection_type` | Checked mechanically |
| **2.8** User namespace support is enabled | The daemon runs with userns-remap so a container root is not the host root. | `GET /info` | Checked mechanically |
| **2.11** Authorization for Docker client commands is enabled | An authorization plugin decides daemon requests. | `GET /info` | Checked mechanically |
| **2.12** Centralized and remote logging is configured | The default logging driver sends container output off the host. | `GET /info` | Checked mechanically |
| **2.13** Live restore is enabled | Containers keep running while the daemon is unavailable. | `GET /info` | Checked mechanically |
| **2.15** A seccomp profile is applied by default | The daemon's default seccomp profile is not disabled. | `GET /info` | Checked mechanically |
| **2.17** Experimental features are not enabled | The daemon is not an experimental build. | `GET /info` | Checked mechanically |

### Section 3: Daemon configuration files

| Control | What it requires | Evidence | Verdict |
|---|---|---|---|
| **3.15** Docker socket file ownership and permissions are correct | /var/run/docker.sock is owned by root:docker with mode 660 or stricter. | none | **Not covered** |
| **3.17** daemon.json ownership and permissions are correct | /etc/docker/daemon.json is owned by root:root with mode 644 or stricter. | none | **Not covered** |

### Section 4: Container images and build files

| Control | What it requires | Evidence | Verdict |
|---|---|---|---|
| **4.1** Containers run as a non-root user | A container declares a user rather than running as root. | `GET /containers/{id}/json` | Checked mechanically |
| **4.5** Image provenance is verified before deployment | Content trust or an equivalent signature check gates what may run. | none | **Not covered** |
| **4.6** Containers declare a health check | Every container has a HEALTHCHECK, from its image or its run configuration. | `GET /containers/{id}/json` | Checked mechanically |

### Section 5: Container runtime

| Control | What it requires | Evidence | Verdict |
|---|---|---|---|
| **5.3** Linux kernel capabilities are restricted | No container adds capabilities beyond Docker's default set. | `GET /containers/{id}/json` | Checked mechanically |
| **5.4** Privileged containers are not used | No container runs with --privileged. | `GET /containers/{id}/json` | Checked mechanically |
| **5.7** Privileged ports are not mapped | No container publishes to a host port below 1024. | `GET /containers/{id}/json` | Checked mechanically |
| **5.9** The host network namespace is not shared | No container runs with --network=host. | `GET /containers/{id}/json` | Checked mechanically |
| **5.10** Container memory usage is limited | Every container has a memory limit, so one cannot exhaust the host. | `GET /containers/{id}/json` | Checked mechanically |
| **5.11** CPU priority is set | Every container has CPU shares or a CPU quota. | `GET /containers/{id}/json` | Checked mechanically |
| **5.12** The container root filesystem is mounted read only | Every container runs with a read-only root filesystem and explicit writable volumes. | `GET /containers/{id}/json` | Checked mechanically |
| **5.14** The on-failure restart policy is bounded | A restarting container uses on-failure with at most five attempts, not always. | `GET /containers/{id}/json` | Checked mechanically |
| **5.15** The host process namespace is not shared | No container runs with --pid=host. | `GET /containers/{id}/json` | Checked mechanically |
| **5.16** The host IPC namespace is not shared | No container runs with --ipc=host. | `GET /containers/{id}/json` | Checked mechanically |
| **5.17** Host devices are not exposed directly | No container is given a host device with --device. | `GET /containers/{id}/json` | Checked mechanically |
| **5.19** Mount propagation is not shared | No bind mount uses shared propagation, which lets a container write to the host mount tree. | `GET /containers/{id}/json` | Checked mechanically |
| **5.20** The host UTS namespace is not shared | No container runs with --uts=host. | `GET /containers/{id}/json` | Checked mechanically |
| **5.21** The default seccomp profile is not disabled | No container runs with seccomp=unconfined. | `GET /containers/{id}/json` | Checked mechanically |
| **5.25** Containers cannot acquire additional privileges | Every container runs with no-new-privileges. | `GET /containers/{id}/json` | Checked mechanically |
| **5.28** The PIDs cgroup limit is used | Every container has a PIDs limit, so a fork bomb cannot exhaust the host. | `GET /containers/{id}/json` | Checked mechanically |
| **5.31** The Docker socket is not mounted inside containers | No container binds /var/run/docker.sock, which is equivalent to host root. | `GET /containers/{id}/json` | Checked mechanically |

### Section 6: Security operations

| Control | What it requires | Evidence | Verdict |
|---|---|---|---|
| **6.1** Image sprawl is avoided | Unused images are removed rather than accumulating on hosts. | `GET /images/json`<br>`scans` | Evidenced from recorded data |
| **6.2** Container sprawl is avoided | Stopped and abandoned containers are removed. | `GET /containers/{id}/json`<br>`audit_logs` | Evidenced from recorded data |

### Section 7: Docker Swarm configuration

| Control | What it requires | Evidence | Verdict |
|---|---|---|---|
| **7.1** Swarm mode is not enabled unless it is needed | A host that does not orchestrate with Swarm leaves Swarm mode inactive. | `GET /info` | Checked mechanically |


## Reading a result

Verdict vocabulary, retention, permission keys and routes are in the
[Compliance packs index](../README.md). Two rules worth repeating here because they decide
whether a run is worth trusting:

- **A check that could not run reports `unknown` with a reason code, never `pass`.** An
  unreachable daemon makes every daemon control `unknown`; an unreadable container list makes
  every container control `unknown`.
- **`unknown` outranks `info` in the rollup.** A run with thirty passing checks and one
  unreachable one does not report as passing.

## Controls with a judgement deliberately withheld

Two controls record state and raise `info` rather than a verdict, because judging them would
mean guessing at intent:

- **1.2.1, engine version.** McHarbor knows which version is running. It does not know which
  version is current for the host's distribution or release channel, so it reports the version
  and stops.
- **7.1, Swarm mode.** Swarm being active is a finding only on a host that is not meant to
  orchestrate with it. The state is recorded; the judgement is yours.

## Two thresholds this pack chose, which you may disagree with

- **5.14, restart policy.** `always` and `unless-stopped` are treated as unbounded and warn;
  `on-failure` warns when its retry count is absent or above five. A container with no restart
  policy is not restarting at all and is left alone.
- **2.12, remote logging.** `json-file`, `local`, `none` and an unreported driver all warn. A
  sidecar that ships the host's JSON logs elsewhere satisfies the control's intent and this
  check cannot see it, so a warn here may be one you accept and record.

Both thresholds are in `modules/compliancepacks/checks.go`, and the facts behind every verdict
are stored with the result, so you can apply your own.
