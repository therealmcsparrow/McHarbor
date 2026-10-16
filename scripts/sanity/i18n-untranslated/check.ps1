# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  i18n translation-coverage sanity check. Mirrors .instructions/sanity/i18n-untranslated.md.
.DESCRIPTION
  The `i18n` check proves a key EXISTS. This check proves it was TRANSLATED.

  A locale bundle that is a verbatim copy of en/ passes every key-resolution
  check in the suite and still ships an English UI to a Dutch user. Seeding a
  new key in all six locales with the English string is this repo's documented
  convention - which is exactly why the untranslated remainder has to be
  measurable instead of invisible.

  Compares every leaf value in each non-en locale against en and reports:
  whole namespace files copied verbatim, multi-word values identical to
  English, short identical values (cognate candidates), empty values, keys
  missing outright, and {{interpolation}} / <0> placeholder drift in strings
  that WERE translated.

  Scans four bundle shapes: src/frontend/core/i18n/locales/<lang>/<ns>.json,
  widgets/*/i18n/<lang>.json, nodes/*/i18n/<lang>.json and
  src/plugins/Plugins/*/i18n/<lang>/<ns>.json. -Paths scopes to a subset the
  same way the other checks do (Get-SanityScope).

  Pure PowerShell 5.1, read-only, no node required.
.EXAMPLE
  .\scripts\sanity\i18n-untranslated\check.ps1
.EXAMPLE
  .\scripts\sanity\i18n-untranslated\check.ps1 -Locale nl,de -MaxHits 500
.EXAMPLE
  .\scripts\sanity\i18n-untranslated\check.ps1 -Paths widgets
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [string[]]$Locale,
  [string[]]$Paths,
  [string]$Ratchet,
  [switch]$WriteRatchet
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
$ratchetMode = [bool]$Ratchet
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$report = New-SanityReport -Name 'i18n-untranslated' -Title 'i18n Translation Coverage Sanity' `
  -Scope 'src/frontend/core/i18n/locales, widgets/*/i18n, nodes/*/i18n, src/plugins/Plugins/*/i18n' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$localesRoot = Join-Path $Root 'src\frontend\core\i18n\locales'
$localesRel = 'src/frontend/core/i18n/locales'
$feGroupRel = $localesRel

if (-not (Test-Path -LiteralPath $localesRoot)) {
  Add-SanityCheck -Report $report -Id 1 -Title 'Locale bundles present' -Skipped -SkipReason "$localesRel not found"
  $summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
  $summary
  exit 0
}

# ---------------------------------------------------------------------------
# Supported locales (read from i18n.ts so the check follows the app, not a copy)
# ---------------------------------------------------------------------------
$supported = @('en', 'nl', 'de', 'es', 'fr', 'pt')
$i18nTs = Join-Path $Root 'src\frontend\core\i18n\i18n.ts'
if (Test-Path -LiteralPath $i18nTs) {
  $m = [regex]::Match((Get-SanityText $i18nTs), 'supportedLanguages\s*=\s*\[([^\]]+)\]')
  if ($m.Success) { $supported = @([regex]::Matches($m.Groups[1].Value, '"([^"]+)"') | ForEach-Object { $_.Groups[1].Value }) }
}
$targets = @($supported | Where-Object { $_ -ne 'en' })
if ($Locale -and $Locale.Count) {
  $wanted = @()
  foreach ($l in $Locale) { foreach ($p in ($l -split ',')) { if ($p.Trim()) { $wanted += $p.Trim() } } }
  $targets = @($targets | Where-Object { $wanted -contains $_ })
}

# ---------------------------------------------------------------------------
# Bundle groups (four shapes; -Paths narrows which ones are scanned)
# ---------------------------------------------------------------------------
function Test-SanityAreaIncluded {
  param([string[]]$Areas, [string]$GroupRel)
  $g = $GroupRel.ToLowerInvariant()
  foreach ($a in $Areas) {
    $al = $a.ToLowerInvariant()
    if ($g -eq $al -or $g.StartsWith($al + '/') -or $al.StartsWith($g + '/')) { return $true }
  }
  return $false
}

$scopeAreas = (Get-SanityScope -Root $Root -Paths $Paths -Default @($feGroupRel, 'widgets', 'nodes', 'src/plugins'))
$groups = New-Object System.Collections.Generic.List[object]

if (Test-SanityAreaIncluded -Areas $scopeAreas -GroupRel $feGroupRel) {
  $groups.Add([pscustomobject]@{ Label = $feGroupRel; Shape = 'nested'; EnDir = (Join-Path $localesRoot 'en') })
}

