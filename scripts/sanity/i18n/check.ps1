# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  i18n sanity check. Mirrors .instructions/sanity/i18n.md.
.DESCRIPTION
  Hardcoded strings, namespace misuse, toast/placeholder literals, backend
  code coverage, locale file parity and locale key-structure parity. The
  key-parity check (11) is implemented natively in PowerShell, so node is
  not required.
.EXAMPLE
  .\scripts\sanity\i18n\check.ps1
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [string[]]$Paths
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$report = New-SanityReport -Name 'i18n' -Title 'i18n Sanity' -Scope 'src/backend, src/frontend, widgets, nodes' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$feWidgetNodeScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend', 'widgets', 'nodes'))
$beScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend'))
$pluginsScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/plugins'))
$tsxArgs = @{ Root = $Root; Paths = $feWidgetNodeScope; Include = @('*.tsx'); ExcludeMatch = @('\.test\.tsx$', '\.stories\.tsx$') }
$tsArgs = @{ Root = $Root; Paths = $feWidgetNodeScope; Include = @('*.ts', '*.tsx'); ExcludeMatch = @('\.test\.tsx?$', '\.stories\.tsx?$', '\.d\.ts$') }
$localesRoot = Join-Path $Root 'src\frontend\core\i18n\locales'

# Machine values that must never be translated (subset of .instructions/donottranslate.md)
$dnt = 'McHarbor|McSparrow|Docker|Kubernetes|Podman|Swarm|Compose|SQLite|JSON|YAML|HTTP|HTTPS|TCP|UDP|SSH|TLS|API|URL|ID|CPU|RAM|GPU|OK|ClusterIP|NodePort|LoadBalancer|ExternalName|Running|Pending|Succeeded|Failed|Unknown|Available|Progressing|GitHub|GitLab|Gitea|Grafana|Prometheus|Traefik|Nginx|Postgres|MySQL|Redis|MongoDB|Slack|Teams|Discord|Telegram|Signal|WhatsApp|SMTP|OIDC|SAML|LDAP|Entra|Okta|Keycloak|OAuth|TOTP|MFA|CSV|PDF|SVG|PNG|CLI|SDK|WebSocket|SSE|CIDR|IPv4|IPv6|DNS|NAT|VPN|AES|RSA|Argon2|xid|Vite|React|Go|Tailwind|Radix|TanStack|Zustand|Ladle'

