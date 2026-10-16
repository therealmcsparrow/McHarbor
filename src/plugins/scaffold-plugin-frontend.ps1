# scaffold-plugin-frontend.ps1 — generates per-plugin frontend bundles
# (UMD mount entry, ConfigForm, SetupGuide, vite config, tsconfig, i18n)
# for each provider plugin that lacks one. Idempotent: skips plugins
# that already have a frontend/src/index.tsx.
#
# Run from the repo root:
#   powershell -ExecutionPolicy Bypass -File src/plugins/scaffold-plugin-frontend.ps1

$ErrorActionPreference = 'Stop'

$pluginRoot = Join-Path $PSScriptRoot 'Plugins'
if (-not (Test-Path $pluginRoot)) {
    Write-Error "Plugins root not found: $pluginRoot"
}

# Per-plugin data: kind, label, fields with validators, help, guides.
# Plain ASCII only — PowerShell parser chokes on smart quotes / arrows.
$plugins = @(
    # ============== COMMUNICATIONS (8) ==============
    @{ slug='comm-slack'; kind='channel'; ns='comm-slack'; title='Slack Notification Channel';
       fields=@(
         @{key='webhook_url';label='Slack incoming webhook URL';type='secret';required=$true;help='Create one at https://api.slack.com/messaging/webhooks. Encrypted at rest.';placeholder='https://hooks.slack.com/services/<team-id>/<bot-id>/<token>'}
         @{key='username';label='Override bot username';type='string';required=$false;help='Leave blank to use the webhook default.'}
         @{key='channel';label='Override channel';type='string';required=$false;help='Public channels: #name. Direct messages: @username.'}
       )
       guide=@(
         'Open https://api.slack.com/messaging/webhooks in your browser.'
         'Click "Create your Slack app" if you do not already have one.'
         'Enable Incoming Webhooks and click "Add New Webhook to Workspace".'
         'Pick the channel Slack should post into and authorize.'
         'Copy the webhook URL Slack shows and paste it into the field above.'
         'Click "Test" to confirm McHarbor can reach it.') }
    @{ slug='comm-discord'; kind='channel'; ns='comm-discord'; title='Discord Notification Channel';
       fields=@(
         @{key='webhook_url';label='Discord webhook URL';type='secret';required=$true;help='Server Settings > Integrations > Webhooks > New Webhook.';placeholder='https://discord.com/api/webhooks/...'}
         @{key='username';label='Override bot username';type='string';required=$false;help='Default: webhook name.'}
         @{key='avatar_url';label='Override avatar URL';type='string';required=$false;help='Public image URL (https://...). Optional.'}
       )
       guide=@(
         'In Discord, open your server settings and choose Integrations.'
         'Click "Create Webhook" and choose the target channel.'
         'Copy the webhook URL and paste it into the field above.'
         'Optionally set a username and avatar to override defaults.'
         'Click "Test" to confirm.') }
    @{ slug='comm-teams'; kind='channel'; ns='comm-teams'; title='Microsoft Teams Notification Channel';
       fields=@(
         @{key='webhook_url';label='Teams incoming webhook URL';type='secret';required=$true;help='Channel > Connectors > Incoming Webhook > Configure.';placeholder='https://*.webhook.office.com/webhookb2/...'}
       )
       guide=@(
         'Open the Teams channel where you want notifications to land.'
         'Click the more actions menu, choose Connectors, then Incoming Webhook > Configure.'
         'Name the webhook, optionally upload an icon, then Create.'
         'Copy the URL Teams shows and paste it above.'
         'Click "Test" to confirm.') }
    @{ slug='comm-gotify'; kind='channel'; ns='comm-gotify'; title='Gotify Notification Channel';
       fields=@(
         @{key='server_url';label='Gotify server URL';type='string';required=$true;help='Base URL of your self-hosted Gotify server.';placeholder='https://gotify.example.com'}
         @{key='app_token';label='Application token';type='secret';required=$true;help='Apps > Create Application. Use the token, not the client token.'}
         @{key='priority';label='Default priority';type='number';required=$false;help='0-10. Defaults to 5 if blank.'}
       )
       guide=@(
         'Open your Gotify server in a browser.'
         'Click Apps in the sidebar, then "Create Application".'
         'Name it "McHarbor" and pick a default priority.'
         'Copy the token (NOT the client token) and paste it above.'
         'Click "Test" to confirm.') }
    @{ slug='comm-ntfy'; kind='channel'; ns='comm-ntfy'; title='ntfy Notification Channel';
       fields=@(
         @{key='server_url';label='ntfy server URL';type='string';required=$true;help='Public default: https://ntfy.sh. Or your self-hosted instance.';placeholder='https://ntfy.sh'}
         @{key='topic';label='Topic';type='string';required=$true;help='The channel subscribers listen on.'}
         @{key='auth_token';label='Access token (optional)';type='secret';required=$false;help='Required only if your server enforces auth.'}
         @{key='priority';label='Default priority';type='number';required=$false;help='1 (min) to 5 (max). Defaults to 3.'}
       )
       guide=@(
         'Pick a topic name unique to McHarbor (e.g. "mcharbor-alerts-prod").'
         'For self-hosted ntfy, add the topic under Access Control if needed.'
         'Paste the server URL and topic above.'
         'If your server requires auth, paste the access token.'
         'Click "Test" - a notification should appear on subscribers.') }
    @{ slug='comm-telegram'; kind='channel'; ns='comm-telegram'; title='Telegram Notification Channel';
       fields=@(
         @{key='bot_token';label='Bot token';type='secret';required=$true;help='Talk to @BotFather, send /newbot, copy the token.';placeholder='123456789:AA...'}
         @{key='chat_id';label='Chat ID';type='string';required=$true;help='Numeric chat ID (use @userinfobot or the channel @-prefix).'}
       )
       guide=@(
         'In Telegram, message @BotFather and send /newbot.'
         'Follow the prompts; copy the HTTP API token BotFather replies with.'
         'Add the bot to your group/channel and send a test message.'
         'Get the chat_id (visit https://api.telegram.org/bot<TOKEN>/getUpdates).'
         'Paste the bot token and chat ID above. Click "Test".') }
    @{ slug='comm-signal'; kind='channel'; ns='comm-signal'; title='Signal Notification Channel';
       fields=@(
         @{key='signal_cli_url';label='signal-cli REST URL';type='string';required=$true;help='Your running signal-cli REST daemon (e.g. http://localhost:8080).'}
         @{key='sender_number';label='Sender number';type='string';required=$true;help='The E.164 number registered with signal-cli.'}
         @{key='recipient_number';label='Recipient number';type='string';required=$true;help='E.164 number or group ID to deliver to.'}
       )
       guide=@(
         'Run signal-cli-rest or bbernhard/signal-cli-rest-api in Docker.'
         'Register a number via signal-cli (or link an existing Signal device).'
         'Set SIGNAL_CLI_URL and the sender + recipient numbers above.'
         'Click "Test" - Signal should deliver a message.') }
    @{ slug='comm-whatsapp'; kind='channel'; ns='comm-whatsapp'; title='WhatsApp Notification Channel';
       fields=@(
         @{key='method';label='Connection method';type='select';required=$true;options=@('business_api','callmebot','twilio');help='business_api: Meta WhatsApp Cloud. callmebot: simple bot. twilio: Twilio.'}
         @{key='access_token';label='Access token';type='secret';required=$true;help='Provider-issued bearer token.'}
         @{key='phone_number_id';label='Phone number ID';type='string';required=$true;help='From Meta Business Manager (Cloud API) or Twilio sender.'}
         @{key='to_number';label='Recipient number';type='string';required=$true;help='E.164 format (e.g. +15551234567).'}
       )
       guide=@(
         'Pick a connection method (Business Cloud API recommended).'
         'For Cloud API: create a Meta Business app and WhatsApp product.'
         'For Twilio: buy a WhatsApp sender and copy the Account SID + auth token.'
         'For CallMeBot: message the bot on WhatsApp to register.'
         'Fill in the matching fields and click "Test".') }
    # ============== STORAGE (8) ==============
    @{ slug='storage-s3'; kind='storage_location'; ns='storage-s3'; title='AWS S3 Storage';
       fields=@(
         @{key='endpoint';label='Endpoint (blank = AWS)';type='string';required=$false;help='Leave blank for AWS. For MinIO/R2 set the custom URL.'}
         @{key='region';label='Region';type='string';required=$true;help='e.g. us-east-1.'}
         @{key='bucket';label='Bucket name';type='string';required=$true;help='S3 bucket to write into.'}
         @{key='access_key_id';label='Access key ID';type='string';required=$true;help='IAM user or role access key.'}
         @{key='secret_access_key';label='Secret access key';type='secret';required=$true;help='Encrypted at rest.'}
         @{key='use_path_style';label='Path-style addressing';type='boolean';required=$false;help='Enable for MinIO and other S3-compatible servers.'}
       )
       guide=@(
         'In AWS, create an IAM user or role with s3:PutObject, s3:GetObject, s3:ListBucket, s3:DeleteObject on the bucket.'
         'Create an access key under "Security credentials" and copy the ID + secret.'
         'In McHarbor, create a new Storage Location, type "AWS S3".'
         'Fill in region and bucket; paste the access key + secret.'
         'Click "Test connection" - McHarbor writes and deletes a probe object.') }
    @{ slug='storage-b2'; kind='storage_location'; ns='storage-b2'; title='Backblaze B2 Storage';
       fields=@(
         @{key='endpoint';label='S3-compatible endpoint';type='string';required=$true;help='e.g. https://s3.us-west-004.backblazeb2.com'}
         @{key='bucket';label='Bucket name';type='string';required=$true;help='B2 bucket to write into.'}
         @{key='key_id';label='Key ID';type='string';required=$true;help='Application key ID from B2.'}
         @{key='application_key';label='Application key';type='secret';required=$true;help='Encrypted at rest.'}
       )
       guide=@(
         'In Backblaze B2, create an Application Key with read+write to the bucket.'
         'Copy the keyID and applicationKey.'
         'In McHarbor, pick "B2" as the storage type.'
         'Fill in the endpoint matching your bucket region.'
         'Click "Test connection".') }
    @{ slug='storage-ftp'; kind='storage_location'; ns='storage-ftp'; title='FTP Storage';
       fields=@(
         @{key='host';label='Host';type='string';required=$true;help='Hostname or IP.'}
         @{key='port';label='Port';type='number';required=$true;help='Default 21.'}
         @{key='username';label='Username';type='string';required=$true}
         @{key='password';label='Password';type='secret';required=$true;help='Encrypted at rest.'}
         @{key='base_path';label='Base path';type='string';required=$false;help='Remote directory to write into (blank = home).'}
       )
       guide=@(
         'In your FTP server, create a user with write access to the destination directory.'
         'Open McHarbor then Storage then New location then FTP.'
         'Fill in host, port, username, password.'
         'Click "Test connection" - McHarbor uploads a probe file.') }
    @{ slug='storage-sftp'; kind='storage_location'; ns='storage-sftp'; title='SFTP Storage';
       fields=@(
         @{key='host';label='Host';type='string';required=$true}
         @{key='port';label='Port';type='number';required=$true;help='Default 22.'}
         @{key='username';label='Username';type='string';required=$true}
         @{key='password';label='Password';type='secret';required=$false;help='Either password or private_key is required.'}
         @{key='private_key';label='Private key (PEM)';type='secret';required=$false;help='PEM-encoded RSA/Ed25519 private key.'}
         @{key='base_path';label='Base path';type='string';required=$false;help='Remote directory.'}
       )
       guide=@(
         'In your SSH server, create a user with write access to the destination.'
         'Generate a key pair with ssh-keygen if using key auth.'
         'Append the public key to ~/.ssh/authorized_keys on the server.'
         'In McHarbor, pick "SFTP", fill in host/port/username and either password or private_key.'
         'Click "Test connection".') }
    @{ slug='storage-samba'; kind='storage_location'; ns='storage-samba'; title='Samba (SMB) Storage';
       fields=@(
         @{key='host';label='Host';type='string';required=$true}
         @{key='share';label='Share name';type='string';required=$true}
         @{key='username';label='Username';type='string';required=$false}
         @{key='password';label='Password';type='secret';required=$false;help='Encrypted at rest.'}
         @{key='base_path';label='Subdirectory';type='string';required=$false;help='Subdirectory inside the share.'}
       )
       guide=@(
         'On the Samba server, ensure the share is writable by the configured user.'
         'Open McHarbor then Storage then New then Samba.'
         'Fill in host (//host/share or just hostname), share name, and credentials.'
         'Click "Test connection".') }
    @{ slug='storage-google-drive'; kind='storage_location'; ns='storage-google-drive'; title='Google Drive Storage';
       fields=@(
         @{key='client_id';label='OAuth client ID';type='string';required=$true;help='From Google Cloud Console > APIs & Services > Credentials.'}
         @{key='client_secret';label='OAuth client secret';type='secret';required=$true;help='Encrypted at rest.'}
         @{key='refresh_token';label='Refresh token';type='secret';required=$true;help='Generated by the OAuth wizard.'}
         @{key='folder_id';label='Drive folder ID';type='string';required=$false;help='Optional: target folder. Defaults to Drive root.'}
       )
       guide=@(
         'In Google Cloud Console, create an OAuth client with the Drive API scope.'
         'Run the hosts OAuth wizard to obtain a refresh token.'
         'Paste the client ID, client secret, and refresh token above.'
         'Click "Test connection" - McHarbor uploads a probe file.') }
    @{ slug='storage-onedrive'; kind='storage_location'; ns='storage-onedrive'; title='OneDrive Storage';
       fields=@(
         @{key='tenant_id';label='Azure AD tenant ID';type='string';required=$true;help='For business. Leave blank for personal accounts.'}
         @{key='client_id';label='Application (client) ID';type='string';required=$true}
         @{key='client_secret';label='Client secret';type='secret';required=$true;help='Encrypted at rest.'}
         @{key='refresh_token';label='Refresh token';type='secret';required=$true}
         @{key='drive_id';label='Drive ID';type='string';required=$false;help='Required for SharePoint drives.'}
       )
       guide=@(
         'Register an Azure AD app with Files.ReadWrite scope.'
         'Generate a client secret and run the OAuth wizard for a refresh token.'
         'For business accounts, fill in tenant_id. For personal, leave it blank.'
         'Click "Test connection".') }
    @{ slug='storage-webdav'; kind='storage_location'; ns='storage-webdav'; title='WebDAV Storage';
       fields=@(
         @{key='endpoint';label='Endpoint URL';type='string';required=$true;help='e.g. https://dav.example.com/remote.php/webdav/'}
         @{key='username';label='Username';type='string';required=$true}
         @{key='password';label='Password';type='secret';required=$true}
         @{key='base_path';label='Base path';type='string';required=$false}
       )
       guide=@(
         'Confirm your WebDAV endpoint with curl: curl -u user:pass -X PROPFIND endpoint.'
         'Open McHarbor then Storage then New then WebDAV.'
         'Fill in endpoint, username, password.'
         'Click "Test connection".') }
    # ============== IDENTITY (3) ==============
    @{ slug='identity-oidc'; kind='identity_provider'; ns='identity-oidc'; title='OpenID Connect Provider';
       fields=@(
         @{key='provider_type';label='Provider preset';type='select';required=$true;options=@('entra_id','google','generic_oidc');help='Pick a preset for default endpoints, or generic for custom OIDC.'}
         @{key='client_id';label='Client ID';type='string';required=$true}
         @{key='client_secret';label='Client secret';type='secret';required=$true;help='Encrypted at rest.'}
         @{key='issuer_url';label='Issuer URL';type='string';required=$true;help='e.g. https://login.microsoftonline.com/<tenant>/v2.0'}
         @{key='tenant_id';label='Tenant ID (Entra)';type='string';required=$false}
         @{key='domain';label='Hosted domain (Google)';type='string';required=$false;help='Restrict to a Google Workspace domain.'}
         @{key='scopes';label='Scopes';type='string';required=$false;help='Default: openid profile email.'}
       )
       guide=@(
         'In your IdP, create an OIDC application.'
         'Set the redirect URI to https://your-mcharbor/api/auth/callback.'
         'Copy the client ID and client secret.'
         'Paste the issuer URL - McHarbor discovers endpoints from /.well-known/openid-configuration.'
         'Click "Test connection".') }
    @{ slug='identity-saml'; kind='identity_provider'; ns='identity-saml'; title='SAML 2.0 Provider';
       fields=@(
         @{key='metadata_url';label='IdP metadata URL';type='string';required=$false;help='Optional. Paste metadata XML if no URL available.'}
         @{key='metadata_xml';label='IdP metadata XML';type='string';required=$false;help='Long blob. Multiline supported.'}
         @{key='entity_id';label='Entity ID';type='string';required=$true;help='urn:example:idp after import.'}
         @{key='sso_url';label='SSO URL';type='string';required=$true}
         @{key='x509_cert';label='IdP X.509 certificate';type='secret';required=$true;help='PEM-encoded, no private key.'}
         @{key='name_id_format';label='NameID format';type='string';required=$false;help='Default: urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress'}
       )
       guide=@(
         'In your IdP admin, add a new SAML SP with ACS URL https://host/api/auth/saml/acs.'
         'Set audience/EntityID to https://host/api/auth/saml/metadata.'
         'Download the IdP metadata XML or note the SSO URL + signing certificate.'
         'Paste metadata URL/XML OR entity_id + sso_url + x509_cert.'
         'Click "Test connection".') }
    @{ slug='identity-ldap'; kind='identity_provider'; ns='identity-ldap'; title='LDAP Provider';
       fields=@(
         @{key='host';label='LDAP host';type='string';required=$true}
         @{key='port';label='Port';type='number';required=$true;help='389 plain, 636 LDAPS.'}
         @{key='use_tls';label='Use TLS (LDAPS)';type='boolean';required=$false}
         @{key='bind_dn';label='Bind DN';type='string';required=$true;help='cn=svc,dc=example,dc=com'}
         @{key='bind_password';label='Bind password';type='secret';required=$true;help='Encrypted at rest.'}
         @{key='user_base_dn';label='User base DN';type='string';required=$true}
         @{key='user_filter';label='User filter';type='string';required=$false;help='Default: (uid=(username))'}
         @{key='group_base_dn';label='Group base DN';type='string';required=$false}
       )
       guide=@(
         'Create a read-only bind account in your LDAP directory.'
         'Note the user base DN (e.g. ou=users,dc=example,dc=com).'
         'In McHarbor, pick LDAP and fill in host/port, bind credentials, and base DN.'
         'Use ldaps:// and check "Use TLS" for production.'
         'Click "Test connection" - McHarbor runs a search probe.') }
    # ============== AI (5) ==============
    @{ slug='ai-provider-anthropic'; kind='ai_provider'; ns='ai-provider-anthropic'; title='Anthropic Claude';
       fields=@(
         @{key='api_key';label='API key';type='secret';required=$true;help='From https://console.anthropic.com/settings/keys.'}
         @{key='default_model';label='Default model';type='string';required=$false;help='e.g. claude-sonnet-4-5-20250929'}
         @{key='base_url';label='Base URL (override)';type='string';required=$false;help='Default: https://api.anthropic.com'}
       )
       guide=@(
         'Create an account at https://console.anthropic.com.'
         'Go to Settings then API Keys then Create Key.'
         'Copy the key (starts with sk-ant-...) and paste above.'
         'Pick a default model (Claude Sonnet 4.5 is a safe start).'
         'Click "Test connection".') }
    @{ slug='ai-provider-openai'; kind='ai_provider'; ns='ai-provider-openai'; title='OpenAI';
       fields=@(
         @{key='api_key';label='API key';type='secret';required=$true;help='From https://platform.openai.com/api-keys.'}
         @{key='default_model';label='Default model';type='string';required=$false;help='e.g. gpt-4o'}
         @{key='base_url';label='Base URL (override)';type='string';required=$false;help='For OpenAI-compatible servers.'}
       )
       guide=@(
         'Create an account at https://platform.openai.com.'
         'Generate an API key under API Keys.'
         'Copy and paste it above.'
         'Click "Test connection".') }
    @{ slug='ai-provider-ollama'; kind='ai_provider'; ns='ai-provider-ollama'; title='Ollama (local)';
       fields=@(
         @{key='base_url';label='Ollama base URL';type='string';required=$true;help='Default: http://localhost:11434/v1';placeholder='http://localhost:11434/v1'}
         @{key='default_model';label='Default model';type='string';required=$false;help='e.g. llama3.1'}
         @{key='api_key';label='API key (optional)';type='secret';required=$false;help='Ollama usually does not require auth.'}
       )
       guide=@(
         'Install Ollama: https://ollama.com/download.'
         'Pull a model: ollama pull llama3.1.'
         'Confirm /v1/models works: curl http://localhost:11434/v1/models.'
         'In McHarbor, paste the base URL above.'
         'Click "Test connection".') }
    @{ slug='ai-provider-mistral'; kind='ai_provider'; ns='ai-provider-mistral'; title='Mistral AI';
       fields=@(
         @{key='api_key';label='API key';type='secret';required=$true;help='From https://console.mistral.ai/api-keys.'}
         @{key='default_model';label='Default model';type='string';required=$false;help='e.g. mistral-large-latest'}
         @{key='base_url';label='Base URL';type='string';required=$false}
       )
       guide=@(
         'Create an account at https://console.mistral.ai.'
         'Generate an API key.'
         'Paste the key above and pick a model.'
         'Click "Test connection".') }
    @{ slug='ai-provider-minimax'; kind='ai_provider'; ns='ai-provider-minimax'; title='MiniMax';
       fields=@(
         @{key='api_key';label='API key';type='secret';required=$true;help='From MiniMax platform.'}
         @{key='default_model';label='Default model';type='string';required=$false}
         @{key='base_url';label='Base URL';type='string';required=$false}
       )
       guide=@(
         'Sign up at the MiniMax platform and generate an API key.'
         'Paste the key and pick a default model.'
         'Click "Test connection".') }
    # ============== EMAIL (3) ==============
    @{ slug='email-smtp'; kind='email_server'; ns='email-smtp'; title='SMTP Email Server';
       fields=@(
         @{key='host';label='SMTP host';type='string';required=$true}
         @{key='port';label='Port';type='number';required=$true;help='25/465/587 typical.'}
         @{key='encryption';label='Encryption';type='select';required=$true;options=@('none','starttls','tls');help='tls = implicit (port 465). starttls = upgrade.'}
         @{key='auth_method';label='Auth method';type='select';required=$true;options=@('plain','login','none')}
         @{key='username';label='Username';type='string';required=$false}
         @{key='password';label='Password';type='secret';required=$false;help='Encrypted at rest.'}
         @{key='from_address';label='From address';type='string';required=$true;help='e.g. alerts@example.com'}
         @{key='from_name';label='From name';type='string';required=$false;help='Optional display name.'}
       )
       guide=@(
         'In your SMTP provider, create an app password or API token.'
         'In McHarbor then Email then New then SMTP, fill in host/port and credentials.'
         'Pick the matching encryption mode.'
         'Click "Test connection" - McHarbor sends a probe email.') }
    @{ slug='email-exchange'; kind='email_server'; ns='email-exchange'; title='Microsoft Exchange Online';
       fields=@(
         @{key='tenant_id';label='Azure AD tenant ID';type='string';required=$true}
         @{key='client_id';label='Application (client) ID';type='string';required=$true}
         @{key='client_secret';label='Client secret';type='secret';required=$true;help='Encrypted at rest.'}
         @{key='from_address';label='From address';type='string';required=$true}
         @{key='from_name';label='From name';type='string';required=$false}
       )
       guide=@(
         'In Entra ID, register an app with Mail.Send application permission.'
         'Grant admin consent and create a client secret.'
         'Paste tenant ID, client ID, client secret, and from address.'
         'Click "Test connection".') }
    @{ slug='email-gmail'; kind='email_server'; ns='email-gmail'; title='Gmail API';
       fields=@(
         @{key='client_id';label='OAuth client ID';type='string';required=$true}
         @{key='client_secret';label='OAuth client secret';type='secret';required=$true}
         @{key='refresh_token';label='Refresh token';type='secret';required=$true}
         @{key='from_address';label='From address';type='string';required=$true;help='The Gmail address that owns the OAuth client.'}
         @{key='from_name';label='From name';type='string';required=$false}
       )
       guide=@(
         'In Google Cloud Console, create an OAuth client with Gmail.send scope.'
         'Run the hosts OAuth wizard for a refresh token.'
         'Paste client ID, client secret, refresh token, and the from address.'
         'Click "Test connection".') }
)