$widgetsRoot = Join-Path $Root 'widgets'
if (Test-Path -LiteralPath $widgetsRoot) {
  foreach ($d in (Get-ChildItem -LiteralPath $widgetsRoot -Directory -ErrorAction SilentlyContinue)) {
    $groupRel = "widgets/$($d.Name)/i18n"
    if (-not (Test-SanityAreaIncluded -Areas $scopeAreas -GroupRel $groupRel)) { continue }
    $enFile = Join-Path $d.FullName 'i18n\en.json'
    if (Test-Path -LiteralPath $enFile) { $groups.Add([pscustomobject]@{ Label = $groupRel; Shape = 'flat'; EnFile = $enFile }) }
  }
}

$nodesRoot = Join-Path $Root 'nodes'
if (Test-Path -LiteralPath $nodesRoot) {
  foreach ($d in (Get-ChildItem -LiteralPath $nodesRoot -Directory -ErrorAction SilentlyContinue)) {
    $groupRel = "nodes/$($d.Name)/i18n"
    if (-not (Test-SanityAreaIncluded -Areas $scopeAreas -GroupRel $groupRel)) { continue }
    $enFile = Join-Path $d.FullName 'i18n\en.json'
    if (Test-Path -LiteralPath $enFile) { $groups.Add([pscustomobject]@{ Label = $groupRel; Shape = 'flat'; EnFile = $enFile }) }
  }
}

$pluginsRoot = Join-Path $Root 'src\plugins\Plugins'
if (-not $ratchetMode -and (Test-Path -LiteralPath $pluginsRoot)) {
  foreach ($d in (Get-ChildItem -LiteralPath $pluginsRoot -Directory -ErrorAction SilentlyContinue)) {
    $groupRel = "src/plugins/Plugins/$($d.Name)/i18n"
    if (-not (Test-SanityAreaIncluded -Areas $scopeAreas -GroupRel $groupRel)) { continue }
    $enDir = Join-Path $d.FullName 'i18n\en'
    if (Test-Path -LiteralPath $enDir) { $groups.Add([pscustomobject]@{ Label = $groupRel; Shape = 'nested'; EnDir = $enDir }) }
  }
}

$totalNsUnits = 0
foreach ($grp in $groups) {
  if ($grp.Shape -eq 'nested') { $totalNsUnits += @(Get-ChildItem -LiteralPath $grp.EnDir -Filter *.json -ErrorAction SilentlyContinue).Count }
  else { $totalNsUnits += 1 }
}

# ---------------------------------------------------------------------------
# Values that are legitimately identical to English
# ---------------------------------------------------------------------------

# Brand names, protocol acronyms and API machine values: a Dutch UI says
# "Docker", "WebSocket" and "ClusterIP" exactly like an English one.
# Subset of .instructions/donottranslate.md, kept in sync with i18n/check.ps1.
$dntWords = @(
  'McHarbor', 'McSparrow', 'mcharbor', 'Docker', 'Dockerfile', 'Kubernetes', 'Podman', 'Swarm', 'Compose',
  'SQLite', 'JSON', 'YAML', 'TOML', 'XML', 'HTTP', 'HTTPS', 'TCP', 'UDP', 'SCTP', 'SSH', 'TLS', 'SSL', 'mTLS',
  'API', 'URL', 'URI', 'ID', 'UUID', 'CPU', 'RAM', 'GPU', 'OK', 'PID', 'UID', 'GID',
  'ClusterIP', 'NodePort', 'LoadBalancer', 'ExternalName', 'ReplicaSet', 'DaemonSet', 'StatefulSet', 'CronJob',
  'Ingress', 'ConfigMap', 'Kubelet', 'Helm', 'kubeconfig', 'RBAC', 'ABAC', 'SCIM', 'OpenAPI',
  'Running', 'Pending', 'Succeeded', 'Failed', 'Unknown', 'Available', 'Progressing', 'ReplicaFailure',
  'GitHub', 'GitLab', 'Gitea', 'Git', 'GitOps', 'Grafana', 'Prometheus', 'Loki', 'Traefik', 'Nginx', 'Apache',
  'Postgres', 'PostgreSQL', 'MySQL', 'MariaDB', 'Redis', 'MongoDB', 'Elasticsearch', 'RabbitMQ', 'Kafka',
  'Slack', 'Teams', 'Discord', 'Telegram', 'Signal', 'WhatsApp', 'Mattermost', 'SMTP', 'IMAP',
  'OIDC', 'SAML', 'LDAP', 'Entra', 'Okta', 'Keycloak', 'Authelia', 'Authentik', 'OAuth', 'TOTP', 'MFA',
  'CSV', 'PDF', 'SVG', 'PNG', 'JPEG', 'CLI', 'SDK', 'UI', 'UX', 'WebSocket', 'SSE', 'CIDR', 'IPv4', 'IPv6',
  'IP', 'DNS', 'NAT', 'VPN', 'VLAN', 'MAC', 'AES', 'RSA', 'SHA', 'Argon2', 'Argon2id', 'bcrypt', 'xid',
  'Vite', 'React', 'Golang', 'Tailwind', 'Radix', 'TanStack', 'Zustand', 'Ladle', 'npm', 'Nodejs',
  'Linux', 'Windows', 'macOS', 'Alpine', 'Ubuntu', 'Debian', 'WSL', 'ARM', 'amd64', 'arm64',
  'AWS', 'Azure', 'GCP', 'Portainer', 'Dockhand', 'Nomad', 'Terraform', 'Ansible',
  'CRUD', 'REST', 'gRPC', 'GraphQL', 'JWT', 'CORS', 'CSRF', 'XSS', 'SQL', 'Regex', 'Cron'
)
$dntSet = @{}
foreach ($w in $dntWords) { $dntSet[$w.ToLowerInvariant()] = $true }