# 1. Hardcoded frontend strings
$hard = @(Find-Pattern @tsxArgs -Pattern '>Yes<|>No<|\btitle="[A-Z][^"]*"|\blabel="[A-Z][^"]*"|>[A-Z][A-Za-z ]+</th>|>[A-Z][A-Za-z ]+</Badge>|>[A-Z][a-z]+( [a-z]+){1,6}</' -NotMatch ("^\s*(//|\*|\{{/\*)|\bt\(|>({0})</|title=`"({0})`"|label=`"({0})`"" -f $dnt))
Add-SanityCheck -Report $report -Id 1 -Title 'Hardcoded user-facing English literals in JSX' -Hits $hard -Mode review -Severity medium `
  -Remediation 'Route through t(...) in the owning namespace. Brand names and API machine values (see .instructions/donottranslate.md) are accepted.' `
  -Command 'rg -n ">Yes<|>No<|title=\"[A-Z][^\"]*\"|label=\"[A-Z][^\"]*\"|>[A-Z][A-Za-z ]+</th>|>[A-Z][A-Za-z ]+</Badge>" src/frontend -g "*.tsx"'

# 2. Missing useTranslation namespace
$bareNs = @(Find-Pattern @tsArgs -Pattern 'useTranslation\(\s*\)')
Add-SanityCheck -Report $report -Id 2 -Title 'Bare useTranslation() without namespace' -Hits $bareNs -Mode strict -Severity medium -Remediation "Always name the namespace: useTranslation('containers'); shared strings use 'common'." -Command 'rg -n "useTranslation\(\)" src/frontend'

# 3. Bare t() misuse spot-check
# `common:` is the documented shared namespace and reaching into it from
# another namespace is exactly the one-off case the remediation accepts, so
# the prefix that matters is any *other* one.
# `i18n.t(...)` is the global instance, called outside a component where there
# is no useTranslation scope to carry the namespace. There the prefix is
# required, not a preference.
$nsPrefixed = @(Find-Pattern @tsArgs -Pattern "\bt\(\s*['`"][a-zA-Z_-]+:[^'`"]+['`"]" -NotMatch "\bt\(\s*['`"]common:|\bi18n\.t\(")
$tCalls = @(Find-Pattern @tsArgs -Pattern "\bt\(\s*['`"]")
Add-SanityCheck -Report $report -Id 3 -Title 'Namespace-prefixed t("ns:key") calls' -Hits $nsPrefixed -Mode review -Severity low `
  -Remediation 'Prefer declaring the namespace in useTranslation and calling t("key"); prefixed keys are accepted only for one-off cross-namespace strings.' `
  -Note ("{0} total t('...') call sites." -f $tCalls.Count) -Command "rg -n `"\bt\('[^)]+:`" src/frontend"

# 4. Hardcoded toast messages
$toasts = @(Find-Pattern @tsArgs -Pattern "toast\.(success|error|info|warning|message)\(\s*['`"][A-Za-z]")
Add-SanityCheck -Report $report -Id 4 -Title 'Hardcoded toast messages' -Hits $toasts -Mode strict -Severity medium -Remediation 'Use t("toast.key") or the translated server message.' -Command "rg -n `"toast\.(success|error|info|warning)\(\s*'`" src/frontend"

# 5. Hardcoded backend error responses
$beHard = @(Find-Pattern -Root $Root -Paths $beScope -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern 'response\.(BadRequest|InternalError|Unauthorized|NotFound|Forbidden|Conflict)\(w,\s*"')
Add-SanityCheck -Report $report -Id 5 -Title 'Backend errors with string literals instead of i18n codes' -Hits $beHard -Mode strict -Severity medium -Remediation 'Use response.BadRequestCode(w, r, i18n.ErrX) and friends.' -Command 'rg -n "response\.(BadRequest|InternalError|Unauthorized|NotFound|Forbidden|Conflict)\(w,\s*\"" src/backend'

# 6. Locale file parity
$supported = @('en', 'nl', 'de', 'es', 'fr', 'pt')
$i18nTs = Join-Path $Root 'src\frontend\core\i18n\i18n.ts'
if (Test-Path -LiteralPath $i18nTs) {
  $m = [regex]::Match((Get-SanityText $i18nTs), 'supportedLanguages\s*=\s*\[([^\]]+)\]')
  if ($m.Success) { $supported = @([regex]::Matches($m.Groups[1].Value, '"([^"]+)"') | ForEach-Object { $_.Groups[1].Value }) }
}
$enDir = Join-Path $localesRoot 'en'
$enFiles = @()
if (Test-Path -LiteralPath $enDir) { $enFiles = @(Get-ChildItem -LiteralPath $enDir -Filter *.json | ForEach-Object { $_.Name }) }
$fileParity = @(); $extraFiles = @()
$onDisk = @(Get-ChildItem -LiteralPath $localesRoot -Directory | ForEach-Object { $_.Name })
foreach ($loc in $onDisk) {
  if ($loc -eq 'en') { continue }
  $locFiles = @(Get-ChildItem -LiteralPath (Join-Path $localesRoot $loc) -Filter *.json | ForEach-Object { $_.Name })
  foreach ($f in $enFiles) { if ($locFiles -notcontains $f) { $fileParity += New-SanityHit -File "src/frontend/core/i18n/locales/$loc/$f" -Line 0 -Text 'MISSING_FILE (exists in en/)' } }
  foreach ($f in $locFiles) { if ($enFiles -notcontains $f) { $extraFiles += New-SanityHit -File "src/frontend/core/i18n/locales/$loc/$f" -Line 0 -Text 'not present in en/' } }
}
$legacyLocales = @($onDisk | Where-Object { $supported -notcontains $_ })
$fileParitySupported = @($fileParity | Where-Object { $h = $_; -not ($legacyLocales | Where-Object { $h.File -match "/locales/$_/" }) })
Add-SanityCheck -Report $report -Id 6 -Title 'Namespace files missing in a supported locale' -Hits $fileParitySupported -Mode strict -Severity high `
  -Remediation 'Copy the en/ file and translate (or keep EN strings as fallback) so the namespace exists.' `
  -Note ("Supported locales: {0}. Legacy locale folders on disk: {1}." -f ($supported -join ', '), $(if ($legacyLocales.Count) { $legacyLocales -join ', ' } else { 'none' })) `
  -Command 'Compare-Object (ls locales/en) (ls locales/nl)'
Add-SanityCheck -Report $report -Id '6b' -Title 'Locale files with no en/ counterpart' -Hits $extraFiles -Mode review -Severity low -Remediation 'Delete or add the en/ source file.'

# 7. Unused-key check (en keys with no literal or dynamic-prefix reference in TS/TSX)
$tsArgsFull = @{ Root = $Root; Paths = @('src/frontend', 'widgets', 'nodes'); Include = @('*.ts', '*.tsx'); ExcludeMatch = @('\.test\.tsx?$', '\.stories\.tsx?$', '\.d\.ts$') }
$tsFiles = @(Get-SanityFiles @tsArgsFull)
$literalSet = New-Object 'System.Collections.Generic.HashSet[string]'
$prefixSet = New-Object 'System.Collections.Generic.HashSet[string]'
$litRx = New-Object System.Text.RegularExpressions.Regex("['`"]([A-Za-z0-9_][A-Za-z0-9_.:\-]*)['`"]")
$dynRx = New-Object System.Text.RegularExpressions.Regex('`([A-Za-z0-9_][A-Za-z0-9_.:\-]*\.)\$\{')
$dynRx2 = New-Object System.Text.RegularExpressions.Regex("['`"]([A-Za-z0-9_][A-Za-z0-9_.:\-]*\.)['`"]\s*\+")
foreach ($f in $tsFiles) {
  $text = Get-SanityText $f.Full
  foreach ($m in $litRx.Matches($text)) { $v = $m.Groups[1].Value; $literalSet.Add($v) | Out-Null; $ci = $v.IndexOf(':'); if ($ci -gt 0) { $literalSet.Add($v.Substring($ci + 1)) | Out-Null } }
  foreach ($m in $dynRx.Matches($text)) { $v = $m.Groups[1].Value; $ci = $v.IndexOf(':'); if ($ci -gt 0) { $v = $v.Substring($ci + 1) }; $prefixSet.Add($v) | Out-Null }
  foreach ($m in $dynRx2.Matches($text)) { $prefixSet.Add($m.Groups[1].Value) | Out-Null }
}
# Backend also emits keys (plugin manifests, API messages) - collect Go string literals too
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend', 'src/plugins') -Include @('*.go', '*.yaml', '*.json') -ExcludeMatch @('_test\.go$', '/locales/', '/i18n/'))) {
  $text = Get-SanityText $f.Full
  foreach ($m in $litRx.Matches($text)) { $literalSet.Add($m.Groups[1].Value) | Out-Null }
}
$unused = @(); $enKeyTotal = 0; $parseErrors = @()
$enData = @{}
foreach ($f in $enFiles) {
  $path = Join-Path $enDir $f
  try { $obj = ConvertFrom-SanityJson (Get-SanityText $path) } catch { $parseErrors += New-SanityHit -File "src/frontend/core/i18n/locales/en/$f" -Line 0 -Text "JSON parse error: $($_.Exception.Message)"; continue }
  $enData[$f] = $obj
  $ns = [System.IO.Path]::GetFileNameWithoutExtension($f)
  foreach ($k in (Get-SanityFlatKeys -Obj $obj)) {
    $enKeyTotal++
    $base = $k -replace '_(one|other|zero|two|few|many)$', ''
    if ($literalSet.Contains($k) -or $literalSet.Contains($base) -or $literalSet.Contains("$ns.$base")) { continue }
    $used = $false
    $parts = $base.Split('.')
    for ($i = $parts.Length - 1; $i -ge 1; $i--) {
      $p = ($parts[0..($i - 1)] -join '.') + '.'
      if ($prefixSet.Contains($p) -or $literalSet.Contains($p.TrimEnd('.'))) { $used = $true; break }
    }
    if (-not $used) { $unused += New-SanityHit -File "src/frontend/core/i18n/locales/en/$f" -Line 0 -Text $k }
  }
}
Add-SanityCheck -Report $report -Id 7 -Title 'en/ keys with no literal or dynamic-prefix reference in code' -Hits $unused -Mode info -Severity low `
  -Remediation 'Delete dead keys or confirm they are consumed via a computed key the scan cannot see (server-provided codes, plugin bundles).' `
  -Note ("{0} en/ keys scanned; a key counts as used when its full path, its namespace-prefixed path, or any parent path followed by `${{...}} appears in TS/TSX/Go/YAML." -f $enKeyTotal)

# 8. Backend i18n code coverage
$codesFile = Join-Path $Root 'src\backend\core\i18n\codes.go'
$msgFile = Join-Path $Root 'src\backend\core\i18n\messages.go'
$missingMsgs = @(); $langCount = 0
if ((Test-Path -LiteralPath $codesFile) -and (Test-Path -LiteralPath $msgFile)) {
  $codes = @([regex]::Matches((Get-SanityText $codesFile), '(?m)^\s*((?:Err|Msg)\w+)\s+MsgCode\s*=') | ForEach-Object { $_.Groups[1].Value } | Sort-Object -Unique)
  $msgText = Get-SanityText $msgFile
  $maps = @([regex]::Matches($msgText, '(?m)^var (messages\w+) = map\[MsgCode\]string\{'))
  $langCount = $maps.Count
  for ($i = 0; $i -lt $maps.Count; $i++) {
    $start = $maps[$i].Index
    $end = if ($i + 1 -lt $maps.Count) { $maps[$i + 1].Index } else { $msgText.Length }
    $body = $msgText.Substring($start, $end - $start)
    $present = New-Object 'System.Collections.Generic.HashSet[string]'
    foreach ($m in [regex]::Matches($body, '(?m)^\s*((?:Err|Msg)\w+)\s*:')) { $present.Add($m.Groups[1].Value) | Out-Null }
    foreach ($c in $codes) { if (-not $present.Contains($c)) { $missingMsgs += New-SanityHit -File 'src/backend/core/i18n/messages.go' -Line 0 -Text ("{0} missing in {1}" -f $c, $maps[$i].Groups[1].Value) } }
  }
  Add-SanityCheck -Report $report -Id 8 -Title 'MsgCode constants without a message in every language map' -Hits $missingMsgs -Mode strict -Severity medium `
    -Remediation 'Add the code to each messages<LANG> map in core/i18n/messages.go (EN string is acceptable until translated).' `
    -Note ("{0} codes in codes.go, {1} language maps in messages.go." -f $codes.Count, $langCount) -Command 'rg -n "Err[A-Z]|Msg[A-Z]" src/backend/core/i18n/codes.go'
} else {
  Add-SanityCheck -Report $report -Id 8 -Title 'MsgCode constants without a message in every language map' -Skipped -SkipReason 'core/i18n/codes.go or messages.go not found'
}

# 9. Locale-aware date/time formatting
# The finding is a *date* formatted outside the shared helpers, which is what
# loses the app's locale. `count.toLocaleString()` on a number is thousands
# separators, a different concern with no helper to bypass — so a call is only
# a date call when it is a date method or the receiver is plainly a Date.
$locDates = @(Find-Pattern -Root $Root -Paths $feWidgetNodeScope -Include @('*.ts', '*.tsx') -Pattern 'toLocale(DateString|TimeString)\(|(new Date\([^)]*\)|parsed|\bdate)\.toLocaleString\(' -ExcludeMatch @('\.test\.tsx?$', '\.stories\.tsx?$', '\.d\.ts$', 'resources/utils/', 'core/i18n/'))
$helpers = @(Find-Pattern -Root $Root -Paths @('src/frontend/resources/utils') -Include @('*.ts') -Pattern 'export (function|const) (formatDate|formatDateTime|formatRelative|getLocale)')
Add-SanityCheck -Report $report -Id 9 -Title 'Direct toLocale*String() calls bypassing shared date helpers' -Hits $locDates -Mode review -Severity low `
  -Remediation 'Use formatDate / formatDateTime from @resources/utils so the active i18n locale is honoured.' -Note ("{0} shared date helper exports found." -f $helpers.Count) -Command 'rg -n "toLocaleString\(|toLocaleDateString\(" src/frontend'

# 10. Hardcoded placeholders
$placeholders = @(Find-Pattern @tsxArgs -Pattern 'placeholder="[A-Za-z]' -NotMatch ('placeholder="({0}|https?://|[a-z0-9._-]+@|\d|[A-Z0-9]+(-[A-Z0-9]+)+")' -f $dnt))
Add-SanityCheck -Report $report -Id 10 -Title 'Hardcoded placeholder text' -Hits $placeholders -Mode strict -Severity medium -Remediation 'placeholder={t("...")}. Example URLs, e-mails, hostnames and machine values are accepted.' -Command 'rg -n "placeholder=\"[A-Za-z]" src/frontend -g "*.tsx"'

# 11. Locale key-structure parity (native port of the node script in i18n.md)
$missingKeys = @(); $missingKeysLegacy = @(); $keyParseErrors = @()
foreach ($loc in $onDisk) {
  if ($loc -eq 'en') { continue }
  $isLegacy = $legacyLocales -contains $loc
  foreach ($f in $enFiles) {
    if (-not $enData.ContainsKey($f)) { continue }
    $locPath = Join-Path (Join-Path $localesRoot $loc) $f
    if (-not (Test-Path -LiteralPath $locPath)) { continue }  # reported by check 6
    try { $locObj = ConvertFrom-SanityJson (Get-SanityText $locPath) } catch { $keyParseErrors += New-SanityHit -File "src/frontend/core/i18n/locales/$loc/$f" -Line 0 -Text "JSON parse error: $($_.Exception.Message)"; continue }
    $locKeys = New-Object 'System.Collections.Generic.HashSet[string]'
    foreach ($k in (Get-SanityFlatKeys -Obj $locObj)) { $locKeys.Add($k) | Out-Null }
    foreach ($k in (Get-SanityFlatKeys -Obj $enData[$f])) {
      if ($locKeys.Contains($k)) { continue }
      if ($k -match '_(one|other|zero|two|few|many)$') {
        $base = $k -replace '_(one|other|zero|two|few|many)$', ''
        if ($locKeys.Contains($base) -or ($locKeys | Where-Object { $_ -like "$base`_*" } | Select-Object -First 1)) { continue }
      }
      $hit = New-SanityHit -File "src/frontend/core/i18n/locales/$loc/$f" -Line 0 -Text "MISSING_KEY $k"
      if ($isLegacy) { $missingKeysLegacy += $hit } else { $missingKeys += $hit }
    }
  }
}
Add-SanityCheck -Report $report -Id 11 -Title 'Keys present in en/ but missing in a supported locale' -Hits ($parseErrors + $keyParseErrors + $missingKeys) -Mode strict -Severity high `
  -Remediation 'Add the key with the same nesting (EN text is acceptable until translated). Missing keys fall back to the raw key string at runtime.' `
  -Note 'Plural variants (_one/_other) count as present when any sibling form exists. Extra keys in non-en locales are not flagged.' -Command 'node -e "<locale parity script from i18n.md>"'
