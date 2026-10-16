// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { AgentDockerImage, AgentDefaultTransferListen } from '../constants';
import type { InstallTokenResponse } from '../hooks/useEnvironmentActions';

type AgentInstallInput = {
  token: string;
  serverUrl: string;
  installScript?: InstallTokenResponse | null;
  caFingerprint?: string;
  insecureTls: boolean;
};

export function buildAgentInstallCommands({
  token,
  serverUrl,
  installScript,
  caFingerprint,
  insecureTls,
}: AgentInstallInput) {
  const agentImage = AgentDockerImage;
  const availablePin = installScript?.caFingerprint || caFingerprint || '';
  const pin = insecureTls ? availablePin : '';
  const insecureFlag = !insecureTls || pin ? 'false' : 'true';
  const pinEnvLine = pin
    ? `  -e MCHARBOR_PIN=${pin} \\
`
    : '';
  const pinComposeLine = pin
    ? `      MCHARBOR_PIN: ${pin}
`
    : '';
  const effectiveServerUrl = installScript?.serverUrl || serverUrl;
  // `forceTls` from the install-token response is the canonical source; fall
  // back to window.location.protocol on fresh mounts where the install-token
  // call has not returned yet.
  const isHttps = installScript?.forceTls ?? (typeof window !== 'undefined' && window.location.protocol === 'https:');

  // Both transfer variables are omitted unless the server has a concrete value
  // to offer. The agent defaults its listener to 0.0.0.0:8788, derives its own
  // advertise addresses from its interfaces, and McHarbor adds the address it
  // observes the agent connect from. Emitting an "agent-host-or-ip" placeholder
  // would hand the operator a command that only works once they edit it.
  const reportedTransferListen = installScript?.transferListen?.trim() || '';
  const transferListen = reportedTransferListen === AgentDefaultTransferListen ? '' : reportedTransferListen;
  const transferAdvertiseUrl = installScript?.transferAdvertiseUrl?.trim() || '';
  const transferComposeLines = [
    transferListen ? '      MCHARBOR_TRANSFER_LISTEN: ' + transferListen : null,
    transferAdvertiseUrl ? '      MCHARBOR_TRANSFER_ADVERTISE_URL: ' + transferAdvertiseUrl : null,
  ]
    .filter((line): line is string => line !== null)
    .map((line) => line + '\n')
    .join('');

  const transferEnvLines = (prefix: string) =>
    [
      transferListen ? prefix + 'MCHARBOR_TRANSFER_LISTEN=' + transferListen : null,
      transferAdvertiseUrl ? prefix + 'MCHARBOR_TRANSFER_ADVERTISE_URL=' + transferAdvertiseUrl : null,
    ]
      .filter((line): line is string => line !== null)
      .map((line) => line + ' \\\n')
      .join('');

  // docker socket: install instruction, not server config. The agent has to
  // mount the host's socket to manage it, so the path belongs in the command
  // the operator copies. Nothing here is read back from an API response.
  const dockerCmd = `IMG=$(curl -fsSL${insecureTls ? ' -k' : ''} -H "Authorization: Bearer ${token}" "${effectiveServerUrl}/api/agent/image/linux/$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')" | docker load | sed -n 's/^Loaded image: //p' | tail -1)
docker tag "$IMG" ${agentImage}
docker rm -f mcharbor-agent 2>/dev/null || true
docker run -d \\
  --name mcharbor-agent \\
  --restart unless-stopped \\
  -p 8788:8788 \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  -e MCHARBOR_URL=${effectiveServerUrl} \\
  -e MCHARBOR_AGENT_TOKEN=${token} \\
  -e MCHARBOR_FORCE_TLS=${isHttps ? '1' : '0'} \\
  -e MCHARBOR_INSECURE=${insecureFlag} \\
${pinEnvLine}  -e DOCKER_HOST=unix:///var/run/docker.sock \\
  -e LOG_LEVEL=info \\
${transferEnvLines('  -e ')}  ${agentImage}`;

  const installUrl = (installMethod: 'docker' | 'binary') =>
    installScript
      ? `curl${insecureTls ? ' -k' : ''} -sSL '${effectiveServerUrl}/api/agent/install/${installScript.token}?method=${installMethod}${insecureTls ? '&insecure=1' : ''}' | bash`
      : '';

  const composeFile = `services:
  mcharbor-agent:
    image: ${agentImage}
    container_name: mcharbor-agent
    restart: unless-stopped
    ports:
      - "8788:8788"
    environment:
      MCHARBOR_URL: ${effectiveServerUrl}
      MCHARBOR_AGENT_TOKEN: ${token}
      MCHARBOR_FORCE_TLS: "${isHttps ? '1' : '0'}"
      MCHARBOR_INSECURE: "${insecureFlag}"
${pinComposeLine}      DOCKER_HOST: unix:///var/run/docker.sock
      LOG_LEVEL: info
      MCHARBOR_DATA_DIR: /var/lib/mcharbor-agent
${transferComposeLines}    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - mcharbor-agent-data:/var/lib/mcharbor-agent

volumes:
  mcharbor-agent-data:`;

  return {
    composeFile,
    dockerCmd,
    installUrl,
    effectiveServerUrl,
    isHttps,
    transferListen,
    transferAdvertiseUrl,
    pin,
  };
}
