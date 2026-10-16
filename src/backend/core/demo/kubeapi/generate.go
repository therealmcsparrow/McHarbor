// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package kubeapi

import (
	"fmt"
	"strings"
	"time"
)

type nsSpec struct {
	name     string
	apps     int
	replicas []int32
	vocab    []string
}

type storageSpec struct {
	name, provisioner, binding string
	isDefault                  bool
	params                     map[string]string
}

type stsSpec struct {
	ns, name, image, style, size, class string
	replicas                            int32
	port                                int32
}

type cronSpec struct {
	ns, name, schedule, image string
	failLast                  bool
}

type faultSpec struct {
	ns, name, fault, image string
	replicas               int32
}

type clusterSpec struct {
	key, name, version string
	arch               string
	net                int
	controlPlanes      int
	workers            int
	cpCPU, cpMem       string
	cpu, mem           string
	zones              []string
	ipPrefix           string
	notReady           string
	osImage, kernel    string
	runtime            string
	ageDays            int
	domain             string
	lbIP               string
	storage            []storageSpec
	system             []string
	apps               []nsSpec
	daemonSets         []string
	statefulSets       []stsSpec
	cronJobs           []cronSpec
	faults             []faultSpec
}

var (
	shopVocab     = []string{"storefront", "cart", "checkout", "catalog", "pricing", "inventory", "recommendations", "reviews", "wishlist", "promotions", "orders", "shipping", "returns", "gift-cards", "loyalty", "customer-profile", "notifications", "image-resizer", "sitemap", "feed-export"}
	paymentsVocab = []string{"ledger", "gateway", "fraud-check", "refunds", "payouts", "invoices", "tax", "currency", "settlement", "webhooks", "disputes", "risk-score", "card-vault", "billing"}
	searchVocab   = []string{"indexer", "query", "suggest", "ranking", "crawler", "synonyms", "spellcheck", "analytics"}
	dataVocab     = []string{"etl", "ingest", "warehouse-sync", "cdc", "stream-join", "dedupe", "enrich", "export", "quality", "lineage", "catalog-sync", "schema-registry"}
	mlVocab       = []string{"feature-store", "model-serving", "embeddings", "batch-scoring", "drift-monitor", "labeling", "notebook-proxy", "experiment-tracker"}
	identityVocab = []string{"auth", "oidc", "session", "mfa", "directory-sync", "audit", "consent"}
	iotVocab      = []string{"telemetry", "device-registry", "firmware", "edge-cache", "sensor-agg", "alarm", "ota", "rules-engine", "time-series-writer", "gateway"}
	appSuffixes   = []string{"", "api", "worker", "consumer", "sync", "scheduler", "exporter", "admin", "cache"}
)

