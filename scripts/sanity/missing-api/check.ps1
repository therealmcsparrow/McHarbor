# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Missing API sanity check. Mirrors .instructions/sanity/missing-api.md.
.DESCRIPTION
  Catalogs every /api path the frontend calls (api.<verb>(...), fetch, EventSource,
  WebSocket) and every chi route the backend registers (with r.Route nesting), then
  reports frontend paths with no backend route, backend routes with no caller, and
  verb mismatches. {param} and ${expr} segments are normalized to * on both sides.
.EXAMPLE
  .\scripts\sanity\missing-api\check.ps1
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$report = New-SanityReport -Name 'missing-api' -Title 'Missing API Sanity' -Scope 'src/frontend (+ widgets, nodes) API calls vs src/backend chi routes' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

# ---- helpers ----
function Read-SanityJsQuoted([string]$text, [int]$start) {
  # $text[$start] is ' or " ; returns @{Value; End}
  $q = $text[$start]; $i = $start + 1; $sb = New-Object System.Text.StringBuilder
  while ($i -lt $text.Length) {
    $c = $text[$i]
    if ($c -eq '\') { [void]$sb.Append($c); if ($i + 1 -lt $text.Length) { [void]$sb.Append($text[$i + 1]) }; $i += 2; continue }
    if ($c -eq $q) { return @{ Value = $sb.ToString(); End = $i } }
    if ($c -eq "`n") { break }
    [void]$sb.Append($c); $i++
  }
  return @{ Value = $sb.ToString(); End = $i }
}
function Read-SanityJsTemplate([string]$text, [int]$start) {
  # $text[$start] is ` ; ${...} expressions are replaced by the marker ${}
  $i = $start + 1; $sb = New-Object System.Text.StringBuilder
  while ($i -lt $text.Length) {
    $c = $text[$i]
    if ($c -eq '\') { [void]$sb.Append($c); if ($i + 1 -lt $text.Length) { [void]$sb.Append($text[$i + 1]) }; $i += 2; continue }
    if ($c -eq '`') { return @{ Value = $sb.ToString(); End = $i } }
    if ($c -eq '$' -and $i + 1 -lt $text.Length -and $text[$i + 1] -eq '{') {
      $depth = 1; $i += 2
      while ($i -lt $text.Length -and $depth -gt 0) {
        $c = $text[$i]
        if ($c -eq '{') { $depth++ }
        elseif ($c -eq '}') { $depth-- }
        elseif ($c -eq '`') { $inner = Read-SanityJsTemplate $text $i; $i = $inner.End }
        elseif ($c -eq "'" -or $c -eq '"') { $inner = Read-SanityJsQuoted $text $i; $i = $inner.End }
        $i++
      }
      [void]$sb.Append('${}'); continue
    }
    [void]$sb.Append($c); $i++
  }
  return @{ Value = $sb.ToString(); End = $i }
}
function ConvertTo-SanityApiPath([string]$raw) {
  # returns normalized path (segments, * for dynamic) or $null when the base is dynamic
  $p = $raw -replace '\$\{\}', '*'
  $p = $p -replace '\?.*$', ''
  if ($p -match '^\*') { return $null }
  if ($p -match '^https?://[^/]+(/.*)$') { $p = $Matches[1] }
  if ($p -notmatch '^/') { return $null }
  if ($p -match '^/api(/|$)') { $p = $p.Substring(4) }
  if (-not $p) { $p = '/' }
  $p = $p -replace '/+', '/'
  $p = $p -replace '\*+', '*'
  # A dynamic chunk glued to the last segment with no further "/" after it is a
  # query string, not a path segment — including when the "?" itself lives
  # inside the interpolation, as in `/containers${envQuery}${q ? '&' : '?'}all=1`.
  $p = $p -replace '([^/])\*[^/]*$', '$1'
  $p = $p -replace '([^/])\*', '$1*'
  if ($p.Length -gt 1) { $p = $p.TrimEnd('/') }
  if (-not $p) { $p = '/' }
  return $p
}
# Whole-path equality with * as a one-segment wildcard on either side. A consumer's trailing * is a
# real id segment, not the open-ended template tail Test-SanityPathMatch tolerates for UI calls.
function Test-SanityPathExact([string]$a, [string]$b) {
  $x = @($a.Trim('/').Split('/') | Where-Object { $_ -ne '' }); $y = @($b.Trim('/').Split('/') | Where-Object { $_ -ne '' })
  if ($x.Length -ne $y.Length) { return $false }
  for ($i = 0; $i -lt $x.Length; $i++) { if ($x[$i] -ne '*' -and $y[$i] -ne '*' -and $x[$i] -ne $y[$i]) { return $false } }
  return $true
}
function Test-SanityPathMatch([string]$fe, [string]$be) {
  $a = @($fe.Trim('/').Split('/') | Where-Object { $_ -ne '' }); $b = @($be.Trim('/').Split('/') | Where-Object { $_ -ne '' })
  for ($i = 0; $i -lt $b.Length; $i++) {
    if ($b[$i] -eq '**') { return $true }
    if ($i -ge $a.Length) { return $false }
    if ($a[$i] -eq '*' -and $i -eq $a.Length - 1 -and $b.Length -gt $a.Length) { return $true }
    if ($a[$i] -eq '*' -or $b[$i] -eq '*') { continue }
    if ($a[$i] -match '\*') { $rx = '^' + ([regex]::Escape($a[$i]) -replace '\\\*', '.*') + '$'; if ($b[$i] -match $rx) { continue } else { return $false } }
    if ($b[$i] -match '\*') { $rx = '^' + ([regex]::Escape($b[$i]) -replace '\\\*', '.*') + '$'; if ($a[$i] -match $rx) { continue } else { return $false } }
    if ($a[$i] -ne $b[$i]) { return $false }
  }
  return ($a.Length -eq $b.Length)
}

# ---- 1. Frontend catalog ----
$feCalls = New-Object System.Collections.Generic.List[object]   # Verb, Path, File, Line, Raw
$dynamic = @()
# The client's methods are get / download / post / put / patch / del — there is
# no `api.delete`. Matching that name meant every DELETE call and every file
# download was invisible to this check, and requiring `api.` and the verb to be
# adjacent hid every call written as a chain across lines.
$apiRx = New-Object System.Text.RegularExpressions.Regex('\bapi\s*\.\s*(get|download|post|put|patch|del|delete)\b\s*(?:<[^(]*?>)?\s*\(\s*')
$urlLitRx = New-Object System.Text.RegularExpressions.Regex("(['`"``])(/api/[^'`"``\s]*)")
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/frontend', 'widgets', 'nodes') -Include @('*.ts', '*.tsx', 'index.html') -ExcludeMatch @('\.test\.tsx?$', '\.spec\.tsx?$', '\.stories\.tsx?$', '\.d\.ts$'))) {
  $text = Get-SanityText $f.Full
  foreach ($m in $apiRx.Matches($text)) {
    $verb = $m.Groups[1].Value.ToUpperInvariant()
    # `del` is DELETE; `download` is a GET that returns the raw Response.
    if ($verb -eq 'DEL') { $verb = 'DELETE' }
    if ($verb -eq 'DOWNLOAD') { $verb = 'GET' }
    $idx = $m.Index + $m.Length
    if ($idx -ge $text.Length) { continue }
    $q = $text[$idx]
    $line = Get-SanityLineNumber -Text $text -Index $m.Index
    if ($q -eq '`') { $lit = Read-SanityJsTemplate $text $idx }
    elseif ($q -eq "'" -or $q -eq '"') { $lit = Read-SanityJsQuoted $text $idx }
    else { $dynamic += New-SanityHit -File $f.Rel -Line $line -Text "api.$($verb.ToLower())(<non-literal>)"; continue }
    $path = ConvertTo-SanityApiPath $lit.Value
    if ($null -eq $path) { $dynamic += New-SanityHit -File $f.Rel -Line $line -Text "api.$($verb.ToLower())(`"$($lit.Value)`") - dynamic base"; continue }
    $feCalls.Add([pscustomobject]@{ Verb = $verb; Path = $path; File = $f.Rel; Line = $line; Raw = $lit.Value })
  }
  # fetch('/api/...'), new EventSource('/api/...'), new WebSocket('.../api/...'), plain '/api/...' literals
  foreach ($m in $urlLitRx.Matches($text)) {
    $raw = $m.Groups[2].Value
    if ($raw -eq '/api/' -or $raw -eq '/api') { continue }
    $lineStart = $text.LastIndexOf("`n", [Math]::Max(0, $m.Index - 1)) + 1
    if ($text.Substring($lineStart, $m.Index - $lineStart) -match '^\s*(//|\*|/\*)') { continue }
    $before = $text.Substring([Math]::Max(0, $m.Index - 80), [Math]::Min(80, $m.Index))
    if ($before -match 'api\.(get|post|put|patch|delete)\b[^(]*\(\s*$') { continue }   # already captured above
    if ($m.Groups[1].Value -eq '`') { $raw = (Read-SanityJsTemplate $text $m.Index).Value }   # full template incl. nested quotes
    $path = ConvertTo-SanityApiPath ($raw -replace '\$\{[^}]*\}', '${}')
    if ($null -eq $path) { continue }
    $after = $text.Substring($m.Index, [Math]::Min(600, $text.Length - $m.Index))
    $verb = 'ANY'   # unknown until proven: only fetch/EventSource/WebSocket/xhr.open carry a verb the scan can read
    if ($before -match '\b(fetch|EventSource|WebSocket)\s*\(\s*$') { $verb = 'GET' }
    $vm = [regex]::Match($after, "method:\s*['`"](GET|POST|PUT|PATCH|DELETE)['`"]")
    if ($vm.Success) { $verb = $vm.Groups[1].Value }
    $xm = [regex]::Match($before, "\.open\(\s*['`"](GET|POST|PUT|PATCH|DELETE)['`"]\s*,\s*$")
    if ($xm.Success) { $verb = $xm.Groups[1].Value }
    $feCalls.Add([pscustomobject]@{ Verb = $verb; Path = $path; File = $f.Rel; Line = (Get-SanityLineNumber -Text $text -Index $m.Index); Raw = $raw })
  }
}
$fePaths = @($feCalls | ForEach-Object { $_.Path } | Sort-Object -Unique)
Add-SanityCheck -Report $report -Id 1 -Title 'Frontend API paths catalogued' -Hits @($fePaths) -Mode info -Severity info -Note ("{0} call sites, {1} distinct normalized paths. {2} calls use a non-literal or dynamic-base URL and cannot be checked." -f $feCalls.Count, $fePaths.Count, $dynamic.Count)
Add-SanityCheck -Report $report -Id '1b' -Title 'Frontend API calls with non-literal / dynamic-base URLs (unchecked)' -Hits $dynamic -Mode info -Severity info

# ---- 2. Backend catalog ----
$beRoutes = New-Object System.Collections.Generic.List[object]   # Verb, Path, File, Line
# The path argument MUST start with "/": chi panics on a pattern that does not,
# so every real route registration has one. Without that anchor the receiver was
# never constrained, and `r.URL.Query().Get("tail")`, `r.Header.Get("X-Forwarded-For")`
# and `values.Get("page")` all registered as routes named `tail`, `X-Forwarded-For`
# and `page` — 90 of this check's 123 review hits were query params and headers.
$verbRx = New-Object System.Text.RegularExpressions.Regex('(?:\.|^\s*)(Get|Post|Put|Patch|Delete|Head|Options|Handle|HandleFunc)\(\s*"(/[^"]*)"')
$methodRx = New-Object System.Text.RegularExpressions.Regex('\.(Method|MethodFunc)\(\s*"?(?:http\.Method)?([A-Za-z]+)"?\s*,\s*"(/[^"]*)"')
$routeRx = New-Object System.Text.RegularExpressions.Regex('\.Route\(\s*"([^"]*)"')
$mountRx = New-Object System.Text.RegularExpressions.Regex('\.Mount\(\s*"([^"]*)"')
# A helper `func mountXRoutes(r *apispec.Router, ...)` registers its routes
# relative to the group its caller passes in (`debugshell.MountContainerRoutes(r,
# app)` inside the containers module's `/{id}` group). Its routes are recorded
# under the helper's name, and rebased onto each call site's prefix afterwards,
# so `/debug-shell` is catalogued as `/containers/*/debug-shell`.
$nestedFuncs = New-Object 'System.Collections.Generic.HashSet[string]'
$nestedCalls = New-Object System.Collections.Generic.List[object]
$nestedDeclRx = New-Object System.Text.RegularExpressions.Regex('^func\s+(?:\([^)]*\)\s*)?(\w+)\(\s*r\s+\*apispec\.Router')
$nestedCallRx = New-Object System.Text.RegularExpressions.Regex('\b(\w+)\(\s*r\s*[,)]')
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend/modules', 'src/backend/core/router', 'src/backend/main.go') -Include @('*.go') -ExcludeMatch @('_test\.go$'))) {
  $lines = Get-SanityLines $f.Full
  $depth = 0; $stack = @()   # entries: @{Depth; Prefix}
  $curFunc = ''
  for ($i = 0; $i -lt $lines.Length; $i++) {
    $line = $lines[$i]
    if ($line -match '^\s*//') { continue }
    $dm = $nestedDeclRx.Match($line)
    if ($dm.Success) { $curFunc = $dm.Groups[1].Value; [void]$nestedFuncs.Add($curFunc) }
    elseif ($line -match '^func\s') { $curFunc = '' }
    $stripped = Remove-SanityStringLiterals $line
    $prefix = ($stack | ForEach-Object { $_.Prefix }) -join ''
    if ($line -notmatch '^func\s') {
      foreach ($cm in $nestedCallRx.Matches($line)) { $nestedCalls.Add([pscustomobject]@{ Name = $cm.Groups[1].Value; Prefix = $prefix }) }
    }
    $routeCountBefore = $beRoutes.Count
    $rm = $routeRx.Match($line)
    if ($rm.Success) {
      $opens = ([regex]::Matches($stripped, '\{')).Count; $closes = ([regex]::Matches($stripped, '\}')).Count
      $stack += @{ Depth = $depth + $opens - $closes; Prefix = $rm.Groups[1].Value }
    }
    $mm = $mountRx.Match($line)
    if ($mm.Success -and $mm.Groups[1].Value -ne '/api') { $beRoutes.Add([pscustomobject]@{ Verb = 'ANY'; Path = ((($prefix + $mm.Groups[1].Value) -replace '/+', '/').TrimEnd('/') + '/**'); File = $f.Rel; Line = ($i + 1) }) }
    foreach ($vm in $verbRx.Matches($line)) {
      $verb = $vm.Groups[1].Value.ToUpperInvariant(); if ($verb -in @('HANDLE', 'HANDLEFUNC')) { $verb = 'ANY' }
      $p = (($prefix + $vm.Groups[2].Value) -replace '/+', '/')
      if ($p.Length -gt 1) { $p = $p.TrimEnd('/') }; if (-not $p) { $p = '/' }
      $p = $p -replace '/\*$', '/**'
      $p = $p -replace '\{[^}]+\}', '*'
      $beRoutes.Add([pscustomobject]@{ Verb = $verb; Path = $p; File = $f.Rel; Line = ($i + 1) })
    }
    foreach ($vm in $methodRx.Matches($line)) {
      $p = (($prefix + $vm.Groups[3].Value) -replace '/+', '/'); if ($p.Length -gt 1) { $p = $p.TrimEnd('/') }
      $p = ($p -replace '/\*$', '/**') -replace '\{[^}]+\}', '*'
      $beRoutes.Add([pscustomobject]@{ Verb = $vm.Groups[2].Value.ToUpperInvariant(); Path = $p; File = $f.Rel; Line = ($i + 1) })
    }
    for ($k = $routeCountBefore; $k -lt $beRoutes.Count; $k++) { $beRoutes[$k] | Add-Member -NotePropertyName Func -NotePropertyValue $curFunc -Force }
    $depth += ([regex]::Matches($stripped, '\{')).Count - ([regex]::Matches($stripped, '\}')).Count
    $stack = @($stack | Where-Object { $_.Depth -le $depth })
  }
}
$rebased = New-Object System.Collections.Generic.List[object]
foreach ($route in $beRoutes) {
  $fn = ''; if ($route.PSObject.Properties['Func']) { $fn = $route.Func }
  $sites = @()
  if ($fn -and $nestedFuncs.Contains($fn)) { $sites = @($nestedCalls | Where-Object { $_.Name -eq $fn -and $_.Prefix }) }
  if ($sites.Count -eq 0) { $rebased.Add($route); continue }
  foreach ($site in $sites) {
    $cp = $site.Prefix -replace '\{[^}]+\}', '*'
    $np = (($cp + $route.Path) -replace '/+', '/')
    $rebased.Add([pscustomobject]@{ Verb = $route.Verb; Path = $np; File = $route.File; Line = $route.Line; Func = $fn })
  }
}
$beRoutes = $rebased
$bePaths =@($beRoutes | ForEach-Object { $_.Path } | Sort-Object -Unique)
Add-SanityCheck -Report $report -Id 2 -Title 'Backend routes catalogued' -Hits @($bePaths) -Mode info -Severity info -Note ("{0} route registrations, {1} distinct normalized paths (r.Route prefixes applied; /api is implicit)." -f $beRoutes.Count, $bePaths.Count)

# ---- 3. Frontend paths with no backend registration ----
# A plugin declares its routes in plugin.yaml (`/api/vms`, `/api/identity-ldap/auth`)
# and the loader registers them at runtime, so the Go catalog above never sees
# them. The manifests are the source of truth for those, read through the same
# collector the frontend contract test uses.
$pluginRoutePaths = @()
$pluginRoutesJs = Join-Path $PSScriptRoot 'plugin-routes.mjs'
if ((Get-Command node -ErrorAction SilentlyContinue) -and (Test-Path -LiteralPath $pluginRoutesJs)) {
  $pluginJson = & node -e "import(process.argv[1]).then(m => console.log(JSON.stringify(m.collectPluginRoutes())))" ([System.Uri]$pluginRoutesJs).AbsoluteUri 2>$null
  if ($LASTEXITCODE -eq 0 -and $pluginJson) {
    $pluginRoutePaths = @(($pluginJson | ConvertFrom-Json) | ForEach-Object { ConvertTo-SanityApiPath ($_.path -replace '\{[^}]*\}', '*') } | Where-Object { $_ } | Sort-Object -Unique)
  }
}
$missing = @(); $missingAccepted = @()
$acceptRx = '^/(plugins|p)(/|$)'
foreach ($p in $fePaths) {
  $match = $false
  foreach ($b in $bePaths) { if (Test-SanityPathMatch $p $b) { $match = $true; break } }
  if ($match) { continue }
  $sites = @($feCalls | Where-Object { $_.Path -eq $p } | ForEach-Object { "$($_.File):$($_.Line)" } | Sort-Object -Unique)
  $hit = New-SanityHit -File $sites[0].Split(':')[0] -Line ([int]$sites[0].Split(':')[1]) -Text ("{0} - no backend route; called from {1}" -f $p, (($sites | Select-Object -First 4) -join ', '))
  $isPluginRoute = $false
  foreach ($pp in $pluginRoutePaths) { if (Test-SanityPathMatch $p $pp) { $isPluginRoute = $true; break } }
  if ($p -match $acceptRx -or $isPluginRoute) { $missingAccepted += $hit } else { $missing += $hit }
}
Add-SanityCheck -Report $report -Id 3 -Title 'Frontend paths with no backend route' -Hits $missing -Mode strict -Severity high `
  -Remediation 'Add the route, delete the call, or fix the path. Templated paths assembled outside the literal may be false positives - check the string assembly.' -Note 'Severity high when the hook sits on a primary flow, medium when behind a feature flag or admin-only surface.'
Add-SanityCheck -Report $report -Id '3b' -Title 'Frontend paths served by dynamic plugin routes (accepted)' -Hits $missingAccepted -Mode info -Severity info -Note 'Plugin routes are registered at runtime (plugin_runtime); cross-check against the plugin manifests.'

# ---- 4. Backend routes with no frontend caller ----
# The catalog above only sees a call whose URL is a literal at the call site.
# `const base = '/rbac/policies'`, a ternary that picks between two paths, and a
# WebSocket template whose host prefix is interpolated all reach real routes
# through a variable — they are the calls 1b already reports as unreadable, and
# check 4 must not also report their routes as dead. A path literal for the
# route appearing anywhere in the frontend is enough evidence for a low-severity
# check; it lands in 4c as inventory rather than being silently dropped.
# Harvested once, not rescanned per route: matching 429 routes against the whole
# concatenated frontend corpus was minutes of regex. One pass collects every
# path-ish literal instead, and the comparison reuses Test-SanityPathMatch.
$looseRx = New-Object System.Text.RegularExpressions.Regex('(?:[''"`]|\})(/[A-Za-z0-9_\-/{}$.*]+)')
function Get-SanityPathLiterals([string[]]$Paths, [string[]]$Include) {
  $set = New-Object 'System.Collections.Generic.HashSet[string]'
  foreach ($f in @(Get-SanityFiles -Root $Root -Paths $Paths -Include $Include -ExcludeMatch @('\.test\.tsx?$', '\.d\.ts$'))) {
    foreach ($m in $looseRx.Matches((Get-SanityText $f.Full))) {
      $raw = $m.Groups[1].Value -replace '\$\{[^}]*\}', '*' -replace '\{[^}]*\}', '*'
      # An absolute WebSocket/EventSource URL interpolates the origin first, so
      # the capture starts at the wrong `}`: `//*/api/system/host-terminal/ws`.
      # The app's own `/api/` prefix is the unambiguous start of the path.
      if ($raw -match '^.*?(/api/.*)$') { $raw = $Matches[1] }
      $lit = ConvertTo-SanityApiPath $raw
      # A literal whose first segment is dynamic (`/${base}/x`, and so `/*/x`
      # here) carries no evidence about which route it is: Test-SanityPathMatch
      # treats a leading `*` as matching any deeper path, so one such entry would
      # accept every route in the catalog. The static-prefix fallback below is
      # what covers those call sites.
      if ($lit -and $lit -ne '/' -and $lit -notmatch '^/\*') { $set.Add($lit) | Out-Null }
    }
  }
  return $set
}
$feLooseLits = Get-SanityPathLiterals -Paths @('src/frontend', 'widgets', 'nodes') -Include @('*.ts', '*.tsx')
# Plugin frontends are separate bundles that legitimately call host routes, and
# nothing above reads them, so a route only a plugin calls used to report as
# having no caller at all. They stay out of checks 1 and 3 on purpose: a
# plugin's own routes are registered at runtime under its slug, so folding them
# into the strict catalog would turn third-party plugin code into check-3
# failures. Here they are evidence of a caller, and nothing more.
$pluginLits = Get-SanityPathLiterals -Paths @('src/plugins') -Include @('*.ts', '*.tsx', '*.js', '*.jsx')
# Plugin sources also carry filesystem paths (/etc/krb5.keytab, /srv/backups),
# asset URLs and half-captured templates. A literal still holding an unresolved
# interpolation is not a path this check can attribute, and a bad entry here
# silently moves a real finding out of check 4, so drop them.
$pluginLits = @($pluginLits | Where-Object { $_ -notmatch '[${}]' })
$pluginLits = New-Object 'System.Collections.Generic.HashSet[string]' (,[string[]]$pluginLits)
# The CLI (generated client + raw requests) and the Terraform provider are
# consumers of the same routes the UI never touches. non-frontend-callers.mjs maps
# each generated-client method to its route through openapi.json and reads raw
# "/api/..." literals, so a route only they call is a caller, not an orphan.
$consumerCalls = @()
$consumerJs = Join-Path $PSScriptRoot 'non-frontend-callers.mjs'
if ((Get-Command node -ErrorAction SilentlyContinue) -and (Test-Path -LiteralPath $consumerJs)) {
  $consumerJson = & node $consumerJs $Root 2>$null
  if ($LASTEXITCODE -eq 0 -and $consumerJson) { $consumerCalls = @(($consumerJson -join "`n") | ConvertFrom-Json) }
}
$consumerPaths = @($consumerCalls | ForEach-Object { $_.path } | Where-Object { $_ } | Sort-Object -Unique)
function Test-SanityRefIn($set, [string]$path) {
  if ($path -eq '/' -or $path -eq '/**') { return $false }
  foreach ($lit in $set) { if (Test-SanityPathMatch $lit $path) { return $true } }
  # A parameterised route often has no literal of its own - only its base does,
  # as in `const base = '/rbac/policies'` then `${base}/${id}/test`. Fall back to
  # the static prefix, but only when the route has a parameter and the prefix is
  # at least two segments deep: a one-segment prefix like /notifications matches
  # the sidebar's router path and would excuse every sub-route under it.
  if ($path -notmatch '\*') { return $false }
  $static = ($path -split '/\*')[0].TrimEnd('/')
  if (@($static.Trim('/').Split('/') | Where-Object { $_ -ne '' }).Count -lt 2) { return $false }
  return $set.Contains($static)
}
$unused = @(); $unusedAccepted = @(); $unusedLoose = @(); $unusedPlugin = @()
# `^/api(/|$)` is the plugin catch-all: router.go mounts app.PluginRoutes at "/"
# inside r.Route("/api"), so it resolves to /api/** while every module route is
# registered without the prefix. It serves routes registered at runtime and can
# never match a frontend path here, so it is inventory, not a dead route.
$beAcceptRx = '^/(agent|health|version|openapi|scim|webhooks|plugins)(/|$)|^/api(/|$)|/webhooks?(/|$)|/internal/|/acs$|/callback$|/metadata$|/oauth/|/sso/|^/auth/(login|logout|setup|status|refresh)'
# A route published in the OpenAPI spec is a product surface: API keys, CI and
# scripts are its consumers, and "this app's own frontend does not call it" says
# nothing about whether it is used. Those are inventory, not a review item.
$documentedPaths = New-Object 'System.Collections.Generic.HashSet[string]'
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('src/backend/modules/openapi') -Include @('*.go'))) {
  foreach ($m in [regex]::Matches((Get-SanityText $f.Full), '"(/[A-Za-z0-9_\-/{}\.]*)":\s*path\(')) {
    $documentedPaths.Add((ConvertTo-SanityApiPath ($m.Groups[1].Value -replace '\{[^}]*\}', '*'))) | Out-Null
  }
}
# The hand-written endpoint reference (`| GET | `/api/x` | ...` rows) is the
# other published surface: a route listed there is documented for API-key, CI
# and script consumers on purpose, whether or not the UI calls it.
foreach ($f in @(Get-SanityFiles -Root $Root -Paths @('.documentation/api/endpoints') -Include @('*.md'))) {
  foreach ($line in (Get-SanityLines $f.Full)) {
    if ($line -notmatch '^\s*\|') { continue }
    foreach ($m in [regex]::Matches($line, '`(/api/[A-Za-z0-9_\-/{}\.*]*)`')) {
      $documentedPaths.Add((ConvertTo-SanityApiPath ($m.Groups[1].Value -replace '\{[^}]*\}', '*'))) | Out-Null
    }
  }
}
# accepted-routes.json lists routes the UI reaches through a composed URL that
# the literal scan cannot follow. An entry counts only while its caller file
# still exists and still names the route's last static segment, so a deleted
# call site turns the route back into a finding instead of hiding it.
$acceptedRoutes = @()
$acceptedFile = Join-Path $PSScriptRoot 'accepted-routes.json'
if (Test-Path -LiteralPath $acceptedFile) {
  foreach ($entry in @((Get-Content -Raw -LiteralPath $acceptedFile | ConvertFrom-Json))) {
    $callerFull = Join-Path $Root ($entry.caller -replace '/', [System.IO.Path]::DirectorySeparatorChar)
    if (-not (Test-Path -LiteralPath $callerFull)) { continue }
    $statics = @($entry.path.Trim('/').Split('/') | Where-Object { $_ -and $_ -ne '*' })
    if ($statics.Count -eq 0 -or -not (Get-SanityText $callerFull).Contains($statics[-1])) { continue }
    $acceptedRoutes += [pscustomobject]@{ norm = $entry.path }
  }
}
# accepted.txt: one "<METHOD> <path>|<reason>" per line, the reason a closed category.
# An accepted route is counted, never listed, so 4b only ever shows new orphans.
$acceptedCategories = @('agent', 'websocket', 'binary-download', 'device-auth', 'inbound-webhook', 'cli-only', 'terraform-only', 'public-callback', 'probe', 'published-api')
$acceptedTxt = @{}
$acceptedByCategory = @{}
$acceptedBadLines = @()
$acceptedTxtFile = Join-Path $PSScriptRoot 'accepted.txt'
if (Test-Path -LiteralPath $acceptedTxtFile) {
  foreach ($l in (Get-SanityLines $acceptedTxtFile)) {
    $t = $l.TrimStart([char]0xFEFF).Trim()
    if (-not $t -or $t.StartsWith('#')) { continue }
    $parts = $t -split '\|', 2
    $lhs = $parts[0].Trim() -split '\s+', 2
    $cat = if ($parts.Count -gt 1) { $parts[1].Trim() } else { '' }
    $reasonKey = ($cat -split '[:\s]', 2)[0]
    if ($lhs.Count -ne 2 -or $acceptedCategories -notcontains $reasonKey) { $acceptedBadLines += New-SanityHit -File 'scripts/sanity/missing-api/accepted.txt' -Line 0 -Text "malformed or unknown category: $t"; continue }
    $acceptedTxt[($lhs[0].ToUpperInvariant() + ' ' + $lhs[1])] = $reasonKey
  }
}
$acceptedCount = 0; $acceptedStructural = 0; $acceptedUnmatched = @($acceptedTxt.Keys)
$unusedConsumer = @()
foreach ($b in $bePaths) {
  $match = $false
  foreach ($p in $fePaths) { if (Test-SanityPathMatch $p $b) { $match = $true; break } }
  if ($match) { continue }
  $regs =@($beRoutes | Where-Object { $_.Path -eq $b })
  $verbs = @($regs | ForEach-Object { $_.Verb } | Sort-Object -Unique)
  $hit = New-SanityHit -File $regs[0].File -Line $regs[0].Line -Text ("{0} [{1}] - no frontend caller" -f $b, ($verbs -join ','))
  # The plugin catch-all (/api/**) serves routes registered at runtime; it can never have a literal caller.
  if ($b -eq '/api/**') { $acceptedStructural++; continue }
  if (Test-SanityRefIn $pluginLits $b) { $unusedPlugin += $hit; continue }
  $consumed = $false
  foreach ($cp in $consumerPaths) { if (Test-SanityPathExact $cp $b) { $consumed = $true; break } }
  if ($consumed) { $unusedConsumer += $hit; continue }
  $keys = @($verbs | ForEach-Object { "$_ $b" })
  $unlisted = @($keys | Where-Object { -not $acceptedTxt.ContainsKey($_) -and -not $acceptedTxt.ContainsKey("ANY $b") })
  if ($unlisted.Count -eq 0) {
    foreach ($k in $keys) { $ck = if ($acceptedTxt.ContainsKey($k)) { $k } else { "ANY $b" }; $acceptedByCategory[$acceptedTxt[$ck]] = 1 + [int]$acceptedByCategory[$acceptedTxt[$ck]]; $acceptedCount++; $acceptedUnmatched = @($acceptedUnmatched | Where-Object { $_ -ne $ck }) }
    continue
  }
  # accepted-routes.json is validated evidence (the caller file must still name the route), so it outranks the published-surface bucket.
  if ($acceptedRoutes | Where-Object { Test-SanityPathExact $_.norm $b }) { $unusedLoose += $hit; continue }
  $documented = $false
  foreach ($d in $documentedPaths) { if ($d -and (Test-SanityPathMatch $d $b)) { $documented = $true; break } }
  if ($documented -or ($b -match $beAcceptRx)) { $unusedAccepted += $hit }
  elseif (Test-SanityRefIn $feLooseLits $b) { $unusedLoose += $hit }
  else { $unused += $hit }
}
# An accepted.txt line that matches no remaining route is stale: the route was removed or gained a caller.
$staleAccepted = @($acceptedUnmatched | ForEach-Object { New-SanityHit -File 'scripts/sanity/missing-api/accepted.txt' -Line 0 -Text "stale entry (no such orphan route any more): $_" })
Add-SanityCheck -Report $report -Id 4 -Title 'Backend routes with no frontend caller and no published spec' -Hits $unused -Mode review -Severity low `
  -Remediation 'Likely dead code or a half-finished feature. If it is an endpoint for an external consumer, document it in modules/openapi so it stops reading as orphaned.' `
  -Note 'Accepted automatically: routes published in the OpenAPI spec, plus /agent, /health, /version, /openapi, /scim, webhooks, /internal, IdP callbacks and pre-auth routes.'
