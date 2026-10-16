# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  App Store catalog sanity check. Mirrors .instructions/sanity/app-catalog.md.
.EXAMPLE
  .\scripts\sanity\app-catalog\check.ps1
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

$report = New-SanityReport -Name 'app-catalog' -Title 'App Catalog Sanity' -Scope 'apps/*.json, src/backend/modules/appstore, src/frontend/modules/appstore' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$appFiles = @(Get-SanityFiles -Root $Root -Paths @('apps') -Include @('*.json') | Where-Object { $_.Name -ne 'index.json' })
$parseErrors = @(); $apps = @()
foreach ($f in $appFiles) {
  try {
    $obj = ConvertFrom-SanityJson (Get-SanityText $f.Full)
    $apps += [pscustomobject]@{ File = $f; Data = $obj }
  } catch {
    $parseErrors += New-SanityHit -File $f.Rel -Line 0 -Text ("parse error: " + $_.Exception.Message)
  }
}

# 1. JSON parses
Add-SanityCheck -Report $report -Id 1 -Title 'App JSON parses' -Hits $parseErrors -Mode strict -Severity high -Remediation 'Fix the JSON syntax; a single broken file aborts catalog seeding.' -Note ("{0} app files scanned." -f $appFiles.Count) -Command 'Get-ChildItem apps -Filter *.json | % { Get-Content -Raw $_ | ConvertFrom-Json }'

# 2. Required fields
$required = @('slug', 'name', 'description', 'category', 'image', 'logo', 'website', 'docsUrl', 'ports', 'volumes', 'envVars', 'version')
$missingFields = @()
foreach ($a in $apps) {
  $missing = @()
  foreach ($field in $required) {
    $v = Get-SanityJsonValue $a.Data $field
    if ($null -eq $v) { $missing += $field; continue }
    if ($v -is [string] -and [string]::IsNullOrWhiteSpace($v)) { $missing += $field }
  }
  if ($missing.Count) { $missingFields += New-SanityHit -File $a.File.Rel -Line 0 -Text ('missing ' + ($missing -join ', ')) }
}
Add-SanityCheck -Report $report -Id 2 -Title 'Required card and install fields present' -Hits $missingFields -Mode strict -Severity medium -Remediation 'Cards need logo/image/category/version; the install dialog needs ports/volumes/envVars; website and docsUrl must not be empty (an empty array is accepted for ports/volumes/envVars).'

# 3. Filename, slug and image shape
$shape = @()
foreach ($a in $apps) {
  $slug = "$(Get-SanityJsonValue $a.Data 'slug')"
  $image = "$(Get-SanityJsonValue $a.Data 'image')"
  if ($a.File.Name -cne "$slug.json") { $shape += New-SanityHit -File $a.File.Rel -Line 0 -Text "slug/filename mismatch, expected $slug.json" }
  if ($slug -cnotmatch '^[a-z0-9]+(-[a-z0-9]+)*$') { $shape += New-SanityHit -File $a.File.Rel -Line 0 -Text "invalid slug '$slug'" }
  if ($image -and $image -notmatch '^[^\s:]+(/[^\s:]+)*:[^\s:/]+$' -and $image -notmatch '^[^\s]+:\d+/[^\s:]+(/[^\s:]+)*:[^\s:/]+$') { $shape += New-SanityHit -File $a.File.Rel -Line 0 -Text "image '$image' has no explicit tag" }
}
Add-SanityCheck -Report $report -Id 3 -Title 'Filename equals <slug>.json, kebab-case slug, image has an explicit tag' -Hits $shape -Mode strict -Severity medium -Remediation 'Rename the file, fix the slug, or pin the image tag (image:1.2.3, never a bare name).'