var clusterSpecs = []clusterSpec{
	{
		key: "prod-eu", name: "prod-eu", version: "v1.30.4", arch: "amd64", net: 42,
		controlPlanes: 3, workers: 9, cpCPU: "4", cpMem: "16Gi", cpu: "16", mem: "64Gi",
		zones: []string{"eu-west-1a", "eu-west-1b", "eu-west-1c"}, ipPrefix: "10.10.0.",
		osImage: "Ubuntu 22.04.4 LTS", kernel: "5.15.0-113-generic", runtime: "containerd://1.7.18",
		ageDays: 412, domain: "prod-eu.demo.example.com", lbIP: "203.0.113.10",
		storage: []storageSpec{
			{name: "fast-ssd", provisioner: "ebs.csi.aws.com", binding: "WaitForFirstConsumer", isDefault: true, params: map[string]string{"type": "gp3"}},
			{name: "standard", provisioner: "ebs.csi.aws.com", binding: "WaitForFirstConsumer", params: map[string]string{"type": "st1"}},
			{name: "nfs-rwx", provisioner: "nfs.csi.k8s.io", binding: "Immediate", params: map[string]string{"server": "nfs.internal", "share": "/exports/k8s"}},
		},
		system: []string{"default", "kube-system", "kube-public", "monitoring", "ingress-nginx", "cert-manager", "argocd", "logging"},
		apps: []nsSpec{
			{name: "shop", apps: 50, replicas: []int32{2, 2, 3, 3, 3, 4, 4, 5, 6, 8}, vocab: shopVocab},
			{name: "payments", apps: 34, replicas: []int32{2, 2, 3, 3, 3, 4, 4, 5, 6}, vocab: paymentsVocab},
			{name: "search", apps: 18, replicas: []int32{2, 2, 3, 3, 4, 5}, vocab: searchVocab},
			{name: "data", apps: 30, replicas: []int32{1, 2, 2, 3, 3, 4}, vocab: dataVocab},
			{name: "ml", apps: 18, replicas: []int32{1, 2, 2, 3, 4}, vocab: mlVocab},
			{name: "identity", apps: 12, replicas: []int32{2, 3, 3, 4}, vocab: identityVocab},
		},
		daemonSets: []string{"node-exporter", "kube-proxy", "fluent-bit"},
		statefulSets: []stsSpec{
			{ns: "data", name: "postgres", image: "postgres:16.3", style: "postgres", size: "50Gi", class: "fast-ssd", replicas: 3, port: 5432},
			{ns: "data", name: "redis", image: "redis:7.2.5", style: "redis", size: "10Gi", class: "fast-ssd", replicas: 3, port: 6379},
			{ns: "data", name: "kafka", image: "bitnami/kafka:3.7.0", style: "infra", size: "100Gi", class: "fast-ssd", replicas: 3, port: 9092},
			{ns: "data", name: "zookeeper", image: "bitnami/zookeeper:3.9.2", style: "infra", size: "10Gi", class: "fast-ssd", replicas: 3, port: 2181},
			{ns: "search", name: "elasticsearch", image: "docker.elastic.co/elasticsearch/elasticsearch:8.14.1", style: "infra", size: "200Gi", class: "fast-ssd", replicas: 5, port: 9200},
			{ns: "monitoring", name: "prometheus-k8s", image: "quay.io/prometheus/prometheus:v2.53.0", style: "infra", size: "100Gi", class: "standard", replicas: 2, port: 9090},
			{ns: "monitoring", name: "alertmanager-main", image: "quay.io/prometheus/alertmanager:v0.27.0", style: "infra", size: "5Gi", class: "standard", replicas: 3, port: 9093},
			{ns: "logging", name: "loki", image: "grafana/loki:3.1.0", style: "infra", size: "50Gi", class: "standard", replicas: 3, port: 3100},
			{ns: "argocd", name: "argocd-application-controller", image: "quay.io/argoproj/argocd:v2.11.3", style: "infra", replicas: 1, port: 8082},
			{ns: "identity", name: "keycloak-db", image: "postgres:16.3", style: "postgres", size: "20Gi", class: "fast-ssd", replicas: 2, port: 5432},
		},
		cronJobs: []cronSpec{
			{ns: "data", name: "nightly-backup", schedule: "0 2 * * *", image: "ghcr.io/acme/pg-backup:1.4.0"},
			{ns: "data", name: "report-rollup", schedule: "*/30 * * * *", image: "ghcr.io/acme/data-rollup:2.1.3"},
			{ns: "shop", name: "cart-cleanup", schedule: "15 * * * *", image: "ghcr.io/acme/shop-cart:3.9.1"},
			{ns: "payments", name: "reconcile", schedule: "0 */6 * * *", image: "ghcr.io/acme/payments-ledger:5.2.0"},
			{ns: "monitoring", name: "backup-verify", schedule: "30 3 * * *", image: "ghcr.io/acme/backup-verify:0.9.2", failLast: true},
			{ns: "logging", name: "log-compactor", schedule: "0 * * * *", image: "grafana/loki:3.1.0"},
		},
		faults: []faultSpec{
			{ns: "payments", name: "fraud-check-worker", fault: "crashloop", replicas: 2},
			{ns: "ml", name: "gpu-trainer", fault: "unschedulable", replicas: 2},
			{ns: "search", name: "indexer", fault: "imagepull", replicas: 1, image: "ghcr.io/acme/search-indexer:v2.8.0-rc1"},
			{ns: "data", name: "stream-join", fault: "oom", replicas: 2},
			{ns: "shop", name: "recommendations", fault: "unready", replicas: 2},
		},
	},
	{
		key: "staging", name: "staging", version: "v1.30.4", arch: "amd64", net: 43,
		controlPlanes: 1, workers: 4, cpCPU: "4", cpMem: "8Gi", cpu: "8", mem: "32Gi",
		zones: []string{"eu-west-1a", "eu-west-1b"}, ipPrefix: "10.20.0.", notReady: "staging-worker-03",
		osImage: "Ubuntu 22.04.4 LTS", kernel: "5.15.0-113-generic", runtime: "containerd://1.7.18",
		ageDays: 205, domain: "staging.demo.example.com", lbIP: "203.0.113.20",
		storage: []storageSpec{
			{name: "standard", provisioner: "ebs.csi.aws.com", binding: "WaitForFirstConsumer", isDefault: true, params: map[string]string{"type": "gp3"}},
		},
		system: []string{"default", "kube-system", "monitoring", "ingress-nginx"},
		apps: []nsSpec{
			{name: "shop", apps: 36, replicas: []int32{1, 2, 2, 2, 3, 3}, vocab: shopVocab},
			{name: "payments", apps: 24, replicas: []int32{1, 2, 2, 2, 3, 3}, vocab: paymentsVocab},
			{name: "search", apps: 14, replicas: []int32{1, 2, 2, 3, 3}, vocab: searchVocab},
		},
		daemonSets: []string{"node-exporter", "kube-proxy", "fluent-bit"},
		statefulSets: []stsSpec{
			{ns: "shop", name: "postgres", image: "postgres:16.3", style: "postgres", size: "20Gi", class: "standard", replicas: 1, port: 5432},
			{ns: "shop", name: "redis", image: "redis:7.2.5", style: "redis", size: "5Gi", class: "standard", replicas: 1, port: 6379},
			{ns: "payments", name: "postgres", image: "postgres:16.3", style: "postgres", size: "20Gi", class: "standard", replicas: 1, port: 5432},
			{ns: "search", name: "elasticsearch", image: "docker.elastic.co/elasticsearch/elasticsearch:8.14.1", style: "infra", size: "50Gi", class: "standard", replicas: 3, port: 9200},
			{ns: "monitoring", name: "prometheus-k8s", image: "quay.io/prometheus/prometheus:v2.53.0", style: "infra", size: "20Gi", class: "standard", replicas: 1, port: 9090},
		},
		cronJobs: []cronSpec{
			{ns: "shop", name: "seed-fixtures", schedule: "0 6 * * 1-5", image: "ghcr.io/acme/shop-fixtures:0.4.1"},
			{ns: "payments", name: "reconcile", schedule: "0 */6 * * *", image: "ghcr.io/acme/payments-ledger:5.3.0-beta.2"},
		},
		faults: []faultSpec{
			{ns: "shop", name: "checkout-worker", fault: "crashloop", replicas: 1},
		},
	},
	{
		key: "edge", name: "edge", version: "v1.29.6+k3s1", arch: "arm64", net: 44,
		controlPlanes: 1, workers: 2, cpCPU: "4", cpMem: "8Gi", cpu: "4", mem: "8Gi",
		zones: []string{"store-017"}, ipPrefix: "192.168.50.",
		osImage: "Debian GNU/Linux 12 (bookworm)", kernel: "6.6.31+rpt-rpi-v8", runtime: "containerd://1.7.17-k3s1",
		ageDays: 96, domain: "edge.demo.example.com", lbIP: "192.168.50.200",
		storage: []storageSpec{
			{name: "local-path", provisioner: "rancher.io/local-path", binding: "WaitForFirstConsumer", isDefault: true},
		},
		system: []string{"default", "kube-system", "monitoring"},
		apps: []nsSpec{
			{name: "iot", apps: 28, replicas: []int32{1, 1, 2, 2}, vocab: iotVocab},
		},
		daemonSets: []string{"node-exporter", "fluent-bit"},
		statefulSets: []stsSpec{
			{ns: "iot", name: "influxdb", image: "influxdb:2.7.6", style: "infra", size: "20Gi", class: "local-path", replicas: 1, port: 8086},
			{ns: "monitoring", name: "prometheus", image: "quay.io/prometheus/prometheus:v2.53.0", style: "infra", size: "10Gi", class: "local-path", replicas: 1, port: 9090},
		},
		cronJobs: []cronSpec{
			{ns: "iot", name: "firmware-sync", schedule: "0 1 * * *", image: "ghcr.io/acme/iot-firmware:1.12.0"},
		},
		faults: []faultSpec{
			{ns: "iot", name: "mqtt-bridge", fault: "crashloop", replicas: 1},
		},
	},
}

type infraSpec struct {
	ns, name, image, style string
	replicas               int32
	port                   int32
	service                bool
}

