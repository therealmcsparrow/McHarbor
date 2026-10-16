# Capacity forecasting

Capacity forecasting answers one question: **when does this run out of space?**

The deliverable is a sentence with a date, such as *"/ reaches 90% on 6 October at the current
rate, 3 days from now"*, rather than a chart. A chart shows you what happened; a date tells you
when to act.

There is no cost model, no rate and no currency anywhere in this feature. It forecasts
space, not spend.

## What it forecasts

| Subject | What it is | Where the reading comes from |
|---|---|---|
| Host filesystem (`rootfs`) | The filesystem the environment's Docker host runs on, one per environment | `statfs(2)` on `/` for a local environment; the remote agent's own host-stats payload for an agent environment. No new agent message was needed |
| Storage destination (`storage_location`) | Each enabled backup destination, which is what a backup actually fills | The destination's own quota report. A destination whose provider cannot report a ceiling is **absent** from the series, never recorded as "0 of 0" |

Docker's reclaimable total (what a prune would return) is reported beside the
idle-resource list rather than forecast. It is a sawtooth, not a trend.

## The series

| | |
|---|---|
| Table | `capacity_samples` |
| Cadence | One sample per subject per hour |
| Retention | The `retention_metrics_rollup_days` setting, 30 days by default |
| Written by | The metrics collector, on the elected leader only |

Each row records its own `bucket_seconds`, so a later cadence change cannot silently
rescale history collected on the old grid. The sample time is truncated to the bucket by
the writer, so two writers landing in the same hour correct one row instead of adding a
second point to the fit.

Filesystem capacity is sampled hourly rather than every 30 seconds for two reasons: it
does not move fast enough to justify 2,880 rows a day per subject, and a fit needs days
rather than minutes.

## How the projection is made

An ordinary least-squares line is fitted through the retained samples, expressed as
percentage points per day, and extended to the red line. Every number is computed in Go
and stored; nothing about the arithmetic comes from a model. A model can be asked to put
a stored finding into words (see [Incidents](../../incidents/README.md)), and that is
optional, per-environment opt-in, and attributed.

The two lines the projection aims at are **75% (amber)** and **90% (red)**. It does not
project to 100%: a filesystem at 100% is already broken, and the useful date is the one it
crosses the red line on. The amber and red dates are projected independently, so a subject
already past amber still reports its red date.

## When it refuses

This is the part of the feature most worth understanding. A date extrapolated from three
samples is worse than no date, because somebody acts on it. So the forecast refuses, by
name, and the refusal reaches the operator as words.

| Refusal | Meaning | What to do |
|---|---|---|
| `short_series` | Fewer than 7 days of history | Wait. The message names both counts: "needs 7 days, has 3" |
| `noisy` | The fit's R² is below 0.7: the line does not describe the points | Nothing. Usage is too variable to project from |
| `not_rising` | The trend is flat or falling | Nothing. There is no date to give |
| `already_over` | Already past the red line | Act now. This needs attention, not a forecast |
| `beyond_horizon` | The crossing is more than 30 days out | Nothing. The forecast will not guess further than the window it read |
| `no_fit` | Fewer than two samples, or every sample at the same instant | Wait for the next hourly tick |

A refused subject is **not** a healthy subject. Neither the alert condition nor the report
treats it as one.

## Where to see it

| Surface | What it shows |
|---|---|
| Environment detail → Host | The projection per subject, with the two numbers it came from, the fit's R², and the window it read. Plus the idle-resource table |
| Dashboard widget "Capacity Headroom" | The four subjects nearest their red line, soonest first |
| Incidents → Behaviour findings | A `host_disk_forecast` or `storage_location_forecast` finding for every subject projected to cross the red line, on the same hourly detector as the memory forecast |
| Reports | The `capacity-utilisation` report type prints a "Projected exhaustion" table, refusals included |
| CLI | `mcharbor capacity list` |

Every one of these is read-only and gated on `host.view`.

## Idle resources

The other half of the feature, and the one with a measurable outcome. It identifies
containers worth reclaiming and never removes anything.

Two signals, kept deliberately apart:

| Signal | What it means | Read from |
|---|---|---|
| **Stopped** | The container's last running observation is strictly older than the threshold | `container_state_samples` |
| **No traffic** | The container ran across substantially the whole window with no network traffic and effectively no CPU | `container_metrics_rollup` |

They are labelled differently because they are not the same claim. A container stopped for
60 days is a fact; a running container with no traffic is a judgement call, and one list
showing both as the same thing would make the safe half untrustworthy.

"Stopped for N days" comes from retained history rather than a live container list, because
a live list knows a container is stopped and not for how long, which is the only thing
that makes it a candidate rather than a container somebody will start in a minute.

**The boundary is strict.** A container stopped for exactly the threshold has reached it and
not passed it, and is not listed.

### Sizes can be unknown, and that is not zero

Writable-layer sizes are not collected for agent environments. `Size: true` makes the
daemon walk every container's diff directory, and the result would then have to cross the
agent WebSocket, so samples an agent pushed carry them as NULL. A row with no size says
so and names the reason. It never shows `0 B`.

### Reclaiming

The row's action is the existing host prune, on `host.manage`. This feature adds the list;
it adds no destructive capability of its own.

## The alert condition

**Hosts & environments → "Days until storage is full"** is McHarbor's first trend-based
alert condition. Every other host condition compares an instantaneous reading.

| | |
|---|---|
| Condition key | `capacity_days` |
| Unit | days |
| Default | `< 14` |
| Target | Environment |
| Evaluated | Every 30 minutes; a fit over the retained series is not a per-tick query |

The source reports days-to-exhaustion as its value and never sees your threshold;
comparing the two is the engine's job, so two sources cannot disagree about what `< 14`
means.

Two behaviours worth knowing:

- **Healthy subjects still report.** Reporting only the subjects in trouble would read to
  the engine as "everything recovered" for the rest, and a rule would go quiet.
- **A refused subject reports nothing at all.** Emitting a large value for it would make
  "not enough data" read as a thousand days of headroom.

## Routes

| Route | Permission | Notes |
|---|---|---|
| `GET /api/metrics/capacity` | `host.view` | Database-only. Keeps answering while the Docker daemon is down, which is exactly when somebody asks how full the disk is |
| `GET /api/metrics/capacity/idle` | `host.view` | Takes `days` (1 to 365, default 14). Read-only |

`host.view` is scoped to the environment. `environments.view` is **global** and was
deliberately not reused: a per-environment capacity read behind a global key would widen
the scope.

## Troubleshooting

**Every subject says "not enough history yet".** The series is hourly and a projection
needs seven days of it. A freshly installed McHarbor takes a week to produce its first
date. This is the intended behaviour.

**A storage destination is missing from the list.** Its provider reports no capacity: an
FTP destination has no quota concept, and SFTP only has one when the server negotiated the
`statvfs@openssh.com` extension. It is recorded as absent rather than as 0 of 0.

**An agent environment's filesystem is missing.** The agent must support host stats.
Upgrade `mcharbor-agent`.

**Every container shows "size unknown".** The environment is an agent environment, or
writable-layer size collection is off in the retention settings.

**A date moved.** The fit reads the whole retained window, so one week of faster growth
moves the date closer. The R² on screen is how much to trust it.
