# McHarbor sanity scripts

Terminal-runnable versions of the checks described in `.instructions/sanity/`.
One folder per check, one `check.ps1` per folder, one runner for all of them.

```
scripts/sanity/
  run-all.ps1              runs every check, prints a summary, writes <stamp>-summary.md
  run-all.cmd              cmd.exe / double-click shim for run-all.ps1
  run-all.sh               Git Bash / WSL shim for run-all.ps1
  _lib/common.ps1          shared helpers (file walker, regex search, report writer)
  <check>/check.ps1        one script per .instructions/sanity/<check>.md
```

## Requirements

- Windows PowerShell 5.1 or PowerShell 7. Nothing else for the grep-style checks.
- `npm` and `go` on the PATH for the `deprecated` and `dependencies` checks (they call
  `npm outdated`, `npm audit`, `go list -m -u`). Pass `-SkipNetwork` to skip those.
- `govulncheck` is optional; when missing, that sub-check is reported as skipped.

No ripgrep. The instruction files quote `rg` commands; the scripts implement the same
regexes with .NET and record the original `rg` command in each report for reference.

## Usage

```powershell
# everything
.\scripts\sanity\run-all.ps1

# a subset
.\scripts\sanity\run-all.ps1 -Check security,permissions,missing-api

# offline / CI without registry access
.\scripts\sanity\run-all.ps1 -SkipNetwork

# treat "review" verdicts as failures too
.\scripts\sanity\run-all.ps1 -FailOn review

# list checks
.\scripts\sanity\run-all.ps1 -List

# one check on its own
.\scripts\sanity\i18n\check.ps1
.\scripts\sanity\deprecated\check.ps1 -SkipNetwork
```

From cmd.exe: `scripts\sanity\run-all.cmd -Check copyright`.
From Git Bash: `./scripts/sanity/run-all.sh -SkipNetwork`.

Common parameters on every script: `-Root <repo>` (auto-detected), `-OutDir` (default
`.results/sanity`), `-Stamp` (default `yyyy-MM-dd-HHmm`, shared across a suite run),
`-MaxHits` (hits listed per check in the report, default 150).

## Output

Reports go to `.results/sanity/<stamp>-<check>.md` (the layout `.instructions/sanity/
README.md` asks for: check name, date and scope, summary table, findings with file:line
and remediation, notes) plus `.results/sanity/<stamp>-summary.md` for the suite.

## Verdicts

| Verdict   | Meaning |
|-----------|---------|
| `pass`    | no hits |
| `fail`    | the check is unambiguous (`strict` mode) and found hits |
| `review`  | hits that need a human to confirm or dismiss (`review` mode) |
| `info`    | inventory only, never affects the exit code |
| `skipped` | a prerequisite was missing (tool, file, network) |

A check's status is `fail` if any of its sub-checks failed, else `review` if any need
review, else `pass`. The runner exits 1 on any `fail` (or on `review` with
`-FailOn review`) and 2 when a check script crashed.

## Checks

| Folder | Instruction file | Network |
|---|---|---|
| security | security.md (delegates check 11 to `scripts/old/audit_coverage_check.ps1`) | no |
| performance | performance.md | no |
| code-quality | code-quality.md | no |
| modularity | modularity.md (honours `baselines/oversized-components.txt`) | no |
| ui-consistency | ui-consistency.md | no |
| error-visualization | error-visualization.md (use `-Plugins <slug>` to scope the plugin sweep) | no |
| copyright | copyright.md | no |
| code-comments | code-comments.md (use `-Changed` / `-Since <ref>` to scope to your diff) | no |
| i18n | i18n.md (locale key parity and t() call-site resolution implemented natively, no node needed) | no |
| i18n-untranslated | i18n-untranslated.md (honours `baselines/i18n-untranslated.txt`) | no |
| go-standards | go-standards.md | no |
| react-standards | react-standards.md | no |
| deprecated | deprecated.md | yes (checks 2-4) |
| dependencies | dependencies.md | yes |
| app-catalog | app-catalog.md | no |
| orphaned-backend | orphaned-backend.md | no |
| orphaned-frontend | orphaned-frontend.md | no |
| missing-api | missing-api.md | no |
| permissions | permissions.md | no |
| plugin-host-coupling | plugin-host-coupling.md | no |
| plugin-isolation | plugin-isolation.md | no |
| duplication | duplication.md (delegates detection to `duplication/detect.mjs`; needs node on PATH) | no |

## Adding a check

1. Create `scripts/sanity/<name>/check.ps1` by copying an existing one.
2. Dot-source `_lib/common.ps1`, build a report with `New-SanityReport`, add
   sub-checks with `Add-SanityCheck` (choose `-Mode strict|review|info`), and finish
   with `Complete-SanityCheck`.
3. The runner discovers the folder automatically; add it to `$order` in `run-all.ps1`
   if it should run in a specific position.

The scripts are read-only. They never modify project files; the only side effect is
the report directory (and, for the security check, the audit-coverage artefacts that
`scripts/old/audit_coverage_check.ps1` already writes).
