// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package computesim

// osProfile is one guest operating system as the plugins and the guest agent
// describe it, so a Windows guest reports itself the way a real one would.
type osProfile struct {
	key        string
	family     string
	variant    string
	name       string
	version    string
	osID       string
	firmware   string
	secureBoot bool
	tpm        bool
	guestAgent bool
	kernel     string
	display    string
}

var osProfiles = map[string]osProfile{
	"ubuntu2404": {key: "ubuntu2404", family: "linux", variant: "ubuntu24.04", name: "Ubuntu 24.04.1 LTS", version: "24.04", osID: "ubuntu", firmware: "uefi", guestAgent: true, kernel: "6.8.0-45-generic", display: "virtio-gpu"},
	"ubuntu2204": {key: "ubuntu2204", family: "linux", variant: "ubuntu22.04", name: "Ubuntu 22.04.5 LTS", version: "22.04", osID: "ubuntu", firmware: "uefi", guestAgent: true, kernel: "5.15.0-122-generic", display: "virtio-gpu"},
	"debian12":   {key: "debian12", family: "linux", variant: "debian12", name: "Debian GNU/Linux 12 (bookworm)", version: "12", osID: "debian", firmware: "uefi", guestAgent: true, kernel: "6.1.0-25-amd64", display: "virtio-gpu"},
	"rocky9":     {key: "rocky9", family: "linux", variant: "rocky9", name: "Rocky Linux 9.4 (Blue Onyx)", version: "9.4", osID: "rocky", firmware: "uefi", guestAgent: true, kernel: "5.14.0-427.33.1.el9_4.x86_64", display: "virtio-gpu"},
	"win2022":    {key: "win2022", family: "windows", variant: "win2k22", name: "Microsoft Windows Server 2022 Standard", version: "10.0.20348", osID: "mswindows", firmware: "uefi", secureBoot: true, tpm: true, guestAgent: true, display: "std"},
	"win11":      {key: "win11", family: "windows", variant: "win11", name: "Microsoft Windows 11 Pro", version: "10.0.22631", osID: "mswindows", firmware: "uefi", secureBoot: true, tpm: true, guestAgent: true, display: "std"},
	"pfsense":    {key: "pfsense", family: "bsd", variant: "freebsd14.0", name: "pfSense CE 2.7.2", version: "14.0-CURRENT", osID: "freebsd", firmware: "bios", display: "std"},
	"freebsd14":  {key: "freebsd14", family: "bsd", variant: "freebsd14.1", name: "FreeBSD 14.1-RELEASE", version: "14.1", osID: "freebsd", firmware: "uefi", display: "std"},
	"talos":      {key: "talos", family: "linux", variant: "talos1.8", name: "Talos Linux v1.8.1", version: "1.8.1", osID: "talos", firmware: "uefi", display: "none"},
}

// vmRole is a kind of guest an operator would plausibly run, with the shape a
// real one of that kind usually has.
type vmRole struct {
	prefix   string
	single   bool
	os       []string
	vcpus    []int
	memoryMB []int64
	diskGB   []int64
	dataGB   int64
	desc     string
}

