// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package estate

type serviceTemplate struct {
	name     string
	image    string
	profile  string
	port     int
	publish  bool
	mount    string
	command  string
	cpu      float64
	memMB    int
	health   bool
	replicas int
	env      []string
}

type stackTemplate struct {
	name     string
	services []serviceTemplate
}

var stackTemplates = map[string]stackTemplate{
	"shop": {name: "shop", services: []serviceTemplate{
		{name: "storefront", image: "ghcr.io/acme/storefront:4.12.3", profile: "web", port: 3000, cpu: 6, memMB: 420, health: true, replicas: 3, env: []string{"NODE_ENV=production", "API_URL=http://api:8080"}},
		{name: "api", image: "ghcr.io/acme/shop-api:2.31.0", profile: "api", port: 8080, cpu: 9, memMB: 610, health: true, replicas: 3, command: "/app/shop-api serve --config /etc/shop/config.yaml"},
		{name: "worker", image: "ghcr.io/acme/shop-api:2.31.0", profile: "worker", cpu: 12, memMB: 380, replicas: 2, command: "/app/shop-api worker --queues orders,email,webhooks"},
		{name: "db", image: "postgres:16.4", profile: "postgres", port: 5432, mount: "/var/lib/postgresql/data", cpu: 14, memMB: 2200, health: true},
		{name: "cache", image: "redis:7.4-alpine", profile: "redis", port: 6379, mount: "/data", cpu: 2, memMB: 180, health: true},
	}},
	"payments": {name: "payments", services: []serviceTemplate{
		{name: "gateway", image: "ghcr.io/acme/payments-gateway:1.8.4", profile: "api", port: 8443, cpu: 7, memMB: 350, health: true, replicas: 2},
		{name: "ledger", image: "ghcr.io/acme/ledger:3.2.1", profile: "api", port: 9000, cpu: 5, memMB: 520, health: true, replicas: 2},
		{name: "reconciler", image: "ghcr.io/acme/ledger:3.2.1", profile: "worker", cpu: 4, memMB: 260, command: "/ledger reconcile --interval 5m"},
		{name: "db", image: "postgres:16.4", profile: "postgres", port: 5432, mount: "/var/lib/postgresql/data", cpu: 11, memMB: 1800, health: true},
	}},
	"auth": {name: "auth", services: []serviceTemplate{
		{name: "keycloak", image: "quay.io/keycloak/keycloak:25.0.6", profile: "java", port: 8080, publish: true, cpu: 5, memMB: 900, health: true, command: "start --optimized"},
		{name: "db", image: "postgres:16.4", profile: "postgres", port: 5432, mount: "/var/lib/postgresql/data", cpu: 3, memMB: 420, health: true},
		{name: "oauth2-proxy", image: "quay.io/oauth2-proxy/oauth2-proxy:v7.7.1", profile: "proxy", port: 4180, cpu: 1, memMB: 40},
	}},
	"observability": {name: "observability", services: []serviceTemplate{
		{name: "prometheus", image: "prom/prometheus:v2.54.1", profile: "prometheus", port: 9090, publish: true, mount: "/prometheus", cpu: 18, memMB: 3100, health: true},
		{name: "grafana", image: "grafana/grafana:11.2.2", profile: "web", port: 3000, publish: true, mount: "/var/lib/grafana", cpu: 3, memMB: 260, health: true},
		{name: "loki", image: "grafana/loki:3.2.0", profile: "api", port: 3100, mount: "/loki", cpu: 8, memMB: 900},
		{name: "promtail", image: "grafana/promtail:3.2.0", profile: "agent", cpu: 2, memMB: 90},
		{name: "alertmanager", image: "prom/alertmanager:v0.27.0", profile: "api", port: 9093, cpu: 1, memMB: 60},
		{name: "node-exporter", image: "prom/node-exporter:v1.8.2", profile: "agent", port: 9100, cpu: 1, memMB: 25},
		{name: "cadvisor", image: "gcr.io/cadvisor/cadvisor:v0.49.1", profile: "agent", port: 8080, cpu: 4, memMB: 140},
	}},
	"messaging": {name: "messaging", services: []serviceTemplate{
		{name: "rabbitmq", image: "rabbitmq:3.13-management", profile: "erlang", port: 5672, mount: "/var/lib/rabbitmq", cpu: 6, memMB: 520, health: true},
		{name: "kafka", image: "bitnami/kafka:3.8.0", profile: "java", port: 9092, mount: "/bitnami/kafka", cpu: 12, memMB: 2400, health: true, replicas: 3},
		{name: "schema-registry", image: "confluentinc/cp-schema-registry:7.7.1", profile: "java", port: 8081, cpu: 2, memMB: 480},
	}},
	"search": {name: "search", services: []serviceTemplate{
		{name: "elasticsearch", image: "docker.elastic.co/elasticsearch/elasticsearch:8.15.2", profile: "java", port: 9200, mount: "/usr/share/elasticsearch/data", cpu: 16, memMB: 4200, health: true, replicas: 3},
		{name: "kibana", image: "docker.elastic.co/kibana/kibana:8.15.2", profile: "web", port: 5601, publish: true, cpu: 3, memMB: 700},
	}},
	"gateway": {name: "gateway", services: []serviceTemplate{
		{name: "traefik", image: "traefik:v3.1.5", profile: "proxy", port: 443, publish: true, mount: "/letsencrypt", cpu: 4, memMB: 110, health: true, command: "--providers.docker --entrypoints.websecure.address=:443"},
		{name: "crowdsec", image: "crowdsecurity/crowdsec:v1.6.3", profile: "agent", cpu: 2, memMB: 150},
	}},
	"storage": {name: "storage", services: []serviceTemplate{
		{name: "minio", image: "minio/minio:RELEASE.2024-09-22T00-33-43Z", profile: "api", port: 9000, publish: true, mount: "/data", cpu: 5, memMB: 800, health: true, command: "server /data --console-address :9001", replicas: 4},
	}},
	"analytics": {name: "analytics", services: []serviceTemplate{
		{name: "clickhouse", image: "clickhouse/clickhouse-server:24.8", profile: "db", port: 8123, mount: "/var/lib/clickhouse", cpu: 22, memMB: 6100, health: true},
		{name: "metabase", image: "metabase/metabase:v0.50.28", profile: "java", port: 3000, publish: true, cpu: 4, memMB: 1300},
		{name: "etl", image: "ghcr.io/acme/etl:0.19.2", profile: "worker", cpu: 9, memMB: 700, command: "python -m etl.run --schedule hourly", replicas: 2},
	}},
	"airflow": {name: "airflow", services: []serviceTemplate{
		{name: "webserver", image: "apache/airflow:2.10.2", profile: "python", port: 8080, publish: true, cpu: 4, memMB: 1100, health: true, command: "webserver"},
		{name: "scheduler", image: "apache/airflow:2.10.2", profile: "python", cpu: 8, memMB: 900, command: "scheduler"},
		{name: "worker", image: "apache/airflow:2.10.2", profile: "worker", cpu: 14, memMB: 1500, command: "celery worker", replicas: 3},
		{name: "db", image: "postgres:16.4", profile: "postgres", port: 5432, mount: "/var/lib/postgresql/data", cpu: 4, memMB: 600, health: true},
		{name: "redis", image: "redis:7.4-alpine", profile: "redis", port: 6379, cpu: 1, memMB: 70},
	}},
	"cms": {name: "cms", services: []serviceTemplate{
		{name: "wordpress", image: "wordpress:6.6.2-php8.3-apache", profile: "php", port: 80, publish: true, mount: "/var/www/html", cpu: 3, memMB: 310, replicas: 2},
		{name: "mariadb", image: "mariadb:11.4", profile: "mysql", port: 3306, mount: "/var/lib/mysql", cpu: 4, memMB: 640, health: true},
	}},
	"nextcloud": {name: "nextcloud", services: []serviceTemplate{
		{name: "app", image: "nextcloud:29.0.7-apache", profile: "php", port: 80, publish: true, mount: "/var/www/html", cpu: 6, memMB: 700, health: true},
		{name: "cron", image: "nextcloud:29.0.7-apache", profile: "worker", cpu: 1, memMB: 120, command: "/cron.sh"},
		{name: "db", image: "mariadb:11.4", profile: "mysql", port: 3306, mount: "/var/lib/mysql", cpu: 3, memMB: 520, health: true},
		{name: "redis", image: "redis:7.4-alpine", profile: "redis", port: 6379, cpu: 1, memMB: 45},
	}},
	"git": {name: "git", services: []serviceTemplate{
		{name: "gitea", image: "gitea/gitea:1.22.2", profile: "api", port: 3000, publish: true, mount: "/data", cpu: 3, memMB: 280, health: true},
		{name: "db", image: "postgres:16.4", profile: "postgres", port: 5432, mount: "/var/lib/postgresql/data", cpu: 2, memMB: 240, health: true},
	}},
	"ci": {name: "ci", services: []serviceTemplate{
		{name: "woodpecker-server", image: "woodpeckerci/woodpecker-server:v2.7.1", profile: "api", port: 8000, publish: true, cpu: 2, memMB: 120},
		{name: "woodpecker-agent", image: "woodpeckerci/woodpecker-agent:v2.7.1", profile: "ci", cpu: 35, memMB: 1100, replicas: 8},
		{name: "registry", image: "registry:2.8.3", profile: "api", port: 5000, publish: true, mount: "/var/lib/registry", cpu: 2, memMB: 90},
		{name: "buildkitd", image: "moby/buildkit:v0.16.0", profile: "ci", cpu: 40, memMB: 2600, replicas: 2},
	}},
	"media": {name: "media", services: []serviceTemplate{
		{name: "jellyfin", image: "jellyfin/jellyfin:10.9.11", profile: "media", port: 8096, publish: true, mount: "/config", cpu: 18, memMB: 1400, health: true},
		{name: "sonarr", image: "lscr.io/linuxserver/sonarr:4.0.9", profile: "arr", port: 8989, publish: true, mount: "/config", cpu: 2, memMB: 260},
		{name: "radarr", image: "lscr.io/linuxserver/radarr:5.11.0", profile: "arr", port: 7878, publish: true, mount: "/config", cpu: 2, memMB: 280},
		{name: "prowlarr", image: "lscr.io/linuxserver/prowlarr:1.24.3", profile: "arr", port: 9696, mount: "/config", cpu: 1, memMB: 150},
		{name: "bazarr", image: "lscr.io/linuxserver/bazarr:1.4.4", profile: "arr", port: 6767, mount: "/config", cpu: 1, memMB: 190},
		{name: "qbittorrent", image: "lscr.io/linuxserver/qbittorrent:4.6.7", profile: "torrent", port: 8080, publish: true, mount: "/config", cpu: 7, memMB: 520},
	}},
	"smarthome": {name: "smarthome", services: []serviceTemplate{
		{name: "homeassistant", image: "ghcr.io/home-assistant/home-assistant:2024.9.3", profile: "python", port: 8123, publish: true, mount: "/config", cpu: 5, memMB: 640, health: true},
		{name: "mosquitto", image: "eclipse-mosquitto:2.0.18", profile: "mqtt", port: 1883, publish: true, mount: "/mosquitto/data", cpu: 1, memMB: 12},
		{name: "zigbee2mqtt", image: "koenkk/zigbee2mqtt:1.40.2", profile: "node", port: 8080, mount: "/app/data", cpu: 2, memMB: 140},
		{name: "node-red", image: "nodered/node-red:4.0.3", profile: "node", port: 1880, publish: true, mount: "/data", cpu: 1, memMB: 170},
	}},
	"homelab-tools": {name: "tools", services: []serviceTemplate{
		{name: "pihole", image: "pihole/pihole:2024.07.0", profile: "dns", port: 53, publish: true, mount: "/etc/pihole", cpu: 1, memMB: 90, health: true},
		{name: "vaultwarden", image: "vaultwarden/server:1.32.1", profile: "api", port: 80, publish: true, mount: "/data", cpu: 1, memMB: 40, health: true},
		{name: "uptime-kuma", image: "louislam/uptime-kuma:1.23.14", profile: "node", port: 3001, publish: true, mount: "/app/data", cpu: 2, memMB: 130},
		{name: "paperless", image: "ghcr.io/paperless-ngx/paperless-ngx:2.12.1", profile: "python", port: 8000, publish: true, mount: "/usr/src/paperless/data", cpu: 3, memMB: 700},
		{name: "immich-server", image: "ghcr.io/immich-app/immich-server:v1.116.2", profile: "node", port: 2283, publish: true, mount: "/usr/src/app/upload", cpu: 6, memMB: 900},
		{name: "immich-ml", image: "ghcr.io/immich-app/immich-machine-learning:v1.116.2", profile: "python", cpu: 12, memMB: 1800},
	}},
	"edge-pos": {name: "pos", services: []serviceTemplate{
		{name: "pos-api", image: "ghcr.io/acme/pos-api:5.4.0", profile: "api", port: 8080, publish: true, cpu: 4, memMB: 260, health: true, replicas: 2},
		{name: "sync", image: "ghcr.io/acme/store-sync:1.3.7", profile: "worker", cpu: 2, memMB: 120, command: "/sync --upstream https://hq.acme.example"},
		{name: "printer-bridge", image: "ghcr.io/acme/printer-bridge:0.9.2", profile: "agent", cpu: 1, memMB: 35},
		{name: "db", image: "postgres:16.4-alpine", profile: "postgres", port: 5432, mount: "/var/lib/postgresql/data", cpu: 2, memMB: 210, health: true},
		{name: "cache", image: "redis:7.4-alpine", profile: "redis", port: 6379, cpu: 1, memMB: 30},
	}},
	"edge-cameras": {name: "cameras", services: []serviceTemplate{
		{name: "frigate", image: "ghcr.io/blakeblackshear/frigate:0.14.1", profile: "media", port: 5000, publish: true, mount: "/media/frigate", cpu: 28, memMB: 1100, health: true},
		{name: "go2rtc", image: "alexxit/go2rtc:1.9.4", profile: "media", port: 1984, cpu: 6, memMB: 80},
	}},
	"dmz": {name: "public", services: []serviceTemplate{
		{name: "nginx", image: "nginx:1.27.2", profile: "nginx", port: 443, publish: true, mount: "/etc/nginx/conf.d", cpu: 3, memMB: 60, health: true, replicas: 2},
		{name: "modsecurity", image: "owasp/modsecurity-crs:4.7-nginx", profile: "nginx", port: 8443, cpu: 4, memMB: 160},
		{name: "mailserver", image: "ghcr.io/docker-mailserver/docker-mailserver:14.0.0", profile: "mail", port: 25, publish: true, mount: "/var/mail", cpu: 2, memMB: 480, health: true},
		{name: "wireguard", image: "lscr.io/linuxserver/wireguard:1.0.20210914", profile: "agent", port: 51820, publish: true, cpu: 1, memMB: 20},
		{name: "bind9", image: "ubuntu/bind9:9.18-22.04_beta", profile: "dns", port: 53, publish: true, cpu: 1, memMB: 45},
	}},
	"ml": {name: "ml", services: []serviceTemplate{
		{name: "jupyterhub", image: "jupyterhub/jupyterhub:5.1.0", profile: "python", port: 8000, publish: true, cpu: 4, memMB: 600},
		{name: "mlflow", image: "ghcr.io/mlflow/mlflow:v2.16.2", profile: "python", port: 5000, publish: true, cpu: 2, memMB: 450},
		{name: "trainer", image: "ghcr.io/acme/trainer:0.7.3-cuda12", profile: "ci", cpu: 70, memMB: 12000, replicas: 2},
		{name: "vllm", image: "vllm/vllm-openai:v0.6.2", profile: "python", port: 8000, cpu: 20, memMB: 9000, health: true},
	}},
	"internal-apps": {name: "internal", services: []serviceTemplate{
		{name: "hr-portal", image: "ghcr.io/acme/hr-portal:1.14.0", profile: "web", port: 3000, cpu: 2, memMB: 240, health: true},
		{name: "wiki", image: "requarks/wiki:2.5.304", profile: "node", port: 3000, publish: true, cpu: 2, memMB: 330},
		{name: "mattermost", image: "mattermost/mattermost-team-edition:10.0.1", profile: "api", port: 8065, publish: true, mount: "/mattermost/data", cpu: 5, memMB: 700, health: true},
		{name: "db", image: "postgres:16.4", profile: "postgres", port: 5432, mount: "/var/lib/postgresql/data", cpu: 5, memMB: 900, health: true},
	}},
}

