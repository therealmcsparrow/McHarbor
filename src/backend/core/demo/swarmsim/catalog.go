// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package swarmsim

type flavour struct {
	key           string
	title         string
	region        string
	prefix        string
	nodeWeight    int
	serviceWeight int
	cpus          int
	memGB         int
	workerArch    string
	autolock      bool
	catalog       []stackTpl
}

type svcTpl struct {
	name       string
	image      string
	profile    string
	replicas   int
	global     bool
	port       uint32
	publish    uint32
	constraint string
	maxPerNode uint64
	cpu        float64
	memMB      int64
	volume     string
	args       []string
	env        []string
	secrets    []string
	configs    []string
	public     bool
	startFirst bool
}

type stackTpl struct {
	name     string
	services []svcTpl
	secrets  []string
	configs  map[string]string
}

var flavours = []flavour{
	{key: "production", title: "Production", region: "eu-west", prefix: "prod", nodeWeight: 6, serviceWeight: 3, cpus: 16, memGB: 64, workerArch: "x86_64", autolock: true, catalog: productionStacks},
	{key: "edge", title: "Edge", region: "eu-central", prefix: "edge", nodeWeight: 3, serviceWeight: 1, cpus: 4, memGB: 8, workerArch: "aarch64", catalog: edgeStacks},
	{key: "staging", title: "Staging", region: "eu-west", prefix: "stg", nodeWeight: 3, serviceWeight: 1, cpus: 8, memGB: 32, workerArch: "x86_64", catalog: productionStacks},
	{key: "analytics", title: "Analytics", region: "us-east", prefix: "ana", nodeWeight: 4, serviceWeight: 1, cpus: 32, memGB: 128, workerArch: "x86_64", catalog: productionStacks},
	{key: "lab", title: "Lab", region: "homelab", prefix: "lab", nodeWeight: 2, serviceWeight: 1, cpus: 4, memGB: 16, workerArch: "aarch64", catalog: edgeStacks},
}

func flavourAt(i int) flavour {
	f := flavours[i%len(flavours)]
	if round := i / len(flavours); round > 0 {
		f.key += "-" + itoa(round+1)
		f.title += " " + itoa(round+1)
		f.prefix += itoa(round + 1)
	}
	return f
}

const (
	onManager = "node.role==manager"
	onWorker  = "node.role==worker"
)

