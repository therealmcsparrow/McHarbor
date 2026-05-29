// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCopy, IconCheck } from '@tabler/icons-react';
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

export { AgentDockerImage } from '../constants';

export async function copyAgentText(text: string): Promise<boolean> {
  if (navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Fall back to a temporary textarea below.
    }
  }

  const textarea = document.createElement('textarea');
  textarea.value = text;
  textarea.setAttribute('readonly', '');
  textarea.style.position = 'fixed';
  textarea.style.left = '-9999px';
  textarea.style.top = '0';
  document.body.appendChild(textarea);
  textarea.focus();
  textarea.select();
  textarea.setSelectionRange(0, text.length);

  try {
    return document.execCommand('copy');
  } catch {
    return false;
  } finally {
    document.body.removeChild(textarea);
  }
}

type AgentTokenDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  token: string;
  serverUrl: string;
  installScript?: InstallTokenResponse | null;
};

export function AgentTokenDialog({ open, onOpenChange, token, serverUrl, installScript }: AgentTokenDialogProps) {
  const { t } = useTranslation('environments');
  const [copied, setCopied] = useState<string | null>(null);

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

  const dockerCmd = `docker run -d \\
  --name mcharbor-agent \\
  --restart unless-stopped \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  -e MCHARBOR_URL=${serverUrl} \\
  -e MCHARBOR_AGENT_TOKEN=${token} \\
  ${AgentDockerImage}`;

  const binaryCmd = `MCHARBOR_URL=${serverUrl} \\
MCHARBOR_AGENT_TOKEN=${token} \\
mcharbor-agent`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>{t('agentToken.title')}</DialogTitle>
          <DialogDescription>
            {t('agentToken.description')}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">{t('agentToken.tokenLabel')}</label>
            <div className="flex items-center gap-2">
              <code className="flex-1 rounded border border-border bg-muted px-3 py-2 font-mono text-xs break-all">
                {token}
              </code>
              <Button
                variant="outline"
                size="icon"
                onClick={() => copyToClipboard(token, 'token')}
                aria-label={t('agentToken.copyToken')}
              >
                {copied === 'token' ? (
                  <IconCheck className="h-4 w-4 text-green-500" />
                ) : (
                  <IconCopy className="h-4 w-4" />
                )}
              </Button>
            </div>
          </div>

          {installScript && (
            <div>
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                {t('agentToken.installScript')}
              </label>
              <div className="relative">
                <pre className="rounded border border-border bg-muted p-3 font-mono text-xs overflow-x-auto whitespace-pre">
                  {installScript.script}
                </pre>
                <Button
                  variant="ghost"
                  size="icon"
                  className="absolute right-1 top-1"
                  onClick={() => copyToClipboard(installScript.script, 'script')}
                  aria-label={t('agentToken.copyScript')}
                >
                  {copied === 'script' ? (
                    <IconCheck className="h-3.5 w-3.5 text-green-500" />
                  ) : (
                    <IconCopy className="h-3.5 w-3.5" />
                  )}
                </Button>
              </div>
              <p className="mt-1 text-xs text-muted-foreground">
                {t('agentToken.scriptExpiry')}
              </p>
            </div>
          )}

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              {t('agentToken.runWithDocker')}
            </label>
            <div className="relative">
              <pre className="rounded border border-border bg-muted p-3 font-mono text-xs overflow-x-auto whitespace-pre">
                {dockerCmd}
              </pre>
              <Button
                variant="ghost"
                size="icon"
                className="absolute right-1 top-1"
                onClick={() => copyToClipboard(dockerCmd, 'docker')}
                aria-label={t('agentToken.copyDockerCommand')}
              >
                {copied === 'docker' ? (
                  <IconCheck className="h-3.5 w-3.5 text-green-500" />
                ) : (
                  <IconCopy className="h-3.5 w-3.5" />
                )}
              </Button>
            </div>
          </div>

          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              {t('agentToken.runAsBinary')}
            </label>
            <div className="relative">
              <pre className="rounded border border-border bg-muted p-3 font-mono text-xs overflow-x-auto whitespace-pre">
                {binaryCmd}
              </pre>
              <Button
                variant="ghost"
                size="icon"
                className="absolute right-1 top-1"
                onClick={() => copyToClipboard(binaryCmd, 'binary')}
                aria-label={t('agentToken.copyBinaryCommand')}
              >
                {copied === 'binary' ? (
                  <IconCheck className="h-3.5 w-3.5 text-green-500" />
                ) : (
                  <IconCopy className="h-3.5 w-3.5" />
                )}
              </Button>
            </div>
          </div>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('agentToken.done')}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
