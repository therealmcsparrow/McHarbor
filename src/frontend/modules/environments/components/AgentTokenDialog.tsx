// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useFlashValue } from '@resources/hooks/useFlash';
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@resources/components/ui/Tabs';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { ManualCopyDialog } from '@resources/components/ManualCopyDialog';
import { copyToClipboard } from '@resources/utils/clipboard';
import type { InstallTokenResponse } from '../hooks/useEnvironmentActions';
import { buildAgentInstallCommands } from './agent-install-commands';
import { AgentCopyBlock } from './AgentCopyBlock';
import { AgentBinaryInstall } from './AgentBinaryInstall';
import { AgentTokenValue } from './AgentTokenValue';

export { AgentDockerImage } from '../constants';

export const copyAgentText = copyToClipboard;

type InstallMethod = 'script' | 'compose' | 'docker' | 'binary';

type AgentTokenDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  token: string;
  serverUrl: string;
  installScript?: InstallTokenResponse | null;
  caFingerprint?: string;
  mode?: 'created' | 'installInfo';
};

export function AgentTokenDialog({
  open: isOpen,
  onOpenChange,
  token,
  serverUrl,
  installScript,
  caFingerprint,
  mode = 'created',
}: AgentTokenDialogProps) {
  const { t } = useTranslation('environments');
  const [pickedMethod, setMethod] = useState<InstallMethod>(installScript ? 'script' : 'compose');
  const { value: copied, flash: flashCopied } = useFlashValue<string>();
  const [insecureTls, setInsecureTls] = useState(
    typeof window !== 'undefined' && window.location.protocol === 'https:',
  );
  const [manualCopy, setManualCopy] = useState<{ value: string; label: string } | null>(null);
  // The one-line script tab only exists when an install token was issued. The
  // regenerate-token path has no script, and a selected tab with no content is
  // an empty dialog.
  const method: InstallMethod = !installScript && pickedMethod === 'script' ? 'compose' : pickedMethod;
  if (installScript && !isOpen && pickedMethod !== 'script') setMethod('script');

  const { composeFile, dockerCmd, installUrl, effectiveServerUrl, isHttps, transferListen, transferAdvertiseUrl, pin } =
    buildAgentInstallCommands({ token, serverUrl, installScript, caFingerprint, insecureTls });

  const copyWithFeedback = async (text: string, key: string, label: string) => {
    const result = await copyAgentText(text);
    if (!result.ok) {
      setManualCopy({ value: text, label });
      return;
    }

    flashCopied(key);
    toast.success(t('toast.copiedToClipboard'));
  };

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
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
            onCopy={copyWithFeedback}
          />

          <div className="flex items-start justify-between gap-3 rounded-lg border border-border p-3">
            <div>
              <Label htmlFor="agent-insecure" className="text-sm font-normal">
                {t('agentToken.insecureTls')}
              </Label>
              <p className="mt-1 text-xs text-muted-foreground">{t('agentToken.insecureTlsHint')}</p>
            </div>
            <Switch id="agent-insecure" checked={insecureTls} onCheckedChange={setInsecureTls} />
          </div>

          <Tabs
            value={method}
            onValueChange={(value) => setMethod(value as InstallMethod)}
            className="flex flex-col gap-3"
          >
            <TabsList aria-label={t('agentToken.methodLabel')}>
              <TabsTrigger value="compose">{t('agentToken.methods.compose')}</TabsTrigger>
              <TabsTrigger value="docker">{t('agentToken.methods.docker')}</TabsTrigger>
              <TabsTrigger value="binary">{t('agentToken.methods.binary')}</TabsTrigger>
              {installScript && <TabsTrigger value="script">{t('agentToken.methods.script')}</TabsTrigger>}
            </TabsList>

            {installScript && (
              <TabsContent value="script" className="space-y-3">
                <AgentCopyBlock
                  label={t('agentToken.installUrlDocker')}
                  value={installUrl('docker')}
                  copyKey="script-docker"
                  copiedKey={copied}
                  ariaLabel={t('agentToken.copyScript')}
                  onCopy={copyWithFeedback}
                  note={t('agentToken.installUrlNote')}
                />
                <AgentCopyBlock
                  label={t('agentToken.installUrlBinary')}
                  value={installUrl('binary')}
                  copyKey="script-binary"
                  copiedKey={copied}
                  ariaLabel={t('agentToken.copyScript')}
                  onCopy={copyWithFeedback}
                  note={t('agentToken.scriptExpiry')}
                />
              </TabsContent>
            )}

            <TabsContent value="compose">
              <AgentCopyBlock
                label={t('agentToken.runWithCompose')}
                value={composeFile}
                copyKey="compose"
                copiedKey={copied}
                ariaLabel={t('agentToken.copyComposeFile')}
                onCopy={copyWithFeedback}
                note={t('agentToken.composeLoadNote')}
              />
            </TabsContent>

            <TabsContent value="docker">
              <AgentCopyBlock
                label={t('agentToken.runWithDocker')}
                value={dockerCmd}
                copyKey="docker"
                copiedKey={copied}
                ariaLabel={t('agentToken.copyDockerCommand')}
                onCopy={copyWithFeedback}
              />
            </TabsContent>

            <TabsContent value="binary">
              <AgentBinaryInstall
                serverUrl={effectiveServerUrl}
                token={token}
                isHttps={isHttps}
                insecureTls={insecureTls}
                transferListen={transferListen}
                transferAdvertiseUrl={transferAdvertiseUrl}
                pin={pin}
                copiedKey={copied}
                onCopy={copyWithFeedback}
              />
            </TabsContent>
          </Tabs>
        </div>

        <DialogFooter>
          <Button onClick={() => onOpenChange(false)}>{t('agentToken.done')}</Button>
        </DialogFooter>
      </DialogContent>

      <ManualCopyDialog
        open={manualCopy !== null}
        onOpenChange={(next) => {
          if (!next) setManualCopy(null);
        }}
        value={manualCopy?.value ?? ''}
        title={
          manualCopy?.label ? `${t('clipboard.manualCopyTitle', { ns: 'common' })}: ${manualCopy.label}` : undefined
        }
      />
    </Dialog>
  );
}