var productionStacks = []stackTpl{
	{name: "traefik", secrets: []string{"acme_dns_token"}, configs: map[string]string{"dynamic.yml": traefikDynamic}, services: []svcTpl{
		{name: "traefik", image: "traefik:v3.1.5", profile: "proxy", global: true, port: 443, publish: 443, constraint: onManager, cpu: 1, memMB: 256, args: []string{"--providers.swarm.endpoint=unix:///var/run/docker.sock", "--entrypoints.websecure.address=:443"}, secrets: []string{"acme_dns_token"}, configs: []string{"dynamic.yml"}, public: true},
	}},
	{name: "monitoring", secrets: []string{"grafana_admin_password"}, configs: map[string]string{"prometheus.yml": prometheusYML, "alertmanager.yml": alertmanagerYML, "promtail.yml": promtailYML}, services: []svcTpl{
		{name: "prometheus", image: "prom/prometheus:v2.54.1", profile: "prometheus", replicas: 1, port: 9090, constraint: onWorker, cpu: 2, memMB: 4096, volume: "/prometheus", configs: []string{"prometheus.yml"}},
		{name: "grafana", image: "grafana/grafana:11.2.2", profile: "web", replicas: 1, port: 3000, cpu: 1, memMB: 512, volume: "/var/lib/grafana", secrets: []string{"grafana_admin_password"}, env: []string{"GF_SECURITY_ADMIN_PASSWORD__FILE=/run/secrets/grafana_admin_password"}, public: true},
		{name: "alertmanager", image: "prom/alertmanager:v0.27.0", profile: "api", replicas: 2, port: 9093, cpu: 0.25, memMB: 128, configs: []string{"alertmanager.yml"}},
		{name: "node-exporter", image: "prom/node-exporter:v1.8.2", profile: "agent", global: true, port: 9100, cpu: 0.2, memMB: 64},
		{name: "cadvisor", image: "gcr.io/cadvisor/cadvisor:v0.49.1", profile: "agent", global: true, port: 8080, cpu: 0.5, memMB: 256},
		{name: "loki", image: "grafana/loki:3.2.0", profile: "api", replicas: 1, port: 3100, constraint: onWorker, cpu: 1, memMB: 1024, volume: "/loki"},
		{name: "promtail", image: "grafana/promtail:3.2.0", profile: "agent", global: true, cpu: 0.2, memMB: 128, configs: []string{"promtail.yml"}},
	}},
	{name: "shop", secrets: []string{"db_password", "stripe_api_key"}, configs: map[string]string{"api.yaml": shopAPIConfig}, services: []svcTpl{
		{name: "storefront", image: "ghcr.io/acme/storefront:4.12.3", profile: "web", replicas: 3, port: 3000, cpu: 0.5, memMB: 512, env: []string{"NODE_ENV=production", "API_URL=http://api:8080"}, public: true, startFirst: true},
		{name: "api", image: "ghcr.io/acme/shop-api:2.31.0", profile: "api", replicas: 3, port: 8080, cpu: 1, memMB: 768, args: []string{"serve", "--config", "/etc/shop/api.yaml"}, secrets: []string{"db_password", "stripe_api_key"}, configs: []string{"api.yaml"}, startFirst: true},
		{name: "worker", image: "ghcr.io/acme/shop-api:2.31.0", profile: "worker", replicas: 2, cpu: 1, memMB: 512, args: []string{"worker", "--queues", "orders,email,webhooks"}, secrets: []string{"db_password"}},
		{name: "db", image: "postgres:16.4", profile: "postgres", replicas: 1, port: 5432, constraint: onWorker, cpu: 2, memMB: 4096, volume: "/var/lib/postgresql/data", secrets: []string{"db_password"}, env: []string{"POSTGRES_PASSWORD_FILE=/run/secrets/db_password", "POSTGRES_DB=shop"}},
		{name: "cache", image: "redis:7.4-alpine", profile: "redis", replicas: 1, port: 6379, cpu: 0.5, memMB: 512},
	}},
	{name: "payments", secrets: []string{"db_password", "hsm_pin"}, services: []svcTpl{
		{name: "gateway", image: "ghcr.io/acme/payments-gateway:1.8.4", profile: "api", replicas: 2, port: 8443, cpu: 1, memMB: 512, secrets: []string{"hsm_pin"}, public: true, startFirst: true},
		{name: "ledger", image: "ghcr.io/acme/ledger:3.2.1", profile: "api", replicas: 2, port: 9000, cpu: 1, memMB: 768, secrets: []string{"db_password"}},
		{name: "reconciler", image: "ghcr.io/acme/ledger:3.2.1", profile: "worker", replicas: 1, cpu: 0.5, memMB: 256, args: []string{"reconcile", "--interval", "5m"}, secrets: []string{"db_password"}},
		{name: "db", image: "postgres:16.4", profile: "postgres", replicas: 1, port: 5432, constraint: onWorker, cpu: 2, memMB: 2048, volume: "/var/lib/postgresql/data", secrets: []string{"db_password"}, env: []string{"POSTGRES_PASSWORD_FILE=/run/secrets/db_password", "POSTGRES_DB=ledger"}},
	}},
	{name: "auth", secrets: []string{"keycloak_admin_password", "db_password", "oauth2_cookie_secret"}, services: []svcTpl{
		{name: "keycloak", image: "quay.io/keycloak/keycloak:25.0.6", profile: "java", replicas: 2, port: 8080, cpu: 1, memMB: 1536, args: []string{"start", "--optimized"}, secrets: []string{"keycloak_admin_password", "db_password"}, public: true},
		{name: "db", image: "postgres:16.4", profile: "postgres", replicas: 1, port: 5432, constraint: onWorker, cpu: 1, memMB: 1024, volume: "/var/lib/postgresql/data", secrets: []string{"db_password"}, env: []string{"POSTGRES_PASSWORD_FILE=/run/secrets/db_password", "POSTGRES_DB=keycloak"}},
		{name: "oauth2-proxy", image: "quay.io/oauth2-proxy/oauth2-proxy:v7.7.1", profile: "proxy", replicas: 2, port: 4180, cpu: 0.25, memMB: 64, secrets: []string{"oauth2_cookie_secret"}},
	}},
	{name: "search", services: []svcTpl{
		{name: "elasticsearch", image: "docker.elastic.co/elasticsearch/elasticsearch:8.15.2", profile: "java", replicas: 3, port: 9200, constraint: onWorker, maxPerNode: 1, cpu: 4, memMB: 8192, volume: "/usr/share/elasticsearch/data", env: []string{"cluster.name=acme-search", "discovery.seed_hosts=tasks.elasticsearch"}},
		{name: "kibana", image: "docker.elastic.co/kibana/kibana:8.15.2", profile: "web", replicas: 1, port: 5601, cpu: 1, memMB: 1024, public: true},
	}},
	{name: "messaging", services: []svcTpl{
		{name: "rabbitmq", image: "rabbitmq:3.13-management", profile: "erlang", replicas: 1, port: 5672, constraint: onWorker, cpu: 1, memMB: 1024, volume: "/var/lib/rabbitmq"},
		{name: "kafka", image: "bitnami/kafka:3.8.0", profile: "java", replicas: 3, port: 9092, cpu: 2, memMB: 3072, volume: "/bitnami/kafka"},
		{name: "schema-registry", image: "confluentinc/cp-schema-registry:7.7.1", profile: "java", replicas: 1, port: 8081, cpu: 0.5, memMB: 768},
	}},
	{name: "analytics", services: []svcTpl{
		{name: "clickhouse", image: "clickhouse/clickhouse-server:24.8", profile: "db", replicas: 1, port: 8123, constraint: onWorker, cpu: 4, memMB: 8192, volume: "/var/lib/clickhouse"},
		{name: "metabase", image: "metabase/metabase:v0.50.28", profile: "java", replicas: 1, port: 3000, cpu: 1, memMB: 2048, public: true},
		{name: "etl", image: "ghcr.io/acme/etl:0.19.2", profile: "python", replicas: 2, cpu: 1, memMB: 1024, args: []string{"python", "-m", "etl.run", "--schedule", "hourly"}},
	}},
	{name: "cms", secrets: []string{"db_password"}, services: []svcTpl{
		{name: "wordpress", image: "wordpress:6.6.2-php8.3-apache", profile: "php", replicas: 2, port: 80, cpu: 0.5, memMB: 512, secrets: []string{"db_password"}, env: []string{"WORDPRESS_DB_HOST=mariadb", "WORDPRESS_DB_PASSWORD_FILE=/run/secrets/db_password"}, public: true},
		{name: "mariadb", image: "mariadb:11.4", profile: "mysql", replicas: 1, port: 3306, constraint: onWorker, cpu: 1, memMB: 1024, volume: "/var/lib/mysql", secrets: []string{"db_password"}},
		{name: "memcached", image: "memcached:1.6.31-alpine", profile: "redis", replicas: 1, port: 11211, cpu: 0.25, memMB: 256},
	}},
	{name: "notifications", services: []svcTpl{
		{name: "api", image: "ghcr.io/acme/notify-api:1.4.0", profile: "node", replicas: 2, port: 8080, cpu: 0.5, memMB: 256, public: true},
		{name: "worker", image: "ghcr.io/acme/notify-api:1.4.0", profile: "worker", replicas: 2, cpu: 0.5, memMB: 256, args: []string{"worker"}},
		{name: "redis", image: "redis:7.4-alpine", profile: "redis", replicas: 1, port: 6379, cpu: 0.25, memMB: 256},
	}},
	{name: "ci", secrets: []string{"drone_rpc_secret"}, services: []svcTpl{
		{name: "gitea", image: "gitea/gitea:1.22.2", profile: "web", replicas: 1, port: 3000, constraint: onWorker, cpu: 1, memMB: 512, volume: "/data", public: true},
		{name: "drone-server", image: "drone/drone:2.24.0", profile: "ci", replicas: 1, port: 80, cpu: 0.5, memMB: 256, secrets: []string{"drone_rpc_secret"}, public: true},
		{name: "drone-runner", image: "drone/drone-runner-docker:1.8.3", profile: "ci", replicas: 2, cpu: 2, memMB: 2048, secrets: []string{"drone_rpc_secret"}},
	}},
	{name: "registry", services: []svcTpl{
		{name: "registry", image: "registry:2.8.3", profile: "api", replicas: 1, port: 5000, constraint: onWorker, cpu: 0.5, memMB: 256, volume: "/var/lib/registry", public: true},
		{name: "ui", image: "joxit/docker-registry-ui:2.5.7", profile: "nginx", replicas: 1, port: 80, cpu: 0.1, memMB: 64},
	}},
	{name: "status", services: []svcTpl{
		{name: "uptime-kuma", image: "louislam/uptime-kuma:1.23.14", profile: "node", replicas: 1, port: 3001, cpu: 0.5, memMB: 256, volume: "/app/data", public: true},
	}},
	{name: "docs", services: []svcTpl{
		{name: "site", image: "ghcr.io/acme/docs:2026.09.3", profile: "nginx", replicas: 2, port: 80, cpu: 0.1, memMB: 64, public: true, startFirst: true},
	}},
	{name: "backup", secrets: []string{"restic_password"}, services: []svcTpl{
		{name: "restic", image: "mazzolino/restic:1.7.2", profile: "worker", replicas: 1, constraint: onManager, cpu: 0.5, memMB: 512, secrets: []string{"restic_password"}, env: []string{"BACKUP_CRON=0 30 3 * * *", "RESTIC_REPOSITORY=s3:https://s3.eu-west.acme.example/backups"}},
	}},
	{name: "", services: []svcTpl{
		{name: "whoami", image: "traefik/whoami:v1.10.3", profile: "web", replicas: 2, port: 80, cpu: 0.1, memMB: 32, public: true},
		{name: "netshoot", image: "nicolaka/netshoot:v0.13", profile: "agent", replicas: 1, cpu: 0.1, memMB: 64, args: []string{"sleep", "infinity"}},
		{name: "swarm-cronjob", image: "crazymax/swarm-cronjob:1.13.0", profile: "agent", replicas: 1, constraint: onManager, cpu: 0.1, memMB: 64},
	}},
}

