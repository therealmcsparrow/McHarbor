// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconCircleCheck, IconCircleMinus, IconCircleX } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import type { CheckResult, CheckStatus } from '@resources/hooks/useSupport';

const tone: Record<CheckStatus, 'success' | 'warning' | 'destructive' | 'secondary'> = {
  pass: 'success',
  warn: 'warning',
  fail: 'destructive',
  skip: 'secondary',
};

const icons: Record<CheckStatus, ReactNode> = {
  pass: <IconCircleCheck className="size-5 text-success" aria-hidden="true" />,
  warn: <IconAlertTriangle className="size-5 text-warning" aria-hidden="true" />,
  fail: <IconCircleX className="size-5 text-destructive" aria-hidden="true" />,
  skip: <IconCircleMinus className="size-5 text-muted-foreground" aria-hidden="true" />,
};

type CheckResultRowProps = {
  id: string;
  result?: CheckResult;
  action?: ReactNode;
};

export function CheckResultRow({ id, result, action }: CheckResultRowProps) {
  const { t } = useTranslation('support');
  const details = Object.entries(result?.details ?? {}).sort(([a], [b]) => a.localeCompare(b));
  return (
    <li className="flex items-start gap-3 border-b border-border py-3 last:border-0" data-testid={`check-${id}`}>
      <span className="mt-0.5">{result ? icons[result.status] : icons.skip}</span>
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="text-sm font-medium text-foreground">{t(`check.${id}`)}</span>
          {result && <Badge variant={tone[result.status]}>{t(`status.${result.status}`)}</Badge>}
          {result && (
            <span className="text-xs text-muted-foreground">
              {t('diagnostics.duration', { ms: result.durationMs })}
            </span>
          )}
        </div>
        <p className="text-sm text-muted-foreground">
          {result ? t(`reason.${id}.${result.reason}`) : t('diagnostics.notRun')}
        </p>
        {details.length > 0 && (
          <dl className="grid grid-cols-[minmax(0,auto)_1fr] gap-x-3 gap-y-0.5 text-xs">
            {details.map(([k, v]) => (
              <div key={k} className="contents">
                <dt className="font-mono text-muted-foreground">{k}</dt>
                <dd className="font-mono break-all text-foreground">{v}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>
      {action}
    </li>
  );
}
