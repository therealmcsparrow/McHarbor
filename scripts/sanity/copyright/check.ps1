# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Copyright header sanity check. Mirrors .instructions/sanity/copyright.md.
.EXAMPLE
  .\scripts\sanity\copyright\check.ps1
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

$report = New-SanityReport -Name 'copyright' -Title 'Copyright Header Sanity' -Scope 'src/backend, src/agent, src/frontend, src/cli, src/terraform-provider-mcharbor, src/plugins, widgets, nodes, scripts, docker (excluding vendor, node_modules, dist, coverage, *.umd.cjs)' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$expected = @{
  '.go'  = @('// Copyright (c) 2026 McSparrow. All rights reserved.', '// McHarbor is licensed under the McHarbor License. See LICENSE for details.')
  '.ts'  = @('// Copyright (c) 2026 McSparrow. All rights reserved.', '// McHarbor is licensed under the McHarbor License. See LICENSE for details.')
  '.tsx' = @('// Copyright (c) 2026 McSparrow. All rights reserved.', '// McHarbor is licensed under the McHarbor License. See LICENSE for details.')
  '.css' = @('/* Copyright (c) 2026 McSparrow. All rights reserved. */', '/* McHarbor is licensed under the McHarbor License. See LICENSE for details. */')
  '.sql' = @('-- Copyright (c) 2026 McSparrow. All rights reserved.', '-- McHarbor is licensed under the McHarbor License. See LICENSE for details.')
}

$sourceRoots = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend', 'src/agent', 'src/frontend', 'src/cli', 'src/terraform-provider-mcharbor', 'src/plugins', 'widgets', 'nodes', 'scripts', 'docker'))
$copyrightExclude = @('\.umd\.cjs$', '/node_modules/', '/dist/')
$files = @(Get-SanityFiles -Root $Root -Paths $sourceRoots -Include @('*.go', '*.ts', '*.tsx', '*.css', '*.sql') -ExcludeMatch $copyrightExclude)
$missing = @(); $wrongYear = @(); $wrongCompany = @(); $wrongLicense = @(); $checked = 0; $generated = 0
foreach ($f in $files) {
  $want = $expected[$f.Ext]
  if ($null -eq $want) { continue }
  $checked++
  $lines = Get-SanityLines $f.Full
  if (($lines | Select-Object -First 5) -match 'Code generated .* DO NOT EDIT') { $generated++; $checked--; continue }
  $offset = 0
  if ($lines.Length -gt 0 -and $lines[0].StartsWith('#!')) { $offset = 1 }
  $l0 = if ($lines.Length -gt $offset) { $lines[$offset].TrimStart([char]0xFEFF) } else { '' }
  $l1 = if ($lines.Length -gt ($offset + 1)) { $lines[$offset + 1] } else { '' }
  if ($l0 -eq $want[0] -and $l1 -eq $want[1]) { continue }
  $reason = 'header missing'
  if ($l0 -match 'Copyright') {
    if ($l0 -match 'Copyright \(c\) 20(2[0-5]|27|1\d)') { $reason = 'wrong year' }
    elseif ($l0 -match 'McDocker' -or $l0 -notmatch 'McSparrow') { $reason = 'wrong company name' }
    elseif ($l0 -ne $want[0]) { $reason = 'copyright line differs' }
    elseif ($l1 -ne $want[1]) { $reason = 'license line missing or wrong' }
  }
  $hit = New-SanityHit -File $f.Rel -Line 1 -Text ("{0}: '{1}'" -f $reason, $l0)
  switch ($reason) {
    'wrong year' { $wrongYear += $hit }
    'wrong company name' { $wrongCompany += $hit }
    'license line missing or wrong' { $wrongLicense += $hit }
    default { $missing += $hit }
  }
}
Add-SanityCheck -Report $report -Id 1 -Title 'Header missing or first line differs' -Hits $missing -Mode strict -Severity medium `
  -Remediation 'Prepend the two-line header (after any shebang) using the comment style for the file type.' -Note ("{0} files checked; {1} generated files (a 'Code generated ... DO NOT EDIT' marker in the first five lines) exempt." -f $checked, $generated)
Add-SanityCheck -Report $report -Id 2 -Title 'Wrong copyright year' -Hits $wrongYear -Mode strict -Severity low -Remediation 'Year must be 2026.'
Add-SanityCheck -Report $report -Id 3 -Title 'Wrong company name' -Hits $wrongCompany -Mode strict -Severity medium -Remediation 'Company is McSparrow; McDocker is the legacy name.'
Add-SanityCheck -Report $report -Id 4 -Title 'License line missing or wrong' -Hits $wrongLicense -Mode strict -Severity medium -Remediation 'Second line must read exactly: McHarbor is licensed under the McHarbor License. See LICENSE for details.'

# Additional grep: legacy names, wrong years, stray license lines anywhere in the file
$legacy = @(Find-Pattern -Root $Root -Paths $sourceRoots -Include @('*.go', '*.ts', '*.tsx', '*.css', '*.sql') -ExcludeMatch $copyrightExclude -Pattern 'McDocker|Copyright \(c\) 20(2[0-5]|27)')
$licenseLines = @(Find-Pattern -Root $Root -Paths $sourceRoots -Include @('*.go', '*.ts', '*.tsx', '*.css', '*.sql') -ExcludeMatch $copyrightExclude -Pattern 'licensed under' -NotMatch 'McHarbor is licensed under the McHarbor License\. See LICENSE for details\.')
Add-SanityCheck -Report $report -Id 5 -Title 'Legacy name or wrong year anywhere in source' -Hits $legacy -Mode strict -Severity low -Command 'rg -n "McDocker|Copyright \(c\) 20(2[0-5]|27)|licensed under" src/backend src/agent src/frontend src/cli src/terraform-provider-mcharbor'
Add-SanityCheck -Report $report -Id 6 -Title 'Non-standard "licensed under" lines' -Hits $licenseLines -Mode review -Severity low -Note 'Third-party license notices embedded in source are accepted; document them here.'

Add-SanityNote -Report $report -Text 'Generated stubs are only exempt when the project explicitly decides so and this result file documents the exemption.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