Add-SanityCheck -Report $report -Id '11b' -Title 'Keys missing in legacy (unsupported) locale folders' -Hits $missingKeysLegacy -Mode info -Severity info -Note 'Legacy locales are tolerated as partial mirrors unless full parity is requested.'

# ---------------------------------------------------------------------------
# 12-14. Call sites that render the raw key instead of a value
#
# Checks 7 and 11 look at the locale files (dead keys, locale parity). This
# block looks at the *call sites*: a t('...') whose key does not exist in the
# en bundle renders the key string itself - "containers.diff.empty" in the UI -
# for every user, English included, and no locale parity check can see it.
#
# The bundle is rebuilt the way core/i18n/i18n.ts assembles it at runtime:
# locales/en/<ns>.json for each staticNamespace, widgets/*/i18n/en.json merged
# into `dashboard`, nodes/*/i18n/en.json as `nodes`.
# ---------------------------------------------------------------------------

$verifier = Join-Path $Root 'scripts/sanity/i18n/verify-keys.mjs'
$rawKeys = @(); $branchKeys = @(); $unknownNsHits = @(); $defaultValueHits = @(); $prefixHits = @(); $computedDefaultHits = @(); $orphanHits = @()
$resolverRan = $false
$resolverNote = 'scripts/sanity/i18n/verify-keys.mjs did not run (node missing or script absent); checks 12, 12b, 12c and 12d are unverified.'