var edgeStacks = []stackTpl{
	{name: "traefik", services: []svcTpl{
		{name: "traefik", image: "traefik:v3.1.5", profile: "proxy", global: true, port: 443, publish: 443, cpu: 0.5, memMB: 128, public: true},
	}},
	{name: "telemetry", configs: map[string]string{"telegraf.conf": telegrafConf}, services: []svcTpl{
		{name: "node-exporter", image: "prom/node-exporter:v1.8.2", profile: "agent", global: true, port: 9100, cpu: 0.1, memMB: 32},
		{name: "promtail", image: "grafana/promtail:3.2.0", profile: "agent", global: true, cpu: 0.1, memMB: 64},
		{name: "telegraf", image: "telegraf:1.32-alpine", profile: "agent", global: true, cpu: 0.2, memMB: 64, configs: []string{"telegraf.conf"}},
	}},
	{name: "iot", secrets: []string{"mqtt_password", "influx_token"}, services: []svcTpl{
		{name: "mosquitto", image: "eclipse-mosquitto:2.0.18", profile: "mqtt", replicas: 1, port: 1883, publish: 1883, constraint: onManager, cpu: 0.25, memMB: 64, volume: "/mosquitto/data", secrets: []string{"mqtt_password"}},
		{name: "mqtt-bridge", image: "ghcr.io/acme/mqtt-bridge:0.9.4", profile: "mqtt", replicas: 3, constraint: onWorker, maxPerNode: 1, cpu: 0.25, memMB: 64, secrets: []string{"mqtt_password"}},
		{name: "sensor-ingest", image: "ghcr.io/acme/sensor-ingest:1.3.2", profile: "worker", replicas: 2, cpu: 0.5, memMB: 128, secrets: []string{"influx_token"}},
		{name: "influxdb", image: "influxdb:2.7-alpine", profile: "db", replicas: 1, port: 8086, constraint: onManager, cpu: 1, memMB: 1024, volume: "/var/lib/influxdb2"},
		{name: "grafana", image: "grafana/grafana:11.2.2", profile: "web", replicas: 1, port: 3000, cpu: 0.5, memMB: 256, public: true},
	}},
	{name: "cdn", services: []svcTpl{
		{name: "varnish", image: "varnish:7.6", profile: "nginx", global: true, port: 80, constraint: onWorker, cpu: 0.5, memMB: 512},
		{name: "origin-cache", image: "nginx:1.27-alpine", profile: "nginx", replicas: 2, port: 8080, cpu: 0.25, memMB: 64, startFirst: true},
	}},
	{name: "dns", services: []svcTpl{
		{name: "coredns", image: "coredns/coredns:1.11.3", profile: "dns", global: true, port: 53, cpu: 0.1, memMB: 64},
	}},
	{name: "vpn", secrets: []string{"wg_private_key"}, services: []svcTpl{
		{name: "wireguard", image: "linuxserver/wireguard:1.0.20210914", profile: "agent", replicas: 1, port: 51820, publish: 51820, constraint: onManager, cpu: 0.1, memMB: 64, secrets: []string{"wg_private_key"}},
	}},
	{name: "", services: []svcTpl{
		{name: "watchtower", image: "containrrr/watchtower:1.7.1", profile: "agent", replicas: 1, constraint: onManager, cpu: 0.1, memMB: 32},
		{name: "whoami", image: "traefik/whoami:v1.10.3", profile: "web", replicas: 1, port: 80, cpu: 0.1, memMB: 32, public: true},
	}},
}