$apos = [string][char]0x2019
$wordSplit = "[^\p{L}\p{M}'$apos-]+"

# Placeholder text a translator (or a seeding script) left behind. These are
# neither English nor the target language: "{{lang missing: Show {{field}}}}"
# renders the literal braces in the UI, and the value is not identical to en,
# so every identical-value comparison in this file walks straight past it.
#
# Two regexes, because the case sensitivity differs. "{{lang missing:" is a
# phrase and is matched loosely; the TODO family must be matched case-SENSITIVE
# or the Spanish and Portuguese word "Todo" ("all", "every") lands in the report
# 11 times - dateRange.allTime, labels.all, schedule.readable.everyMonthAt and
# friends are perfectly good translations.
$markerLooseRx = New-Object System.Text.RegularExpressions.Regex('\{\{\s*lang\s+missing\s*:|\[\s*untranslated\s*\]', [System.Text.RegularExpressions.RegexOptions]::IgnoreCase)
$markerStrictRx = New-Object System.Text.RegularExpressions.Regex('\{\{\s*(TODO|FIXME|XXX|UNTRANSLATED)\b|^\s*(TODO|FIXME|XXX|UNTRANSLATED)\s*[:\-]')

# A leaf value is "not really a sentence" - and therefore allowed to be
# identical - when nothing translatable survives the strip below.
function Get-SanityTranslatableWords {
  param([string]$Value)
  if ($Value -match '^\s*[\{\[].*[:"].*[\}\]]\s*$') { return @() }   # JSON/code sample, e.g. {"key": "value"}
  $v = [regex]::Replace($Value, '(\{\{[^}]*\}\}|[0-9]+([.,][0-9]+)?)\s?(ms|kib|mib|gib|kb|mb|gb|tb|px|bps)\b|(\{\{[^}]*\}\}|[0-9]+([.,][0-9]+)?)(s|m|h|d|b|%)(?![\p{L}])', ' ')   # attached unit: {{ms}}ms, 5s
  $v = [regex]::Replace($v, '\{\{[^}]*\}\}', ' ')      # {{count}}, {{name, number}}
  $v = [regex]::Replace($v, '\$t\([^)]*\)', ' ')           # nested $t(ns:key)
  $v = [regex]::Replace($v, '<[^>]*>', ' ')                # <0>, <strong>
  $v = [regex]::Replace($v, '[0-9]+([.,][0-9]+)?', ' ')
  $words = New-Object System.Collections.Generic.List[string]
  foreach ($tok in [regex]::Split($v, $wordSplit)) {
    $t = $tok.Trim(@('-', "'", [char]0x2019))
    if ($t.Length -lt 2) { continue }                       # "a", single letters, stray punctuation
    if ($dntSet.ContainsKey($t.ToLowerInvariant())) { continue }
    if ($t -cmatch '^[A-Z0-9]{2,}$') { continue }           # bare acronym: TTL, CPU, HTTP
    if ($t -cmatch '^[a-z]+[A-Z]') { continue }             # lowerCamel identifier
    $words.Add($t)
  }
  return $words.ToArray()
}

