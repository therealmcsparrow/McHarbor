# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
<#
.SYNOPSIS
  Plugin host coupling sanity check. Mirrors .instructions/sanity/plugin-host-coupling.md.
.DESCRIPTION
  Every hit is HIGH: the host has hardcoded knowledge of a plugin slug and the
  plugin is no longer self-contained. Fix by moving the knowledge into the
  plugin manifest or Register* hook, never by adding an exemption here.
.EXAMPLE
  .\scripts\sanity\plugin-host-coupling\check.ps1
#>
[CmdletBinding()]
param(
  [string]$Root,
  [string]$OutDir = '.results/sanity',
  [string]$Stamp,
  [int]$MaxHits = 150,
  [string[]]$Paths,
  [string]$AllowList
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '..\_lib\common.ps1')
$Root = Get-SanityRoot -Root $Root -ScriptDir $PSScriptRoot
if (-not $Stamp) { $Stamp = Get-SanityStamp }

$report = New-SanityReport -Name 'plugin-host-coupling' -Title 'Plugin Host Coupling Sanity' -Scope 'src/backend/modules, src/frontend/modules, src/frontend/core, src/frontend/resources, src/backend/core/security, src/frontend/core/auth, locales' -Stamp $Stamp -Root $Root
Write-SanityHeader $report

$hostScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules', 'src/frontend/modules', 'src/frontend/core', 'src/frontend/resources'))
$permScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/core/security', 'src/backend/core/rbac', 'src/backend/modules/auth', 'src/frontend/core/auth'))
$menuScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend/modules/settings', 'src/frontend/modules/admin'))
$routeScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules'))
$localeScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/frontend/core/i18n/locales'))

