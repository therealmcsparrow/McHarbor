# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Duplication sanity check. Mirrors .instructions/sanity/duplication.md.
.DESCRIPTION
  Near-duplicate file detection by normalised 5-line shingles and Jaccard
  similarity. Identifiers taken from each file's own path are normalised away,
  which is what makes groq_client.go and together_client.go comparable; without
  that, every vendor-prefixed clone reads as original code.

  The detection is Node (scripts/sanity/duplication/detect.mjs), the same way
  the i18n check delegates to verify-keys.mjs. One implementation is shared
  between this check and the modularize skill, so the two cannot disagree about
  what a duplicate is - the two-resolver problem .claude/rules/i18n-keys.md
  documents.

  Accepted duplication lives in accepted.json and does not fail. It appears in
  the drift watch instead, with each member's similarity to its family
  reference measured against a recorded baseline. Drift is the finding worth
  having: it means one copy of a family was fixed and the others were not.
.PARAMETER Paths
  Repo-relative subtrees to scan instead of the whole repo.
.PARAMETER Changed
  Scope to the subtrees touched in the working tree.
.PARAMETER UpdateBaseline
  Rewrite the drift baselines in accepted.json from the current tree. Do this
  deliberately, when a family has legitimately moved, never to clear a finding.
.EXAMPLE
  .\scripts\sanity\duplication\check.ps1
.EXAMPLE
  .\scripts\sanity\duplication\check.ps1 -Paths src/frontend -ReportThreshold 0.35
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [string[]]$Paths,
  [switch]$Changed,
  [switch]$UpdateBaseline,
  [double]$ReportThreshold = 0.55,
  [double]$FailThreshold = 0.70,
  [int]$MinLocForFail = 200
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot

# inv formats a number the same way on every machine. The default formatting
# follows the operator's culture, which turns 0.9732 into "0,9732" in a report
# and in a command line meant to be copied and pasted.
function inv {
  param([double]$Value, [int]$Digits = 4)
  return $Value.ToString('F' + $Digits, [System.Globalization.CultureInfo]::InvariantCulture)
}

if (-not $Stamp) { $Stamp = Get-SanityStamp }

$report = New-SanityReport -Name 'duplication' -Title 'Duplication Sanity' -Scope 'repository (src, widgets, nodes)' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$node = Get-Command node -ErrorAction SilentlyContinue
if (-not $node) {
  foreach ($id in 1, 2, 3, 4) {
    Add-SanityCheck -Report $report -Id $id -Title 'duplication' -Skipped -SkipReason 'node is not on PATH'
  }
  $summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
  $summary
  exit 0
}

$scope = @()
if ($Changed) {
  $scope = @(& git -C $Root status --porcelain 2>$null |
    ForEach-Object { ($_ -replace '^...', '') -replace '^"|"$', '' } |
    Where-Object { $_ -match '^(src|widgets|nodes)/' } |
    ForEach-Object { ($_ -split '/')[0..1] -join '/' } | Sort-Object -Unique)
}
if ($Paths) { $scope = $Paths }

$detect = Join-Path $PSScriptRoot 'detect.mjs'
$argv = @($detect, '--root', $Root, '--json',
  '--report-threshold', (inv $ReportThreshold 4),
  '--fail-threshold', (inv $FailThreshold 4),
  '--min-loc-fail', ([string]$MinLocForFail))
if ($scope.Count) { $argv += @('--paths', ($scope -join ',')) }
if ($UpdateBaseline) { $argv += '--update-baseline' }

$raw = & node @argv 2>$null
if (-not $raw) { throw 'detect.mjs produced no output' }
$data = ($raw -join "`n") | ConvertFrom-Json

function Convert-Cluster {
  param($Cluster)
  $head = $Cluster.files[0]
  $rest = @($Cluster.files | Select-Object -Skip 1 | ForEach-Object { $_.path })
  $text = "{0} files, {1} LOC, redundant~{2}, jaccard {3}-{4} :: {5}" -f `
    $Cluster.files.Count, $Cluster.loc, $Cluster.redundant, (inv $Cluster.minSimilarity 2), (inv $Cluster.maxSimilarity 2), ($rest -join ' | ')
  New-SanityHit -File $head.path -Line 0 -Text $text
}

$failing = @($data.failing | ForEach-Object { Convert-Cluster $_ })
$extended = @($data.extended | ForEach-Object {
  $h = Convert-Cluster $_
  New-SanityHit -File $h.File -Line 0 -Text ("[{0}] undeclared member(s): {1}" -f $_.family, ($_.unmatched -join ', '))
})
$reported = @($data.reported | ForEach-Object { Convert-Cluster $_ })
$drifted = @()
foreach ($w in $data.drifted) {
  foreach ($m in $w.drifted) {
    $drifted += New-SanityHit -File $m.path -Line 0 -Text ("[{0}] similarity to {1}: {2} -> {3} ({4})" -f `
      $w.family, (Split-Path $w.reference -Leaf), $m.baseline, $m.similarity, $m.delta)
  }
}