# Templates (single-quoted here-strings so PowerShell treats them as literal)
$indexTpl = @'
// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

/**
 * __SLUG__ frontend UMD entry.
 *
 * Mounts the plugin config form into the host edit dialog. The host
 * loads this bundle via window.McharborPlugin___SLUGUS__.
 */

import { createRoot, type Root } from 'react-dom/client';
import { ConfigForm } from './ConfigForm';
import { SetupGuide } from './SetupGuide';

let root: Root | null = null;

export function mount(
  el: HTMLElement,
  ctx: {
    value: Record<string, string>;
    onChange: (v: Record<string, string>) => void;
    onError?: (field: string, msg: string | null) => void;
    t?: (key: string, vars?: Record<string, unknown>) => string;
  },
): () => void {
  root = createRoot(el);
  root.render(
    <ConfigForm
      value={ctx.value}
      onChange={ctx.onChange}
      onError={ctx.onError}
      t={ctx.t}
    />,
  );
  return () => {
    root?.unmount();
    root = null;
  };
}

export { ConfigForm, SetupGuide };
export default { mount };
'@

$configFormTpl = @'
// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ChangeEvent } from 'react';
import { Input } from '@resources/components/ui/Input';
import { Textarea } from '@resources/components/ui/Textarea';
import { Switch } from '@resources/components/ui/Switch';
import { Stack } from '@resources/components/ui/Layout';
import { FormField } from '@resources/components/ui/Form';

