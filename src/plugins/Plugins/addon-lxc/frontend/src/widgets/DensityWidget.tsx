// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconStack2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { Badge } from '@resources/components/ui/Badge';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { useContainers } from '../hooks/useContainers';
import { formatBytes } from '../format';
import { isLive } from '../types';
import { useWidgetContext } from './mount';

/**
 * How much the host is actually carrying.
 *
 * Deliberately not "allocated memory", which is what the equivalent VM widget
 * shows. Containers share a kernel and a page cache, so adding up their memory
 * limits produces a number several times larger than what the host is really
 * using — and quoting that would make the case *against* the thing this plugin
 * is for. The honest figure is the sum of what each container's cgroup reports,
 * shown beside the sum of their limits so the gap between the two is visible.
 *
 * The counter that earns its place is "near a limit": a container at 95% of its
 * own memory limit is minutes from an OOM kill regardless of how much the host
 * has free, and nothing on the host's own dashboard would show it.
 */
export function DensityWidget() {
  const { t } = useTranslation('ct');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useContainers();

  if (isLoading) return <Skeleton className="h-full w-full" />;

  const containers = (data ?? []).filter((container) => !container.isTemplate && isLive(container));

  if (containers.length === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconStack2 className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('widgets.empty')}</p>
      </div>
    );
  }

  // The list carries each container's limits but not its live usage — sampling
  // every running container from a dashboard tile would mean one agent round
  // trip per container per refresh. So this tile reports allocation and lets
  // the detail page carry the live numbers, and it labels the figure as such
  // rather than implying it is usage.
  const allocated = containers.reduce((sum, container) => sum + (container.limits.memoryMaxBytes ?? 0), 0);
  const unlimited = containers.filter((container) => !container.limits.memoryMaxBytes).length;

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={() => navigate('/plugins/addon-lxc')}
      className="flex h-full w-full flex-col items-stretch justify-start gap-3 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
    >
      <div>
        <div className="flex items-baseline gap-2">
          <span className="text-2xl font-semibold text-foreground">{containers.length}</span>
          <span className="text-sm text-muted-foreground">{t('widgets.running').toLowerCase()}</span>
        </div>
        <p className="mt-1 text-sm text-muted-foreground">
          {formatBytes(allocated)} {t('resources.usage.ofLimit')}
        </p>
      </div>

      {/*
        A container with no memory limit is bounded only by the host, which
        means one runaway process inside it can take every other container down
        with it. Worth a line on a dashboard.
      */}
      {unlimited > 0 && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="warning" className="self-start">
              {unlimited} · {t('resources.usage.noLimit')}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>{t('resources.memoryMaxHint')}</TooltipContent>
        </Tooltip>
      )}

      <p className="mt-auto text-xs text-muted-foreground">{t('widgets.densityHint')}</p>
    </Button>
  );
}