func infraFor(spec clusterSpec) []infraSpec {
	out := []infraSpec{
		{ns: "kube-system", name: "coredns", image: "registry.k8s.io/coredns/coredns:v1.11.1", style: "infra", replicas: 2, port: 53},
		{ns: "kube-system", name: "metrics-server", image: "registry.k8s.io/metrics-server/metrics-server:v0.7.1", style: "infra", replicas: 1, port: 10250, service: true},
		{ns: "monitoring", name: "kube-state-metrics", image: "registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.12.0", style: "infra", replicas: 1, port: 8080, service: true},
		{ns: "monitoring", name: "grafana", image: "grafana/grafana:11.1.0", style: "app", replicas: 1, port: 3000, service: true},
	}
	if spec.key == "edge" {
		return out
	}
	controllers := int32(2)
	if spec.key == "prod-eu" {
		controllers = 3
	}
	out = append(out,
		infraSpec{ns: "monitoring", name: "prometheus-operator", image: "quay.io/prometheus-operator/prometheus-operator:v0.75.0", style: "infra", replicas: 1, port: 8080, service: true},
		infraSpec{ns: "ingress-nginx", name: "ingress-nginx-controller", image: "registry.k8s.io/ingress-nginx/controller:v1.10.1", style: "nginx", replicas: controllers, port: 80},
	)
	if spec.key != "prod-eu" {
		return out
	}
	return append(out,
		infraSpec{ns: "kube-system", name: "cluster-autoscaler", image: "registry.k8s.io/autoscaling/cluster-autoscaler:" + strings.SplitN(spec.version, "+", 2)[0], style: "infra", replicas: 1, port: 8085},
		infraSpec{ns: "monitoring", name: "blackbox-exporter", image: "quay.io/prometheus/blackbox-exporter:v0.25.0", style: "infra", replicas: 1, port: 9115, service: true},
		infraSpec{ns: "ingress-nginx", name: "ingress-nginx-defaultbackend", image: "registry.k8s.io/defaultbackend-amd64:1.5", style: "nginx", replicas: 1, port: 8080, service: true},
		infraSpec{ns: "cert-manager", name: "cert-manager", image: "quay.io/jetstack/cert-manager-controller:v1.15.1", style: "infra", replicas: 1, port: 9402, service: true},
		infraSpec{ns: "cert-manager", name: "cert-manager-cainjector", image: "quay.io/jetstack/cert-manager-cainjector:v1.15.1", style: "infra", replicas: 1, port: 9402},
		infraSpec{ns: "cert-manager", name: "cert-manager-webhook", image: "quay.io/jetstack/cert-manager-webhook:v1.15.1", style: "infra", replicas: 1, port: 10250, service: true},
		infraSpec{ns: "argocd", name: "argocd-server", image: "quay.io/argoproj/argocd:v2.11.3", style: "infra", replicas: 2, port: 8080, service: true},
		infraSpec{ns: "argocd", name: "argocd-repo-server", image: "quay.io/argoproj/argocd:v2.11.3", style: "infra", replicas: 2, port: 8081, service: true},
		infraSpec{ns: "argocd", name: "argocd-applicationset-controller", image: "quay.io/argoproj/argocd:v2.11.3", style: "infra", replicas: 1, port: 7000},
		infraSpec{ns: "argocd", name: "argocd-dex-server", image: "ghcr.io/dexidp/dex:v2.38.0", style: "infra", replicas: 1, port: 5556, service: true},
		infraSpec{ns: "argocd", name: "argocd-notifications-controller", image: "quay.io/argoproj/argocd:v2.11.3", style: "infra", replicas: 1, port: 9001},
		infraSpec{ns: "argocd", name: "argocd-redis", image: "redis:7.2.5", style: "redis", replicas: 1, port: 6379, service: true},
		infraSpec{ns: "logging", name: "loki-gateway", image: "nginxinc/nginx-unprivileged:1.27-alpine", style: "nginx", replicas: 2, port: 8080, service: true},
		infraSpec{ns: "logging", name: "log-router", image: "fluent/fluentd:v1.17-1", style: "infra", replicas: 2, port: 24224, service: true},
	)
}

func (s *Sim) generate(spec clusterSpec) *cluster {
	r := s.rng
	c := &cluster{
		key: spec.key, name: spec.name, version: spec.version, arch: spec.arch, net: spec.net,
		spec:    spec,
		wlIndex: map[string]*workload{}, pods: map[string]*pod{}, static: map[string][]static{},
	}
	born := s.base.Add(-time.Duration(spec.ageDays) * 24 * time.Hour)

	total := spec.controlPlanes + spec.workers
	for i := 0; i < total; i++ {
		cp := i < spec.controlPlanes
		name := fmt.Sprintf("%s-worker-%02d", spec.key, i-spec.controlPlanes+1)
		cpu, mem, itype := spec.cpu, spec.mem, "m6i.4xlarge"
		if cp {
			name = fmt.Sprintf("%s-cp-%d", spec.key, i+1)
			cpu, mem, itype = spec.cpCPU, spec.cpMem, "m6i.xlarge"
		}
		if spec.arch == "arm64" {
			itype = "raspberrypi-5"
		}
		n := &node{
			name: name, idx: i + 1, controlPlane: cp, arch: spec.arch,
			zone: spec.zones[i%len(spec.zones)], cpu: cpu, mem: mem, ready: name != spec.notReady,
			internalIP: fmt.Sprintf("%s%d", spec.ipPrefix, 10+i),
			created:    born.Add(time.Duration(i) * 7 * time.Minute),
		}
		if !cp {
			n.created = born.Add(time.Duration(r.between(0, spec.ageDays/2)) * 24 * time.Hour)
		}
		n.uid = uidFor(spec.key, "node", name)
		n.labels = map[string]string{
			"kubernetes.io/hostname":           name,
			"kubernetes.io/arch":               spec.arch,
			"kubernetes.io/os":                 "linux",
			"beta.kubernetes.io/arch":          spec.arch,
			"beta.kubernetes.io/os":            "linux",
			"topology.kubernetes.io/zone":      n.zone,
			"node.kubernetes.io/instance-type": itype,
		}
		if cp {
			n.labels["node-role.kubernetes.io/control-plane"] = ""
		}
		n.rv = c.nextRV()
		c.nodes = append(c.nodes, n)
	}

	names := append(append([]string{}, spec.system...), appNames(spec)...)
	for i, name := range names {
		created := born.Add(time.Duration(i) * time.Minute)
		if i >= len(spec.system) {
			created = born.Add(time.Duration(r.between(2, spec.ageDays/3+2)) * 24 * time.Hour)
		}
		ns := &namespace{name: name, uid: uidFor(spec.key, "ns", name), created: created,
			labels: map[string]string{"kubernetes.io/metadata.name": name}}
		if i >= len(spec.system) {
			ns.labels["team"] = name
			ns.labels["pod-security.kubernetes.io/enforce"] = "baseline"
		}
		ns.rv = c.nextRV()
		c.namespaces = append(c.namespaces, ns)
	}

	s.genStorageClasses(c, spec, born)
	s.genInfra(c, spec)
	s.genApps(c, spec)
	s.genDaemonSets(c, spec, born)
	s.genStatefulSets(c, spec, born)
	s.genCronJobs(c, spec)
	for _, w := range c.workloads {
		s.seedPods(c, w)
	}
	s.genServices(c, spec)
	s.genIngresses(c, spec)
	s.genConfig(c, spec)
	s.genPolicy(c, spec)
	s.genEvents(c)
	return c
}

