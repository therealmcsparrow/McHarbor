// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCheck } from '@tabler/icons-react';
import type { PrerequisiteReport } from '../hooks/useEnvironmentActions';

type PrerequisiteItem = PrerequisiteReport['items'][number];

type PrerequisiteItemRowProps = {
  item: PrerequisiteItem;
};

export function PrerequisiteItemRow({ item }: PrerequisiteItemRowProps) {
  const { t } = useTranslation('environments');

  return (
    <li className="flex items-start gap-2 text-xs">
      {item.present ? (
        <IconCheck className="mt-0.5 h-3.5 w-3.5 shrink-0 text-success" aria-hidden="true" />
      ) : (
        <IconAlertTriangle
          className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${item.required ? 'text-destructive' : 'text-warning'}`}
          aria-hidden="true"
        />
      )}
      <span className="min-w-0 flex-1">
        <span className="text-foreground">{item.label}</span>
        {!item.present && item.required ? (
          <span className="ml-1 text-destructive">{t('create.prereq.required')}</span>
        ) : null}
        {item.detail ? <span className="block text-muted-foreground">{item.detail}</span> : null}
        {!item.present && item.remediation ? (
          <code className="mt-0.5 block rounded bg-muted px-1.5 py-1 font-mono text-[11px] break-all text-muted-foreground">
            {item.remediation}
          </code>
        ) : null}
        {!item.present && !item.remediation ? (
          <span className="block text-muted-foreground">{t('create.prereq.noRemediation')}</span>
        ) : null}
      </span>
    </li>
  );
}
