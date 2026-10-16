// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconAlertTriangle, IconRefresh } from '@tabler/icons-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { formatDateCustom } from '@resources/utils/format';

type DegradedStateBannerProps = {
  error?: unknown;
  failureCount?: number;
  updatedAt?: number;
  onRetry?: () => void;
};

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : null;
}

export function DegradedStateBanner({ error, failureCount = 0, updatedAt, onRetry }: DegradedStateBannerProps) {
  const { t } = useTranslation('common');
  if (!error && failureCount === 0) return null;

  const lastAttempt = updatedAt
    ? formatDateCustom(updatedAt, { hour: '2-digit', minute: '2-digit', second: '2-digit' })
    : t('degradedState.unknownTime');

  return (
    <div className="flex flex-col gap-3 rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm text-warning sm:flex-row sm:items-center sm:justify-between">
      <div className="flex min-w-0 items-start gap-3">
        <IconAlertTriangle className="mt-0.5 size-4 shrink-0 text-warning" />
        <div className="min-w-0">
          <p className="font-medium text-foreground">{t('degradedState.title')}</p>
          <p className="text-muted-foreground">
            {t('degradedState.description', { count: failureCount, time: lastAttempt })}
          </p>
          {errorMessage(error) && (
            <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{errorMessage(error)}</p>
          )}
        </div>
      </div>
      {onRetry && (
        <Button variant="outline" size="sm" onClick={onRetry}>
          <IconRefresh className="size-4" />
          {t('actions.refresh')}
        </Button>
      )}
    </div>
  );
}