var vmRoles = map[string]vmRole{
	"db-primary":    {prefix: "db-primary", os: []string{"rocky9", "ubuntu2404"}, vcpus: []int{8, 16}, memoryMB: []int64{32768, 65536}, diskGB: []int64{80}, dataGB: 500, desc: "PostgreSQL primary"},
	"db-replica":    {prefix: "db-replica", os: []string{"rocky9", "ubuntu2404"}, vcpus: []int{8}, memoryMB: []int64{32768}, diskGB: []int64{80}, dataGB: 500, desc: "PostgreSQL streaming replica"},
	"k8s-master":    {prefix: "k8s-master", os: []string{"ubuntu2204", "ubuntu2404"}, vcpus: []int{4}, memoryMB: []int64{8192}, diskGB: []int64{60}, desc: "Kubernetes control plane"},
	"k8s-worker":    {prefix: "k8s-worker", os: []string{"ubuntu2204", "ubuntu2404"}, vcpus: []int{8, 16}, memoryMB: []int64{32768, 65536}, diskGB: []int64{120, 200}, desc: "Kubernetes worker"},
	"talos-cp":      {prefix: "talos-cp", os: []string{"talos"}, vcpus: []int{4}, memoryMB: []int64{8192}, diskGB: []int64{50}, desc: "Talos control plane"},
	"win-ad-dc":     {prefix: "win-ad-dc", os: []string{"win2022"}, vcpus: []int{2, 4}, memoryMB: []int64{4096, 8192}, diskGB: []int64{80}, desc: "Active Directory domain controller"},
	"win-sql":       {prefix: "win-sql", os: []string{"win2022"}, vcpus: []int{8}, memoryMB: []int64{32768}, diskGB: []int64{120}, dataGB: 400, desc: "SQL Server 2022"},
	"win-rds":       {prefix: "win-rds", os: []string{"win2022"}, vcpus: []int{8}, memoryMB: []int64{16384}, diskGB: []int64{150}, desc: "Remote Desktop session host"},
	"gitlab-runner": {prefix: "gitlab-runner", os: []string{"ubuntu2404", "debian12"}, vcpus: []int{4, 8}, memoryMB: []int64{8192, 16384}, diskGB: []int64{100}, desc: "CI runner"},
	"elastic":       {prefix: "elastic", os: []string{"rocky9"}, vcpus: []int{8}, memoryMB: []int64{32768}, diskGB: []int64{60}, dataGB: 1000, desc: "Elasticsearch data node"},
	"haproxy":       {prefix: "haproxy", os: []string{"debian12"}, vcpus: []int{2}, memoryMB: []int64{2048, 4096}, diskGB: []int64{20}, desc: "Load balancer"},
	"vault":         {prefix: "vault", os: []string{"debian12"}, vcpus: []int{2}, memoryMB: []int64{4096}, diskGB: []int64{20}, desc: "HashiCorp Vault"},
	"jumpbox":       {prefix: "jumpbox", os: []string{"ubuntu2404"}, vcpus: []int{2}, memoryMB: []int64{2048}, diskGB: []int64{40}, desc: "SSH bastion"},
	"app":           {prefix: "app", os: []string{"ubuntu2404", "rocky9", "debian12"}, vcpus: []int{4}, memoryMB: []int64{8192, 16384}, diskGB: []int64{60}, desc: "Application server"},
	"kafka":         {prefix: "kafka-broker", os: []string{"rocky9"}, vcpus: []int{8}, memoryMB: []int64{32768}, diskGB: []int64{60}, dataGB: 2000, desc: "Kafka broker"},
	"spark":         {prefix: "spark-worker", os: []string{"ubuntu2204"}, vcpus: []int{16, 32}, memoryMB: []int64{65536, 131072}, diskGB: []int64{200}, desc: "Spark worker"},
	"clickhouse":    {prefix: "clickhouse", os: []string{"ubuntu2404"}, vcpus: []int{16}, memoryMB: []int64{65536}, diskGB: []int64{80}, dataGB: 4000, desc: "ClickHouse shard"},
	"trino":         {prefix: "trino-coord", os: []string{"rocky9"}, vcpus: []int{8}, memoryMB: []int64{32768}, diskGB: []int64{60}, desc: "Trino coordinator"},
	"minio":         {prefix: "minio", os: []string{"debian12"}, vcpus: []int{4}, memoryMB: []int64{16384}, diskGB: []int64{40}, dataGB: 8000, desc: "MinIO object storage"},
	"airflow":       {prefix: "airflow-sched", os: []string{"ubuntu2404"}, vcpus: []int{4}, memoryMB: []int64{8192}, diskGB: []int64{60}, desc: "Airflow scheduler"},
	"pfsense":       {prefix: "pfsense-edge", single: true, os: []string{"pfsense"}, vcpus: []int{2}, memoryMB: []int64{4096}, diskGB: []int64{32}, desc: "Edge firewall"},
	"truenas":       {prefix: "truenas", single: true, os: []string{"freebsd14"}, vcpus: []int{4}, memoryMB: []int64{32768}, diskGB: []int64{32}, desc: "Storage appliance"},
	"win11-gaming":  {prefix: "win11-gaming", single: true, os: []string{"win11"}, vcpus: []int{8}, memoryMB: []int64{32768}, diskGB: []int64{500}, desc: "GPU passthrough desktop"},
	"win11":         {prefix: "win11-lab", os: []string{"win11"}, vcpus: []int{4}, memoryMB: []int64{8192}, diskGB: []int64{128}, desc: "Windows test desktop"},
	"ubuntu-dev":    {prefix: "ubuntu-dev", os: []string{"ubuntu2404"}, vcpus: []int{4, 8}, memoryMB: []int64{8192, 16384}, diskGB: []int64{80}, desc: "Developer workstation"},
	"home-assist":   {prefix: "home-assistant", single: true, os: []string{"debian12"}, vcpus: []int{2}, memoryMB: []int64{4096}, diskGB: []int64{32}, desc: "Home Assistant OS"},
	"k3s":           {prefix: "k3s-node", os: []string{"debian12", "ubuntu2404"}, vcpus: []int{4}, memoryMB: []int64{8192}, diskGB: []int64{64}, desc: "k3s node"},
	"docker-host":   {prefix: "docker-host", os: []string{"debian12"}, vcpus: []int{4}, memoryMB: []int64{16384}, diskGB: []int64{120}, desc: "Docker host"},
	"stg-app":       {prefix: "stg-app", os: []string{"ubuntu2404"}, vcpus: []int{4}, memoryMB: []int64{8192}, diskGB: []int64{60}, desc: "Staging application server"},
	"stg-db":        {prefix: "stg-db", os: []string{"rocky9"}, vcpus: []int{4}, memoryMB: []int64{16384}, diskGB: []int64{60}, dataGB: 200, desc: "Staging database"},
	"win11-dev":     {prefix: "win11-dev", os: []string{"win11"}, vcpus: []int{4}, memoryMB: []int64{16384}, diskGB: []int64{128}, desc: "Windows build box"},
	"build-agent":   {prefix: "build-agent", os: []string{"ubuntu2204"}, vcpus: []int{8}, memoryMB: []int64{16384}, diskGB: []int64{150}, desc: "Build agent"},
	"waf":           {prefix: "waf", os: []string{"debian12"}, vcpus: []int{2}, memoryMB: []int64{4096}, diskGB: []int64{20}, desc: "Web application firewall"},
	"bastion":       {prefix: "bastion", os: []string{"debian12"}, vcpus: []int{2}, memoryMB: []int64{2048}, diskGB: []int64{20}, desc: "DMZ bastion host"},
	"mail-relay":    {prefix: "mail-relay", os: []string{"debian12"}, vcpus: []int{2}, memoryMB: []int64{2048}, diskGB: []int64{40}, desc: "Outbound SMTP relay"},
	"reverse-proxy": {prefix: "reverse-proxy", os: []string{"ubuntu2404"}, vcpus: []int{2}, memoryMB: []int64{4096}, diskGB: []int64{20}, desc: "Public reverse proxy"},
}