Add-SanityCheck -Report $report -Id 1 -Title 'Undocumented clone family' -Hits $failing -Mode strict -Severity high `
  -Remediation 'Either factor the shared code out, or - if the duplication is deliberate - add it to scripts/sanity/duplication/accepted.json with a family name and a one-line reason, and record its baseline.' `
  -Note ("Fails at Jaccard >= {0} when the largest file is >= {1} LOC; below that it is reported under check 3. {2} of {3} file(s) fingerprinted." -f (inv $FailThreshold 2), $MinLocForFail, $data.scanned, $data.filesSeen) `
  -Command 'node scripts/sanity/duplication/detect.mjs'

Add-SanityCheck -Report $report -Id 2 -Title 'Accepted family gained an undeclared member' -Hits $extended -Mode strict -Severity high `
  -Remediation 'A family is a documented set, not an inferred one. Add the new member to its accepted.json entry in the same change that creates it, then re-run with -UpdateBaseline.' `
  -Command 'node scripts/sanity/duplication/detect.mjs'

Add-SanityCheck -Report $report -Id 3 -Title 'Similar files below the fail threshold' -Hits $reported -Mode review -Severity medium `
  -Remediation 'Judgement call. Extract it, accept it in accepted.json, or leave it - but decide rather than letting it accumulate.' `
  -Command ("node scripts/sanity/duplication/detect.mjs --report-threshold {0}" -f (inv $ReportThreshold 2))

Add-SanityCheck -Report $report -Id 4 -Title 'Drift inside an accepted family' -Hits $drifted -Mode strict -Severity high `
  -Remediation 'One copy of a family moved and the others did not. Apply the same change to every sibling, or record the new baseline if the divergence is intended.' `
  -Note ("Tolerance {0}. This is the finding the allowlist exists to produce: accepted duplication is only safe while the copies stay in step." -f (inv $data.thresholds.tolerance 2)) `
  -Command 'node scripts/sanity/duplication/detect.mjs'

$watchRows = @()
foreach ($w in $data.drift) {
  foreach ($m in $w.members) {
    $watchRows += , @($w.family, $m.path, (inv $m.similarity),
      $(if ($null -eq $m.baseline) { '-' } else { inv $m.baseline }),
      $(if ($null -eq $m.delta) { '-' } else { inv $m.delta }))
  }
}
$familyCount = @($data.drift | ForEach-Object { $_.family } | Sort-Object -Unique).Count
if ($watchRows.Count) {
  Add-SanityCheck -Report $report -Id 5 -Title 'Drift watch (accepted families, inventory)' -Hits @() -Mode info -Severity info `
    -Note ("{0} accepted cluster(s) in {1} declared family/families, {2} watched member(s), ~{3} accepted redundant LOC. Similarity is measured against the family reference, which is the median member by length - the largest file is often the one vendor that is not quite the same API." -f `
      $data.totals.acceptedClusters, $familyCount, $watchRows.Count, $data.totals.redundantLoc) `
    -Table @{ Headers = @('Family', 'File', 'Similarity', 'Baseline', 'Delta'); Rows = $watchRows }
}

$gapHits = @()
foreach ($g in @($data.baselineGaps)) {
  if ($null -eq $g) { continue }
  foreach ($u in $g.unbaselined) {
    $gapHits += New-SanityHit -File $u.member -Line 0 -Text ("[{0}] no baseline for {1} -> {2}" -f $g.family, $u.reference, $u.member)
  }
}
Add-SanityCheck -Report $report -Id 6 -Title 'Accepted family with an unbaselined non-test pair' -Hits $gapHits -Mode info -Severity info `
  -Remediation 'Record the pair with -UpdateBaseline and review that only this family moved in accepted.json. Without it a one-sided change to the production files is invisible to check 4 even though their tests are watched.' `
  -Note 'Fires when a family declares more than one non-test pattern and a non-test pair has no recorded similarity. Information only.' `
  -Command 'node scripts/sanity/duplication/detect.mjs'

Add-SanityNote -Report $report -Text 'Accepted duplication is a deliberate trade, not a backlog. D-1: plugins share no library code, because a package linked into N .so files couples their release cycles and their ABI. D-2: the addon-* clusters are permanent. D-4: src/agent, src/cli and src/terraform-provider-mcharbor are separate Go modules and cannot import the host, so a shared contract exists as a copy on each side.'
Add-SanityNote -Report $report -Text 'The cost of that trade is drift, and check 4 is what makes it visible. A bug fixed in one family member exists in up to thirteen others.'
Add-SanityNote -Report $report -Text 'Do not widen an accepted.json pattern to silence check 1 or 2. Widening is how a new undocumented clone family becomes invisible.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