type hostSpec struct {
	key     string
	name    string
	region  string
	role    string
	target  int
	cpus    int
	memGB   int
	diskTB  int
	distro  string
	compute bool
	arch    string
	stacks  []string
}

var hostSpecs = []hostSpec{
	{key: "prod-eu-1", name: "Production EU 1", region: "eu-west", role: "production", target: 165, cpus: 64, memGB: 512, diskTB: 8, distro: "ubuntu", compute: true, arch: "x86_64", stacks: []string{"shop", "payments", "auth", "observability", "messaging", "search", "gateway", "storage", "internal-apps"}},
	{key: "prod-eu-2", name: "Production EU 2", region: "eu-west", role: "production", target: 150, cpus: 64, memGB: 512, diskTB: 8, distro: "ubuntu", compute: true, arch: "x86_64", stacks: []string{"shop", "payments", "observability", "messaging", "search", "gateway", "storage"}},
	{key: "prod-eu-3", name: "Production EU 3", region: "eu-central", role: "production", target: 140, cpus: 48, memGB: 384, diskTB: 6, distro: "debian", arch: "x86_64", stacks: []string{"shop", "auth", "observability", "gateway", "analytics", "search"}},
	{key: "prod-us-1", name: "Production US 1", region: "us-east", role: "production", target: 160, cpus: 64, memGB: 512, diskTB: 8, distro: "ubuntu", compute: true, arch: "x86_64", stacks: []string{"shop", "payments", "auth", "observability", "messaging", "gateway", "storage", "search"}},
	{key: "prod-us-2", name: "Production US 2", region: "us-west", role: "production", target: 130, cpus: 48, memGB: 384, diskTB: 6, distro: "rocky", arch: "x86_64", stacks: []string{"shop", "payments", "observability", "gateway", "messaging"}},
	{key: "staging-eu", name: "Staging EU", region: "eu-west", role: "staging", target: 75, cpus: 32, memGB: 192, diskTB: 4, distro: "ubuntu", compute: true, arch: "x86_64", stacks: []string{"shop", "payments", "auth", "observability", "messaging", "search"}},
	{key: "staging-us", name: "Staging US", region: "us-east", role: "staging", target: 60, cpus: 32, memGB: 192, diskTB: 4, distro: "ubuntu", arch: "x86_64", stacks: []string{"shop", "payments", "auth", "observability"}},
	{key: "dev", name: "Development", region: "eu-west", role: "development", target: 70, cpus: 24, memGB: 128, diskTB: 2, distro: "fedora", compute: true, arch: "x86_64", stacks: []string{"shop", "payments", "auth", "cms", "git", "search", "ml"}},
	{key: "ci-runners", name: "CI Runners", region: "eu-west", role: "ci", target: 45, cpus: 96, memGB: 384, diskTB: 4, distro: "ubuntu", arch: "x86_64", stacks: []string{"ci", "git"}},
	{key: "data-platform", name: "Data Platform", region: "eu-central", role: "data", target: 75, cpus: 96, memGB: 768, diskTB: 24, distro: "ubuntu", compute: true, arch: "x86_64", stacks: []string{"analytics", "airflow", "messaging", "storage", "ml", "observability"}},
	{key: "edge-store-01", name: "Edge · Store Amsterdam", region: "edge", role: "edge", target: 18, cpus: 8, memGB: 32, diskTB: 1, distro: "debian", arch: "aarch64", stacks: []string{"edge-pos", "edge-cameras"}},
	{key: "edge-store-02", name: "Edge · Store Rotterdam", region: "edge", role: "edge", target: 18, cpus: 8, memGB: 32, diskTB: 1, distro: "debian", arch: "aarch64", stacks: []string{"edge-pos", "edge-cameras"}},
	{key: "edge-store-03", name: "Edge · Store Berlin", region: "edge", role: "edge", target: 16, cpus: 8, memGB: 32, diskTB: 1, distro: "debian", arch: "aarch64", stacks: []string{"edge-pos", "edge-cameras"}},
	{key: "edge-store-04", name: "Edge · Store Paris", region: "edge", role: "edge", target: 16, cpus: 8, memGB: 32, diskTB: 1, distro: "debian", arch: "aarch64", stacks: []string{"edge-pos", "edge-cameras"}},
	{key: "homelab", name: "Homelab", region: "home", role: "homelab", target: 48, cpus: 16, memGB: 128, diskTB: 32, distro: "debian", compute: true, arch: "x86_64", stacks: []string{"media", "smarthome", "homelab-tools", "nextcloud", "git", "observability"}},
	{key: "dmz", name: "DMZ", region: "eu-west", role: "dmz", target: 26, cpus: 16, memGB: 64, diskTB: 2, distro: "alpine", compute: true, arch: "x86_64", stacks: []string{"dmz", "gateway", "cms"}},
}