# Slug list: the fixed list from the instruction file plus every plugin folder that exists on disk.
$slugs = @('comm-telegram', 'comm-signal', 'comm-whatsapp', 'email-smtp', 'email-sendgrid', 'email-mailgun', 'email-postmark', 'email-aws-ses', 'identity-okta', 'identity-azure-ad', 'identity-google', 'identity-github', 'identity-gitlab', 'identity-keycloak', 'identity-authentik', 'identity-jumpcloud', 'identity-onelogin', 'identity-ping', 'identity-duo', 'identity-centrify', 'ai-provider-openai', 'ai-provider-anthropic', 'ai-provider-ollama', 'ai-provider-google', 'ai-provider-mistral', 'ai-provider-cohere', 'ai-provider-perplexity', 'storage-onedrive-personal', 'storage-onedrive-business', 'storage-sharepoint-online', 'storage-sftp', 'storage-samba', 'storage-ftp', 'storage-nfs', 'storage-box', 'storage-dropbox', 'storage-google-drive', 'storage-backblaze', 'storage-wasabi', 'storage-aws-s3', 'storage-minio', 'storage-azure-blob', 'storage-pcloud')
$pluginDir = Join-Path $Root 'src\plugins\Plugins'
if (Test-Path -LiteralPath $pluginDir) {
  foreach ($d in (Get-ChildItem -LiteralPath $pluginDir -Directory)) { if ($d.Name -notmatch '^_' -and $slugs -notcontains $d.Name) { $slugs += $d.Name } }
}
$slugRx = '\b(' + (($slugs | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')\b'
$hostExempt = @('src/backend/modules/plugin_runtime/', 'src/backend/modules/plugin_store/', 'src/backend/modules/plugin_sources/', 'src/backend/modules/appstore/plugins/')

# 1. Hardcoded plugin slugs in host modules
$slugHits = @(Find-Pattern -Root $Root -Paths $hostScope -Include @('*.go', '*.ts', '*.tsx', '*.json') -ExcludeMatch @('_test\.go$', '\.test\.tsx?$') -Pattern $slugRx -NotMatch '^\s*(//|/\*|\*|\{/\*)')
$slugHits = @($slugHits | Where-Object { $h = $_; -not ($hostExempt | Where-Object { $h.File.StartsWith($_) }) })
Add-SanityCheck -Report $report -Id 1 -Title 'Hardcoded plugin slugs in host modules' -Hits $slugHits -Mode strict -Severity high `
  -Remediation 'Register through the Plugin interface / manifest; the host must not know slugs.' -Note ("Exempt plugin host modules: {0}. {1} slugs checked (instruction list + src/plugins/Plugins folders)." -f ($hostExempt -join ', '), $slugs.Count) `
  -Command 'rg -n -e "(comm-telegram|...|storage-pcloud)" src/backend/modules src/frontend/modules'

# 2. Direct imports of plugin backends
$directImports = @(Find-Pattern -Root $Root -Paths $hostScope -Include @('*.go', '*.ts', '*.tsx') -Pattern 'Plugins/(comm-|email-|identity-|ai-provider-|storage-|addon-|logging-|scanner-|snapshot-)')
Add-SanityCheck -Report $report -Id 2 -Title 'Direct imports of plugin backend/frontend code' -Hits $directImports -Mode strict -Severity high -Remediation 'Plugin backends load via plugin.Open; never link them into the host binary or bundle.' -Command 'rg -n -e "Plugins/(comm-|email-|identity-|ai-provider-|storage-)" src/backend/modules src/frontend/modules'

# 3. Hardcoded plugin keys in permissions
$permKeys = @(Find-Pattern -Root $Root -Paths $permScope -Include @('*.go', '*.ts', '*.tsx') -Pattern '["''](comm-|email-|identity-|ai-provider-|storage-|addon-|logging-|scanner-|snapshot-)[a-z0-9-]+\.[a-z0-9-]+["'']')
Add-SanityCheck -Report $report -Id 3 -Title 'Hardcoded plugin permission keys in host RBAC' -Hits $permKeys -Mode strict -Severity high -Remediation 'Plugin keys come from manifest adds_permissions or AccessRegistry.AddPermission.' -Command 'rg -n -e "[\x27\"](comm-|email-|identity-|ai-provider-|storage-)[a-z0-9-]+\.[a-z0-9-]+[\x27\"]" src/backend/core/security src/backend/modules/auth src/frontend/core/auth'

# 4. Hardcoded plugin menu entries
$menuA = @(Find-Pattern -Root $Root -Paths $menuScope -Include @('*.ts', '*.tsx') -ExcludeMatch @('\.test\.tsx?$') -IgnoreCase -Pattern '\b(telegram|whatsapp|sendgrid|mailgun|postmark|okta|keycloak|onedrive|sharepoint|sftp|samba|dropbox|box\.com)\b')
$menuB = @(Find-Pattern -Root $Root -Paths $menuScope -Include @('*.ts', '*.tsx') -ExcludeMatch @('\.test\.tsx?$') -Pattern '\bSignal\b')
Add-SanityCheck -Report $report -Id 4 -Title 'Hardcoded plugin menu labels in settings/admin' -Hits ($menuA + $menuB) -Mode strict -Severity high -Remediation 'Plugin menu entries are added via the runtime MenuRegistry.' -Note '"Signal" is matched case-sensitively (the messenger brand) to avoid AbortSignal false positives.' -Command 'rg -n -e "(telegram|whatsapp|signal|sendgrid|mailgun|postmark|okta|keycloak|onedrive|sharepoint|sftp|samba|dropbox|box\.com)" src/frontend/modules/settings src/frontend/modules/admin'

# 5. Hardcoded plugin routes
# identity-providers and storage-locations are host modules, not plugin families.
$routes = @(Find-Pattern -Root $Root -Paths $routeScope -Include @('*.go') -ExcludeMatch @('_test\.go$') -Pattern '/api/(comm-|email-|identity-(?!providers\b)|ai-provider-|storage-(?!locations\b)|addon-|logging-|scanner-|snapshot-)')

# The second half of check 5: the slug-shaped pattern above cannot see /api/vms, because a
# route does not carry its plugin's name. So read every path any plugin manifest declares
# under contributes.routes and flag its static prefix (up to the first placeholder) as a
# literal anywhere in host code, Go or TypeScript, whichever way the placeholders are spelled.
$routePrefixes = New-Object 'System.Collections.Generic.HashSet[string]'
if (Test-Path -LiteralPath $pluginDir) {
  $declRx = New-Object System.Text.RegularExpressions.Regex('(?m)^\s*-\s*method:\s*\w+\s*\r?\n\s*path:\s*"?(/api/[^"\s]+)"?')
  foreach ($manifest in (Get-ChildItem -LiteralPath $pluginDir -Directory | Where-Object { $_.Name -notmatch '^_' } | ForEach-Object { Join-Path $_.FullName 'plugin.yaml' } | Where-Object { Test-Path -LiteralPath $_ })) {
    foreach ($m in $declRx.Matches([System.IO.File]::ReadAllText($manifest))) {
      $declared = $m.Groups[1].Value
      $brace = $declared.IndexOf('{')
      if ($brace -ge 0) { $declared = $declared.Substring(0, $brace) }
      [void]$routePrefixes.Add($declared)
    }
  }
}
$declaredHits = @()
if ($routePrefixes.Count -gt 0) {
  $prefixRx = '(' + ((@($routePrefixes) | Sort-Object { $_.Length } -Descending | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')(?![A-Za-z0-9_-])'
  $declaredHits = @(Find-Pattern -Root $Root -Paths $hostScope -Include @('*.go', '*.ts', '*.tsx') -ExcludeMatch @('_test\.go$', '\.test\.tsx?$') -Pattern $prefixRx -NotMatch '^\s*(//|/\*|\*|\{/\*)')
  $declaredHits = @($declaredHits | Where-Object { $h = $_; -not ($hostExempt | Where-Object { $h.File.StartsWith($_) }) })
  # The api client prepends /api itself, so frontend code names a plugin route without it
  # ('/reporting/reports'). Only a quoted literal that starts with a declared prefix counts,
  # and only in TypeScript, so a host route that merely shares a word with a plugin is left alone.
  $clientPrefixes = @($routePrefixes | ForEach-Object { $_.Substring(4) } | Where-Object { $_.Length -gt 1 } | Sort-Object { $_.Length } -Descending | ForEach-Object { [regex]::Escape($_) })
  if ($clientPrefixes.Count -gt 0) {
    $clientRx = '(?<=[' + [char]39 + [char]34 + [char]96 + '])(' + ($clientPrefixes -join '|') + ')(?![A-Za-z0-9_-])'
    $clientHits = @(Find-Pattern -Root $Root -Paths $hostScope -Include @('*.ts', '*.tsx') -ExcludeMatch @('\.test\.tsx?$') -Pattern $clientRx -NotMatch '^\s*(//|/\*|\*|\{/\*)')
    $declaredHits = @($declaredHits + $clientHits)
  }
}
Add-SanityCheck -Report $report -Id 5 -Title 'Hardcoded plugin routes in host modules' -Hits ($routes + $declaredHits) -Mode strict -Severity high -Remediation 'Routes are registered through Registry.Protected / Registry.Auth / Registry.Public; a route a plugin serves reaches the host through a contributes.capabilities descriptor, never as a literal.' -Note ("{0} route prefixes read from src/plugins/Plugins/*/plugin.yaml contributes.routes." -f $routePrefixes.Count) -Command 'rg -nP -e "/api/(comm-|email-|identity-(?!providers\b)|ai-provider-|storage-(?!locations\b))" src/backend/modules'

# 6. Plugin-coupled translation files
$locA = @(Find-Pattern -Root $Root -Paths $localeScope -Include @('*.json') -IgnoreCase -Pattern '\b(telegram|whatsapp|sendgrid|mailgun|postmark|onedrive|sharepoint|dropbox|box\.com)\b')
# "Signal" is matched case-sensitively (the messenger brand) to avoid
# AbortSignal false positives. The Docker *stop signal* label is a host
# concept whose French translation is legitimately "Signal d'arrêt", so the
# stopSignal key is excluded by name rather than by wording it worse. The
# incidents bundle keys its own "signal": "Signal" (an incident signal, not
# the messenger), so a key literally named signal is excluded the same way.
$locB = @(Find-Pattern -Root $Root -Paths $localeScope -Include @('*.json') -Pattern '\bSignal\b' -NotMatch '"(stopSignal|signal)"\s*:' | Where-Object { $_.File -match '(^|/)en/' })
Add-SanityCheck -Report $report -Id 6 -Title 'Plugin-specific strings in the host locale set' -Hits ($locA + $locB) -Mode strict -Severity high -Remediation 'Plugin strings live in the plugin''s own i18n/<lang>/<ns>.json served via /api/plugins/loader/i18n.' -Command 'rg -n -e "(telegram|whatsapp|signal|sendgrid|mailgun|postmark|onedrive|sharepoint|dropbox|box\.com)" src/frontend/core/i18n/locales'

# 7. Plugin channel types and scanner engines named in host workflow code
$workflowScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules/workflows', 'src/frontend/modules/workflows', 'src/frontend/resources/nodes', 'nodes'))
$providerKeys = New-Object 'System.Collections.Generic.HashSet[string]'
if (Test-Path -LiteralPath $pluginDir) {
  $formRx = New-Object System.Text.RegularExpressions.Regex('(?m)resource:\s*"?channel"?\s*\r?\n\s*kind:\s*"?[\w-]+"?\s*\r?\n\s*key:\s*"?([a-z0-9_]+)"?')
  $engineRx = New-Object System.Text.RegularExpressions.Regex('ScannerType\s*=\s*"([a-z0-9_-]+)"')
  foreach ($plugin in (Get-ChildItem -LiteralPath $pluginDir -Directory | Where-Object { $_.Name -notmatch '^_' })) {
    $manifest = Join-Path $plugin.FullName 'plugin.yaml'
    if (Test-Path -LiteralPath $manifest) {
      foreach ($m in $formRx.Matches([System.IO.File]::ReadAllText($manifest))) { [void]$providerKeys.Add($m.Groups[1].Value) }
    }
    $backend = Join-Path $plugin.FullName 'backend'
    if (Test-Path -LiteralPath $backend) {
      foreach ($go in (Get-ChildItem -LiteralPath $backend -Filter '*.go' -File | Where-Object { $_.Name -notmatch '_test\.go$' })) {
        foreach ($m in $engineRx.Matches([System.IO.File]::ReadAllText($go.FullName))) { [void]$providerKeys.Add($m.Groups[1].Value) }
      }
    }
  }
}
$providerHits = @()
if ($providerKeys.Count -gt 0) {
  $q = '[' + [char]39 + [char]34 + [char]96 + ']'
  $providerRx = $q + '(' + ((@($providerKeys) | Sort-Object { $_.Length } -Descending | ForEach-Object { [regex]::Escape($_) }) -join '|') + ')' + $q
  $providerHits = @(Find-Pattern -Root $Root -Paths $workflowScope -Include @('*.go', '*.ts', '*.tsx') -ExcludeMatch @('_test\.go$', '\.test\.tsx?$') -Pattern $providerRx -NotMatch '^\s*(//|/\*|\*|\{/\*)')
}
Add-SanityCheck -Report $report -Id 7 -Title 'Plugin channel types and scanner engines in host workflow code' -Hits $providerHits -Mode strict -Severity high -Remediation 'A workflow node lists channels through an email-channel / workflow-node-alias capability and scanners through /scans/scanners; it never names a provider the plugin registers.' -Note ("{0} provider keys read from plugin.yaml channel forms and scanner-plugin ScannerType constants." -f $providerKeys.Count) -Command 'rg -n -e "[\x27\"](smtp|slack|trivy|...)[\x27\"]" src/backend/modules/workflows src/frontend/modules/workflows src/frontend/resources/nodes nodes'

# 8. The retired compute bridge
$bridgeScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend', 'src/plugins/Plugins', 'src/cli', 'src/agent', 'src/contracts', 'nodes', 'widgets'))
$bridgeHits = @(Find-Pattern -Root $Root -Paths $bridgeScope -Include @('*.go', '*.yaml', '*.yml') -Pattern '(core|specs)/compute(?![A-Za-z0-9_])')
Add-SanityCheck -Report $report -Id 8 -Title 'References to the retired compute bridge' -Hits $bridgeHits -Mode strict -Severity high -Remediation 'core/compute and core/plugin/specs/compute are gone. A plugin reaches the agent through its own agent-rpc declaration and host.AgentRPC; the ceiling of what it may send lives in core/agent/pluginroutes.go.' -Note 'Go and YAML sources under src/ and the node and widget catalogs; comments count, because a pointer to a deleted package sends the next reader nowhere.' -Command 'rg -nP -e "(core|specs)/compute(?![A-Za-z0-9_])" -g "*.go" -g "*.yaml" src nodes widgets'

# 9. Plugin guest and target kinds in host code
# A guest kind (vm, ct, lxc, kvm) or a runbook target kind (k8s_*, swarm_*) belongs to the plugin
# that declares it. The excluded core packages are the agent protocol, the demo fixtures, the
# migrations and the plugin contract, which carry these words as wire vocabulary, not as knowledge.
$kindScope = (Get-SanityScope -Root $Root -Paths $Paths -Default @('src/backend/modules', 'src/backend/core', 'src/frontend/core', 'src/frontend/modules', 'src/frontend/resources'))
$kindExcluded = @('^src/backend/core/(agent|agentrpc|demo|db/migrations|plugin/specs)/', '/i18n/locales/', '_test\.go$', '\.test\.tsx?$', '\.stories\.tsx?$')
if (-not $AllowList) { $AllowList = Join-Path $PSScriptRoot 'allow-kinds.json' }
$kindAllowed = @{}
if (Test-Path -LiteralPath $AllowList) {
  foreach ($entry in (Get-Content -LiteralPath $AllowList -Raw -Encoding UTF8 | ConvertFrom-Json).exemptions) {
    if (-not $entry.reason) { throw "allow-kinds: $($entry.file) / $($entry.literal) has no reason" }
    $kindAllowed[($entry.file + '|' + $entry.literal)] = $true
  }
}
$kindRx = '([' + [char]39 + [char]34 + '])(vm|ct|lxc|kvm|k8s_[a-z0-9_]+|swarm_[a-z0-9_]+)\1'
$kindHits = @(Find-Pattern -Root $Root -Paths $kindScope -Include @('*.go', '*.ts', '*.tsx') -ExcludeMatch $kindExcluded -Pattern $kindRx -NotMatch '^\s*(//|/\*|\*|\{/\*)')
$kindHits = @($kindHits | Where-Object {
    $h = $_
    $literals = @([regex]::Matches($h.Text, $kindRx) | ForEach-Object { $_.Groups[2].Value })
    @($literals | Where-Object { -not $kindAllowed.ContainsKey($h.File + '|' + $_) }).Count -gt 0
  })
Add-SanityCheck -Report $report -Id 9 -Title 'Plugin guest/target kinds in host code' -Hits $kindHits -Mode strict -Severity high -Remediation 'Read the kind from what the plugin declares (console-subject, backup-row-actions, runbook-provider targets) through core/plugincaps or usePluginCapabilities; a host-owned word that merely shares the spelling goes in allow-kinds.json with its file and reason.' -Note ("{0} named exemptions read from {1}. Tests, stories, locale bundles, comment lines and core/agent, core/agentrpc, core/demo, core/db/migrations, core/plugin/specs are skipped." -f $kindAllowed.Count, (Split-Path -Leaf $AllowList)) -Command 'rg -n -e "([\x27\"])(vm|ct|lxc|kvm|k8s_[a-z0-9_]+|swarm_[a-z0-9_]+)[\x27\"]" src/backend/modules src/backend/core src/frontend/core src/frontend/modules src/frontend/resources'

Add-SanityNote -Report $report -Text 'Zero hits everywhere means: OK, no hardcoded plugin coupling in the host.'

$summary = Complete-SanityCheck -Report $report -OutDir $OutDir -MaxHits $MaxHits
$summary
if ($summary.Status -eq 'fail') { exit 1 } else { exit 0 }
