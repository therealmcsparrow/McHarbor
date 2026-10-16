// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconBox } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Badge } from '@resources/components/ui/Badge';
import { useContainers } from '../hooks/useContainers';
import { isLive } from '../types';
import { useWidgetContext } from './mount';

/**
 * How many containers are running, and which ones need attention.
 *
 * "Needs attention" is the useful number and is deliberately not just
 * "stopped": a container someone shut down on purpose is fine. What matters is
 * a crash, or a host nobody can reach.
 */
export function SummaryWidget() {
  const { t } = useTranslation('ct');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useContainers();

  if (isLoading) return <Skeleton className="h-full w-full" />;

  const containers = (data ?? []).filter((container) => !container.isTemplate);
  const running = containers.filter((container) => isLive(container)).length;
  const attention = containers.filter((container) => container.state === 'crashed' || !container.hostAvailable);

  if (containers.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconBox className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('widgets.empty')}</p>
      </div>
    );
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={() => navigate('/plugins/addon-lxc')}
      className="flex h-full w-full flex-col items-stretch justify-start gap-3 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
    >
      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold text-foreground">{running}</span>
        <span className="text-sm text-muted-foreground">
          / {containers.length} {t('widgets.running').toLowerCase()}
        </span>
      </div>

      {attention.length > 0 && (
        <div className="min-h-0 flex-1 space-y-1 overflow-y-auto">
          {attention.slice(0, 5).map((container) => (
            <div key={container.id} className="flex items-center justify-between gap-2">
              <span className="min-w-0 truncate text-sm text-foreground">{container.name}</span>
              <Badge variant={container.state === 'crashed' ? 'destructive' : 'warning'}>
                {t(`state.${container.state}`, container.state)}
              </Badge>
            </div>
          ))}
        </div>
      )}
    </Button>
  );
}
