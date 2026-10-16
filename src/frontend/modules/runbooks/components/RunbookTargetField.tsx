// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useRunbookTargets } from '../hooks/useRunbookTargets';
import { RunbookPickField } from './RunbookPickField';
import type { TargetKind } from '@resources/types/runbooks';

type RunbookTargetFieldProps = {
  kind: TargetKind;
  value: string;
  onChange: (value: string) => void;
  envId: string;
  cluster?: string;
  ariaLabel: string;
  emptyLabel?: string;
};

export function RunbookTargetField({
  kind,
  value,
  onChange,
  envId,
  cluster,
  ariaLabel,
  emptyLabel,
}: RunbookTargetFieldProps) {
  const { t } = useTranslation('runbooks');
  const targets = useRunbookTargets(kind, envId, cluster);

  return (
    <div className="flex min-w-0 flex-col gap-1">
      <RunbookPickField
        key={targets.isSuccess ? 'loaded' : 'loading'}
        value={value}
        onChange={onChange}
        options={targets.data ?? []}
        ariaLabel={ariaLabel}
        placeholder={targets.isLoading ? t('editor.targetsLoading') : t('editor.pickTarget')}
        emptyOption={emptyLabel ? { value: '', label: emptyLabel } : undefined}
      />
      {targets.isError && (
        <p className="text-xs text-muted-foreground">
          {[t('editor.targetsFailed'), targets.error?.message].filter(Boolean).join(' — ')}
        </p>
      )}
    </div>
  );
}
