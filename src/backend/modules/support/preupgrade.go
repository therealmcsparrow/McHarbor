// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package support

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/therealmcsparrow/mcharbor/core/runningwork"
	appversion "github.com/therealmcsparrow/mcharbor/core/version"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
)

// Pre-upgrade check IDs beyond the diagnostics ones it reuses.
const (
	CheckTargetMigrations = "target_migrations"
	CheckPluginCompat     = "plugin_compat"
	CheckBusy             = "busy"
)

// ErrInvalidVersion is returned for a target that is not a McHarbor version string.
var ErrInvalidVersion = errors.New("support: invalid target version")

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([-+][a-zA-Z0-9.]+)?$`)

// rangeToken is one comparator of a manifest `mcharbor:` range, such as ">=2026.09.01".
var rangeToken = regexp.MustCompile(`^(>=|<=|>|<|=|\^|~)?([0-9]+\.[0-9]+\.[0-9]+)$`)

// rangeSpacing splits ">=1.0.0<2.0.0" into two comparators.
var rangeSpacing = regexp.MustCompile(`([0-9])\s*([><=~^])`)

// parseVersion reads the three numeric parts; the calendar notation's leading zeros are
// fine because each part is compared as a number, the same way the agent gate does.
func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	core, _, _ := strings.Cut(strings.TrimPrefix(strings.TrimSpace(v), "v"), "-")
	core, _, _ = strings.Cut(core, "+")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func compareVersion(a, b [3]int) int {
	for i := 0; i < 3; i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

// RangeAllows reports whether version satisfies a manifest `mcharbor:` range. The second
// result is false when the range cannot be read, which the report shows as unknown rather
// than guessing either way.
func RangeAllows(rng, version string) (bool, bool) {
	v, ok := parseVersion(version)
	if !ok {
		return false, false
	}
	rng = strings.TrimSpace(rng)
	if rng == "" || rng == "*" {
		return true, true
	}
	spaced := rangeSpacing.ReplaceAllString(rng, "$1 $2")
	for _, tok := range strings.Fields(spaced) {
		m := rangeToken.FindStringSubmatch(tok)
		if m == nil {
			return false, false
		}
		want, _ := parseVersion(m[2])
		c := compareVersion(v, want)
		allowed := true
		switch m[1] {
		case ">=":
			allowed = c >= 0
		case ">":
			allowed = c > 0
		case "<=":
			allowed = c <= 0
		case "<":
			allowed = c < 0
		case "", "=":
			allowed = c == 0
		case "^":
			allowed = c >= 0 && v[0] == want[0]
		case "~":
			allowed = c >= 0 && v[0] == want[0] && v[1] == want[1]
		}
		if !allowed {
			return false, true
		}
	}
	return true, true
}

type installedPlugin struct {
	Slug, Version string
}

func (s *Service) installedPlugins(ctx context.Context) ([]installedPlugin, error) {
	rows, err := s.d.DB.QueryContext(ctx, `SELECT slug, version FROM appstore_plugins WHERE enabled = 1 ORDER BY slug LIMIT 1000`)
	if err != nil {
		return nil, fmt.Errorf("listing installed plugins: %w", err)
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()
	var out []installedPlugin
	for rows.Next() {
		var p installedPlugin
		if err := rows.Scan(&p.Slug, &p.Version); err != nil {
			return nil, fmt.Errorf("scanning plugin: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// manifestRange reads the `mcharbor:` range from an installed plugin's manifest.
func (s *Service) manifestRange(p installedPlugin) (string, error) {
	if strings.ContainsAny(p.Slug+p.Version, `/\`) || strings.Contains(p.Slug+p.Version, "..") {
		return "", errors.New("support: unsafe plugin directory name")
	}
	raw, err := os.ReadFile(filepath.Join(s.d.DataDir, "plugins", p.Slug+"@"+p.Version, "plugin.yaml"))
	if err != nil {
		return "", err
	}
	var m struct {
		Mcharbor string `yaml:"mcharbor"`
	}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	return m.Mcharbor, nil
}

// PreUpgrade measures the running instance against target. It changes nothing and never
// blocks the update path; it only reports what an admin should know before starting one.
func (s *Service) PreUpgrade(ctx context.Context, target string) (PreUpgradeReport, error) {
	current := appversion.Current()
	target = strings.TrimPrefix(strings.TrimSpace(target), "v")
	if target == "" {
		target = current
	}
	if !versionPattern.MatchString(target) {
		return PreUpgradeReport{}, ErrInvalidVersion
	}
	rep := PreUpgradeReport{CurrentVersion: current, TargetVersion: target, DatabaseDriver: s.d.DatabaseDriver}
	diag, err := s.Diagnostics(ctx, []string{CheckDataDir, CheckDocker, CheckEncryptionKey, CheckBackupKey})
	if err != nil {
		return PreUpgradeReport{}, err
	}
	rep.Checks = append(rep.Checks, s.targetMigrations(ctx))
	rep.Checks = append(rep.Checks, diag.Checks...)
	compat, check := s.pluginCompat(ctx, target)
	rep.Plugins = compat
	rep.Checks = append(rep.Checks, check, s.busy(ctx))
	return rep, nil
}

// targetMigrations says what can be known: this build's ledger. The target image's
// migration set is not knowable until it is pulled, and the result says so explicitly.
func (s *Service) targetMigrations(ctx context.Context) CheckResult {
	res := s.checkMigrations(ctx)
	res.ID = CheckTargetMigrations
	if res.Status == StatusPass {
		res.Status, res.Reason = StatusSkip, "target_unknown"
	}
	return res
}

func (s *Service) pluginCompat(ctx context.Context, target string) ([]PluginCompat, CheckResult) {
	res := CheckResult{ID: CheckPluginCompat}
	plugins, err := s.installedPlugins(ctx)
	if err != nil {
		s.d.Logger.Warn("support: plugin compatibility", "error", err)
		res.Status, res.Reason = StatusWarn, "unreadable"
		return []PluginCompat{}, res
	}
	out := make([]PluginCompat, 0, len(plugins))
	var bad, unknown []string
	for _, p := range plugins {
		c := PluginCompat{Slug: p.Slug, Version: p.Version, Compatible: CompatUnknown}
		rng, err := s.manifestRange(p)
		if err == nil {
			c.Range = rng
			if ok, readable := RangeAllows(rng, target); readable {
				c.Compatible = map[bool]string{true: CompatYes, false: CompatNo}[ok]
			}
		}
		switch c.Compatible {
		case CompatNo:
			bad = append(bad, p.Slug)
		case CompatUnknown:
			unknown = append(unknown, p.Slug)
		}
		out = append(out, c)
	}
	res.Details = map[string]string{"installed": strconv.Itoa(len(plugins))}
	switch {
	case len(bad) > 0:
		res.Status, res.Reason = StatusWarn, "incompatible"
		res.Details["plugins"] = strings.Join(bad, ", ")
	case len(unknown) > 0:
		res.Status, res.Reason = StatusWarn, "unknown"
		res.Details["plugins"] = strings.Join(unknown, ", ")
	case len(plugins) == 0:
		res.Status, res.Reason = StatusSkip, "none"
	default:
		res.Status, res.Reason = StatusPass, "ok"
	}
	return out, res
}

// busy counts work an upgrade restart would kill; an unreadable count is reported as a
// warning because restarting over work that could not be ruled out is the risk itself.
func (s *Service) busy(ctx context.Context) CheckResult {
	res := CheckResult{ID: CheckBusy, Details: map[string]string{}}
	running := 0
	// bounded by runningwork.Count, a fixed list of two kinds.
	for _, r := range runningwork.Count(ctx, s.d.DB) {
		key := string(r.Kind)
		if r.Err != nil {
			s.d.Logger.Warn("support: busy check", "query", key, "error", r.Err)
			res.Details[key] = "unknown"
			running++
			continue
		}
		res.Details[key] = strconv.Itoa(r.Count)
		running += r.Count
	}
	s.mu.Lock()
	if len(s.running) > 0 {
		res.Details["supportBundles"] = strconv.Itoa(len(s.running))
		running += len(s.running)
	}
	s.mu.Unlock()
	if running > 0 {
		res.Status, res.Reason = StatusWarn, "running"
		return res
	}
	res.Status, res.Reason = StatusPass, "idle"
	return res
}
