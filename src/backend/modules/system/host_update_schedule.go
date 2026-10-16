// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package system

import (
	"context"
	"database/sql"
	"github.com/therealmcsparrow/mcharbor/internal/closeutil"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// weekDays maps Go's weekday names onto the three-letter codes the schedule
// columns store, which is the same vocabulary the Docker auto-update window
// already uses.
var weekDays = map[time.Weekday]string{
	time.Monday:    "mon",
	time.Tuesday:   "tue",
	time.Wednesday: "wed",
	time.Thursday:  "thu",
	time.Friday:    "fri",
	time.Saturday:  "sat",
	time.Sunday:    "sun",
}

// scheduleTick is how often the scheduler looks for a due environment. A check
// is scheduled to the minute, so anything finer would only burn wakeups.
const scheduleTick = time.Minute

// parseUptimeSeconds turns /proc/uptime's float into whole seconds.
func parseUptimeSeconds(value string) int64 {
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil {
		return 0
	}
	return int64(seconds)
}

// splitDays turns the stored comma list into the array the API returns.
func splitDays(stored string) []string {
	days := make([]string, 0, 7)
	for _, part := range strings.Split(stored, ",") {
		trimmed := strings.ToLower(strings.TrimSpace(part))
		if _, ok := dayCodes[trimmed]; ok {
			days = append(days, trimmed)
		}
	}
	return days
}

// dayCodes is the closed set of day codes a schedule may carry.
var dayCodes = map[string]bool{
	"mon": true, "tue": true, "wed": true, "thu": true,
	"fri": true, "sat": true, "sun": true,
}

// normaliseDays validates and canonicalises the day list, dropping anything the
// caller invented and keeping calendar order rather than submission order.
func normaliseDays(days []string) string {
	seen := make(map[string]bool, len(days))
	for _, day := range days {
		trimmed := strings.ToLower(strings.TrimSpace(day))
		if dayCodes[trimmed] {
			seen[trimmed] = true
		}
	}
	ordered := make([]string, 0, 7)
	for _, day := range []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"} {
		if seen[day] {
			ordered = append(ordered, day)
		}
	}
	return strings.Join(ordered, ",")
}

// validClockTime accepts only a 24-hour HH:MM, because the value is compared
// against the server clock as a string and a loose format would never match.
func validClockTime(value string) bool {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return false
	}
	return parsed.Format("15:04") == strings.TrimSpace(value)
}

// StartScheduler runs the automatic host update check until ctx ends.
//
// It is minute-granular and deliberately stateless beyond the stored
// last-checked stamp: an environment is due when today is one of its days, the
// clock has reached its time, and it has not already been checked since that
// time today. A missed window (server down, agent offline) is simply picked up
// on the next tick within the same day.
func (s *HostUpdateService) StartScheduler(ctx context.Context) {
	s.startScheduler(ctx)
}

// hostUpdateLeader, when set, keeps scheduled host updates on the elected node so a
// multi-node install does not run the same package upgrade twice.
var hostUpdateLeader atomic.Pointer[func(context.Context) bool]

// SetHostUpdateLeader installs the leader check for the host-update scheduler.
func SetHostUpdateLeader(fn func(context.Context) bool) {
	if fn == nil {
		hostUpdateLeader.Store(nil)
		return
	}
	hostUpdateLeader.Store(&fn)
}

func (s *HostUpdateService) startScheduler(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(scheduleTick)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.PruneJobs()
				if lead := hostUpdateLeader.Load(); lead != nil && !(*lead)(ctx) {
					continue
				}
				s.runDueChecks(ctx)
			}
		}
	}()
}

