// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCheck, IconX } from '@tabler/icons-react';
import { Card } from '@resources/components/ui/Card';
import { StatusError } from '@resources/components/ui/StatusError';
import type { ReadinessRow } from './hostReadiness';

interface HostReadinessChecklistProps {
  rows: ReadinessRow[];
}

export function HostReadinessChecklist({ rows }: HostReadinessChecklistProps) {
  const { t } = useTranslation('compute');

  return (
    <Card className="p-5">
      <h2 className="text-sm font-medium text-foreground">{t('hosts.tests.checklist')}</h2>
      <p className="mt-1 text-xs text-muted-foreground">{t('hosts.tests.checklistHint')}</p>
      <ul className="mt-4 space-y-2">
        {rows.map((row) => (
          <li key={row.key} className="flex flex-wrap items-center gap-2 text-sm">
            {row.state === 'pass' && <IconCheck className="size-4 shrink-0 text-success" aria-hidden="true" />}
            {row.state === 'warn' && <IconAlertTriangle className="size-4 shrink-0 text-warning" aria-hidden="true" />}
            {row.state === 'fail' && <IconX className="size-4 shrink-0 text-destructive" aria-hidden="true" />}
            <span className="text-foreground">{t(`hosts.tests.rows.${row.key}`)}</span>
            <span className="text-xs text-muted-foreground">{t(`hosts.tests.state.${row.state}`)}</span>
            {row.detailKey && <span className="text-xs text-muted-foreground">{t(row.detailKey)}</span>}
            <StatusError error={row.detail} />
          </li>
        ))}
      </ul>
    </Card>
  );
}