// ctOS is one container distribution with the init system its rootfs boots.
type ctOS struct {
	family  string
	version string
	pretty  string
	init    string
}

var ctOSes = map[string]ctOS{
	"ubuntu": {family: "ubuntu", version: "24.04", pretty: "Ubuntu 24.04.1 LTS", init: "systemd"},
	"debian": {family: "debian", version: "12", pretty: "Debian GNU/Linux 12 (bookworm)", init: "systemd"},
	"alpine": {family: "alpine", version: "3.21", pretty: "Alpine Linux v3.21", init: "openrc"},
	"rocky":  {family: "rocky", version: "9.4", pretty: "Rocky Linux 9.4 (Blue Onyx)", init: "systemd"},
}

// ctRole is a kind of system container, with its usual size and process set.
type ctRole struct {
	prefix string
	single bool
	os     []string
	cpus   []int
	memMB  []int64
	diskGB int64
	procs  []string
	desc   string
}

var ctRoles = map[string]ctRole{
	"nginx":       {prefix: "nginx-proxy", os: []string{"debian", "alpine"}, cpus: []int{1, 2}, memMB: []int64{512, 1024}, diskGB: 8, procs: []string{"nginx: master process", "nginx: worker process"}, desc: "Reverse proxy"},
	"postgres":    {prefix: "postgres", os: []string{"debian", "ubuntu"}, cpus: []int{2, 4}, memMB: []int64{4096, 8192}, diskGB: 50, procs: []string{"postgres -D /var/lib/postgresql/16/main", "postgres: checkpointer", "postgres: walwriter"}, desc: "PostgreSQL"},
	"redis":       {prefix: "redis-cache", os: []string{"alpine", "debian"}, cpus: []int{1}, memMB: []int64{1024, 2048}, diskGB: 4, procs: []string{"redis-server *:6379"}, desc: "Redis cache"},
	"grafana":     {prefix: "grafana", os: []string{"ubuntu"}, cpus: []int{2}, memMB: []int64{1024}, diskGB: 10, procs: []string{"grafana server"}, desc: "Dashboards"},
	"prometheus":  {prefix: "prometheus", os: []string{"debian"}, cpus: []int{2, 4}, memMB: []int64{4096, 8192}, diskGB: 100, procs: []string{"prometheus --config.file=/etc/prometheus/prometheus.yml"}, desc: "Metrics"},
	"bind":        {prefix: "bind-dns", os: []string{"debian"}, cpus: []int{1}, memMB: []int64{512}, diskGB: 4, procs: []string{"named -u bind"}, desc: "Authoritative DNS"},
	"ansible":     {prefix: "ansible-ctl", single: true, os: []string{"ubuntu"}, cpus: []int{2}, memMB: []int64{2048}, diskGB: 16, procs: []string{"sshd: /usr/sbin/sshd -D"}, desc: "Ansible control node"},
	"syslog":      {prefix: "syslog", os: []string{"rocky"}, cpus: []int{2}, memMB: []int64{2048}, diskGB: 80, procs: []string{"rsyslogd -n"}, desc: "Central syslog"},
	"ntp":         {prefix: "ntp", os: []string{"alpine"}, cpus: []int{1}, memMB: []int64{256}, diskGB: 2, procs: []string{"chronyd -d"}, desc: "Time server"},
	"gitea":       {prefix: "gitea", single: true, os: []string{"debian"}, cpus: []int{2}, memMB: []int64{2048}, diskGB: 40, procs: []string{"gitea web"}, desc: "Git forge"},
	"pihole":      {prefix: "pihole", single: true, os: []string{"debian"}, cpus: []int{1}, memMB: []int64{512}, diskGB: 4, procs: []string{"pihole-FTL -f"}, desc: "DNS sinkhole"},
	"unifi":       {prefix: "unifi-controller", single: true, os: []string{"ubuntu"}, cpus: []int{2}, memMB: []int64{2048}, diskGB: 16, procs: []string{"java -jar /usr/lib/unifi/lib/ace.jar start"}, desc: "UniFi Network"},
	"jellyfin":    {prefix: "jellyfin", single: true, os: []string{"ubuntu"}, cpus: []int{4}, memMB: []int64{4096}, diskGB: 32, procs: []string{"jellyfin --datadir /var/lib/jellyfin"}, desc: "Media server"},
	"mqtt":        {prefix: "mqtt-broker", single: true, os: []string{"alpine"}, cpus: []int{1}, memMB: []int64{256}, diskGB: 2, procs: []string{"mosquitto -c /etc/mosquitto/mosquitto.conf"}, desc: "MQTT broker"},
	"zigbee":      {prefix: "zigbee2mqtt", single: true, os: []string{"alpine"}, cpus: []int{1}, memMB: []int64{512}, diskGB: 2, procs: []string{"node index.js"}, desc: "Zigbee bridge"},
	"nextcloud":   {prefix: "nextcloud", single: true, os: []string{"debian"}, cpus: []int{4}, memMB: []int64{4096}, diskGB: 200, procs: []string{"php-fpm: master process", "apache2 -k start"}, desc: "File sync"},
	"wireguard":   {prefix: "wireguard", single: true, os: []string{"alpine"}, cpus: []int{1}, memMB: []int64{256}, diskGB: 2, procs: []string{"wg-quick up wg0"}, desc: "VPN endpoint"},
	"adguard":     {prefix: "adguard", single: true, os: []string{"alpine"}, cpus: []int{1}, memMB: []int64{512}, diskGB: 4, procs: []string{"AdGuardHome -s run"}, desc: "DNS filter"},
	"vaultwarden": {prefix: "vaultwarden", single: true, os: []string{"debian"}, cpus: []int{1}, memMB: []int64{512}, diskGB: 4, procs: []string{"vaultwarden"}, desc: "Password manager"},
	"homebridge":  {prefix: "homebridge", single: true, os: []string{"debian"}, cpus: []int{1}, memMB: []int64{1024}, diskGB: 8, procs: []string{"homebridge -I"}, desc: "HomeKit bridge"},
	"mailhog":     {prefix: "mailhog", os: []string{"alpine"}, cpus: []int{1}, memMB: []int64{256}, diskGB: 2, procs: []string{"MailHog"}, desc: "Test mail catcher"},
	"minio-gw":    {prefix: "minio-gw", os: []string{"debian"}, cpus: []int{2}, memMB: []int64{2048}, diskGB: 16, procs: []string{"minio gateway"}, desc: "S3 gateway"},
	"jupyter":     {prefix: "jupyter", os: []string{"ubuntu"}, cpus: []int{4}, memMB: []int64{8192}, diskGB: 50, procs: []string{"jupyter-lab --ip 0.0.0.0"}, desc: "Notebook server"},
	"haproxy-ct":  {prefix: "lb", os: []string{"alpine"}, cpus: []int{1}, memMB: []int64{512}, diskGB: 2, procs: []string{"haproxy -f /etc/haproxy/haproxy.cfg"}, desc: "TCP load balancer"},
}