if ((Get-Command node -ErrorAction SilentlyContinue) -and (Test-Path -LiteralPath $verifier)) {
  # One resolver, not two. This block used to carry its own PowerShell
  # implementation of key resolution; it skipped every call that passed a
  # defaultValue and ignored the { ns: '...' } option, so on 2026-09-11 it
  # reported 9 problems where verify-keys.mjs found 328. The node resolver is
  # also what the per-edit hook runs, so the sweep and the hook can no longer
  # disagree about what a broken key is.
  $verifierPaths = @($feWidgetNodeScope | Where-Object { $_ -match '\.tsx?$' } | ForEach-Object { Join-Path $Root ($_ -replace '/', [System.IO.Path]::DirectorySeparatorChar) } | Where-Object { Test-Path -LiteralPath $_ })
  $verifierArgs = @('--json') + $verifierPaths
  $raw = & node $verifier @verifierArgs 2>$null
  if ($LASTEXITCODE -eq 0 -and $raw) {
    $resolverRan = $true
    $records = @(ConvertFrom-SanityJson ([string]::Join('', $raw)))
    foreach ($r in $records) {
      $hit = New-SanityHit -File $r.file -Line $r.line -Text ("{0} - {1}" -f $r.key, $r.message)
      switch ($r.kind) {
        'missing'      { $rawKeys += $hit }
        'object'       { $branchKeys += $hit }
        'namespace'    { $unknownNsHits += $hit }
        'defaultValue' { $defaultValueHits += $hit }
        'prefix' { $prefixHits += $hit }
        'computedDefault' { $computedDefaultHits += $hit }
        'orphan' { $orphanHits += $hit }
      }
    }
    $resolverNote = ("Resolved by scripts/sanity/i18n/verify-keys.mjs across src/frontend, widgets/ and nodes/ - the same resolver the per-edit hook runs. {0} problem(s) reported." -f $records.Count)
  }
}