# ---------------------------------------------------------------------------
# Baseline: values frozen as legitimately identical ("locale|namespace|key")
# ---------------------------------------------------------------------------
$baselinePath = Join-Path $Root '.instructions\sanity\baselines\i18n-untranslated.txt'
$baseline = @{}
$baselineCount = 0
if (Test-Path -LiteralPath $baselinePath) {
  foreach ($line in (Get-SanityLines $baselinePath)) {
    $l = $line.Trim([char]0xFEFF).Trim()
    if (-not $l -or $l.StartsWith('#')) { continue }
    $baseline[$l.ToLowerInvariant()] = $true
    $baselineCount++
  }
}

# ---------------------------------------------------------------------------
# Load bundles
# ---------------------------------------------------------------------------
function Get-SanityLeafMap {
  param([string]$Path)
  $map = New-Object 'System.Collections.Generic.Dictionary[string,object]'
  if (-not (Test-Path -LiteralPath $Path)) { return , $map }
  $text = Get-SanityText $Path
  if ([string]::IsNullOrWhiteSpace($text)) { return , $map }
  try { $obj = ConvertFrom-SanityJson $text } catch { return , $map }
  foreach ($k in (Get-SanityFlatKeys -Obj $obj)) {
    $val = $obj
    $ok = $true
    foreach ($part in $k.Split('.')) { $val = Get-SanityJsonValue $val $part; if ($null -eq $val) { $ok = $false; break } }
    if ($ok) { $map[$k] = $val }
  }
  return , $map
}

# ---------------------------------------------------------------------------
# Sweep
# ---------------------------------------------------------------------------
$copiedFiles = @()       # 1. whole namespace verbatim
$identicalLong = @()     # 2. multi-word identical
$identicalShort = @()    # 3. one/two-word identical (cognate candidates)
$machineIdentical = @()  # 4. identical but purely machine/brand (inventory)
$emptyValues = @()       # 5. empty in the locale, non-empty in en
$markerValues = @()      # 6. placeholder marker left in the value
$missingKeys = @()       # 7. key absent from the locale entirely
$placeholderDrift = @()  # 8. translated but interpolation drifted
$rows = @()              # 9. coverage table
$identKeys = @{}
$enKeysByNs = @{}
$nsRows = @()            # 10. per-namespace breakdown
$wordsCache = @{}