func appNames(spec clusterSpec) []string {
	out := make([]string, 0, len(spec.apps))
	for _, a := range spec.apps {
		out = append(out, a.name)
	}
	return out
}

func (c *cluster) nsCreated(name string) time.Time {
	for _, n := range c.namespaces {
		if n.name == name {
			return n.created
		}
	}
	return time.Time{}
}

func (s *Sim) addWorkload(c *cluster, w *workload) {
	w.uid = uidFor(c.key, w.kind, w.ns, w.name)
	if w.labels == nil {
		w.labels = map[string]string{"app": w.name, "app.kubernetes.io/name": w.name, "app.kubernetes.io/part-of": w.ns}
	}
	if w.tplAnnotations == nil {
		w.tplAnnotations = map[string]string{}
	}
	w.generation = 1
	switch w.kind {
	case "Deployment":
		revs := 1
		if s.rng.chance(0.35) {
			revs = s.rng.between(2, 4)
		}
		for i := 1; i <= revs; i++ {
			hash := s.rng.suffix(10)
			rs := &replicaSet{
				name: w.name + "-" + hash, hash: hash, revision: i,
				uid:     uidFor(c.key, "rs", w.ns, w.name, hash),
				created: w.created.Add(time.Duration(i-1) * time.Duration(s.rng.between(1, 30)) * 24 * time.Hour),
			}
			rs.rv = c.nextRV()
			w.rs = append(w.rs, rs)
		}
		w.generation = int64(revs)
		w.tplVersion, w.rolledVersion = 1, 1
	case "StatefulSet", "DaemonSet":
		w.rs = []*replicaSet{{hash: s.rng.suffix(10)}}
	}
	w.rv = c.nextRV()
	c.workloads = append(c.workloads, w)
	c.wlIndex[wlKey(w.kind, w.ns, w.name)] = w
}

func (s *Sim) appContainer(ns, name, style string, port int32) container {
	r := s.rng
	return container{
		name: name, image: fmt.Sprintf("ghcr.io/acme/%s-%s:%d.%d.%d", ns, name, r.between(1, 6), r.between(0, 24), r.between(0, 9)),
		port: port, style: style,
		cpu:    pick(r, []string{"50m", "100m", "100m", "250m", "250m", "500m", "1"}),
		mem:    pick(r, []string{"64Mi", "128Mi", "256Mi", "256Mi", "512Mi", "1Gi", "2Gi"}),
		cpuLim: pick(r, []string{"500m", "1", "2"}),
		memLim: pick(r, []string{"512Mi", "1Gi", "2Gi", "4Gi"}),
	}
}

func (s *Sim) genInfra(c *cluster, spec clusterSpec) {
	for _, in := range infraFor(spec) {
		w := &workload{kind: "Deployment", ns: in.ns, name: in.name, replicas: in.replicas,
			created: c.nsCreated(in.ns).Add(time.Duration(s.rng.between(1, 90)) * time.Minute)}
		if in.ns == "kube-system" {
			w.labels = map[string]string{"k8s-app": in.name, "app": in.name}
		}
		w.containers = []container{{name: in.name, image: in.image, port: in.port, style: in.style,
			cpu: "100m", mem: "128Mi", cpuLim: "1", memLim: "512Mi"}}
		w.serviceAccount = in.name
		s.addWorkload(c, w)
		if in.service {
			s.addService(c, in.ns, in.name, map[string]string{"app": in.name}, 80, in.port, "ClusterIP", w.created)
		}
	}
}

func (s *Sim) genApps(c *cluster, spec clusterSpec) {
	r := s.rng
	for _, ns := range spec.apps {
		used := map[string]bool{}
		names := make([]string, 0, ns.apps)
		faults := map[string]faultSpec{}
		for _, f := range spec.faults {
			if f.ns == ns.name {
				names = append(names, f.name)
				used[f.name] = true
				faults[f.name] = f
			}
		}
		vocab := append([]string{}, ns.vocab...)
		for i := len(vocab) - 1; i > 0; i-- {
			j := r.intn(i + 1)
			vocab[i], vocab[j] = vocab[j], vocab[i]
		}
	fill:
		for _, suf := range appSuffixes {
			for _, e := range vocab {
				if len(names) >= ns.apps {
					break fill
				}
				n := e
				if suf != "" {
					n = e + "-" + suf
				}
				if !used[n] {
					used[n] = true
					names = append(names, n)
				}
			}
		}
		nsBorn := c.nsCreated(ns.name)
		age := int(s.base.Sub(nsBorn).Hours())
		for _, name := range names {
			w := &workload{kind: "Deployment", ns: ns.name, name: name, replicas: pick(r, ns.replicas),
				created: nsBorn.Add(time.Duration(r.between(1, max(2, age-2))) * time.Hour)}
			style := "app"
			if strings.HasSuffix(name, "gateway") || strings.HasSuffix(name, "proxy") || name == "storefront" {
				style = "nginx"
			}
			w.containers = []container{s.appContainer(ns.name, name, style, 8080)}
			if r.chance(0.15) {
				w.containers = append(w.containers, container{name: "envoy", image: "envoyproxy/envoy:v1.30.2", port: 15001,
					style: "envoy", cpu: "50m", mem: "64Mi", cpuLim: "500m", memLim: "256Mi"})
			}
			if r.chance(0.5) {
				w.serviceAccount = name
			}
			if f, ok := faults[name]; ok {
				w.fault = f.fault
				w.replicas = f.replicas
				if f.image != "" {
					w.containers[0].image = f.image
				}
				if f.fault == "unschedulable" {
					w.containers[0].cpu, w.containers[0].mem = "8", "48Gi"
				}
			}
			s.addWorkload(c, w)
			if isServing(name) {
				s.addService(c, ns.name, name, map[string]string{"app": name}, 80, 8080, "ClusterIP", w.created)
			}
		}
	}
}

func isServing(name string) bool {
	for _, suf := range []string{"-worker", "-consumer", "-sync", "-scheduler", "-exporter", "-cache"} {
		if strings.HasSuffix(name, suf) {
			return false
		}
	}
	return true
}

