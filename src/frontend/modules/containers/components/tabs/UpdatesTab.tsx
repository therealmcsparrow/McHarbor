// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconAlertTriangle, IconArrowsExchange, IconCircleCheck, IconRefresh } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { useCheckContainerUpdates, useContainerUpdateResults } from '../../hooks/useContainerUpdates';

type UpdatesTabProps = {
  containerId: string;
  containerImage: string;
};

export function UpdatesTab({ containerId, containerImage }: UpdatesTabProps) {
  const { t } = useTranslation('containers');
  const checkMutation = useCheckContainerUpdates();
  const results = useContainerUpdateResults();
  const result = results?.get(containerId);

  useEffect(() => {
    if (!result && !checkMutation.isPending) {
      checkMutation.mutate([containerId]);
    }
    // run once on mount per container
    // oxlint-disable-next-line react/exhaustive-deps -- runs once per container; re-running on result or pending would loop the check
  }, [containerId]);

  const running = checkMutation.isPending;

  return (
    <div className="space-y-4">
      <div className="mc-glass flex items-center justify-between rounded-lg p-4">
        <div>
          <div className="text-sm font-medium text-foreground">{t('updates.title')}</div>
          <div className="mt-1 font-mono text-xs break-all text-muted-foreground">{containerImage}</div>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={() => checkMutation.mutate([containerId])}
          disabled={running}
          aria-label={t('updates.checkUpdates')}
        >
          <IconRefresh className={`size-4 ${running ? 'animate-spin' : ''}`} />
          {t('updates.checkUpdates')}
        </Button>
      </div>

      {running && !result ? (
        <div className="mc-glass flex h-32 items-center justify-center rounded-lg">
          <Spinner size="md" />
        </div>
      ) : result ? (
        <div className="mc-glass rounded-lg p-4">
          <div className="flex items-center gap-2">
            {result.updateAvailable ? (
              <>
                <IconArrowsExchange className="size-5 text-primary" />
                <Badge variant="default">{t('updates.available')}</Badge>
              </>
            ) : result.error ? (
              <>
                <IconAlertTriangle className="size-5 text-destructive" />
                <Badge variant="destructive">{t('updates.checkFailed')}</Badge>
              </>
            ) : (
              <>
                <IconCircleCheck className="size-5 text-success" />
                <Badge variant="success">{t('updates.upToDate')}</Badge>
              </>
            )}
          </div>

          <dl className="mt-4 grid grid-cols-1 gap-3 text-xs sm:grid-cols-2">
            <div>
              <dt className="text-muted-foreground">{t('updates.currentDigest')}</dt>
              <dd className="mt-1 font-mono break-all text-foreground">{result.currentDigest || '—'}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">{t('updates.remoteDigest')}</dt>
              <dd className="mt-1 font-mono break-all text-foreground">{result.remoteDigest || '—'}</dd>
            </div>
          </dl>

          {result.error && (
            <div className="mt-3 rounded-md border border-destructive/40 bg-destructive/10 p-2 text-xs text-destructive">
              {result.error}
            </div>
          )}
        </div>
      ) : (
        <div className="mc-glass rounded-lg p-6 text-center text-sm text-muted-foreground">{t('updates.empty')}</div>
      )}
    </div>
  );
}