foreach ($loc in $targets) {
  $locTotal = 0; $locSame = 0; $locMissing = 0; $locEmpty = 0; $locMarker = 0; $locFrozen = 0

  foreach ($grp in $groups) {
    if ($grp.Shape -eq 'nested') {
      $nsFilesGrp = @(Get-ChildItem -LiteralPath $grp.EnDir -Filter *.json -ErrorAction SilentlyContinue | ForEach-Object { $_.Name } | Sort-Object)
      $localeParent = Split-Path $grp.EnDir -Parent
      $locDirGrp = Join-Path $localeParent $loc
      $locDirExists = Test-Path -LiteralPath $locDirGrp
    } else {
      $nsFilesGrp = @('en.json')
      $localeParent = Split-Path $grp.EnFile -Parent
      $locDirExists = $true
    }

    foreach ($f in $nsFilesGrp) {
      $enPath = if ($grp.Shape -eq 'nested') { Join-Path $grp.EnDir $f } else { $grp.EnFile }
      $enMap = Get-SanityLeafMap $enPath
      if ($enMap.Count -eq 0) { continue }

      $ns = if ($grp.Shape -eq 'nested') { [System.IO.Path]::GetFileNameWithoutExtension($f) } else { ($grp.Label -replace '^.*/([^/]+)/i18n$', '$1') }
      $nsQualified = if ($grp.Label -eq $feGroupRel) { $ns } else { "$($grp.Label)/$ns" }
      if (-not $enKeysByNs.ContainsKey($nsQualified)) { $enKeysByNs[$nsQualified] = @($enMap.Keys | Where-Object { $enMap[$_] -is [string] }) }
      if ($grp.Shape -eq 'nested') { $locPath = Join-Path $locDirGrp $f; $locRel = "$($grp.Label)/$loc/$f" }
      else { $locPath = Join-Path $localeParent "$loc.json"; $locRel = "$($grp.Label)/$loc.json" }

      if (-not $locDirExists -or -not (Test-Path -LiteralPath $locPath)) {
        # A whole missing namespace file/locale is the i18n check's job to flag
        # (check 6 for the frontend group). Count the strings so the coverage
        # percentage stays honest; do not re-report it here.
        $locTotal += $enMap.Count; $locMissing += $enMap.Count
        continue
      }
      $locMap = Get-SanityLeafMap $locPath

      $nsTotal = 0; $nsSame = 0; $nsReal = 0; $nsRealSame = 0
      foreach ($k in $enMap.Keys) {
        $enVal = $enMap[$k]
        if ($enVal -isnot [string]) { continue }
        $locTotal++; $nsTotal++
        $enWords = $wordsCache[$enVal]
        if ($null -eq $enWords) { $enWords = @(Get-SanityTranslatableWords $enVal); $wordsCache[$enVal] = $enWords }
        $isReal = ($enWords.Count -gt 0)
        if ($isReal) { $nsReal++ }

        if (-not $locMap.ContainsKey($k)) {
          $locMissing++
          $missingKeys += New-SanityHit -File $locRel -Line 0 -Text "$k | en: $enVal"
          continue
        }
        $locVal = $locMap[$k]
        if ($locVal -isnot [string]) { continue }

        if ([string]::IsNullOrWhiteSpace($locVal)) {
          if (-not [string]::IsNullOrWhiteSpace($enVal)) {
            $locEmpty++
            $emptyValues += New-SanityHit -File $locRel -Line 0 -Text "$k = (empty) | en: $enVal"
          }
          continue
        }

        # -ceq, not -eq: a translation that differs only in casing is still a
        # translation attempt and belongs in the drift check below, not here.
        if ($locVal -ceq $enVal) {
          $nsSame++; $locSame++
          if ($isReal) { $nsRealSame++ }
          $ik = "$loc|$nsQualified"
          if (-not $identKeys.ContainsKey($ik)) { $identKeys[$ik] = New-Object 'System.Collections.Generic.HashSet[string]' }
          [void]$identKeys[$ik].Add($k)
          if ($baseline.ContainsKey("$loc|$nsQualified|$k".ToLowerInvariant())) { $locFrozen++; continue }
          $words = $enWords
          if ($words.Count -eq 0) {
            $machineIdentical += New-SanityHit -File $locRel -Line 0 -Text "$k = $enVal"
          } elseif ($words.Count -ge 3) {
            $identicalLong += New-SanityHit -File $locRel -Line 0 -Text "$k = $enVal"
          } else {
            $identicalShort += New-SanityHit -File $locRel -Line 0 -Text "$k = $enVal"
          }
          continue
        }

        if ($markerLooseRx.IsMatch($locVal) -or $markerStrictRx.IsMatch($locVal)) {
          $locMarker++
          $markerValues += New-SanityHit -File $locRel -Line 0 -Text "$k = $locVal"
          continue
        }

        # Translated - now make sure it still interpolates.
        $enPh = @([regex]::Matches($enVal, '\{\{\s*([^}]*?)\s*\}\}') | ForEach-Object { ($_.Groups[1].Value -split ',')[0].Trim() } | Sort-Object -Unique)
        $locPh = @([regex]::Matches($locVal, '\{\{\s*([^}]*?)\s*\}\}') | ForEach-Object { ($_.Groups[1].Value -split ',')[0].Trim() } | Sort-Object -Unique)
        $lost = @($enPh | Where-Object { $locPh -notcontains $_ })
        $invented = @($locPh | Where-Object { $enPh -notcontains $_ })
        $enTags = @([regex]::Matches($enVal, '</?\d+>') | ForEach-Object { $_.Value } | Sort-Object)
        $locTags = @([regex]::Matches($locVal, '</?\d+>') | ForEach-Object { $_.Value } | Sort-Object)
        $tagDrift = (($enTags -join '') -ne ($locTags -join ''))
        if ($lost.Count -or $invented.Count -or $tagDrift) {
          $detail = @()
          if ($lost.Count) { $detail += ('lost ' + (($lost | ForEach-Object { '{{' + $_ + '}}' }) -join ', ')) }
          if ($invented.Count) { $detail += ('invented ' + (($invented | ForEach-Object { '{{' + $_ + '}}' }) -join ', ')) }
          if ($tagDrift) { $detail += ("tag drift (en: {0} / {1}: {2})" -f ($enTags -join ''), $loc, ($locTags -join '')) }
          $placeholderDrift += New-SanityHit -File $locRel -Line 0 -Text ("{0} - {1} | en: {2} | {3}: {4}" -f $k, ($detail -join '; '), $enVal, $loc, $locVal)
        }
      }

      if ($nsReal -ge 5 -and $nsRealSame -eq $nsReal) {
        $copiedFiles += New-SanityHit -File $locRel -Line 0 -Text "all $nsReal translatable string(s) identical to en ($($nsTotal - $nsReal) machine/brand string(s) not counted)"
      }
      if ($nsTotal -gt 0) {
        $nsRows += , @($loc, $nsQualified, "$nsTotal", "$nsSame", ('{0:N1}' -f (100.0 * ($nsTotal - $nsSame) / $nsTotal)))
      }
    }
  }

  $translated = $locTotal - $locSame - $locMissing - $locEmpty - $locMarker
  $pct = if ($locTotal -gt 0) { 100.0 * $translated / $locTotal } else { 0 }
  $rows += , @($loc, "$locTotal", "$translated", "$locSame", "$locMissing", "$locEmpty", "$locMarker", "$locFrozen", ('{0:N1}%' -f $pct))
}