func (s *Sim) genDaemonSets(c *cluster, spec clusterSpec, born time.Time) {
	images := map[string]container{
		"node-exporter": {name: "node-exporter", image: "quay.io/prometheus/node-exporter:v1.8.1", port: 9100, style: "infra", cpu: "50m", mem: "64Mi", cpuLim: "250m", memLim: "128Mi"},
		"kube-proxy":    {name: "kube-proxy", image: "registry.k8s.io/kube-proxy:" + spec.version, port: 10249, style: "infra", cpu: "100m", mem: "64Mi", cpuLim: "500m", memLim: "256Mi"},
		"fluent-bit":    {name: "fluent-bit", image: "fluent/fluent-bit:3.0.7", port: 2020, style: "infra", cpu: "50m", mem: "64Mi", cpuLim: "500m", memLim: "256Mi"},
	}
	nsFor := map[string]string{"node-exporter": "monitoring", "kube-proxy": "kube-system", "fluent-bit": "logging"}
	for _, name := range spec.daemonSets {
		ns := nsFor[name]
		if c.nsCreated(ns).IsZero() {
			ns = "kube-system"
		}
		w := &workload{kind: "DaemonSet", ns: ns, name: name, containers: []container{images[name]},
			created: born.Add(time.Duration(s.rng.between(10, 180)) * time.Minute), serviceAccount: name}
		if ns == "kube-system" {
			w.labels = map[string]string{"k8s-app": name, "app": name}
		}
		w.replicas = int32(len(c.nodes))
		s.addWorkload(c, w)
	}
}

func (s *Sim) genStatefulSets(c *cluster, spec clusterSpec, born time.Time) {
	for _, st := range spec.statefulSets {
		created := c.nsCreated(st.ns).Add(time.Duration(s.rng.between(1, 48)) * time.Hour)
		w := &workload{kind: "StatefulSet", ns: st.ns, name: st.name, replicas: st.replicas, created: created,
			serviceName: st.name + "-headless", serviceAccount: st.name,
			containers: []container{{name: st.name, image: st.image, port: st.port, style: st.style,
				cpu: "500m", mem: "2Gi", cpuLim: "2", memLim: "4Gi"}}}
		s.addWorkload(c, w)
		s.addService(c, st.ns, st.name, map[string]string{"app": st.name}, st.port, st.port, "ClusterIP", created)
		s.addHeadless(c, st.ns, st.name+"-headless", st.name, st.port, created)
		if st.size == "" {
			continue
		}
		for i := int32(0); i < st.replicas; i++ {
			claim := fmt.Sprintf("data-%s-%d", st.name, i)
			s.addVolume(c, st.ns, claim, st.class, st.size, "Bound", created.Add(time.Duration(i)*time.Minute), map[string]string{"app": st.name})
		}
	}
	if spec.key == "prod-eu" {
		s.addVolume(c, "ml", "datasets-cache", "nfs-rwx", "2Ti", "Pending", s.base.Add(-3*time.Hour), map[string]string{"app": "gpu-trainer"})
		s.addVolume(c, "data", "legacy-export", "standard", "500Gi", "Released", born.Add(40*24*time.Hour), nil)
		s.addVolume(c, "shop", "media-uploads", "nfs-rwx", "250Gi", "Bound", c.nsCreated("shop").Add(2*time.Hour), map[string]string{"app": "image-resizer"})
	}
}

func (s *Sim) genCronJobs(c *cluster, spec clusterSpec) {
	for _, cj := range spec.cronJobs {
		created := c.nsCreated(cj.ns).Add(time.Duration(s.rng.between(2, 72)) * time.Hour)
		uid := uidFor(c.key, "CronJob", cj.ns, cj.name)
		interval := cronInterval(cj.schedule)
		last := s.base.Add(-time.Duration(s.rng.between(1, int(interval/time.Minute))) * time.Minute).Truncate(time.Minute)
		lastOK := last
		img := container{name: cj.name, image: cj.image, style: "job", cpu: "250m", mem: "256Mi", cpuLim: "1", memLim: "1Gi"}
		for i := 1; i >= 0; i-- {
			at := last.Add(-time.Duration(i) * interval)
			jobName := fmt.Sprintf("%s-%d", cj.name, at.Unix()/60)
			w := &workload{kind: "Job", ns: cj.ns, name: jobName, replicas: 1, created: at,
				containers: []container{img}, cronJob: cj.name, cronJobUID: uid, restartPolicy: "OnFailure",
				completed: at.Add(time.Duration(s.rng.between(20, 400)) * time.Second), fault: "completed"}
			w.labels = map[string]string{"job-name": jobName, "batch.kubernetes.io/job-name": jobName}
			if i == 0 && cj.failLast {
				w.jobFailed = true
				w.fault = "failed"
				lastOK = at.Add(-interval)
			}
			s.addWorkload(c, w)
		}
		obj := M{
			"apiVersion": "batch/v1", "kind": "CronJob",
			"metadata": meta(cj.name, cj.ns, uid, c.nextRV(), created, map[string]string{"app": cj.name}, nil, nil),
			"spec": M{
				"schedule": cj.schedule, "concurrencyPolicy": "Forbid", "suspend": false,
				"successfulJobsHistoryLimit": 3, "failedJobsHistoryLimit": 1,
				"jobTemplate": M{"metadata": M{}, "spec": M{"backoffLimit": 2, "template": M{
					"metadata": M{"labels": map[string]string{"app": cj.name}},
					"spec":     M{"restartPolicy": "OnFailure", "containers": []M{containerSpec(img)}},
				}}},
			},
			"status": M{"lastScheduleTime": ts(last), "lastSuccessfulTime": ts(lastOK)},
		}
		c.static["cronjobs"] = append(c.static["cronjobs"], static{ns: cj.ns, name: cj.name, obj: obj})
	}
	if spec.key != "prod-eu" {
		return
	}
	img := container{name: "migrate", image: "ghcr.io/acme/payments-ledger:5.2.0", style: "job", cpu: "100m", mem: "128Mi", cpuLim: "500m", memLim: "512Mi"}
	for i, ns := range []string{"payments", "shop", "identity"} {
		at := s.base.Add(-time.Duration(2+i*9) * 24 * time.Hour)
		name := "db-migrate-" + s.rng.suffix(5)
		w := &workload{kind: "Job", ns: ns, name: name, replicas: 1, created: at, containers: []container{img},
			restartPolicy: "Never", completed: at.Add(47 * time.Second), fault: "completed"}
		w.labels = map[string]string{"job-name": name, "batch.kubernetes.io/job-name": name}
		s.addWorkload(c, w)
	}
}

func cronInterval(schedule string) time.Duration {
	f := strings.Fields(schedule)
	if len(f) != 5 {
		return 24 * time.Hour
	}
	var n int
	switch {
	case strings.HasPrefix(f[0], "*/"):
		if _, err := fmt.Sscanf(f[0], "*/%d", &n); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	case strings.HasPrefix(f[1], "*/"):
		if _, err := fmt.Sscanf(f[1], "*/%d", &n); err == nil && n > 0 {
			return time.Duration(n) * time.Hour
		}
	case f[1] == "*":
		return time.Hour
	}
	return 24 * time.Hour
}