Add-SanityCheck -Report $report -Id 12 -Title 't() call sites with no matching key in the en bundle (UI renders the raw key)' -Hits $rawKeys -Mode strict -Severity high `
  -Remediation 'Add the key to src/frontend/core/i18n/locales/en/<namespace>.json (then the other locales, see check 11), or fix the key/namespace at the call site. i18next echoes the key when the lookup fails, so these strings show up as "some.dotted.key" in the running UI.' `
  -Note $resolverNote `
  -Command 'node scripts/sanity/i18n/verify-keys.mjs'

Add-SanityCheck -Report $report -Id '12b' -Title 'Keys that resolve to an object instead of a string' -Hits $branchKeys -Mode strict -Severity high `
  -Remediation 'Point the call at a leaf key. i18next returns the key unchanged when the lookup lands on a nested object (unless returnObjects is set).'

Add-SanityCheck -Report $report -Id '12c' -Title 'Namespaces referenced by a call site that are not loaded' -Hits $unknownNsHits -Mode review -Severity high `
  -Remediation 'Add the namespace to staticNamespaces in core/i18n/i18n.ts and ship locales/<lang>/<ns>.json. Namespaces a plugin registers at runtime via addResourceBundle are expected here and can be dismissed.'

Add-SanityCheck -Report $report -Id '12d' -Title 'defaultValue at a t() call site (banned - hides a missing key)' -Hits $defaultValueHits -Mode strict -Severity high `
  -Remediation 'Delete the defaultValue and put the English string in locales/en/<ns>.json, then run node scripts/sanity/i18n/sync-locales.mjs --write. A defaultValue renders whether or not the key exists, so the miss is invisible in English and ships as English in the other five locales. node scripts/sanity/i18n/adopt-defaults.mjs --write performs the migration.' `
  -Note 'Was unenforced until 2026-09-11: the old resolver skipped any call containing a defaultValue, which is how 237 of them accumulated.'

Add-SanityCheck -Report $report -Id '12e' -Title 'Computed key whose whole group is missing from the bundle' -Hits $prefixHits -Mode strict -Severity high `
  -Remediation 'Add the key group to locales/en/<ns>.json (then run node scripts/sanity/i18n/sync-locales.mjs --write), or point the call at the namespace that already holds the group. A computed key resolves under a fixed prefix; when nothing in the bundle starts with that prefix, every value the expression can produce renders the raw dotted key in the UI.' `
  -Note 'The prefix is read from the template literal, or from a same-file helper that returns one - t(labelKey(x)) is the shape that hid the missing notifications.position.* group until 2026-09-11.'

Add-SanityCheck -Report $report -Id '12f' -Title 'defaultValue on a computed key (confirm the value set is open-ended)' -Hits $computedDefaultHits -Mode review -Severity medium `
  -Remediation 'If the value set is closed, map it to literal keys and drop the defaultValue (rule 6). If it is genuinely open-ended - a plugin label, a backend-supplied step name - the default is the documented fallback and the hit can be dismissed.' `
  -Note 'Review, not fail: most of these are legitimate, and a check that fails on all of them is one people dismiss wholesale. Check 12e catches the unambiguous case - a group that is missing entirely.'

Add-SanityCheck -Report $report -Id '12g' -Title 'Key exists in no namespace at all (file receives t as a prop)' -Hits $orphanHits -Mode strict -Severity high `
  -Remediation 'Add the key, or fix it at the call site. These files take t as a prop, so the namespace belongs to the caller and cannot be resolved here - but a key that exists in NO bundle under ANY namespace is broken whatever the caller declares.' `
  -Note 'Conservative by construction: it can only fire when nothing anywhere defines the key, so it cannot produce a false positive. Covers ~400 calls in 33 files that were skipped outright before 2026-09-11.'

# 13. en values that render as nothing useful even when the lookup succeeds
$emptyValues = @(); $echoValues = @()
$echoAccepted = @{}
$echoAcceptedPath = Join-Path $PSScriptRoot 'echo-accepted.json'
if (Test-Path -LiteralPath $echoAcceptedPath) { foreach ($p in (Get-Content -LiteralPath $echoAcceptedPath -Raw -Encoding UTF8 | ConvertFrom-Json).PSObject.Properties) { $echoAccepted[$p.Name] = $p.Value } }
foreach ($f in $enFiles) {
  if (-not $enData.ContainsKey($f)) { continue }
  $obj = $enData[$f]
  foreach ($k in (Get-SanityFlatKeys -Obj $obj)) {
    $val = $obj
    $ok = $true
    foreach ($part in $k.Split('.')) { $val = Get-SanityJsonValue $val $part; if ($null -eq $val) { $ok = $false; break } }
    if (-not $ok) { continue }
    if ($val -isnot [string]) { continue }
    if ([string]::IsNullOrWhiteSpace($val)) { $emptyValues += New-SanityHit -File "src/frontend/core/i18n/locales/en/$f" -Line 0 -Text "$k = (empty)"; continue }
    # -ceq, not -eq: 'name': 'Name' is a correct translation, 'name': 'name' is not.
    # A dotted lowerCamel value is a key someone pasted into the value slot.
    # A word that is its own English ("cores", "1h") is listed in echo-accepted.json as ns:key with the exact accepted value.
    $acceptedKey = ($f -replace '\.json$', '') + ':' + $k
    if ($echoAccepted.ContainsKey($acceptedKey) -and ($val -ceq $echoAccepted[$acceptedKey].value)) { continue }
    if (($val -ceq $k) -or ($val -ceq (($k -split '\.')[-1])) -or ($val -cmatch '^[a-z][A-Za-z0-9]*(\.[A-Za-z0-9_]+)+$')) {
      $echoValues += New-SanityHit -File "src/frontend/core/i18n/locales/en/$f" -Line 0 -Text ("{0} = {1}" -f $k, $val)
    }
  }
}
Add-SanityCheck -Report $report -Id 13 -Title 'en/ values that are empty or echo their own key' -Hits ($emptyValues + $echoValues) -Mode review -Severity medium `
  -Remediation 'Write the real English string. An empty value renders as blank; a value equal to its key is indistinguishable from a failed lookup.' `
  -Note 'The comparison is case-sensitive: "name": "Name" is a real translation and is not flagged, "name": "name" and "title": "page.header.title" are. Machine identifiers that legitimately equal their key can be dismissed.'

# 14. Keys the static scan cannot resolve
$dynamicInventory = @(Find-Pattern -Root $Root -Paths $feWidgetNodeScope -Include @('*.ts', '*.tsx') -ExcludeMatch @('\.test\.tsx?$', '\.stories\.tsx?$', '\.d\.ts$', 'core/i18n/') -Pattern '(?<![\w$.])t\(\s*`[^`]*\$\{')
Add-SanityCheck -Report $report -Id 14 -Title 'Computed t(`...${x}`) keys (not verifiable statically)' -Hits $dynamicInventory -Mode info -Severity info `
  -Remediation 'Prefer an explicit map from the runtime value to a literal key, or keep a key group in en/ covering every value the expression can produce - a template key that misses renders the interpolated key string in the UI.'

function Test-SanityScopeIncludes {
  param([string[]]$Scope, [string]$Rel)
  $g = $Rel.ToLowerInvariant()
  foreach ($s in $Scope) {
    $sl = $s.ToLowerInvariant()
    if ($g -eq $sl -or $g.StartsWith($sl + '/') -or $sl.StartsWith($g + '/')) { return $true }
  }
  return $false
}

$localesRel = 'src/frontend/core/i18n/locales'
$localesScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @($localesRel))
$mojibakeRx = New-Object System.Text.RegularExpressions.Regex('â€|Ã[\x80-\xbf]|Â[\x80-\xbf]')
$mojibake = @()
if ($localesScope.Count -gt 0) {
  foreach ($loc in $onDisk) {
    if (-not (Test-SanityScopeIncludes -Scope $localesScope -Rel "$localesRel/$loc")) { continue }
    $locDir = Join-Path $localesRoot $loc
    $locFiles = @(Get-ChildItem -LiteralPath $locDir -Filter *.json -ErrorAction SilentlyContinue | ForEach-Object { $_.Name })
    foreach ($f in $locFiles) {
      $path = Join-Path $locDir $f
      try { $obj = ConvertFrom-SanityJson (Get-SanityText $path) } catch { continue }
      foreach ($k in (Get-SanityFlatKeys -Obj $obj)) {
        $val = $obj
        $ok = $true
        foreach ($part in $k.Split('.')) { $val = Get-SanityJsonValue $val $part; if ($null -eq $val) { $ok = $false; break } }
        if (-not $ok -or $val -isnot [string]) { continue }
        if ($mojibakeRx.IsMatch($val)) { $mojibake += New-SanityHit -File "$localesRel/$loc/$f" -Line 0 -Text ("{0} = {1}" -f $k, $val) }
      }
    }
  }
}
Add-SanityCheck -Report $report -Id 15 -Title 'Mojibake in a locale value (UTF-8 decoded as Latin-1/Windows-1252)' -Hits $mojibake -Mode strict -Severity high `
  -Remediation 'Re-enter the value as real UTF-8 text; a byte-level find/replace of the mangled sequence risks a second mangling.' `
  -Note 'Regex: â€|Ã[\x80-\xbf]|Â[\x80-\xbf]. These three byte shapes are the fingerprint of a UTF-8 multi-byte sequence (an em dash, ellipsis, curly quote or accented Latin-1 letter) read back as Latin-1/Windows-1252, or the reverse. The JSON still parses - the file is syntactically fine, the string is simply the wrong bytes - so no other check in this suite sees it. The third alternative excludes a literal trailing space so a legitimate word like "Â la carte" is not flagged; all confirmed hits match the first alternative regardless.'

