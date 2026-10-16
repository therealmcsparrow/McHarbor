// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { AgentCopyBlock } from './AgentCopyBlock';

type AgentBinaryInstallProps = {
  serverUrl: string;
  token: string;
  isHttps: boolean;
  insecureTls: boolean;
  transferListen: string;
  transferAdvertiseUrl: string;
  pin?: string;
  copiedKey: string | null;
  onCopy: (value: string, key: string, label: string) => void;
};

export function AgentBinaryInstall({
  serverUrl,
  token,
  isHttps,
  insecureTls,
  transferListen,
  transferAdvertiseUrl,
  pin = '',
  copiedKey,
  onCopy,
}: AgentBinaryInstallProps) {
  const { t } = useTranslation('environments');
  const insecureFlag = !insecureTls || pin ? 'false' : 'true';
  const curlInsecure = insecureTls ? ' -k' : '';
  const pinEnvLine = pin
    ? `MCHARBOR_PIN=${pin} \\
`
    : '';
  const pinUnitLine = pin
    ? `Environment=MCHARBOR_PIN=${pin}
`
    : '';

  const downloadCmd = `OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in x86_64) ARCH=amd64 ;; aarch64) ARCH=arm64 ;; armv7l) ARCH=arm ;; esac
curl${curlInsecure} -fsSL -o /usr/local/bin/mcharbor-agent \
  ${serverUrl}/api/agent/binary/$OS/$ARCH
chmod +x /usr/local/bin/mcharbor-agent`;

  const transferEnvLines = [
    transferListen ? 'MCHARBOR_TRANSFER_LISTEN=' + transferListen : null,
    transferAdvertiseUrl ? 'MCHARBOR_TRANSFER_ADVERTISE_URL=' + transferAdvertiseUrl : null,
  ]
    .filter((line): line is string => line !== null)
    .map((line) => line + ' \\\n')
    .join('');

  const transferUnitLines = [
    transferListen ? 'Environment=MCHARBOR_TRANSFER_LISTEN=' + transferListen : null,
    transferAdvertiseUrl ? 'Environment=MCHARBOR_TRANSFER_ADVERTISE_URL=' + transferAdvertiseUrl : null,
  ]
    .filter((line): line is string => line !== null)
    .map((line) => line + '\n')
    .join('');

  // docker socket: install command the operator runs on the agent host; the path is the agent's own default, never server config.
  const foregroundCmd = `MCHARBOR_URL=${serverUrl} \
MCHARBOR_AGENT_TOKEN=${token} \
MCHARBOR_FORCE_TLS=${isHttps ? '1' : '0'} \
MCHARBOR_INSECURE=${insecureFlag} \
${pinEnvLine}DOCKER_HOST=unix:///var/run/docker.sock \
LOG_LEVEL=info \
${transferEnvLines}mcharbor-agent`;

  const serviceCmd = `cat > /etc/systemd/system/mcharbor-agent.service <<'UNIT'
[Unit]
Description=McHarbor Remote Agent
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/mcharbor-agent
Environment=MCHARBOR_URL=${serverUrl}
Environment=MCHARBOR_AGENT_TOKEN=${token}
Environment=MCHARBOR_FORCE_TLS=${isHttps ? '1' : '0'}
Environment=MCHARBOR_INSECURE=${insecureFlag}
${pinUnitLine}Environment=DOCKER_HOST=unix:///var/run/docker.sock
Environment=LOG_LEVEL=info
Environment=MCHARBOR_COMPOSE_DIR=/var/lib/mcharbor-agent/compose
Environment=MCHARBOR_DATA_DIR=/var/lib/mcharbor-agent
${transferUnitLines}Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT
chmod 600 /etc/systemd/system/mcharbor-agent.service
mkdir -p /var/lib/mcharbor-agent/compose
systemctl daemon-reload
systemctl enable --now mcharbor-agent
journalctl -u mcharbor-agent -f`;

  return (
    <div className="space-y-3">
      <p className="text-xs text-muted-foreground">{t('agentToken.binaryIntro')}</p>

      <AgentCopyBlock
        label={t('agentToken.binaryDownload')}
        value={downloadCmd}
        copyKey="binaryDownload"
        copiedKey={copiedKey}
        ariaLabel={t('agentToken.copyBinaryDownload')}
        onCopy={onCopy}
        note={t('agentToken.binaryDownloadNote')}
      />

      <AgentCopyBlock
        label={t('agentToken.runAsBinary')}
        value={foregroundCmd}
        copyKey="binary"
        copiedKey={copiedKey}
        ariaLabel={t('agentToken.copyBinaryCommand')}
        onCopy={onCopy}
        note={t('agentToken.binaryForegroundNote')}
      />

      <AgentCopyBlock
        label={t('agentToken.runAsService')}
        value={serviceCmd}
        copyKey="binaryService"
        copiedKey={copiedKey}
        ariaLabel={t('agentToken.copyServiceCommand')}
        onCopy={onCopy}
        note={t('agentToken.serviceNote')}
      />

      <p className="text-xs text-muted-foreground">{t('agentToken.serviceOtherPlatforms')}</p>
    </div>
  );
}
