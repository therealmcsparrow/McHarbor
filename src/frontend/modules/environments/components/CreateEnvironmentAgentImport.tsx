// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import type { CreateEnvironmentFields } from '../hooks/useCreateEnvironmentFields';

export function CreateEnvironmentAgentImport({ f }: { f: CreateEnvironmentFields }) {
  const { t } = useTranslation('environments');

  return (
    <>
      <div className="flex items-start gap-3 pt-1">
        <Switch
          id="agent-import-existing"
          checked={f.importAgentToken}
          onCheckedChange={(checked) => {
            f.setImportAgentToken(checked);
            if (!checked) f.setExistingAgentToken('');
          }}
          className="mt-1"
        />
        <Label htmlFor="agent-import-existing" className="cursor-pointer">
          <div className="font-medium">{t('create.useExistingToken')}</div>
          <p className="mt-0.5 text-xs text-muted-foreground">{t('create.useExistingTokenDescription')}</p>
        </Label>
      </div>

      {f.importAgentToken && (
        <div>
          <Label className="mb-2">{t('create.existingTokenLabel')}</Label>
          <Input
            type="text"
            value={f.existingAgentToken}
            onChange={(e) => f.setExistingAgentToken(e.target.value)}
            placeholder={t('create.existingTokenPlaceholder')}
            autoComplete="off"
            spellCheck={false}
            className="font-mono text-xs"
          />
          <p className="mt-1 text-xs text-muted-foreground">{t('create.existingTokenHelp')}</p>
        </div>
      )}
    </>
  );
}
