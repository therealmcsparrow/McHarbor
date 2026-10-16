// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

package main

import "strings"

// hostUpgradeRetryNotice is the line an upgrade prints when it refreshed the
// package metadata after a download failure and is about to try once more.
const hostUpgradeRetryNotice = "McHarbor: package lists refreshed, retrying once"

// hostUpgradeOptions is how far one upgrade is allowed to go. It mirrors the
// generator in the server's modules/system, which drives local hosts.
type hostUpgradeOptions struct {
	WithNewPackages bool
	SkipUnavailable bool
	RemoveUnused    bool
}

// hostUpgradePlan is how one package manager upgrades and how it recovers from
// a failed download: retryPattern is matched against the upgrade's own output,
// so a dependency conflict, a held lock or a full disk never triggers a retry.
// autoremove is empty for a manager with no safe way to drop unused packages.
type hostUpgradePlan struct {
	manager      string
	probe        string
	refresh      string
	upgrade      string
	retryPattern string
	retryRefresh string
	retryUpgrade string
	autoremove   string
}

// hostUpgradePlans returns one plan per supported manager, in probe order.
//
// Skip unavailable maps onto the manager's own "carry on without it" switch
// where one exists (apt, dnf, yum); zypper, apk and pacman have no safe
// equivalent, so the option is ignored there rather than approximated. Remove
// unused packages follows the same split: apt, dnf, yum and pacman have one,
// apk and zypper do not.
func hostUpgradePlans(opts hostUpgradeOptions) []hostUpgradePlan {
	aptFlags := ""
	if opts.WithNewPackages {
		aptFlags += "--with-new-pkgs "
	}
	if opts.SkipUnavailable {
		aptFlags += "--fix-missing "
	}
	dnfFlags, yumFlags := "", ""
	if opts.SkipUnavailable {
		dnfFlags = "--skip-broken --setopt=skip_if_unavailable=True "
		yumFlags = "--skip-broken "
	}
	aptUpgrade := "env DEBIAN_FRONTEND=noninteractive apt-get -y " + aptFlags + "upgrade"
	return []hostUpgradePlan{
		{
			manager: "apt", probe: "apt-get",
			refresh:      "apt-get update",
			upgrade:      aptUpgrade,
			retryPattern: "Unable to fetch some archives|E: Failed to fetch",
			retryRefresh: "apt-get update",
			retryUpgrade: aptUpgrade,
			autoremove:   "env DEBIAN_FRONTEND=noninteractive apt-get -y autoremove",
		},
		{
			manager: "dnf", probe: "dnf",
			upgrade:      "dnf -y " + dnfFlags + "upgrade",
			retryPattern: "Failed to download|Cannot download|Error downloading packages|Curl error|Status code: 404",
			retryRefresh: "dnf clean metadata",
			retryUpgrade: "dnf -y --refresh " + dnfFlags + "upgrade",
			autoremove:   "dnf -y autoremove",
		},
		{
			manager: "yum", probe: "yum",
			upgrade:      "yum -y " + yumFlags + "update",
			retryPattern: "Failed to download|Cannot download|Error downloading packages|Curl error|Status code: 404|No more mirrors to try",
			retryRefresh: "yum clean metadata",
			retryUpgrade: "yum -y " + yumFlags + "update",
			autoremove:   "yum -y autoremove",
		},
		{
			manager: "apk", probe: "apk",
			refresh:      "apk update",
			upgrade:      "apk upgrade",
			retryPattern: "fetch .*failed|ERROR: .+: (unable to|temporary error|network error)",
			retryRefresh: "apk update",
			retryUpgrade: "apk upgrade",
		},
		{
			manager: "pacman", probe: "pacman",
			upgrade:      "pacman -Syu --noconfirm",
			retryPattern: "failed retrieving file|failed to retrieve some files",
			retryRefresh: "pacman -Syy --noconfirm",
			retryUpgrade: "pacman -Su --noconfirm",
			autoremove:   "mch_orphans=$(pacman -Qdtq 2>/dev/null); if [ -n \"$mch_orphans\" ]; then pacman -Rns --noconfirm $mch_orphans; fi",
		},
		{
			manager: "zypper", probe: "zypper",
			refresh:      "zypper --non-interactive refresh",
			upgrade:      "zypper --non-interactive update",
			retryPattern: `Download \(curl\) error|File .* not found on medium`,
			retryRefresh: "zypper --non-interactive refresh --force",
			retryUpgrade: "zypper --non-interactive update",
		},
	}
}