export type ConfigFieldType = 'string' | 'number' | 'boolean' | 'secret' | 'select' | 'multiline';

export type ConfigField = {
  key: string;
  label: string;
  type: ConfigFieldType;
  required: boolean;
  help?: string;
  placeholder?: string;
  options?: string[];
};

export const FIELDS: ConfigField[] = __FIELDS__;

type Props = {
  value: Record<string, string>;
  onChange: (next: Record<string, string>) => void;
  onError?: (field: string, msg: string | null) => void;
  t?: (key: string, vars?: Record<string, unknown>) => string;
};

export function ConfigForm({ value, onChange }: Props) {
  function set(key: string, v: string) {
    onChange({ ...value, [key]: v });
  }
  return (
    <Stack gap={4}>
      {FIELDS.map((f) => (
        <FieldRow
          key={f.key}
          field={f}
          value={value[f.key] ?? ''}
          onChange={(v) => set(f.key, v)}
        />
      ))}
    </Stack>
  );
}

function FieldRow({
  field,
  value,
  onChange,
}: {
  field: ConfigField;
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <FormField label={field.label} required={field.required} help={field.help} htmlFor={field.key}>
      {field.type === 'secret' ? (
        <Input
          id={field.key}
          type="password"
          value={value}
          placeholder={field.placeholder}
          onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        />
      ) : field.type === 'number' ? (
        <Input
          id={field.key}
          type="number"
          value={value}
          placeholder={field.placeholder}
          onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        />
      ) : field.type === 'boolean' ? (
        <Switch checked={value === 'true'} onCheckedChange={(c) => onChange(c ? 'true' : 'false')} />
      ) : field.type === 'select' ? (
        <select
          id={field.key}
          className="w-full rounded-md border border-border bg-card px-3 py-2 text-sm"
          value={value}
          onChange={(e) => onChange(e.target.value)}
        >
          {(field.options ?? []).map((o) => (
            <option key={o} value={o}>{o}</option>
          ))}
        </select>
      ) : field.type === 'multiline' ? (
        <Textarea
          id={field.key}
          value={value}
          onChange={(e: ChangeEvent<HTMLTextAreaElement>) => onChange(e.target.value)}
        />
      ) : (
        <Input
          id={field.key}
          type="text"
          value={value}
          placeholder={field.placeholder}
          onChange={(e: ChangeEvent<HTMLInputElement>) => onChange(e.target.value)}
        />
      )}
    </FormField>
  );
}
'@

