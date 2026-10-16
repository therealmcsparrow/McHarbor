# Scrape configuration

Three steps: a key that can do nothing but scrape, a Prometheus job that uses it, and the
dashboard.

## 1. A scrape-only API key

Create the key as a user who holds `metrics.scrape`. The Admin role does, through its
wildcard. A key's rights are the intersection of what it is granted and what its owner
has, so the key below can scrape and do nothing else, whatever its owner can do.

In the interface: **Security → API keys**, grant only `metrics.scrape`, no environments.
From a script:

```bash
curl -sk -X POST https://mcharbor.example:8705/api/api-keys \
  -H 'Authorization: Bearer mch_<an admin key>' \
  -H 'Content-Type: application/json' \
  -d '{"name":"prometheus","scopes":["metrics.scrape"]}'
```

The response carries the plaintext key once, in `key`. Store it in a file only Prometheus
can read, for example `/etc/prometheus/mcharbor.token`.

A custom role that should be able to scrape needs `metrics.scrape` added to it under
**Security → Roles**; it is in the **metrics** category.

## 2. The Prometheus job

```yaml
scrape_configs:
  - job_name: mcharbor
    scheme: https
    metrics_path: /api/metrics
    scrape_interval: 30s
    authorization:
      type: Bearer
      credentials_file: /etc/prometheus/mcharbor.token
    tls_config:
      ca_file: /etc/prometheus/mcharbor-root-ca.pem
    static_configs:
      - targets: ['mcharbor.example:8705']
```

- **TLS.** McHarbor serves HTTPS by default with a certificate from its own local
  authority. Download that root under **Security → HTTPS** and give it to Prometheus as
  `ca_file`. With your own certificate from a public authority, drop `tls_config`. If
  `TLS_ENABLED=false`, set `scheme: http`.
- **Interval.** Anything from 15 s up is fine; the scrape reads memory only and costs the
  same at any rate. The dashboard's rate windows assume at least one scrape per minute.
- **High availability.** With more than one McHarbor node, scrape each node. Every node
  reports its own requests; background work that only the leader runs (the metrics
  collector, the audit verifier) appears only on the leader.

Check the job from the Prometheus host before relying on it:

```bash
curl -s --cacert mcharbor-root-ca.pem \
  -H "Authorization: Bearer $(cat /etc/prometheus/mcharbor.token)" \
  https://mcharbor.example:8705/api/metrics | head
```

The first lines are `# HELP` and `# TYPE` comments. A JSON body means the request was
refused; see [Troubleshooting](./troubleshooting.md).

## 3. The Grafana dashboard

The repository ships `packaging/grafana/mcharbor-control-plane.json`. In Grafana:
**Dashboards → New → Import**, upload the file, choose your Prometheus data source.

It has five rows (Overview, HTTP and API, Agent transport, Background work, Runtime and
storage) and two variables: the data source, and `job`, which lists every job that
reports `mcharbor_build_info`, so several McHarbor instances can share one dashboard.

Three panels are worth knowing by name:

| Panel | Why |
|---|---|
| **Seconds since last completed request** | Climbs for an agent that holds its socket open but has stopped answering. The agent keeps reporting `active (running)` on its host; this is the panel that shows it is not |
| **Permission denials by key** | A steady line on one key is almost always a role or an API key missing that key, not an attack |
| **Plugins by state** | Every plugin moving to `failed` in one step is what a host rebuild that changed a plugin-linked package looks like |