// hostPlan says how many guests of which roles one compute host carries; the
// weights are what makes production look busy and the DMZ look sparse.
type hostPlan struct {
	vms      int
	cts      int
	vmRoles  []string
	ctRoles  []string
	pools    []string
	networks []string
}

var hostPlans = map[string]hostPlan{
	"prod-eu-1": {vms: 22, cts: 10,
		vmRoles:  []string{"db-primary", "db-replica", "k8s-master", "k8s-worker", "k8s-worker", "win-ad-dc", "win-sql", "gitlab-runner", "elastic", "haproxy", "vault", "app", "app", "jumpbox"},
		ctRoles:  []string{"nginx", "redis", "prometheus", "bind", "syslog", "ntp", "postgres"},
		pools:    []string{"local", "tank", "fast-nvme"},
		networks: []string{"vm-lan", "prod-vlan", "backend"}},
	"prod-eu-2": {vms: 18, cts: 8,
		vmRoles:  []string{"db-replica", "k8s-worker", "k8s-worker", "k8s-master", "win-ad-dc", "win-rds", "elastic", "haproxy", "app", "app", "gitlab-runner"},
		ctRoles:  []string{"nginx", "redis", "prometheus", "bind", "ntp"},
		pools:    []string{"local", "tank"},
		networks: []string{"vm-lan", "prod-vlan"}},
	"prod-us-1": {vms: 20, cts: 9,
		vmRoles:  []string{"db-primary", "db-replica", "talos-cp", "k8s-worker", "k8s-worker", "win-ad-dc", "win-sql", "app", "app", "haproxy", "vault", "gitlab-runner"},
		ctRoles:  []string{"nginx", "redis", "grafana", "prometheus", "syslog", "bind"},
		pools:    []string{"local", "tank", "fast-nvme"},
		networks: []string{"vm-lan", "prod-vlan", "backend"}},
	"data-platform": {vms: 20, cts: 12,
		vmRoles:  []string{"kafka", "spark", "spark", "clickhouse", "trino", "minio", "airflow", "db-primary", "elastic"},
		ctRoles:  []string{"jupyter", "postgres", "redis", "minio-gw", "grafana", "prometheus"},
		pools:    []string{"local", "tank", "fast-nvme", "archive"},
		networks: []string{"vm-lan", "backend", "prod-vlan"}},
	"homelab": {vms: 15, cts: 25,
		vmRoles:  []string{"pfsense", "truenas", "win11-gaming", "home-assist", "ubuntu-dev", "k3s", "k3s", "docker-host", "win11", "talos-cp", "ubuntu-dev"},
		ctRoles:  []string{"pihole", "unifi", "jellyfin", "mqtt", "zigbee", "nextcloud", "wireguard", "adguard", "vaultwarden", "homebridge", "gitea", "nginx", "postgres", "redis", "grafana", "prometheus", "mailhog"},
		pools:    []string{"local", "tank", "archive"},
		networks: []string{"lan", "lab-nat"}},
	"staging-eu": {vms: 10, cts: 7,
		vmRoles:  []string{"stg-app", "stg-app", "stg-db", "k8s-worker", "win-ad-dc", "haproxy"},
		ctRoles:  []string{"nginx", "redis", "postgres", "mailhog"},
		pools:    []string{"local", "tank"},
		networks: []string{"vm-lan", "prod-vlan"}},
	"dev": {vms: 9, cts: 6,
		vmRoles:  []string{"ubuntu-dev", "ubuntu-dev", "win11-dev", "build-agent", "gitlab-runner"},
		ctRoles:  []string{"postgres", "redis", "mailhog", "gitea"},
		pools:    []string{"local", "tank"},
		networks: []string{"vm-lan", "lab-nat"}},
	"dmz": {vms: 6, cts: 3,
		vmRoles:  []string{"pfsense", "waf", "bastion", "mail-relay", "reverse-proxy"},
		ctRoles:  []string{"haproxy-ct", "wireguard", "bind"},
		pools:    []string{"local", "fast-nvme"},
		networks: []string{"vm-lan", "prod-vlan"}},
}

var snapshotNames = []string{"baseline", "pre-upgrade", "before-patch-tuesday", "post-install", "pre-kernel-6.8", "known-good", "pre-migration", "weekly"}
