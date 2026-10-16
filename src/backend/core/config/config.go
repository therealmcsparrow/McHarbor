// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// Package config loads McHarbor's environment-driven configuration.
package config

import (
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is the full runtime configuration, populated from environment variables.
type Config struct {
	// App
	Port   int    `env:"PORT" envDefault:"8705"`
	Host   string `env:"HOST" envDefault:"0.0.0.0"`
	Secret string `env:"MCHARBOR_SECRET"`
	// SecretFile names a file holding the secret, normally a Docker secret, so the value
	// never sits in the container's Config.Env where every docker inspect shows it.
	SecretFile string `env:"MCHARBOR_SECRET_FILE"`

	// TLSSANHosts is the parsed DNSNames list fed into the
	// self-signed cert generator at boot and at every regenerate
	// (Phase 7 default-HTTPS). Computed in Load() from Host,
	// SELF_SIGNED_SAN_EXTRA, and the canonical localhost entries.
	// Not an env var — derived state.
	TLSSANHosts []string
	// TLSSANIPs is the parsed IPAddresses list for the same cert.
	// Always includes 127.0.0.1 plus Host when it's a literal IP.
	TLSSANIPs []string

	// Database
	DatabasePath string `env:"DATABASE_PATH" envDefault:"./data/mcharbor.db"`
	// DBDriver selects the SQL backend: "sqlite" (default, embedded
	// single-file), "postgres", or "mysql" (covers MySQL 8+ and
	// MariaDB 10.6+). Postgres and MySQL are external services
	// used for active-active multi-node deployments.
	DBDriver string `env:"MCHARBOR_DB_DRIVER" envDefault:"sqlite"`
	// DBDSN is the connection string for the external drivers.
	// Postgres expects a libpq-style URL
	// (postgres://user:pw@host:5432/db?sslmode=disable). MySQL /
	// MariaDB expects a go-sql-driver/mysql DSN
	// (user:pw@tcp(host:3306)/db?parseTime=true&loc=UTC&tls=true).
	// Unused for SQLite.
	DBDSN string `env:"MCHARBOR_DB_DSN"`
	// DBDSNFile is the file form of DBDSN, a Docker secret, so the database password never
	// sits in the container's Config.Env.
	DBDSNFile string `env:"MCHARBOR_DB_DSN_FILE"`
	// NodeID identifies this McHarbor instance in coordinator
	// leader-election log lines. If unset, a random 6-char suffix
	// is used. The same value should be reused across restarts of
	// the same node so operators can correlate logs.
	NodeID string `env:"MCHARBOR_NODE_ID"`

	// Docker
	DockerHost      string `env:"DOCKER_HOST" envDefault:"unix:///var/run/docker.sock"`
	DockerTLSVerify string `env:"DOCKER_TLS_VERIFY"`
	DockerCertPath  string `env:"DOCKER_CERT_PATH"`

	// Auth
	AuthDisable        bool `env:"AUTH_DISABLE" envDefault:"false"`
	ForceSecureCookies bool `env:"FORCE_SECURE_COOKIES" envDefault:"false"`

	// Encryption
	EncryptionKey string `env:"ENCRYPTION_KEY"`
	// EncryptionKeyFile is the file form of EncryptionKey, for the same reason as SecretFile.
	EncryptionKeyFile       string `env:"ENCRYPTION_KEY_FILE"`
	BackupEncryptionKeyFile string `env:"BACKUP_ENCRYPTION_KEY_FILE" envDefault:"/run/secrets/mcharbor_backup_key"`
	DataDir                 string `env:"DATA_DIR" envDefault:"./data"`
	// PluginCatalogDir is the read-only mount of the dev plugin
	// catalog (e.g. /mnt/plugin-catalog). The plugin_runtime
	// module's icon endpoint falls back to looking here for
	// uninstalled plugins so the picker tile and the catalog card
	// can render per-slug icons without the host having to bundle
	// plugin assets. Set to "" to disable the fallback.
	PluginCatalogDirs []string `env:"MCHARBOR_PLUGIN_CATALOG_DIRS" envSeparator:"," envDefault:""`

	// PluginCatalogRoot is an optional local directory of plugin tarballs.
	// Empty by default: plugins ship from the remote catalog, not the image,
	// so a local root exists only where an operator mounts one.
	PluginCatalogRoot string `env:"MCHARBOR_PLUGIN_CATALOG_ROOT" envDefault:""`
	// ThemeCatalogRoot is the theme-pack equivalent of PluginCatalogRoot.
	ThemeCatalogRoot string `env:"MCHARBOR_THEME_CATALOG_ROOT" envDefault:"/mnt/themes-catalog"`
	// InstallKind records how McHarbor was installed — "container", "deb",
	// "rpm", "apk" or "tarball" — because the update path differs per
	// format and sniffing the environment gets nested cases wrong.
	InstallKind string `env:"MCHARBOR_INSTALL_KIND" envDefault:"container"`

	// TrustedProxyCIDRs lists the networks whose forwarding headers may be
	// believed when attributing an API-key request to a client address.
	//
	// Empty is the secure default and means "trust nothing": every
	// Forwarded / X-Forwarded-For / X-Real-IP header is ignored and the
	// socket peer is the client. Set it only to the reverse proxies you
	// actually run — a value of 0.0.0.0/0 would let any caller claim any
	// origin, which is worse than not resolving at all.
	//
	// Accepts CIDRs and bare addresses: "10.0.0.0/8,192.0.2.7".
	TrustedProxyCIDRs []string `env:"MCHARBOR_TRUSTED_PROXY_CIDRS" envSeparator:"," envDefault:""`

	// TLS — when cert files are missing on disk the server
	// auto-generates a self-signed cert (see core/certgen). Setting
	// TLS_ENABLED=false opts out of TLS entirely and falls back to
	// plain HTTP on the main listener (the redirect listener still
	// 308s). The transfer listener uses the same cert as the main
	// listener; production deployments behind a real domain should
	// upload a cert via the settings UI or use apps/acme-companion.
	TLSEnabled       bool          `env:"TLS_ENABLED" envDefault:"true"`
	TLSForceHTTPS    bool          `env:"TLS_FORCE_HTTPS" envDefault:"true"`
	TLSMinVersion    TLSMinVersion `env:"TLS_MIN_VERSION" envDefault:"772"` // tls.VersionTLS13 — accepts "1.3"/"13"/"0x0304"/"772"; drop to "1.2" for pre-2019 clients
	HSTSMaxAge       int           `env:"HSTS_MAX_AGE" envDefault:"0"`      // off; opt-in via env
	HTTPRedirectAddr string        `env:"HTTP_REDIRECT_ADDR" envDefault:""`
	// HTTPListenAddr enables an OPTIONAL second main listener that
	// serves the full app over plain HTTP on the given address
	// (e.g. ":8706"). Set to "" to disable. Independent of the
	// main TLS-or-HTTP listener on cfg.Port — when both are
	// configured the app is reachable on two protocols at once
	// (HTTP and HTTPS) on different ports. Cookies set via the
	// HTTP listener use `SameSite=Lax` without the `Secure`
	// attribute, so they work over plain HTTP.
	HTTPListenAddr     string `env:"HTTP_LISTEN_ADDR" envDefault:""`
	TransferPort       string `env:"TRANSFER_PORT" envDefault:":8788"`
	ForceAgentTLS      bool   `env:"FORCE_AGENT_TLS" envDefault:"true"`
	SelfSignedSANExtra string `env:"SELF_SIGNED_SAN_EXTRA" envDefault:""` // comma-sep extra DNS entries

	// Logging
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	LogJSON  bool   `env:"LOG_JSON" envDefault:"false"`

	// Tracing. Both use the standard OpenTelemetry names so an operator's
	// existing collector config carries over; neither set means no spans.
	OTLPEndpoint       string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:""`
	OTLPTracesEndpoint string `env:"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT" envDefault:""`

	// ReconcileInterval is how often the background loop compares every desired
	// state with reality; 0 disables it and leaves drift checks request-driven.
	ReconcileInterval time.Duration `env:"MCHARBOR_RECONCILE_INTERVAL" envDefault:"60s"`

	// PushContact identifies the operator to the browser push services, as
	// VAPID requires; a mailto: or https: URL they can use to reach someone.
	PushContact string `env:"MCHARBOR_PUSH_CONTACT" envDefault:"https://www.mcharbor.eu"`

	// CORS
	AllowedOrigins string `env:"ALLOWED_ORIGINS" envDefault:""`

	// ContentSecurityPolicy is the CSP the server sends on everything it serves.
	// Resolved in resolveContentSecurityPolicy: unset means the default, set-but-empty disables it.
	ContentSecurityPolicy string `env:"CONTENT_SECURITY_POLICY"`
	// CSPReportOnly sends the policy as Content-Security-Policy-Report-Only, which is how a
	// new or tightened policy is trialled across the plugin surface before it enforces.
	CSPReportOnly bool `env:"CONTENT_SECURITY_POLICY_REPORT_ONLY" envDefault:"false"`

	// App Store
	AppStoreCatalogURL string `env:"APPSTORE_CATALOG_URL" envDefault:""`
	AppStoreSyncCron   string `env:"APPSTORE_SYNC_CRON" envDefault:""`
}

// ErrSecretSourceConflict is returned when a secret is given both as a variable and as a
// _FILE path; picking one silently would hide which value a node actually runs with.
var ErrSecretSourceConflict = errors.New("a secret is set both directly and through its _FILE variant")

// ErrSecretFileEmpty is returned when a _FILE path names an empty file, which would
// otherwise start McHarbor with a generated key that no other node shares.
var ErrSecretFileEmpty = errors.New("secret file is empty")

// Load parses the environment into a Config and validates it.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.resolveSecretFiles(); err != nil {
		return nil, err
	}
	cfg.deriveDatabasePath()
	cfg.resolveContentSecurityPolicy()
	cfg.computeTLSSANs()
	return cfg, nil
}

// resolveSecretFiles reads MCHARBOR_SECRET_FILE, ENCRYPTION_KEY_FILE and MCHARBOR_DB_DSN_FILE
// into their fields, refusing a variable set both ways and a file that is missing or empty.
func (c *Config) resolveSecretFiles() error {
	secret, err := secretFromFile("MCHARBOR_SECRET", c.Secret, c.SecretFile)
	if err != nil {
		return err
	}
	key, err := secretFromFile("ENCRYPTION_KEY", c.EncryptionKey, c.EncryptionKeyFile)
	if err != nil {
		return err
	}
	dsn, err := secretFromFile("MCHARBOR_DB_DSN", c.DBDSN, c.DBDSNFile)
	if err != nil {
		return err
	}
	c.Secret, c.EncryptionKey, c.DBDSN = secret, key, dsn
	return nil
}

// secretFromFile returns value, or the trimmed contents of path when path is set.
func secretFromFile(name, value, path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return value, nil
	}
	if strings.TrimSpace(value) != "" {
		return "", fmt.Errorf("%s and %s_FILE are both set, keep one: %w", name, name, ErrSecretSourceConflict)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading %s_FILE %s: %w", name, path, err)
	}
	secret := strings.TrimSpace(string(raw))
	if secret == "" {
		return "", fmt.Errorf("%s_FILE %s: %w", name, path, ErrSecretFileEmpty)
	}
	return secret, nil
}

// deriveDatabasePath makes DatabasePath follow DataDir when DATABASE_PATH was
// never set, so a bare DATA_DIR=... run doesn't split its database off to
// ./data/mcharbor.db beside the binary instead of under DataDir.
func (c *Config) deriveDatabasePath() {
	if _, set := os.LookupEnv("DATABASE_PATH"); set {
		return
	}
	c.DatabasePath = filepath.Join(c.DataDir, "mcharbor.db")
}

// computeTLSSANs builds the SAN lists used by the self-signed cert
// generator, and is the single source of truth for them: the boot path
// and Settings -> "regenerate self-signed" both read TLSSANHosts /
// TLSSANIPs from here, so the two cannot drift apart.
//
// DNS names: "localhost" always, the OS hostname, "host.docker.internal"
// (how a container-hosted McHarbor is reached from its own host), Host
// when it is a hostname rather than a bind address, and any comma-
// separated SELF_SIGNED_SAN_EXTRA entries.
//
// IPs: loopback (127.0.0.1 and ::1) always, Host when it is a literal
// IP, and every address bound to a live non-loopback interface — a cert
// that only covers localhost fails the moment the operator opens
// McHarbor from another machine on the LAN, which is the normal case.
// SELF_SIGNED_SAN_EXTRA entries that parse as IPs are routed here
// instead of into DNSNames, where they would never match.
func (c *Config) computeTLSSANs() {
	hosts := []string{"localhost"}
	if hn, err := os.Hostname(); err == nil {
		hn = strings.TrimSpace(hn)
		if hn != "" && hn != "localhost" && !looksLikeIP(hn) {
			hosts = append(hosts, hn)
		}
	}
	hosts = append(hosts, "host.docker.internal")
	if c.Host != "" && c.Host != "0.0.0.0" && !looksLikeIP(c.Host) {
		hosts = append(hosts, c.Host)
	}

	ips := []string{"127.0.0.1", "::1"}
	if c.Host != "" && c.Host != "0.0.0.0" && c.Host != "::" && looksLikeIP(c.Host) {
		ips = append(ips, c.Host)
	}
	ips = append(ips, interfaceIPs()...)

	if extra := strings.TrimSpace(c.SelfSignedSANExtra); extra != "" {
		for _, entry := range strings.Split(extra, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			if ip := net.ParseIP(entry); ip != nil {
				ips = append(ips, ip.String())
				continue
			}
			hosts = append(hosts, entry)
		}
	}

	c.TLSSANHosts = dedupStrings(hosts)
	c.TLSSANIPs = dedupStrings(ips)
}

// interfaceIPs returns the unicast addresses of every interface that is
// up and not loopback — the "IP of the adapter" an operator types into
// the address bar. Link-local IPv6 is skipped: it is never what the
// browser connects to and it would bloat the SAN list on hosts with many
// virtual adapters. Enumeration failures are ignored; the loopback SANs
// still produce a usable local cert.
func interfaceIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	out := []string{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}
			out = append(out, ip.String())
		}
	}
	return out
}

func looksLikeIP(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '.' && c != ':' && (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return s != ""
}

func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Addr returns the host:port the HTTP server binds to.
func (c *Config) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// TracesEndpoint returns the OTLP traces endpoint, preferring the signal-specific
// variable the way the OpenTelemetry SDK does; empty means tracing stays off.
func (c *Config) TracesEndpoint() string {
	if c.OTLPTracesEndpoint != "" {
		return c.OTLPTracesEndpoint
	}
	return c.OTLPEndpoint
}

// AllowedOriginList returns the deduplicated extra origins accepted for WebSocket and CORS checks.
func (c *Config) AllowedOriginList() []string {
	if c.AllowedOrigins != "" {
		origins := strings.Split(c.AllowedOrigins, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		return origins
	}

	return []string{
		"http://localhost:8173",
		"https://localhost:8173",
		fmt.Sprintf("http://localhost:%d", c.Port),
		fmt.Sprintf("https://localhost:%d", c.Port),
		"http://localhost:8705",
		"https://localhost:8705",
	}
}

// DBConfig returns a database.Config for the Open() helper. When
// MCHARBOR_DB_DRIVER is unset or "sqlite", the legacy single-file
// path is used (full backward compatibility). When the driver is
// "postgres", "mysql" or "mssql", MCHARBOR_DB_DSN is required and is
// forwarded verbatim to the active driver.
func (c *Config) DBConfig() (driverName, pathOrDSN string) {
	switch strings.ToLower(strings.TrimSpace(c.DBDriver)) {
	case "", "sqlite", "sqlite3":
		return "sqlite", c.DatabasePath
	case "postgres", "postgresql", "pgsql":
		return "postgres", strings.TrimSpace(c.DBDSN)
	case "mysql", "mariadb":
		return "mysql", strings.TrimSpace(c.DBDSN)
	case "mssql", "sqlserver", "azuresql":
		return "mssql", strings.TrimSpace(c.DBDSN)
	default:
		// Startup refuses an unknown driver in dbconfig.Resolve, so this branch
		// only answers read-only callers such as the support bundle.
		return "sqlite", c.DatabasePath
	}
}

// LogSlogLevel maps the configured log level string to a slog.Level, defaulting to info.
func (c *Config) LogSlogLevel() slog.Level {
	switch c.LogLevel {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// TLSMinVersion is the minimum TLS protocol version accepted by the
// server. The env var (TLS_MIN_VERSION) accepts human-friendly
// formats so operators don't have to memorise wire constants:
//
//	"1.2", "12"          → tls.VersionTLS12 (0x0303, 771)
//	"1.3", "13"          → tls.VersionTLS13 (0x0304, 772)
//	"0x0303", "0x0304"   → same, hex
//	"771", "772"         → same, decimal wire constant
//
// Values below TLS 1.2 are rejected at config load because modern
// clients consider TLS 1.0/1.1 insecure.
type TLSMinVersion uint16

// UnmarshalText parses the env var. Implements encoding.TextUnmarshaler
// so caarlos0/env picks it up automatically.
func (m *TLSMinVersion) UnmarshalText(b []byte) error {
	s := strings.TrimSpace(string(b))
	s = strings.TrimPrefix(s, "TLS ")
	s = strings.TrimPrefix(s, "tls ")
	var v uint16
	switch strings.ToLower(s) {
	case "1.0":
		return errors.New("tls: refusing TLS 1.0; minimum supported is TLS 1.2")
	case "1.1":
		return errors.New("tls: refusing TLS 1.1; minimum supported is TLS 1.2")
	case "1.2", "12":
		v = tls.VersionTLS12
	case "1.3", "13":
		v = tls.VersionTLS13
	case "0x0303":
		v = tls.VersionTLS12
	case "0x0304":
		v = tls.VersionTLS13
	default:
		// Accept any raw uint16 (decimal or 0x hex). Lets operators
		// pin to an exact constant if they really want to.
		n, err := strconv.ParseUint(s, 0, 16)
		if err != nil {
			return fmt.Errorf("tls: unrecognized TLS_MIN_VERSION %q (want 1.2 / 1.3 / 0x0303 / 771)", s)
		}
		v = uint16(n)
	}
	if v < tls.VersionTLS12 {
		return fmt.Errorf("tls: refusing minVersion 0x%04x; minimum supported is TLS 1.2 (0x0303)", v)
	}
	*m = TLSMinVersion(v)
	return nil
}