var standaloneSecrets = []string{"registry_credentials", "tls_wildcard_acme_example"}

const traefikDynamic = `tls:
  options:
    default:
      minVersion: VersionTLS12
http:
  middlewares:
    secure-headers:
      headers:
        stsSeconds: 31536000
        frameDeny: true
`

const prometheusYML = `global:
  scrape_interval: 15s
  evaluation_interval: 30s
scrape_configs:
  - job_name: node
    dns_sd_configs:
      - names: [tasks.node-exporter]
        type: A
        port: 9100
  - job_name: cadvisor
    dns_sd_configs:
      - names: [tasks.cadvisor]
        type: A
        port: 8080
`

const alertmanagerYML = `route:
  receiver: ops
  group_by: [alertname, service]
receivers:
  - name: ops
    webhook_configs:
      - url: http://notifications_api:8080/hooks/alertmanager
`

const promtailYML = `server:
  http_listen_port: 9080
clients:
  - url: http://loki:3100/loki/api/v1/push
scrape_configs:
  - job_name: docker
    docker_sd_configs:
      - host: unix:///var/run/docker.sock
`

const shopAPIConfig = `listen: :8080
database:
  host: db
  name: shop
  password_file: /run/secrets/db_password
cache:
  url: redis://cache:6379/0
payments:
  provider: stripe
  key_file: /run/secrets/stripe_api_key
`

const telegrafConf = `[agent]
  interval = "10s"
[[inputs.mqtt_consumer]]
  servers = ["tcp://mosquitto:1883"]
  topics = ["sensors/#"]
[[outputs.influxdb_v2]]
  urls = ["http://influxdb:8086"]
  organization = "acme"
  bucket = "edge"
`