// runDueChecks checks every environment whose schedule has come around.
func (s *HostUpdateService) runDueChecks(ctx context.Context) {
	now := time.Now()
	today := weekDays[now.Weekday()]

	rows, err := s.database.QueryContext(ctx, `
		SELECT id, host_update_check_time, host_update_check_days, host_update_last_checked_at, 0
		FROM environments
		WHERE host_update_check_enabled = 1
		UNION ALL
		SELECT id, host_update_check_time, host_update_check_days, host_update_last_checked_at, 1
		FROM host_update_self
		WHERE host_update_check_enabled = 1
		LIMIT 1000`)
	if err != nil {
		s.logger.Error("system: reading host update schedules failed", "error", err)
		return
	}
	defer func() { closeutil.Report(rows.Close(), "rows") }()

	type due struct{ target HostTarget }
	pending := make([]due, 0, 8)
	for rows.Next() {
		var envID, checkTime, checkDays string
		var lastChecked sql.NullString
		var self int
		if err := rows.Scan(&envID, &checkTime, &checkDays, &lastChecked, &self); err != nil {
			s.logger.Error("system: reading a host update schedule failed", "error", err)
			continue
		}
		if !strings.Contains(checkDays, today) {
			continue
		}
		if !dueNow(now, checkTime, lastChecked.String) {
			continue
		}
		target := EnvTarget(envID)
		if self == 1 {
			target = SelfTarget()
		}
		pending = append(pending, due{target: target})
	}
	if err := rows.Err(); err != nil {
		s.logger.Error("system: iterating host update schedules failed", "error", err)
		return
	}

	for _, entry := range pending {
		checkCtx, cancel := context.WithTimeout(ctx, hostUpdateCheckTimeout)
		if _, err := s.Check(checkCtx, entry.target); err != nil {
			s.logger.Warn("system: scheduled host update check failed",
				"target", entry.target.rowID(), "error", err)
		}
		cancel()
	}
}

// dueNow reports whether the scheduled minute has passed today and no check has
// run since it.
func dueNow(now time.Time, checkTime, lastChecked string) bool {
	scheduled, err := time.Parse("15:04", strings.TrimSpace(checkTime))
	if err != nil {
		return false
	}
	target := time.Date(now.Year(), now.Month(), now.Day(),
		scheduled.Hour(), scheduled.Minute(), 0, 0, now.Location())
	if now.Before(target) {
		return false
	}
	if lastChecked == "" {
		return true
	}
	previous, err := time.Parse(time.RFC3339, lastChecked)
	if err != nil {
		return true
	}
	return previous.Before(target)
}

// hostOSInfoScript is the local-environment twin of the agent's probe: the same
// KEY=VALUE contract, wrapped in the chroot the utility container needs.
const hostOSInfoScript = `chroot /host sh -lc '
. /etc/os-release 2>/dev/null
echo MCHARBOR_PRETTY="$PRETTY_NAME"
echo MCHARBOR_DISTRO="$ID"
echo MCHARBOR_VERSION="$VERSION_ID"
echo MCHARBOR_KERNEL="$(uname -r 2>/dev/null)"
echo MCHARBOR_ARCH="$(uname -m 2>/dev/null)"
echo MCHARBOR_HOSTNAME="$(hostname 2>/dev/null)"
echo MCHARBOR_UPTIME="$(cut -d" " -f1 /proc/uptime 2>/dev/null)"
echo MCHARBOR_VIRT="$(systemd-detect-virt 2>/dev/null || echo none)"
if command -v apt-get >/dev/null 2>&1; then echo MCHARBOR_MANAGER=apt
elif command -v dnf >/dev/null 2>&1; then echo MCHARBOR_MANAGER=dnf
elif command -v yum >/dev/null 2>&1; then echo MCHARBOR_MANAGER=yum
elif command -v apk >/dev/null 2>&1; then echo MCHARBOR_MANAGER=apk
elif command -v pacman >/dev/null 2>&1; then echo MCHARBOR_MANAGER=pacman
elif command -v zypper >/dev/null 2>&1; then echo MCHARBOR_MANAGER=zypper
else echo MCHARBOR_MANAGER=unknown
fi
if [ -f /var/run/reboot-required ] || [ -f /run/reboot-required ]; then echo MCHARBOR_REBOOT=1
else echo MCHARBOR_REBOOT=0
fi
'`