func (s *Sim) seedPods(c *cluster, w *workload) {
	r := s.rng
	switch w.kind {
	case "DaemonSet":
		for _, n := range c.nodes {
			created := maxTime(w.created, n.created).Add(time.Duration(r.between(5, 90)) * time.Second)
			if r.chance(0.3) {
				created = s.base.Add(-time.Duration(r.between(2, 40*24)) * time.Hour)
			}
			s.newPod(c, w, w.name+"-"+r.suffix(5), n.name, created, created.Add(15*time.Second))
		}
		return
	case "StatefulSet":
		for i := int32(0); i < w.replicas; i++ {
			created := w.created.Add(time.Duration(i) * 40 * time.Second)
			if r.chance(0.4) {
				created = s.base.Add(-time.Duration(r.between(2, 30*24)) * time.Hour)
			}
			s.newPod(c, w, fmt.Sprintf("%s-%d", w.name, i), "", created, created.Add(30*time.Second))
		}
		return
	case "Job":
		s.newPod(c, w, w.name+"-"+r.suffix(5), "", w.created, w.created.Add(3*time.Second))
		return
	}
	rs := w.rs[len(w.rs)-1]
	for i := int32(0); i < w.replicas; i++ {
		created := maxTime(rs.created, w.created).Add(time.Duration(i) * 20 * time.Second)
		if r.chance(0.35) {
			created = s.base.Add(-time.Duration(r.between(1, 20*24)) * time.Hour)
		}
		if w.fault == "unschedulable" || w.fault == "imagepull" || w.fault == "crashloop" {
			created = s.base.Add(-time.Duration(r.between(40, 600)) * time.Minute)
		}
		p := s.newPod(c, w, rs.name+"-"+r.suffix(5), "", created, created.Add(25*time.Second))
		switch w.fault {
		case "crashloop":
			p.restarts = int32(r.between(18, 140))
		case "oom":
			p.restarts = int32(r.between(2, 6))
		default:
			if r.chance(0.08) {
				p.restarts = int32(r.between(1, 4))
			}
		}
	}
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func (s *Sim) pickNode(c *cluster) *node {
	eligible := make([]*node, 0, len(c.nodes))
	for _, n := range c.nodes {
		if !n.controlPlane && n.ready && !n.unschedulable {
			eligible = append(eligible, n)
		}
	}
	if len(eligible) == 0 {
		for _, n := range c.nodes {
			if n.ready && !n.unschedulable {
				eligible = append(eligible, n)
			}
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	return eligible[s.rng.intn(len(eligible))]
}

func (s *Sim) newPod(c *cluster, w *workload, name, nodeName string, created, readyAt time.Time) *pod {
	p := &pod{
		ns: w.ns, name: name, owner: w, created: created, readyAt: readyAt,
		containers: w.containers, fault: w.fault, serviceAccount: w.serviceAccount,
	}
	p.uid = uidFor(c.key, "pod", w.ns, name, created.Format(time.RFC3339Nano))
	p.labels = map[string]string{"app": w.name}
	for _, k := range []string{"app.kubernetes.io/name", "k8s-app", "job-name", "batch.kubernetes.io/job-name"} {
		if v, ok := w.labels[k]; ok {
			p.labels[k] = v
		}
	}
	switch w.kind {
	case "Deployment":
		p.ownerRS = w.rs[len(w.rs)-1]
		p.hash = p.ownerRS.hash
		p.labels["pod-template-hash"] = p.hash
	case "StatefulSet":
		p.labels["statefulset.kubernetes.io/pod-name"] = name
		p.labels["controller-revision-hash"] = w.name + "-" + w.rs[0].hash
	case "DaemonSet":
		p.labels["controller-revision-hash"] = w.rs[0].hash
		p.labels["pod-template-generation"] = "1"
	case "Job":
		p.labels["controller-uid"] = w.uid
		p.labels["batch.kubernetes.io/controller-uid"] = w.uid
	}
	if p.fault != "unschedulable" {
		n := c.nodeByName(nodeName)
		if n == nil {
			n = s.pickNode(c)
		}
		if n != nil {
			p.node = n.name
			n.ipCounter++
			p.ip = fmt.Sprintf("10.%d.%d.%d", c.net, n.idx, 2+(n.ipCounter%250))
		}
	}
	p.rv = c.nextRV()
	c.pods[podKey(p.ns, p.name)] = p
	return p
}

func (s *Sim) addService(c *cluster, ns, name string, selector map[string]string, port, target int32, typ string, created time.Time) *service {
	sv := &service{ns: ns, name: name, uid: uidFor(c.key, "svc", ns, name), created: created, selector: selector,
		port: port, target: target, portName: "http", typ: typ,
		clusterIP: fmt.Sprintf("10.96.%d.%d", s.rng.between(1, 254), s.rng.between(2, 254)),
		labels:    map[string]string{"app": name}}
	if port != 80 {
		sv.portName = fmt.Sprintf("tcp-%d", port)
	}
	sv.rv = c.nextRV()
	c.services = append(c.services, sv)
	return sv
}

func (s *Sim) addHeadless(c *cluster, ns, name, app string, port int32, created time.Time) {
	sv := s.addService(c, ns, name, map[string]string{"app": app}, port, port, "ClusterIP", created)
	sv.headless = true
	sv.clusterIP = "None"
}

func (s *Sim) addVolume(c *cluster, ns, claim, class, size, phase string, created time.Time, labels map[string]string) {
	pvName := "pvc-" + uidFor(c.key, "pv", ns, claim)
	claimUID := uidFor(c.key, "pvc", ns, claim)
	modes := []string{"ReadWriteOnce"}
	if class == "nfs-rwx" {
		modes = []string{"ReadWriteMany"}
	}
	if phase != "Released" {
		spec := M{"accessModes": modes, "storageClassName": class, "volumeMode": "Filesystem",
			"resources": M{"requests": M{"storage": size}}}
		status := M{"phase": phase}
		if phase == "Bound" {
			spec["volumeName"] = pvName
			status["accessModes"] = modes
			status["capacity"] = M{"storage": size}
		}
		c.static["persistentvolumeclaims"] = append(c.static["persistentvolumeclaims"], static{ns: ns, name: claim, obj: M{
			"apiVersion": "v1", "kind": "PersistentVolumeClaim",
			"metadata": meta(claim, ns, claimUID, c.nextRV(), created, labels, map[string]string{
				"volume.kubernetes.io/storage-provisioner": provisionerOf(class)}, nil),
			"spec": spec, "status": status,
		}})
	}
	if phase == "Pending" {
		return
	}
	pvPhase, policy := "Bound", "Delete"
	if phase == "Released" {
		pvPhase, policy = "Released", "Retain"
	}
	c.static["persistentvolumes"] = append(c.static["persistentvolumes"], static{name: pvName, obj: M{
		"apiVersion": "v1", "kind": "PersistentVolume",
		"metadata": meta(pvName, "", uidFor(c.key, "pvobj", pvName), c.nextRV(), created.Add(3*time.Second), nil,
			map[string]string{"pv.kubernetes.io/provisioned-by": provisionerOf(class)}, nil),
		"spec": M{
			"capacity": M{"storage": size}, "accessModes": modes,
			"persistentVolumeReclaimPolicy": policy, "storageClassName": class, "volumeMode": "Filesystem",
			"claimRef": M{"apiVersion": "v1", "kind": "PersistentVolumeClaim", "namespace": ns, "name": claim, "uid": claimUID},
			"csi":      M{"driver": provisionerOf(class), "volumeHandle": "vol-" + hashHex(17, pvName)},
		},
		"status": M{"phase": pvPhase},
	}})
}

func provisionerOf(class string) string {
	switch class {
	case "nfs-rwx":
		return "nfs.csi.k8s.io"
	case "local-path":
		return "rancher.io/local-path"
	}
	return "ebs.csi.aws.com"
}

func (s *Sim) genStorageClasses(c *cluster, spec clusterSpec, born time.Time) {
	for _, sc := range spec.storage {
		ann := map[string]string{}
		if sc.isDefault {
			ann["storageclass.kubernetes.io/is-default-class"] = "true"
		}
		obj := M{
			"apiVersion": "storage.k8s.io/v1", "kind": "StorageClass",
			"metadata":             meta(sc.name, "", uidFor(c.key, "sc", sc.name), c.nextRV(), born.Add(20*time.Minute), nil, ann, nil),
			"provisioner":          sc.provisioner,
			"reclaimPolicy":        "Delete",
			"volumeBindingMode":    sc.binding,
			"allowVolumeExpansion": sc.provisioner != "rancher.io/local-path",
		}
		if len(sc.params) > 0 {
			obj["parameters"] = sc.params
		}
		c.static["storageclasses"] = append(c.static["storageclasses"], static{name: sc.name, obj: obj})
	}
}

func (s *Sim) genServices(c *cluster, spec clusterSpec) {
	born := c.nsCreated("default")
	k := &service{ns: "default", name: "kubernetes", uid: uidFor(c.key, "svc", "default", "kubernetes"), created: born,
		port: 443, target: 6443, portName: "https", typ: "ClusterIP", clusterIP: "10.96.0.1",
		labels: map[string]string{"component": "apiserver", "provider": "kubernetes"}}
	k.rv = c.nextRV()
	dns := &service{ns: "kube-system", name: "kube-dns", uid: uidFor(c.key, "svc", "kube-system", "kube-dns"), created: born,
		selector: map[string]string{"k8s-app": "coredns"}, port: 53, target: 53, portName: "dns", typ: "ClusterIP", clusterIP: "10.96.0.10",
		protocol: "UDP", labels: map[string]string{"k8s-app": "kube-dns", "kubernetes.io/name": "CoreDNS"},
		extraPorts: []servicePort{{name: "dns-tcp", port: 53, target: 53, protocol: "TCP"}, {name: "metrics", port: 9153, target: 9153, protocol: "TCP"}}}
	dns.rv = c.nextRV()
	c.services = append([]*service{k, dns}, c.services...)
	if !c.nsCreated("ingress-nginx").IsZero() {
		sv := s.addService(c, "ingress-nginx", "ingress-nginx-controller", map[string]string{"app": "ingress-nginx-controller"}, 80, 80, "LoadBalancer", c.nsCreated("ingress-nginx"))
		sv.lbIP = spec.lbIP
		sv.extraPorts = []servicePort{{name: "https", port: 443, target: 443, protocol: "TCP"}}
	}
	if spec.key == "edge" {
		sv := s.addService(c, "iot", "mqtt", map[string]string{"app": "mqtt-bridge"}, 1883, 1883, "LoadBalancer", c.nsCreated("iot"))
		sv.lbIP = spec.lbIP
	}
}

func (s *Sim) genIngresses(c *cluster, spec clusterSpec) {
	if c.nsCreated("ingress-nginx").IsZero() {
		return
	}
	for _, sv := range c.services {
		if sv.headless || sv.typ != "ClusterIP" || sv.selector == nil || sv.ns == "logging" {
			continue
		}
		public := sv.name == "storefront" || sv.name == "grafana" || sv.name == "argocd-server" ||
			strings.HasSuffix(sv.name, "gateway") || (strings.HasSuffix(sv.name, "-api") && s.rng.chance(0.3))
		if !public {
			continue
		}
		host := fmt.Sprintf("%s.%s", sv.name, spec.domain)
		secret := sv.name + "-tls"
		created := sv.created.Add(2 * time.Minute)
		obj := M{
			"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
			"metadata": meta(sv.name, sv.ns, uidFor(c.key, "ing", sv.ns, sv.name), c.nextRV(), created, map[string]string{"app": sv.name},
				map[string]string{"cert-manager.io/cluster-issuer": "letsencrypt-prod", "nginx.ingress.kubernetes.io/proxy-body-size": "16m"}, nil),
			"spec": M{
				"ingressClassName": "nginx",
				"tls":              []M{{"hosts": []string{host}, "secretName": secret}},
				"rules": []M{{"host": host, "http": M{"paths": []M{{"path": "/", "pathType": "Prefix",
					"backend": M{"service": M{"name": sv.name, "port": M{"number": sv.port}}}}}}}},
			},
			"status": M{"loadBalancer": M{"ingress": []M{{"ip": spec.lbIP}}}},
		}
		c.static["ingresses"] = append(c.static["ingresses"], static{ns: sv.ns, name: sv.name, obj: obj})
		s.addSecret(c, sv.ns, secret, "kubernetes.io/tls", []string{"tls.crt", "tls.key"}, created,
			map[string]string{"cert-manager.io/certificate-name": sv.name})
	}
}

func (s *Sim) addSecret(c *cluster, ns, name, typ string, keys []string, created time.Time, ann map[string]string) {
	data := M{}
	for _, k := range keys {
		data[k] = ""
	}
	c.static["secrets"] = append(c.static["secrets"], static{ns: ns, name: name, obj: M{
		"apiVersion": "v1", "kind": "Secret", "type": typ, "data": data,
		"metadata": meta(name, ns, uidFor(c.key, "secret", ns, name), c.nextRV(), created, nil, ann, nil),
	}})
}

func (s *Sim) addConfigMap(c *cluster, ns, name string, data map[string]string, created time.Time) {
	c.static["configmaps"] = append(c.static["configmaps"], static{ns: ns, name: name, obj: M{
		"apiVersion": "v1", "kind": "ConfigMap", "data": data,
		"metadata": meta(name, ns, uidFor(c.key, "cm", ns, name), c.nextRV(), created, nil, nil, nil),
	}})
}

func (s *Sim) addServiceAccount(c *cluster, ns, name string, created time.Time) {
	c.static["serviceaccounts"] = append(c.static["serviceaccounts"], static{ns: ns, name: name, obj: M{
		"apiVersion": "v1", "kind": "ServiceAccount",
		"metadata": meta(name, ns, uidFor(c.key, "sa", ns, name), c.nextRV(), created, nil, nil, nil),
	}})
}

func (s *Sim) genConfig(c *cluster, spec clusterSpec) {
	for _, ns := range c.namespaces {
		s.addConfigMap(c, ns.name, "kube-root-ca.crt", map[string]string{
			"ca.crt": "-----BEGIN CERTIFICATE-----\nDEMO-MODE-PLACEHOLDER-NOT-A-CERTIFICATE-" + hashHex(24, c.key) + "\n-----END CERTIFICATE-----\n",
		}, ns.created)
		s.addServiceAccount(c, ns.name, "default", ns.created)
		if isAppNS(spec, ns.name) {
			s.addSecret(c, ns.name, "regcred", "kubernetes.io/dockerconfigjson", []string{".dockerconfigjson"}, ns.created.Add(time.Minute), nil)
		}
	}
	sys := c.nsCreated("kube-system")
	s.addConfigMap(c, "kube-system", "coredns", map[string]string{"Corefile": ".:53 {\n    errors\n    health\n    kubernetes cluster.local in-addr.arpa ip6.arpa\n    forward . /etc/resolv.conf\n    cache 30\n}\n"}, sys)
	s.addConfigMap(c, "kube-system", "kubeadm-config", map[string]string{"ClusterConfiguration": "kubernetesVersion: " + spec.version + "\n"}, sys)
	for _, ns := range []string{"monitoring", "ingress-nginx", "cert-manager", "argocd"} {
		if created := c.nsCreated(ns); !created.IsZero() {
			s.addSecret(c, ns, fmt.Sprintf("sh.helm.release.v1.%s.v%d", ns, s.rng.between(2, 14)), "helm.sh/release.v1", []string{"release"}, created.Add(time.Minute), nil)
		}
	}
	for _, w := range c.workloads {
		if w.kind == "Job" {
			continue
		}
		if w.serviceAccount != "" {
			s.addServiceAccount(c, w.ns, w.serviceAccount, w.created)
		}
		if w.kind == "Deployment" && s.rng.chance(0.4) {
			s.addConfigMap(c, w.ns, w.name+"-config", map[string]string{
				"LOG_LEVEL":     pick(s.rng, []string{"info", "info", "debug", "warn"}),
				"HTTP_PORT":     "8080",
				"FEATURE_FLAGS": pick(s.rng, []string{"new-checkout,fast-path", "", "beta-ui", "async-writes"}),
			}, w.created)
		}
		if w.kind != "DaemonSet" && s.rng.chance(0.3) {
			s.addSecret(c, w.ns, w.name+"-credentials", "Opaque", []string{"username", "password"}, w.created, nil)
		}
	}
}

func (s *Sim) genPolicy(c *cluster, spec clusterSpec) {
	for _, a := range spec.apps {
		created := c.nsCreated(a.name).Add(time.Minute)
		pols := []static{
			{ns: a.name, name: "default-deny-ingress", obj: M{"spec": M{"podSelector": M{}, "policyTypes": []string{"Ingress"}}}},
			{ns: a.name, name: "allow-ingress-nginx", obj: M{"spec": M{"podSelector": M{}, "policyTypes": []string{"Ingress"},
				"ingress": []M{{"from": []M{{"namespaceSelector": M{"matchLabels": M{"kubernetes.io/metadata.name": "ingress-nginx"}}}}}}}}},
			{ns: a.name, name: "allow-same-namespace", obj: M{"spec": M{"podSelector": M{}, "policyTypes": []string{"Ingress"},
				"ingress": []M{{"from": []M{{"podSelector": M{}}}}}}}},
		}
		if a.name == "payments" {
			pols = append(pols, static{ns: a.name, name: "allow-psp-egress", obj: M{"spec": M{
				"podSelector": M{"matchLabels": M{"app": "gateway"}}, "policyTypes": []string{"Egress"},
				"egress": []M{{"to": []M{{"ipBlock": M{"cidr": "0.0.0.0/0", "except": []string{"10.0.0.0/8"}}}},
					"ports": []M{{"protocol": "TCP", "port": 443}}}}}}})
		}
		for _, p := range pols {
			p.obj["apiVersion"] = "networking.k8s.io/v1"
			p.obj["kind"] = "NetworkPolicy"
			p.obj["metadata"] = meta(p.name, p.ns, uidFor(c.key, "np", p.ns, p.name), c.nextRV(), created, nil, nil, nil)
			c.static["networkpolicies"] = append(c.static["networkpolicies"], p)
		}
		c.static["limitranges"] = append(c.static["limitranges"], static{ns: a.name, name: "default-limits", obj: M{
			"apiVersion": "v1", "kind": "LimitRange",
			"metadata": meta("default-limits", a.name, uidFor(c.key, "lr", a.name), c.nextRV(), created, nil, nil, nil),
			"spec": M{"limits": []M{{"type": "Container",
				"default":        M{"cpu": "500m", "memory": "512Mi"},
				"defaultRequest": M{"cpu": "100m", "memory": "128Mi"},
				"max":            M{"cpu": "8", "memory": "48Gi"}}}},
		}})
		c.quotas = append(c.quotas, &quota{ns: a.name, uid: uidFor(c.key, "quota", a.name), created: created, rv: c.nextRV()})
	}
	for _, w := range c.workloads {
		if w.kind != "Deployment" || w.replicas < 2 || w.fault != "" || !isAppNS(spec, w.ns) || !s.rng.chance(0.25) {
			continue
		}
		h := &hpa{ns: w.ns, name: w.name, uid: uidFor(c.key, "hpa", w.ns, w.name), created: w.created.Add(time.Hour),
			target: w.name, min: w.replicas, max: w.replicas * 3, util: s.rng.between(25, 88)}
		h.rv = c.nextRV()
		c.hpas = append(c.hpas, h)
	}
}

func isAppNS(spec clusterSpec, ns string) bool {
	for _, a := range spec.apps {
		if a.name == ns {
			return true
		}
	}
	return false
}
