// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import type { InstallTokenResponse } from '../hooks/useEnvironmentActions';
import { AgentDockerImage } from '../constants';
import { AgentCopyBlock } from './AgentCopyBlock';
import { AgentTokenValue } from './AgentTokenValue';

export { AgentDockerImage } from '../constants';

export async function copyAgentText(text: string): Promise<boolean> {
  if (!navigator.clipboard?.writeText) {
    return false;
  }

  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

type AgentTokenDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  token: string;
  serverUrl: string;
  installScript?: InstallTokenResponse | null;
  mode?: 'created' | 'installInfo';
};

export function AgentTokenDialog({
  open,
  onOpenChange,
  token,
  serverUrl,
  installScript,
  mode = 'created',
}: AgentTokenDialogProps) {
  const { t } = useTranslation('environments');
  const [copied, setCopied] = useState<string | null>(null);
  const agentImage = installScript?.agentImage || AgentDockerImage;
  const effectiveServerUrl = installScript?.serverUrl || serverUrl;
  const transferListen = installScript?.transferListen || '0.0.0.0:8788';
  const transferAdvertiseUrl = installScript?.transferAdvertiseUrl || 'http://agent-host-or-ip:8788';

  const copyToClipboard = async (text: string, key: string) => {
    const copied = await copyAgentText(text);
    if (!copied) {
      toast.error(t('toast.copyFailed'));
      return;
    }

    setCopied(key);
    toast.success(t('toast.copiedToClipboard'));
    setTimeout(() => setCopied(null), 2000);
  };

  const dockerCmd = `docker pull ${agentImage}
docker rm -f mcharbor-agent 2>/dev/null || true
docker run -d \\
  --name mcharbor-agent \\
  --restart unless-stopped \\
  -p 8788:8788 \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  -e MCHARBOR_URL=${effectiveServerUrl} \\
  -e MCHARBOR_AGENT_TOKEN=${token} \\
  -e DOCKER_HOST=unix:///var/run/docker.sock \\
  -e LOG_LEVEL=info \\
  -e MCHARBOR_TRANSFER_LISTEN=${transferListen} \\
  -e MCHARBOR_TRANSFER_ADVERTISE_URL=${transferAdvertiseUrl} \\
  ${agentImage}`;

  const binaryCmd = `MCHARBOR_URL=${effectiveServerUrl} \\
MCHARBOR_AGENT_TOKEN=${token} \\
DOCKER_HOST=unix:///var/run/docker.sock \\
LOG_LEVEL=info \\
MCHARBOR_TRANSFER_LISTEN=${transferListen} \\
MCHARBOR_TRANSFER_ADVERTISE_URL=${transferAdvertiseUrl} \\
mcharbor-agent`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t(mode === 'installInfo' ? 'agentToken.installInfoTitle' : 'agentToken.title')}</DialogTitle>
          <DialogDescription>
            {t(mode === 'installInfo' ? 'agentToken.installInfoDescription' : 'agentToken.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <AgentTokenValue
            label={t('agentToken.tokenLabel')}
            token={token}
            copiedKey={copied}
            ariaLabel={t('agentToken.copyToken')}
            onCopy={copyToClipboard}
          />

          {installScript && (
            <AgentCopyBlock
              label={t('agentToken.installScript')}
              value={installScript.script}
              copyKey="script"
              copiedKey={copied}
              ariaLabel={t('agentToken.copyScript')}
              onCopy={copyToClipboard}
              note={t('agentToken.scriptExpiry')}
            />
          )}

          <AgentCopyBlock
            label={t('agentToken.runWithDocker')}
            value={dockerCmd}
            copyKey="docker"
            copiedKey={copied}
            ariaLabel={t('agentToken.copyDockerCommand')}
            onCopy={copyToClipboard}
          />

          <AgentCopyBlock
            label={t('agentToken.runAsBinary')}
            value={binaryCmd}
            copyKey="binary"
            copiedKey={copied}
            ariaLabel={t('agentToken.copyBinaryCommand')}
            onCopy={copyToClipboard}
          />
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('agentToken.done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