# ---------------------------------------------------------------------------
# Checks
# ---------------------------------------------------------------------------

Add-SanityCheck -Report $report -Id 1 -Title 'Namespace bundles that are a verbatim copy of en/' -Hits $copiedFiles -Mode strict -Severity high `
  -Remediation 'Translate the namespace. Every string in the file is byte-identical to English, so the locale renders an English UI for that whole feature area even though every key resolves.' `
  -Note 'Bundles with fewer than 5 translatable strings are exempt. Strings that check 4 would class as machine/brand (interpolations, IDs, units, acronyms, brand names) are not counted, so a file of mostly `{{x}}ms` and ID strings is judged on its real words.' `
  -Command 'Compare-Object (cat locales/en/containers.json) (cat locales/nl/containers.json)'

Add-SanityCheck -Report $report -Id 2 -Title 'Multi-word values identical to English (untranslated)' -Hits $identicalLong -Mode strict -Severity high `
  -Remediation 'Translate the value in the locale file. Three or more translatable words identical to English is not a cognate - it is the seeded English string that a sync script copied in and nobody came back to.' `
  -Note 'Brand names, acronyms, {{interpolations}}, numbers and lowerCamel identifiers are stripped before the words are counted, so "Restart {{count}} Docker containers" counts 2 words, not 4.' `
  -Command 'scripts\sanity\i18n-untranslated\check.ps1 -Locale nl -MaxHits 1000'

Add-SanityCheck -Report $report -Id 3 -Title 'One- or two-word values identical to English (cognate or untranslated)' -Hits $identicalShort -Mode review -Severity medium `
  -Remediation 'Confirm each one. "Server", "Port" and "Import" really are identical in several of these languages; "Overview", "Settings" and "Remove" are not. Freeze the genuine cognates in .instructions/sanity/baselines/i18n-untranslated.txt as "locale|namespace|key" so they stop reappearing.' `
  -Note 'Review, not fail: short strings are where real cross-language identity actually happens, and a check that fails on all of them is one people dismiss wholesale. Check 2 carries the unambiguous case.'

Add-SanityCheck -Report $report -Id 4 -Title 'Identical values that are pure machine/brand strings (accepted)' -Hits $machineIdentical -Mode info -Severity info `
  -Remediation 'No action. Listed so the identical-value totals in check 8 reconcile, and so a real word that the do-not-translate list swallowed by mistake stays visible.' `
  -Note 'Sourced from .instructions/donottranslate.md. A value lands here when nothing survives once interpolations, tags, numbers, acronyms and brand names are stripped.'