# 4. Port, volume, env var structure
$structure = @()
foreach ($a in $apps) {
  $issues = @()
  foreach ($port in (Get-SanityJsonArray $a.Data 'ports')) {
    if ($null -eq $port) { continue }
    $hostPort = Get-SanityJsonValue $port 'host'; $containerPort = Get-SanityJsonValue $port 'container'; $proto = Get-SanityJsonValue $port 'protocol'
    $hostN = 0; $contN = 0
    if (-not [int]::TryParse("$hostPort", [ref]$hostN)) { $hostN = 0 }
    if (-not [int]::TryParse("$containerPort", [ref]$contN)) { $contN = 0 }
    if ($hostN -lt 1 -or $contN -lt 1) { $issues += "invalid port mapping $hostPort->$containerPort" }
    if ($null -ne $proto -and "$proto" -notin @('tcp', 'udp')) { $issues += "unsupported protocol '$proto'" }
  }
  foreach ($vol in (Get-SanityJsonArray $a.Data 'volumes')) {
    if ($null -eq $vol) { continue }
    if ([string]::IsNullOrWhiteSpace("$(Get-SanityJsonValue $vol 'host')") -or [string]::IsNullOrWhiteSpace("$(Get-SanityJsonValue $vol 'container')")) { $issues += 'volume without host/container path' }
  }
  foreach ($env in (Get-SanityJsonArray $a.Data 'envVars')) {
    if ($null -eq $env) { continue }
    $key = Get-SanityJsonValue $env 'key'; $def = Get-SanityJsonValue $env 'default'; $desc = Get-SanityJsonValue $env 'description'
    if ([string]::IsNullOrWhiteSpace("$key") -or $null -eq $def -or [string]::IsNullOrWhiteSpace("$desc")) { $issues += "env var '$key' missing key/default/description" }
  }
  if ($issues.Count) { $structure += New-SanityHit -File $a.File.Rel -Line 0 -Text (($issues | Select-Object -Unique) -join '; ') }
}
Add-SanityCheck -Report $report -Id 4 -Title 'Port, volume and env var structure' -Hits $structure -Mode strict -Severity medium -Remediation 'Ports need host/container >= 1 and tcp/udp; volumes need both paths; env vars need key, default (may be empty string) and description.'

# 5. Store card copy regressions
$copy = @(Find-Pattern -Root $Root -Paths @('src/frontend/modules/appstore', 'src/frontend/core/i18n/locales') -Include @('*.ts', '*.tsx', '*.json') -Pattern 'availableForInstall|Available for install')
$badge = @(Find-Pattern -Root $Root -Paths @('src/frontend/modules/appstore/components/AppCard.tsx', 'src/frontend/modules/appstore/components/AppDetailDialog.tsx') -Pattern 'IconCheck|appStore\.installed\)')
Add-SanityCheck -Report $report -Id '5a' -Title '"Available for install" copy reintroduced' -Hits $copy -Mode strict -Severity low -Remediation 'Remove; cards show only the green Installed grouping.' -Command 'rg -n "availableForInstall|Available for install" src/frontend/modules/appstore src/frontend/core/i18n/locales'
Add-SanityCheck -Report $report -Id '5b' -Title 'Separate installed badge on card / detail footer' -Hits $badge -Mode review -Severity low -Remediation 'Installation-count badges inside the Installed grouping are accepted; a separate installed badge is not.' -Command 'rg -n "IconCheck|appStore\.installed\)" src/frontend/modules/appstore/components/AppCard.tsx'

# 6. Uninstall route and UI hook exist
$uninstall = @(Find-Pattern -Root $Root -Paths @('src/backend', 'src/frontend') -Include @('*.go', '*.ts', '*.tsx') -Pattern 'HandleUninstall|DELETE /app-store/installations|/app-store/installations/|useStreamUninstall|RemoveInstalledStack')
$uninstallMissing = @()
foreach ($needle in @('HandleUninstall', 'useStreamUninstall', 'RemoveInstalledStack')) {
  if (-not ($uninstall | Where-Object { $_.Text -match $needle })) { $uninstallMissing += "no reference to $needle found in src/backend or src/frontend" }
}
Add-SanityCheck -Report $report -Id 6 -Title 'Uninstall route, hook and stack removal adapter present' -Hits $uninstallMissing -Mode strict -Severity high -Remediation 'The install dialog exposes uninstall; the backend DELETE route, the useStreamUninstall hook and the stack removal adapter must all exist.' -Note ("{0} uninstall-related references found." -f $uninstall.Count) -Command 'rg -n "HandleUninstall|useStreamUninstall|RemoveInstalledStack" src/backend src/frontend'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
