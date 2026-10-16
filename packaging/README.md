# packaging/

Inputs to the build's native artifacts: the `nfpm.yaml` that produces the `.deb` and `.rpm`, the
tarball layout, the systemd units and environment file they install, and the Grafana
dashboards shipped alongside. Files here are built into something; they are not copied by hand.

Examples to copy and adapt — Compose, Helm, Kubernetes manifests, reverse proxies — live in
[`deploy/`](../deploy/README.md).
