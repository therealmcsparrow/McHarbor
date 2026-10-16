// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import type { CreateEnvironmentFields } from '../hooks/useCreateEnvironmentFields';
import { AgentPrerequisitesPanel } from './AgentPrerequisitesPanel';
import { AgentHostChecksPanel } from './AgentHostChecksPanel';

type Props = {
  f: CreateEnvironmentFields;
  probing: boolean;
  installing: boolean;
  onProbe: () => void;
  onInstall: () => void;
};

export function CreateEnvironmentSshSection({ f, probing, installing, onProbe, onInstall }: Props) {
  const { t } = useTranslation('environments');

  return (
    <>
      <div className="flex items-start gap-3">
        <Switch id="agent-save-credentials" checked={f.saveCredentials} onCheckedChange={f.setSaveCredentials} />
        <div className="min-w-0">
          <Label htmlFor="agent-save-credentials" className="text-sm font-normal">
            {t('create.saveCredentials')}
          </Label>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('create.saveCredentialsHelp')}</p>
        </div>
      </div>

      <AgentHostChecksPanel
        canProbe={f.isSSHDeployReady}
        buildRequest={f.sshProbeRequest}
        results={f.hostChecks}
        onResult={(scope, state) => f.setHostChecks((current) => ({ ...current, [scope]: state }))}
      />

      <AgentPrerequisitesPanel
        report={f.prereqReport}
        error={f.prereqError}
        probing={probing}
        canProbe={f.isSSHDeployReady}
        selected={f.prereqInstall}
        output={f.prereqOutput}
        installing={installing}
        rootPassword={f.rootPassword}
        onRootPasswordChange={f.setRootPassword}
        onInstall={onInstall}
        onProbe={onProbe}
        onSelectedChange={f.setPrereqInstall}
      />
    </>
  );
}