$acceptedSummary = (($acceptedByCategory.Keys | Sort-Object | ForEach-Object { "{0}: {1}" -f $_, $acceptedByCategory[$_] }) -join ', ')
Add-SanityCheck -Report $report -Id '4b' -Title 'Backend routes with no caller anywhere (published or accepted category, not in accepted.txt)' -Hits ($unusedAccepted + $acceptedBadLines) -Mode info -Severity info `
  -Remediation 'Classify each route in scripts/sanity/missing-api/accepted.txt with a closed category (agent, websocket, binary-download, device-auth, inbound-webhook, cli-only, terraform-only, public-callback, probe, published-api), or treat it as a dead route and remove it.' `
  -Note ("{0} route verb(s) accepted via accepted.txt (counted, not listed){1}; {2} structural (/api/** plugin catch-all). Callers counted: src/frontend, widgets, nodes, plugin frontends, src/cli, src/terraform-provider-mcharbor." -f $acceptedCount, $(if ($acceptedSummary) { " [$acceptedSummary]" } else { '' }), $acceptedStructural)
Add-SanityCheck -Report $report -Id '4f' -Title 'accepted.txt entries that no longer match an orphan route (stale)' -Hits $staleAccepted -Mode review -Severity low -Remediation 'Delete the line: the route was removed, or it now has a caller.'
Add-SanityCheck -Report $report -Id '4c' -Title 'Backend routes reached through a non-literal URL (accepted)' -Hits $unusedLoose -Mode info -Severity info -Note 'A path literal for the route exists in the frontend but the call site assembles the URL from a variable, so the strict catalog cannot attribute it. Cross-check against 1b.'
Add-SanityCheck -Report $report -Id '4e' -Title 'Backend routes called only by the CLI or the Terraform provider (accepted)' -Hits $unusedConsumer -Mode info -Severity info -Note 'The route is reached through the generated client (operationId -> route via openapi.json) or a raw /api/ literal in src/cli or src/terraform-provider-mcharbor. The UI never calls it, by design.'
Add-SanityCheck -Report $report -Id '4d' -Title 'Backend routes called by a plugin frontend (accepted)' -Hits $unusedPlugin -Mode info -Severity info -Note 'Nothing in src/frontend calls these, but a plugin under src/plugins does. Plugin frontends are separate bundles with their own callers; their own slug-scoped routes are registered at runtime and are not checked here.'

# ---- 5. Verb mismatch ----
$verbMismatch = @()
foreach ($c in ($feCalls | Sort-Object Path, Verb -Unique)) {
  $matching = @($beRoutes | Where-Object { Test-SanityPathMatch $c.Path $_.Path })
  if ($matching.Count -eq 0) { continue }
  if ($c.Verb -eq 'ANY') { continue }
  if ($matching | Where-Object { $_.Verb -eq $c.Verb -or $_.Verb -eq 'ANY' }) { continue }
  $verbMismatch += New-SanityHit -File $c.File -Line $c.Line -Text ("{0} {1} - backend registers only [{2}] for {3}" -f $c.Verb, $c.Path, (($matching | ForEach-Object { $_.Verb } | Sort-Object -Unique) -join ','), $matching[0].Path)
}
Add-SanityCheck -Report $report -Id 5 -Title 'Verb mismatch on otherwise-matching paths' -Hits $verbMismatch -Mode strict -Severity high -Remediation 'Align the HTTP verb on one side; the call currently 404s/405s silently.'

Add-SanityNote -Report $report -Text 'Path normalization: chi {param} and JS ${expr} become *; a trailing ${query} glued to a segment is treated as a query string; chi /* catch-alls match any suffix.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