$setupGuideTpl = @'
// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { Stack } from '@resources/components/ui/Layout';
import { Heading } from '@resources/components/ui/Typography';

export type GuideStep = string;

export const STEPS: GuideStep[] = __STEPS__;

export function SetupGuide() {
  return (
    <Stack gap={3}>
      <Heading level={2}>Setup Guide</Heading>
      <ol className="ml-5 list-decimal space-y-2 text-sm text-foreground">
        {STEPS.map((s, i) => (
          <li key={i}>{s}</li>
        ))}
      </ol>
    </Stack>
  );
}
'@

$viteTpl = @'
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { resolve } from 'node:path';

const slug = '__SLUG__';

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: resolve(__dirname, '../frontend'),
    emptyOutDir: false,
    lib: {
      entry: resolve(__dirname, 'src/index.tsx'),
      formats: ['umd'],
      name: `McharborPlugin_${slug.replace(/-/g, '_')}`,
      fileName: () => `${slug}.umd.cjs`,
    },
    rollupOptions: {
      external: ['react', 'react-dom', 'react-dom/client', 'react-i18next', 'i18next'],
      output: {
        globals: {
          react: 'React',
          'react-dom': 'ReactDOM',
          'react-dom/client': 'ReactDOMClient',
          'react-i18next': 'ReactI18next',
          i18next: 'i18next',
        },
      },
    },
  },
});
'@

