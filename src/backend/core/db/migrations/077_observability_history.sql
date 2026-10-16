-- Copyright (c) 2026 McSparrow. All rights reserved.
-- McHarbor is licensed under the McHarbor License. See LICENSE for details.

-- Container health history, uptime rollups, metrics history, and the
-- lifecycle timestamp normalisation.

-- Container exec history (per environment).
CREATE TABLE IF NOT EXISTS container_exec_history (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    environment_id TEXT NOT NULL,
    container_id TEXT NOT NULL,
    container_name TEXT,
    command TEXT NOT NULL DEFAULT '[]',
    work_dir TEXT,
    user_name TEXT,
    exit_code INTEGER NOT NULL DEFAULT 0,
    stdout TEXT NOT NULL DEFAULT '',
    stderr TEXT NOT NULL DEFAULT '',
    duration_ms INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_exec_history_user ON container_exec_history(user_id);
CREATE INDEX IF NOT EXISTS idx_exec_history_env ON container_exec_history(environment_id);
CREATE INDEX IF NOT EXISTS idx_exec_history_container ON container_exec_history(container_id);
CREATE INDEX IF NOT EXISTS idx_exec_history_created ON container_exec_history(created_at);


-- A stable identity for a container across recreation.
--
-- Every per-container table below carries `container_key` beside
-- `container_id`. The 64-hex id Docker mints per container is not stable
-- across a *recreate*: changing an image tag, an environment variable, a port
-- or a volume makes Compose remove the old container and create a new one,
-- and Docker hands the new one a brand-new id. Restart and stop/start keep the
-- id; recreate does not. Keyed on the id alone, every read of the form
-- `WHERE container_id = ?` returns zero rows the moment a workload is
-- updated: the resource graphs go blank, the uptime strip restarts from
-- nothing, and the health strip loses its probe log.
--
-- `container_key` is derived once, by the collector, from what the container
-- declares about itself (core/docker/identity.go):
--
--     compose:<project>/<service>/<number>   from the com.docker.compose.* labels
--     swarm:<service>/<slot>                 from the com.docker.swarm.* labels
--     name:<name>                            everything else
--
-- The trailing number/slot keeps replicas of one scaled service apart, in the
-- same way the container name already does. A Compose recreate preserves all
-- three label values, so the key survives exactly the event the id does not.
--
-- The column is nullable, and readers also match rows without a key on
-- `container_name`:
--
--     (container_key = ? OR (container_key IS NULL AND container_name = ?))
--
-- `container_id` is kept on every row. It is no longer the identity, but it
-- is still the only way to answer "which physical container produced this
-- sample", which is what makes a recreate visible in the data rather than
-- silently smoothed over.
--
-- The `_identity` lookup indexes mirror the per-id ones, since the key is the
-- leading predicate of every per-container read. The unique dedupe indexes on
-- (container_id, <timestamp>) deliberately stay per-id: they guard against a
-- retried collector tick writing the same physical sample twice.


-- Per-probe healthcheck history and periodic up/down samples, used by the
-- Health and Uptime cards on the container detail Overview tab.
--
-- Why a table is needed at all:
--
--   Docker keeps only the last FIVE entries in State.Health.Log, and the
--   event stream emits `health_status` only on TRANSITIONS. Neither can
--   render a 30-probe strip, and neither survives a page reload. The
--   collector in modules/containers/health_collector.go polls on the same
--   30s cadence as the metrics collector and appends whatever it has not
--   seen before.
--
-- container_health_probes
--   One row per healthcheck probe Docker actually ran. `probe_start` is
--   Docker's own Start timestamp for the probe, which makes the
--   (container_id, probe_start) pair a natural dedupe key across polls —
--   the same probe shows up in State.Health.Log for up to five polls in a
--   row and must only be stored once.
--
--   exit_code follows the Docker healthcheck contract:
--     0 = healthy, 1 = unhealthy, 2 = reserved (treated as unhealthy).
--
-- container_state_samples
--   One row per container per collector tick. This is the uptime series:
--   `running` is the 0/1 the strip renders, `state` and `health` carry the
--   detail shown in the per-block tooltip. Sampling (rather than deriving
--   from start/stop events) means a container that was never touched still
--   produces an unbroken series, and a missed event cannot silently
--   invent uptime.

CREATE TABLE IF NOT EXISTS container_health_probes (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT REFERENCES environments(id) ON DELETE CASCADE,
    container_id    TEXT NOT NULL,
    container_name  TEXT,
    probe_start     TEXT NOT NULL,
    probe_end       TEXT,
    exit_code       INTEGER NOT NULL DEFAULT 0,
    duration_ms     INTEGER NOT NULL DEFAULT 0,
    output          TEXT,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    container_key   TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_container_health_probes_unique
    ON container_health_probes(container_id, probe_start);
CREATE INDEX IF NOT EXISTS idx_container_health_probes_lookup
    ON container_health_probes(environment_id, container_id, probe_start DESC);
CREATE INDEX IF NOT EXISTS idx_container_health_probes_created
    ON container_health_probes(created_at);
CREATE INDEX IF NOT EXISTS idx_container_health_probes_identity
    ON container_health_probes(environment_id, container_key, probe_start DESC);

CREATE TABLE IF NOT EXISTS container_state_samples (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT REFERENCES environments(id) ON DELETE CASCADE,
    container_id    TEXT NOT NULL,
    container_name  TEXT,
    state           TEXT NOT NULL,
    running         INTEGER NOT NULL DEFAULT 0,
    health          TEXT,
    sampled_at      TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    container_key   TEXT
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_container_state_samples_unique
    ON container_state_samples(container_id, sampled_at);
CREATE INDEX IF NOT EXISTS idx_container_state_samples_lookup
    ON container_state_samples(environment_id, container_id, sampled_at DESC);
CREATE INDEX IF NOT EXISTS idx_container_state_samples_created
    ON container_state_samples(created_at);
CREATE INDEX IF NOT EXISTS idx_container_state_samples_identity
    ON container_state_samples(environment_id, container_key, sampled_at DESC);


-- Daily rollup of the health and uptime series, so reporting can answer a
-- 30- or 90-day question that the raw tables cannot.
--
-- Why this table exists:
--
--   container_state_samples holds one row per container per 30 seconds and is
--   pruned after 7 days (healthHistoryRetentionDays). That is the right trade
--   for the Overview tab's 30-block strip, and the wrong one for a report
--   whose cover says "1 August to 31 August": the raw series simply does not
--   reach back that far. A report that silently covers seven days while
--   claiming thirty is worse than no report at all.
--
--   So the collector rolls each finished day up into one row per container
--   immediately BEFORE it prunes, and the aggregate endpoint stitches the
--   rollup days together with the raw tail for today. Retention here is 400
--   days — long enough for a year-over-year comparison, and cheap: one row
--   per container per day rather than 2,880.
--
--   `day` is a plain YYYY-MM-DD in UTC. Reports are generated in the reader's
--   timezone but bucketed in UTC, because a rollup that shifts with the
--   viewer would make two runs of the same report disagree.
--
--   The counters mirror the raw columns one for one so the two sources can be
--   summed together without a conversion step:
--     samples/running_samples  -> container_state_samples.running
--     unhealthy_samples        -> container_state_samples.health = 'unhealthy'
--     outages                  -> running 1 -> 0 transitions within the day
--     transitions              -> any running-state change within the day
--     probes/probes_passed     -> container_health_probes.exit_code
--
--   sample_seconds records the sampling grid the day was collected on, so a
--   future change to the 30s cadence cannot silently rescale historic
--   downtime figures.

CREATE TABLE IF NOT EXISTS container_uptime_daily (
    id                TEXT PRIMARY KEY,
    environment_id    TEXT REFERENCES environments(id) ON DELETE CASCADE,
    container_id      TEXT NOT NULL,
    container_name    TEXT,
    day               TEXT NOT NULL,
    samples           INTEGER NOT NULL DEFAULT 0,
    running_samples   INTEGER NOT NULL DEFAULT 0,
    unhealthy_samples INTEGER NOT NULL DEFAULT 0,
    outages           INTEGER NOT NULL DEFAULT 0,
    transitions       INTEGER NOT NULL DEFAULT 0,
    probes            INTEGER NOT NULL DEFAULT 0,
    probes_passed     INTEGER NOT NULL DEFAULT 0,
    probe_ms_total    INTEGER NOT NULL DEFAULT 0,
    probe_ms_max      INTEGER NOT NULL DEFAULT 0,
    sample_seconds    INTEGER NOT NULL DEFAULT 30,
    created_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    container_key     TEXT
);

-- The rollup is written with INSERT OR REPLACE keyed on this pair, so a
-- re-run for the same day corrects the row rather than duplicating it.
CREATE UNIQUE INDEX IF NOT EXISTS idx_container_uptime_daily_unique
    ON container_uptime_daily(container_id, day);
CREATE INDEX IF NOT EXISTS idx_container_uptime_daily_lookup
    ON container_uptime_daily(environment_id, day DESC);
CREATE INDEX IF NOT EXISTS idx_container_uptime_daily_day
    ON container_uptime_daily(day);
CREATE INDEX IF NOT EXISTS idx_container_uptime_daily_identity
    ON container_uptime_daily(environment_id, container_key, day DESC);


-- Tiered resource-metric history: raw 30s samples that age into 10-minute
-- averages, for the container Overview graphs and for any report that has to
-- answer a question older than the live stream.
--
-- Why these tables are needed at all:
--
--   The metrics collector already fetches AllContainerStats every 30 seconds,
--   sums the per-container numbers into ONE host_metrics row, and throws the
--   detail away (modules/metrics/collector.go). So per-container CPU, memory,
--   network and block I/O have never been stored anywhere: the live stream at
--   /metrics/containers/{id}/stream is real time only, and a graph built from
--   it resets on every page reload. host_metrics itself was pruned at a
--   hardcoded 24 hours, so even the aggregate could not reach further back.
--
--   Nothing new is fetched from Docker for this. The samples are already in
--   memory on every tick; they are simply written down before being summed.
--
-- The two tiers:
--
--   Tier 1 (container_metrics, host_metrics) keeps one row per 30s tick for
--   `retention_metrics_raw_hours` (default 24). This is the resolution the
--   Overview graphs render.
--
--   Tier 2 (container_metrics_rollup, host_metrics_rollup) keeps one row per
--   10-minute bucket for `retention_metrics_rollup_days` (default 30), then
--   deletes it. A bucket is folded only once it lies ENTIRELY past the raw
--   cutoff, which is why compaction happens at 24h+10min rather than at 24h.
--   Folding a partial bucket would write an average over fewer samples than
--   the bucket claims, and re-running would then disagree with itself.
--
--   `bucket_seconds` is recorded per row rather than assumed, so a future
--   change to either the 30s cadence or the 10-minute bucket cannot silently
--   rescale history that was collected on the old grid.
--
-- Gauges vs counters — the reason the rollup columns are not symmetrical:
--
--   cpu_percent and mem_used are GAUGES: a value that stands on its own at the
--   instant it was read. Averaging them across a bucket is meaningful, and the
--   max is kept alongside so a 10-minute average can never hide a spike.
--
--   net_rx/net_tx/block_read/block_write are COUNTERS: cumulative totals since
--   the container started. Their average across a bucket is a meaningless
--   number (it depends on how long the container has been up, not on what it
--   did during those ten minutes). So the rollup stores the LAST value in the
--   bucket, which keeps the cumulative series continuous, AND the DELTA across
--   the bucket, which is the throughput. A container that restarts mid-bucket
--   resets its counter to 0; the collector clamps the resulting negative delta
--   to zero rather than writing a nonsense spike.
--
--   size_rw / size_root_fs are nullable because they are collected on a much
--   slower hourly sub-tick (they need ContainerList with Size:true, which is
--   far too expensive to run every 30 seconds). SQL AVG() ignores NULLs, so
--   the sparse column folds into the rollup with no special case.
--
-- GPU utilisation charts alongside CPU, memory, network and disk. One number
-- per sample, not one per card: the collector writes the *mean* utilisation
-- across visible GPUs. A max would make one pinned card look like a saturated
-- host; a sum would exceed 100% and break an axis that every other percentage
-- series assumes is bounded. NULL is meaningful and is why the GPU columns
-- are nullable with no default: it distinguishes "this host has no GPU
-- telemetry" from "the GPUs were idle". The history reader omits the series
-- entirely when every bucket is NULL.

-- ─── Tier 1: raw per-container samples ────────────────────────────────

CREATE TABLE IF NOT EXISTS container_metrics (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT REFERENCES environments(id) ON DELETE CASCADE,
    container_id    TEXT NOT NULL,
    container_name  TEXT,
    cpu_percent     REAL NOT NULL DEFAULT 0,
    mem_used        INTEGER NOT NULL DEFAULT 0,
    mem_limit       INTEGER NOT NULL DEFAULT 0,
    net_rx          INTEGER NOT NULL DEFAULT 0,
    net_tx          INTEGER NOT NULL DEFAULT 0,
    block_read      INTEGER NOT NULL DEFAULT 0,
    block_write     INTEGER NOT NULL DEFAULT 0,
    pids            INTEGER NOT NULL DEFAULT 0,
    size_rw         INTEGER,
    size_root_fs    INTEGER,
    sampled_at      TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    container_key   TEXT
);

-- The collector writes every container of a tick with one shared `sampled_at`,
-- so this pair is the natural dedupe key: a retried tick corrects rather than
-- duplicates.
CREATE UNIQUE INDEX IF NOT EXISTS idx_container_metrics_unique
    ON container_metrics(container_id, sampled_at);
CREATE INDEX IF NOT EXISTS idx_container_metrics_lookup
    ON container_metrics(environment_id, container_id, sampled_at DESC);
-- Compaction and prune both scan by time across all containers at once.
CREATE INDEX IF NOT EXISTS idx_container_metrics_sampled
    ON container_metrics(sampled_at);
CREATE INDEX IF NOT EXISTS idx_container_metrics_identity
    ON container_metrics(environment_id, container_key, sampled_at DESC);

-- Tier 1 for the environment aggregate is the existing host_metrics.
ALTER TABLE host_metrics ADD COLUMN gpu_percent REAL;

-- ─── Tier 2: 10-minute per-container averages ─────────────────────────

CREATE TABLE IF NOT EXISTS container_metrics_rollup (
    id                TEXT PRIMARY KEY,
    environment_id    TEXT REFERENCES environments(id) ON DELETE CASCADE,
    container_id      TEXT NOT NULL,
    container_name    TEXT,
    bucket_start      TEXT NOT NULL,
    bucket_seconds    INTEGER NOT NULL DEFAULT 600,
    samples           INTEGER NOT NULL DEFAULT 0,
    cpu_avg           REAL NOT NULL DEFAULT 0,
    cpu_max           REAL NOT NULL DEFAULT 0,
    mem_used_avg      INTEGER NOT NULL DEFAULT 0,
    mem_used_max      INTEGER NOT NULL DEFAULT 0,
    mem_limit_max     INTEGER NOT NULL DEFAULT 0,
    net_rx_last       INTEGER NOT NULL DEFAULT 0,
    net_tx_last       INTEGER NOT NULL DEFAULT 0,
    block_read_last   INTEGER NOT NULL DEFAULT 0,
    block_write_last  INTEGER NOT NULL DEFAULT 0,
    net_rx_delta      INTEGER NOT NULL DEFAULT 0,
    net_tx_delta      INTEGER NOT NULL DEFAULT 0,
    block_read_delta  INTEGER NOT NULL DEFAULT 0,
    block_write_delta INTEGER NOT NULL DEFAULT 0,
    size_rw_avg       INTEGER,
    size_root_fs_avg  INTEGER,
    created_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    container_key     TEXT
);

-- Written with INSERT OR REPLACE keyed on this pair, so a re-run for the same
-- bucket corrects the row rather than duplicating it.
CREATE UNIQUE INDEX IF NOT EXISTS idx_container_metrics_rollup_unique
    ON container_metrics_rollup(container_id, bucket_start);
CREATE INDEX IF NOT EXISTS idx_container_metrics_rollup_lookup
    ON container_metrics_rollup(environment_id, container_id, bucket_start DESC);
CREATE INDEX IF NOT EXISTS idx_container_metrics_rollup_bucket
    ON container_metrics_rollup(bucket_start);
CREATE INDEX IF NOT EXISTS idx_container_metrics_rollup_identity
    ON container_metrics_rollup(environment_id, container_key, bucket_start DESC);

-- ─── Tier 2: 10-minute environment-aggregate averages ─────────────────
--
-- gpu_avg and gpu_max mirror the cpu_avg / cpu_max pair so the same chart
-- code reads both without a special case.

CREATE TABLE IF NOT EXISTS host_metrics_rollup (
    id                TEXT PRIMARY KEY,
    environment_id    TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    bucket_start      TEXT NOT NULL,
    bucket_seconds    INTEGER NOT NULL DEFAULT 600,
    samples           INTEGER NOT NULL DEFAULT 0,
    cpu_avg           REAL NOT NULL DEFAULT 0,
    cpu_max           REAL NOT NULL DEFAULT 0,
    mem_percent_avg   REAL NOT NULL DEFAULT 0,
    mem_percent_max   REAL NOT NULL DEFAULT 0,
    mem_used_avg      INTEGER NOT NULL DEFAULT 0,
    mem_used_max      INTEGER NOT NULL DEFAULT 0,
    mem_total_max     INTEGER NOT NULL DEFAULT 0,
    net_rx_last       INTEGER NOT NULL DEFAULT 0,
    net_tx_last       INTEGER NOT NULL DEFAULT 0,
    block_read_last   INTEGER NOT NULL DEFAULT 0,
    block_write_last  INTEGER NOT NULL DEFAULT 0,
    net_rx_delta      INTEGER NOT NULL DEFAULT 0,
    net_tx_delta      INTEGER NOT NULL DEFAULT 0,
    block_read_delta  INTEGER NOT NULL DEFAULT 0,
    block_write_delta INTEGER NOT NULL DEFAULT 0,
    created_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at        TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    gpu_avg           REAL,
    gpu_max           REAL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_host_metrics_rollup_unique
    ON host_metrics_rollup(environment_id, bucket_start);
CREATE INDEX IF NOT EXISTS idx_host_metrics_rollup_bucket
    ON host_metrics_rollup(bucket_start);


-- Sampled health and uptime series for remote agent environments.
--
-- Why a table is needed at all:
--
--   Before this, an agent had exactly one observable: environments.agent_status,
--   a single mutable cell holding 'connected' or 'disconnected'. It is
--   overwritten on every transition and keeps no history, so nothing could
--   answer "was this agent up an hour ago", "how often did it flap overnight",
--   or "is the link slow right now" — the questions the Health and Uptime cards
--   on the container detail Overview tab already answer for containers.
--
--   The keepalive cannot supply that history either. core/agent/pool.go pings
--   every connected agent on a configurable interval, and both halves are
--   stamped on the connection, which yields liveness plus a round-trip
--   latency, but those live in process memory and die with the connection.
--   Persisting a sample per tick is what makes the series survive a reconnect,
--   a page reload, and a restart of McHarbor itself.
--
-- agent_health_samples
--   One row per agent environment per 30-second bucket, written by
--   modules/agent/health_collector.go. `sampled_at` is truncated to the bucket
--   boundary by both the collector and the on-read sampler, so the unique index
--   on (environment_id, sampled_at) turns a collision between the two into a
--   no-op rather than a duplicate block in the strip. This is the same
--   arrangement container_state_samples uses, and for the same reason.
--
--   `connected` is the 0/1 the uptime strip renders. `health` is the derived
--   tri-state the health strip renders:
--
--     healthy    socket up, pong seen within one ping interval, latency below
--                the degraded threshold
--     degraded   socket up but the keepalive is stale (older than two ping
--                intervals) or the round trip is over the latency threshold
--     unhealthy  no socket
--     unknown    an agent environment that has never connected at all
--
--   The two columns are deliberately not redundant: an agent can be connected
--   and degraded at the same time, which is precisely the state that was
--   invisible before and the reason this work exists.
--
--   `latency_ms` is the last measured ping/pong round trip, or NULL when there
--   is no live connection to measure. It is nullable rather than 0 because a
--   disconnected agent has no latency, and storing 0 would draw a floor on the
--   latency chart that reads as a perfect link.
--
--   `ping_interval_seconds` records the keepalive cadence the sample was taken
--   under. The interval is a user setting (settings: agent.pingInterval), so
--   without this a later change to it would silently rescale how historic
--   samples should be read — the same reasoning behind sample_seconds on
--   container_uptime_daily.
--
--   Retention is 7 days, pruned hourly by the collector, matching
--   healthHistoryRetentionDays for the container series.

CREATE TABLE IF NOT EXISTS agent_health_samples (
    id                    TEXT PRIMARY KEY,
    environment_id        TEXT NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
    connected             INTEGER NOT NULL DEFAULT 0,
    health                TEXT NOT NULL DEFAULT 'unknown',
    latency_ms            INTEGER,
    agent_version         TEXT,
    ping_interval_seconds INTEGER NOT NULL DEFAULT 30,
    sampled_at            TEXT NOT NULL,
    created_at            TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at            TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_health_samples_unique
    ON agent_health_samples(environment_id, sampled_at);
CREATE INDEX IF NOT EXISTS idx_agent_health_samples_lookup
    ON agent_health_samples(environment_id, sampled_at DESC);
-- Prune scans by time across every environment at once.
CREATE INDEX IF NOT EXISTS idx_agent_health_samples_sampled
    ON agent_health_samples(sampled_at);


-- The remote agent's own log, replayed from its local buffer.
--
-- While an agent cannot reach McHarbor it keeps Docker events, container
-- stats and its own log lines in a SQLite file on the host. On reconnect it
-- replays everything in acknowledged batches. Events land in lifecycle_events
-- / container_events and samples in container_metrics / host_metrics, the
-- tables the online collectors already write; the agent's own log lands
-- here. The id is minted by the agent so a batch that was persisted but whose
-- ack was lost collides with itself on replay instead of duplicating.
CREATE TABLE IF NOT EXISTS agent_logs (
    id             TEXT PRIMARY KEY,
    environment_id TEXT REFERENCES environments(id) ON DELETE CASCADE,
    level          TEXT NOT NULL DEFAULT 'info',
    message        TEXT NOT NULL,
    attrs          TEXT,
    logged_at      TEXT NOT NULL,
    created_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at     TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

CREATE INDEX IF NOT EXISTS idx_agent_logs_env_time
    ON agent_logs(environment_id, logged_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_logs_logged_at
    ON agent_logs(logged_at);


-- Repair lifecycle_events.timestamp, which was storing int64 nanoseconds where
-- every reader expects RFC3339.
--
-- What went wrong:
--
--   modules/activity/collector.go passed Docker's `event.TimeNano` — an int64
--   count of nanoseconds since the epoch, e.g. 1787236463897873700 — straight
--   into the timestamp column. The other writer, lifecycle.Service.Record,
--   correctly writes time.Now().UTC().Format(time.RFC3339). On the install
--   where this was found, 1,449,934 rows carried the nanosecond form and zero
--   carried RFC3339, because essentially every row comes from the collector.
--
-- Why it mattered more than a cosmetic inconsistency:
--
--   PurgeByRetention deleted with `WHERE timestamp < ?` against an RFC3339
--   cutoff. SQLite compares TEXT lexicographically, and '1' sorts before '2',
--   so EVERY nanosecond row compared less than a 2026-… cutoff. Running that
--   predicate matched 1,450,023 of 1,450,023 rows: the retention "Clear now"
--   button would have deleted the entire table, including events written
--   seconds earlier. julianday() on the column also returns NULL, so any
--   chronological query silently returned nothing.
--
--   This is the same trap already documented in activity/collector.go for
--   container_events, where it was solved with julianday(). lifecycle_events
--   was added later and reintroduced a worse variant.
--
-- The conversion:
--
--   Integer division by 1e9 turns nanoseconds into whole seconds, and
--   'unixepoch' + strftime renders the RFC3339 form the column is supposed to
--   hold. Sub-second precision is dropped, which is acceptable: nothing reads
--   this column at finer than one-second resolution, and the alternative is
--   keeping a format no query can use.
--
--   The WHERE guard makes this idempotent and safe to re-run. RFC3339 values
--   all begin '2' for any date this product will see, so `NOT LIKE '2%'`
--   selects exactly the unconverted rows and can never corrupt a correct one.
--   The extra typeof/GLOB test rejects anything that is neither format rather
--   than turning it into 1970-01-01.

UPDATE lifecycle_events
SET timestamp = strftime('%Y-%m-%dT%H:%M:%SZ', timestamp / 1000000000, 'unixepoch'),
    updated_at = CURRENT_TIMESTAMP
WHERE timestamp NOT LIKE '2%'
  AND timestamp GLOB '[0-9]*'
  AND CAST(timestamp AS INTEGER) > 0;

CREATE TABLE lifecycle_events_new (
    id              TEXT PRIMARY KEY,
    environment_id  TEXT REFERENCES environments(id) ON DELETE SET NULL,
    subject_type    TEXT NOT NULL CHECK(subject_type IN ('container','image','volume','network','stack','vm','ct','pool')),
    subject_id      TEXT NOT NULL,
    subject_name    TEXT,
    event_type      TEXT NOT NULL,
    action          TEXT NOT NULL,
    state           TEXT,
    severity        TEXT NOT NULL CHECK(severity IN ('info','success','warning','error')),
    metadata        TEXT,
    source          TEXT NOT NULL DEFAULT 'docker' CHECK(length(source) > 0),
    timestamp       TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    created_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP),
    updated_at      TEXT NOT NULL DEFAULT (CURRENT_TIMESTAMP)
);

INSERT INTO lifecycle_events_new (
    id, environment_id, subject_type, subject_id, subject_name,
    event_type, action, state, severity, metadata, source, timestamp,
    created_at, updated_at
)
SELECT
    id, environment_id, subject_type, subject_id, subject_name,
    event_type, action, state, severity, metadata, source, timestamp,
    created_at, updated_at
FROM lifecycle_events;

DROP TABLE lifecycle_events;

ALTER TABLE lifecycle_events_new RENAME TO lifecycle_events;

CREATE INDEX IF NOT EXISTS idx_lifecycle_events_timestamp
    ON lifecycle_events(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_env_time
    ON lifecycle_events(environment_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_subject
    ON lifecycle_events(subject_type, subject_id);
CREATE INDEX IF NOT EXISTS idx_lifecycle_events_severity
    ON lifecycle_events(severity, timestamp DESC);