Add-SanityCheck -Report $report -Id 5 -Title 'Locale values that are empty while en/ has a string' -Hits $emptyValues -Mode strict -Severity high `
  -Remediation 'Write the translation, or copy the English string back in. An empty value is worse than an untranslated one: i18next treats it as a successful lookup and renders nothing, so the label simply vanishes from the UI.'

Add-SanityCheck -Report $report -Id 6 -Title 'Locale values carrying an untranslated marker' -Hits $markerValues -Mode strict -Severity high `
  -Remediation 'Replace the marker with the real translation. A value like "{{lang missing: Container filesystem}}" is not a fallback - i18next has no such directive, so the braces and the marker text render verbatim in the UI.' `
  -Note 'Markers matched: {{lang missing: ...}} and [untranslated] case-insensitively, plus {{TODO ...}} / a leading TODO: / FIXME: / XXX: / UNTRANSLATED: case-SENSITIVELY - the Spanish and Portuguese word "Todo" ("all", "every") is a real translation and must not match. These values differ from en, so every identical-value check above walks straight past them; without this check they count as translated.'

Add-SanityCheck -Report $report -Id 7 -Title 'Keys present in en/ and absent from the locale (untranslated by omission)' -Hits $missingKeys -Mode review -Severity medium `
  -Remediation 'Seed them with the English string (a sync script, or by hand for widgets/nodes/plugins), then translate. Until then the key falls back to en at runtime - it renders, but in English.' `
  -Note 'Key parity for the frontend bundle is owned by check 11 of the i18n check; it is repeated here at review severity because a missing key is an untranslated key, and the coverage percentages in check 9 would lie without it. Widgets, nodes and plugin bundles have no separate parity check, so this is the only place a missing key in one of those surfaces is reported.'

Add-SanityCheck -Report $report -Id 8 -Title 'Translated values whose interpolation placeholders drifted' -Hits $placeholderDrift -Mode strict -Severity high `
  -Remediation 'Restore the exact {{name}} placeholders and <0>/<1> Trans tags from the en/ string. A lost placeholder silently drops the value from the rendered sentence; an invented one renders the literal braces.' `
  -Note 'Only compared on values that were actually translated - an identical copy cannot drift, and a value carrying a marker (check 6) is skipped so the same string is not reported twice.'

$covTable = @{ Headers = @('Locale', 'Strings', 'Translated', 'Identical to en', 'Missing', 'Empty', 'Marker', 'Frozen', 'Translated %'); Rows = $rows }
Add-SanityCheck -Report $report -Id 9 -Title 'Translation coverage per locale' -Hits @() -Mode info -Severity info -Table $covTable `
  -Remediation 'Inventory, never a failure on its own - read the table, not the PASS. "Translated %" counts only values that actually differ from English; identical, missing, empty and marker values all count against it.' `
  -Note ("Locales compared: {0}. Bundle groups scanned: {1} ({2} namespace units total). Baseline entries loaded: {3}." -f ($targets -join ', '), $groups.Count, $totalNsUnits, $baselineCount)

$worst = @($nsRows | Sort-Object { [double]$_[4] } | Select-Object -First 60 | ForEach-Object { , @($_[0], $_[1], $_[2], $_[3], ($_[4] + '%')) })
$nsTable = @{ Headers = @('Locale', 'Namespace', 'Strings', 'Identical to en', 'Differs from en %'); Rows = $worst }
Add-SanityCheck -Report $report -Id 10 -Title 'Least-translated namespaces (worst 60)' -Hits @() -Mode info -Severity info -Table $nsTable `
  -Remediation 'Inventory, never a failure on its own. Use it to pick the next namespace to translate - a namespace at 0% is check 1, a namespace at 90% is nearly done.' `
  -Note 'Counts identical values only, so it does not subtract the marker values check 6 reports; the per-locale totals in check 9 do. Namespace names outside src/frontend/core/i18n/locales are prefixed with their group path (widgets/<slug>/i18n, nodes/<slug>/i18n, src/plugins/Plugins/<slug>/i18n) to disambiguate.'

Add-SanityNote -Report $report -Text 'This check owns translation; the i18n check owns key existence. A key can resolve perfectly and still be untranslated - that is precisely the gap this check closes.'
Add-SanityNote -Report $report -Text 'Seeding a new key in all six locales with the English string is the documented convention (.claude/rules/i18n-keys.md rule 3). This check measures the debt that convention creates; it does not forbid it.'
Add-SanityNote -Report $report -Text 'Never translate the entries in .instructions/donottranslate.md - check 4 lists the identical values that were accepted for exactly that reason.'
Add-SanityNote -Report $report -Text 'Freeze accepted identical values in .instructions/sanity/baselines/i18n-untranslated.txt (one "locale|namespace|key" per line) rather than widening the do-not-translate word list. For widgets/nodes/plugins the namespace half of that key is the group-qualified name shown in check 10 (e.g. widgets/activity-feed/i18n/en), not the bare namespace.'
Add-SanityNote -Report $report -Text 'Bundle shapes differ: src/frontend/core/i18n/locales and src/plugins/Plugins/*/i18n nest namespace files under a directory per language (<lang>/<ns>.json); widgets/*/i18n and nodes/*/i18n are flat, one file per language (<lang>.json) that is itself the whole namespace.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary

if ($ratchetMode) {
  $ratchetPath = if ([System.IO.Path]::IsPathRooted($Ratchet)) { $Ratchet } else { Join-Path $Root $Ratchet }
  if ($WriteRatchet) {
    $sb = New-Object System.Text.StringBuilder
    [void]$sb.Append('{' + "`n" + '  "schema": 2,' + "`n" + '  "scope": ["' + $feGroupRel + '", "widgets/*/i18n", "nodes/*/i18n"],' + "`n" + '  "bundles": {')
    $first = $true
    foreach ($nsq in ($enKeysByNs.Keys | Sort-Object { $_ } -CaseSensitive)) {
      [string[]]$ks = @($enKeysByNs[$nsq])
      [Array]::Sort($ks, [System.StringComparer]::Ordinal)
      $idx = @{}
      for ($n = 0; $n -lt $ks.Length; $n++) { $idx[$ks[$n]] = $n }
      $parts = @()
      foreach ($loc in $targets) {
        $set = $identKeys["$loc|$nsq"]
        $nums = @()
        if ($set) { $nums = @($set | Where-Object { $idx.ContainsKey($_) } | ForEach-Object { $idx[$_] } | Sort-Object) }
        $parts += ('"{0}": {1}' -f $loc, (ConvertTo-Json -InputObject ([object[]]$nums) -Compress))
      }
      if (-not $first) { [void]$sb.Append(',') }
      $first = $false
      [void]$sb.Append("`n    " + (ConvertTo-Json -InputObject ([string]$nsq) -Compress) + ': {"keys": ' + (ConvertTo-Json -InputObject ([object[]]$ks) -Compress) + ', "identical": {' + ($parts -join ', ') + '}}')
    }
    [void]$sb.Append("`n  }`n}`n")
    $dir = Split-Path $ratchetPath -Parent
    if (-not (Test-Path -LiteralPath $dir)) { New-Item -ItemType Directory -Path $dir -Force | Out-Null }
    [System.IO.File]::WriteAllText($ratchetPath, $sb.ToString(), (New-Object System.Text.UTF8Encoding($false)))
    Write-Host ("   ratchet baseline written: {0} ({1} bundles)" -f $Ratchet, $enKeysByNs.Count)
    exit 0
  }
  if (-not (Test-Path -LiteralPath $ratchetPath)) {
    Write-Host ("   ratchet: baseline not found: {0}" -f $Ratchet) -ForegroundColor Red
    exit 1
  }
  $base = [System.IO.File]::ReadAllText($ratchetPath, [System.Text.Encoding]::UTF8) | ConvertFrom-Json
  if ($base.schema -ne 2) {
    Write-Host ("   ratchet: unsupported baseline schema {0}" -f $base.schema) -ForegroundColor Red
    exit 1
  }
  $problems = New-Object System.Collections.Generic.List[string]
  $checked = 0
  foreach ($bp in $base.bundles.PSObject.Properties) {
    $nsq = $bp.Name
    $bundle = $bp.Value
    $baseKeys = @($bundle.keys)
    foreach ($loc in $targets) {
      $now = $identKeys["$loc|$nsq"]
      if (-not $now) { continue }
      $frozen = New-Object 'System.Collections.Generic.HashSet[string]'
      $ip = $bundle.identical.PSObject.Properties[$loc]
      if ($null -ne $ip) { foreach ($n in @($ip.Value)) { [void]$frozen.Add([string]$baseKeys[[int]$n]) } }
      $baseSet = New-Object 'System.Collections.Generic.HashSet[string]' (, [string[]]$baseKeys)
      $reverted = @($now | Where-Object { $baseSet.Contains($_) -and -not $frozen.Contains($_) } | Sort-Object)
      $checked += $baseKeys.Count
      if ($reverted.Count) {
        $sample = ($reverted | Select-Object -First 3) -join ', '
        $problems.Add("$loc/$nsq - $($reverted.Count) translated key(s) went back to English (e.g. $sample)")
      }
    }
  }
  if ($problems.Count) {
    Write-Host '   ratchet: FAIL' -ForegroundColor Red
    foreach ($msg in $problems) { Write-Host ("   - {0}" -f $msg) -ForegroundColor Red }
    exit 1
  }
  Write-Host ("   ratchet: OK ({0} baseline bundles, no baseline key went back to English; keys added since are ignored)" -f @($base.bundles.PSObject.Properties).Count) -ForegroundColor Green
  exit 0
}

if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