$tsconfigTpl = @'
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "jsx": "react-jsx",
    "strict": true,
    "noEmit": true,
    "esModuleInterop": true,
    "allowSyntheticDefaultImports": true,
    "skipLibCheck": true,
    "isolatedModules": true,
    "resolveJsonModule": true
  },
  "include": ["src/**/*.ts", "src/**/*.tsx"]
}
'@

$i18nEnTpl = @'
{
  "menu": { "root": "__TITLE__" },
  "page": {
    "title": "__TITLE__",
    "subtitle": "__NS__ plugin for McHarbor."
  },
  "fields": __FIELDS_I18N__,
  "help": __HELP_I18N__
}
'@

function Emit-PluginFrontend($p) {
    $slug = $p.slug
    $ns = $p.ns
    $title = $p.title
    $fields = $p.fields
    $guide = $p.guide

    $pluginDir = Join-Path $pluginRoot $slug
    $frontendDir = Join-Path $pluginDir 'frontend'
    $srcDir = Join-Path $frontendDir 'src'
    $i18nEnDir = Join-Path $pluginDir 'i18n/en'

    if (-not (Test-Path $pluginDir)) {
        Write-Warning "Skipping $slug - plugin directory not found."
        return
    }

    $indexPath = Join-Path $srcDir 'index.tsx'
    if (Test-Path $indexPath) {
        Write-Host "Skipping $slug - frontend already exists."
        return
    }

    New-Item -ItemType Directory -Force -Path $srcDir | Out-Null
    New-Item -ItemType Directory -Force -Path $i18nEnDir | Out-Null

    # Serialize fields as TS array literal
    $fieldsTsParts = @()
    foreach ($f in $fields) {
        $optsPart = ''
        if ($f.options) {
            $optsArr = ($f.options | ForEach-Object { "'$_'" }) -join ', '
            $optsPart = ", options: [$optsArr]"
        }
        $phPart = ''
        if ($f.placeholder) {
            $ph = $f.placeholder -replace "'", "''"
            $phPart = ", placeholder: '$ph'"
        }
        $helpPart = ''
        if ($f.help) {
            $h = $f.help -replace "'", "''"
            $helpPart = ", help: '$h'"
        }
        $fieldsTsParts += "{ key: '$($f.key)', label: '$($f.label)', type: '$($f.type)', required: $($f.required.ToString().ToLower())$optsPart$phPart$helpPart }"
    }
    $fieldsTs = $fieldsTsParts -join ",`n    "

    $slugUnderscored = $slug -replace '-', '_'
    $indexBody = $indexTpl -replace '__SLUG__', $slug -replace '__SLUGUS__', $slugUnderscored
    $configBody = $configFormTpl -replace '__FIELDS__', $fieldsTs
    $stepsJson = ($guide | ForEach-Object { "'$_'" }) -join ', '
    $guideBody = $setupGuideTpl -replace '__STEPS__', $stepsJson
    $viteBody = $viteTpl -replace '__SLUG__', $slug

    Set-Content -Path (Join-Path $srcDir 'index.tsx') -Value $indexBody -Encoding UTF8
    Set-Content -Path (Join-Path $srcDir 'ConfigForm.tsx') -Value $configBody -Encoding UTF8
    Set-Content -Path (Join-Path $srcDir 'SetupGuide.tsx') -Value $guideBody -Encoding UTF8
    Set-Content -Path (Join-Path $frontendDir 'vite-plugin.config.ts') -Value $viteBody -Encoding UTF8
    Set-Content -Path (Join-Path $frontendDir 'tsconfig.json') -Value $tsconfigTpl -Encoding UTF8

    # i18n
    $fieldsI18nParts = @()
    $helpI18nParts = @()
    foreach ($f in $fields) {
        $lab = $f.label -replace '"', '\"'
        $fieldsI18nParts += "`"$($f.key)`": `"$lab`""
        if ($f.help) {
            $h = $f.help -replace '"', '\"'
            $helpI18nParts += "`"$($f.key)`": `"$h`""
        }
    }
    $fieldsI18n = $fieldsI18nParts -join ', '
    $helpI18n = $helpI18nParts -join ', '
    $titleEsc = $title -replace '"', '\"'
    $nsEsc = $ns -replace '"', '\"'
    $i18nBody = $i18nEnTpl -replace '__TITLE__', $titleEsc -replace '__NS__', $nsEsc -replace '__FIELDS_I18N__', "{ $fieldsI18n }" -replace '__HELP_I18N__', "{ $helpI18n }"
    Set-Content -Path (Join-Path $i18nEnDir "$ns.json") -Value $i18nBody -Encoding UTF8

    Write-Host "Scaffolded frontend for $slug"
}

$scaffolded = 0
$skipped = 0
foreach ($p in $plugins) {
    $pluginDir = Join-Path $pluginRoot $p.slug
    $srcDir = Join-Path $pluginDir 'frontend/src'
    $wasPresent = Test-Path (Join-Path $srcDir 'index.tsx')
    Emit-PluginFrontend $p
    if ($wasPresent) { $skipped++ } else { $scaffolded++ }
}

Write-Host "Done. scaffolded=$scaffolded skipped=$skipped of $($plugins.Count) plugins."