# wire-plugin-forms.ps1 — adds contributes.forms to each provider
# plugin's manifest so the plugin UMD bundle mounts in the host's
# edit dialog. Idempotent: skips manifests that already declare forms.
#
# Run:
#   powershell -ExecutionPolicy Bypass -File src/plugins/wire-plugin-forms.ps1

$ErrorActionPreference = 'Stop'

$pluginRoot = Join-Path $PSScriptRoot 'Plugins'

# Plugin slug -> resource kind mapping. Matches the host edit-dialog
# PluginFormOutlet `resource` prop. Adding a new provider plugin = add
# one entry here.
$kinds = @{
    'comm-slack' = 'channel'
    'comm-discord' = 'channel'
    'comm-teams' = 'channel'
    'comm-gotify' = 'channel'
    'comm-ntfy' = 'channel'
    'comm-telegram' = 'channel'
    'comm-signal' = 'channel'
    'comm-whatsapp' = 'channel'
    'storage-s3' = 'storage_location'
    'storage-b2' = 'storage_location'
    'storage-ftp' = 'storage_location'
    'storage-sftp' = 'storage_location'
    'storage-samba' = 'storage_location'
    'storage-google-drive' = 'storage_location'
    'storage-onedrive' = 'storage_location'
    'storage-webdav' = 'storage_location'
    'identity-oidc' = 'identity_provider'
    'identity-saml' = 'identity_provider'
    'identity-ldap' = 'identity_provider'
    'ai-provider-anthropic' = 'ai_provider'
    'ai-provider-openai' = 'ai_provider'
    'ai-provider-ollama' = 'ai_provider'
    'ai-provider-mistral' = 'ai_provider'
    'ai-provider-minimax' = 'ai_provider'
    'email-smtp' = 'email_server'
    'email-exchange' = 'email_server'
    'email-gmail' = 'email_server'
}

$updated = 0
$skipped = 0

foreach ($slug in $kinds.Keys) {
    $manifestPath = Join-Path $pluginRoot "$slug/plugin.yaml"
    if (-not (Test-Path $manifestPath)) {
        Write-Warning "Skipping $slug - plugin.yaml not found."
        continue
    }

    $text = Get-Content -Raw -Path $manifestPath

    # Skip if forms: already declared
    if ($text -match '^\s*forms:\s*$' -or $text -match '^\s*forms:\s*\[.*\]') {
        Write-Host "Skipping $slug - forms already declared."
        $skipped++
        continue
    }

    $kind = $kinds[$slug]
    # Build the forms block. Two entries per plugin:
    #   config_form  - the inputs the host renders for this provider type
    #   setup_guide  - the step-by-step instructions in a side panel
    $formsBlock = @"

  # Form extensions. The host edit dialog looks up a plugin for each
  # (resource, kind) slot and mounts the plugin's UMD bundle into the
  # dialog. This plugin owns the config_form + setup_guide for its
  # resource type; remove an entry to fall back to host-rendered forms.
  forms:
    - resource: "$kind"
      kind:     "config_form"
    - resource: "$kind"
      kind:     "setup_guide"
"@

    # Insert immediately before `signature: ""` (the canonical trailer
    # every manifest carries). Fall back to end-of-file if absent.
    if ($text -match '(?m)^\s*signature:\s*""\s*$') {
        $newText = [regex]::Replace($text, '(?m)^(\s*signature:\s*""\s*)$', ($formsBlock + "`n$1"), 1)
    } else {
        $newText = $text + $formsBlock
    }

    # Backup before write
    $backupDir = Join-Path $PSScriptRoot '../.references/backup/phase9-plugin-form-manifests'
    New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
    $backupPath = Join-Path $backupDir "$slug-plugin.yaml"
    Copy-Item -Path $manifestPath -Destination $backupPath -Force

    Set-Content -Path $manifestPath -Value $newText -Encoding UTF8
    Write-Host "Added forms to $slug ($kind)"
    $updated++
}

Write-Host "Done. updated=$updated skipped=$skipped of $($kinds.Count) plugins."