$pluginLangs = @('nl', 'de', 'es', 'fr', 'pt')
$pluginEnDirs = @()
$pluginMissingLang = @()
$pluginMissingFile = @()
$pluginMissingKey = @()
if ($pluginsScope.Count -gt 0) {
  $pluginEnDirsAll = @(Get-SanityPluginPaths -Root $Root -SubPath 'i18n/en')
  $pluginEnDirs = @($pluginEnDirsAll | Where-Object { Test-SanityScopeIncludes -Scope $pluginsScope -Rel ($_ -replace '/i18n/en$', '') })
  foreach ($enRel in $pluginEnDirs) {
    $enDirFull = Join-Path $Root ($enRel -replace '/', '\')
    $pluginRel = $enRel -replace '/i18n/en$', ''
    $enJsonFiles = @(Get-ChildItem -LiteralPath $enDirFull -Filter *.json -ErrorAction SilentlyContinue | ForEach-Object { $_.Name } | Sort-Object)
    $enKeyMap = @{}
    foreach ($f in $enJsonFiles) {
      try { $enKeyMap[$f] = @(Get-SanityFlatKeys -Obj (ConvertFrom-SanityJson (Get-SanityText (Join-Path $enDirFull $f)))) } catch { $enKeyMap[$f] = @() }
    }
    foreach ($lang in $pluginLangs) {
      $langDirFull = Join-Path (Split-Path $enDirFull -Parent) $lang
      $langRel = "$pluginRel/i18n/$lang"
      if (-not (Test-Path -LiteralPath $langDirFull)) {
        $pluginMissingLang += New-SanityHit -File $langRel -Line 0 -Text "no $lang/ sibling of i18n/en (ships English only)"
        continue
      }
      foreach ($f in $enJsonFiles) {
        $langFile = Join-Path $langDirFull $f
        $langFileRel = "$langRel/$f"
        if (-not (Test-Path -LiteralPath $langFile)) {
          $pluginMissingFile += New-SanityHit -File $langFileRel -Line 0 -Text "missing (exists in i18n/en/$f)"
          continue
        }
        try { $lobj = ConvertFrom-SanityJson (Get-SanityText $langFile) } catch { $pluginMissingFile += New-SanityHit -File $langFileRel -Line 0 -Text 'JSON parse error'; continue }
        $lKeys = New-Object 'System.Collections.Generic.HashSet[string]'
        foreach ($k in (Get-SanityFlatKeys -Obj $lobj)) { $lKeys.Add($k) | Out-Null }
        foreach ($k in $enKeyMap[$f]) { if (-not $lKeys.Contains($k)) { $pluginMissingKey += New-SanityHit -File $langFileRel -Line 0 -Text "MISSING_KEY $k" } }
      }
    }
  }
}
Add-SanityCheck -Report $report -Id 16 -Title 'Plugin i18n missing a language sibling (src/plugins/Plugins/*/i18n/en with no nl/de/es/fr/pt)' -Hits $pluginMissingLang -Mode strict -Severity high `
  -Remediation 'Add the locale directory under the plugin i18n/ folder (English text is acceptable until translated - see the i18n-untranslated check for the debt that convention creates).' `
  -Note ("{0} plugin i18n/en director(y/ies) scanned; a whole missing language directory is reported once here rather than once per namespace file inside it." -f $pluginEnDirs.Count)
Add-SanityCheck -Report $report -Id '16b' -Title 'Plugin locale namespace file missing (language directory exists, file does not)' -Hits $pluginMissingFile -Mode strict -Severity high -Remediation 'Copy the i18n/en/<file>.json file into the locale directory.'
Add-SanityCheck -Report $report -Id '16c' -Title 'Plugin locale key missing (present in i18n/en, absent from the sibling)' -Hits $pluginMissingKey -Mode review -Severity medium -Remediation 'Add the key with the same nesting; English text is acceptable until translated.'

Add-SanityNote -Report $report -Text 'Never translate the entries in .instructions/donottranslate.md; check 1 and 10 exclude the most common machine values automatically.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