// hostRemoveUnusedNotice and hostRemoveUnusedSkipNotice are the lines the script
// prints before the cleanup, or instead of it when the manager has none.
const (
	hostRemoveUnusedNotice     = "McHarbor: removing unused packages"
	hostRemoveUnusedSkipNotice = "McHarbor: this package manager has no safe way to remove unused packages, skipping"
)

// hostAutoremoveStep is the script fragment that cleans up after an upgrade
// that exited 0, so a failed upgrade never also removes packages. A failing
// cleanup leaves its own exit code in mch_rc, which the script then returns.
func hostAutoremoveStep(plan hostUpgradePlan) string {
	if plan.autoremove == "" {
		return "  if [ \"$mch_rc\" -eq 0 ]; then\n    echo \"" + hostRemoveUnusedSkipNotice + "\"\n  fi\n"
	}
	return "  if [ \"$mch_rc\" -eq 0 ]; then\n" +
		"    echo \"" + hostRemoveUnusedNotice + "\"\n" +
		"    " + plan.autoremove + "\n" +
		"    mch_rc=$?\n" +
		"  fi\n"
}

// hostUpgradeBody is the shell program that upgrades the host. It is wrapped in
// single quotes by the caller, so it must never contain one.
//
// The upgrade's output is teed to a temp file so a failure can be classified
// after the fact while the operator still watches every line live.
func hostUpgradeBody(opts hostUpgradeOptions) string {
	var b strings.Builder
	b.WriteString(`
mch_log=$(mktemp 2>/dev/null || echo /tmp/mcharbor-host-update.$$)
trap "rm -f $mch_log $mch_log.rc" EXIT
mch_try() {
  { "$@"; echo $? > "$mch_log.rc"; } 2>&1 | tee "$mch_log"
  mch_rc=$(cat "$mch_log.rc" 2>/dev/null || echo 1)
}
`)
	for i, plan := range hostUpgradePlans(opts) {
		if i == 0 {
			b.WriteString("if ")
		} else {
			b.WriteString("elif ")
		}
		b.WriteString("command -v " + plan.probe + " >/dev/null 2>&1; then\n")
		b.WriteString("  echo MCHARBOR_MANAGER=" + plan.manager + "\n")
		if plan.refresh != "" {
			b.WriteString("  " + plan.refresh + " || exit $?\n")
		}
		b.WriteString("  mch_try " + plan.upgrade + "\n")
		b.WriteString(`  if [ "$mch_rc" -ne 0 ] && grep -Eq "` + plan.retryPattern + `" "$mch_log"; then` + "\n")
		b.WriteString("    echo \"McHarbor: a download failed, refreshing package lists\"\n")
		b.WriteString("    if " + plan.retryRefresh + "; then\n")
		b.WriteString("      echo \"" + hostUpgradeRetryNotice + "\"\n")
		b.WriteString("      mch_try " + plan.retryUpgrade + "\n")
		b.WriteString("    fi\n")
		b.WriteString("  fi\n")
		if opts.RemoveUnused {
			b.WriteString(hostAutoremoveStep(plan))
		}
		b.WriteString("  exit \"$mch_rc\"\n")
	}
	b.WriteString("else\n")
	b.WriteString("  echo MCHARBOR_MANAGER=unknown\n")
	b.WriteString("  echo \"no supported package manager found\" >&2\n")
	b.WriteString("  exit 1\n")
	b.WriteString("fi\n")
	return b.String()
}
