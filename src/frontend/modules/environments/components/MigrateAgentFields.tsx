// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';

type MigrateAgentFieldsProps = {
  deployAgent: boolean;
  onDeployAgentChange: (value: boolean) => void;
  insecureTls: boolean;
  onInsecureTlsChange: (value: boolean) => void;
  agentToken: string;
  onAgentTokenChange: (value: string) => void;
  immediate: boolean;
  onImmediateChange: (value: boolean) => void;
};

export function MigrateAgentFields({
  deployAgent,
  onDeployAgentChange,
  insecureTls,
  onInsecureTlsChange,
  agentToken,
  onAgentTokenChange,
  immediate,
  onImmediateChange,
}: MigrateAgentFieldsProps) {
  const { t } = useTranslation('environments');

  return (
    <>
      <div className="flex items-start justify-between gap-3">
        <div>
          <Label htmlFor="migrate-deploy" className="text-sm font-normal">
            {t('migrate.deployAgent')}
          </Label>
          <p className="mt-1 text-xs text-muted-foreground">
            {deployAgent ? t('migrate.deployAgentHint') : t('migrate.deployAgentOffHint')}
          </p>
        </div>
        <Switch id="migrate-deploy" checked={deployAgent} onCheckedChange={onDeployAgentChange} />
      </div>

      {deployAgent && (
        <div className="flex items-start justify-between gap-3">
          <div>
            <Label htmlFor="migrate-insecure" className="text-sm font-normal">
              {t('migrate.insecureTls')}
            </Label>
            <p className="mt-1 text-xs text-muted-foreground">{t('migrate.insecureTlsHint')}</p>
          </div>
          <Switch id="migrate-insecure" checked={insecureTls} onCheckedChange={onInsecureTlsChange} />
        </div>
      )}

      <div>
        <Label htmlFor="migrate-agent-token" className="mb-2">
          {t('migrate.existingToken')}
        </Label>
        <Input
          id="migrate-agent-token"
          value={agentToken}
          onChange={(changeEvent) => onAgentTokenChange(changeEvent.target.value)}
          placeholder={t('migrate.existingTokenPlaceholder')}
          autoComplete="off"
          spellCheck={false}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('migrate.existingTokenHint')}</p>
      </div>

      <div className="flex items-start justify-between gap-3">
        <div>
          <Label htmlFor="migrate-immediate" className="text-sm font-normal">
            {t('migrate.immediate')}
          </Label>
          <p className="mt-1 text-xs text-muted-foreground">
            {immediate ? t('migrate.immediateHint') : t('migrate.gracefulHint')}
          </p>
        </div>
        <Switch id="migrate-immediate" checked={immediate} onCheckedChange={onImmediateChange} />
      </div>
    </>
  );
}
