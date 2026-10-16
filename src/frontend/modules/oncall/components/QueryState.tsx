// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';

type QueryStateProps = {
  isLoading: boolean;
  error: unknown;
  onRetry: () => void;
  missingPermission?: string | null;
  isEmpty: boolean;
  emptyText: string;
  children: ReactNode;
};

export function QueryState({
  isLoading,
  error,
  onRetry,
  missingPermission,
  isEmpty,
  emptyText,
  children,
}: QueryStateProps) {
  const { t } = useTranslation('oncall');
  if (missingPermission) {
    return (
      <p className="py-8 text-center text-sm text-muted-foreground">
        {t('state.noAccess', { permission: missingPermission })}
      </p>
    );
  }
  if (isLoading) {
    return (
      <output className="flex justify-center py-10" aria-label={t('state.loading')}>
        <Spinner size="lg" />
      </output>
    );
  }
  if (error) {
    return (
      <div className="flex flex-col items-center gap-3 py-8">
        <div className="flex items-center gap-2 text-sm text-destructive">
          <span>{t('state.loadFailed')}</span>
          <StatusError
            error={error instanceof Error ? error.message : typeof error === 'string' ? error : JSON.stringify(error)}
          />
        </div>
        <Button variant="outline" className="min-h-11" onClick={onRetry}>
          {t('state.retry')}
        </Button>
      </div>
    );
  }
  if (isEmpty) {
    return <p className="py-8 text-center text-sm text-muted-foreground">{emptyText}</p>;
  }
  return <>{children}</>;
}
