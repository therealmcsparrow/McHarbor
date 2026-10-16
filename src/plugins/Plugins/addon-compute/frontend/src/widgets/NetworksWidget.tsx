// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconNetwork } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Skeleton } from '@resources/components/ui/Skeleton';
import { useOverview } from '../hooks/useCompute';
import { useWidgetContext } from './mount';
import type { Network } from '../types';

/**
 * The guest networks each host offers, and which of them a guest can actually
 * be attached to.
 *
 * A host with pools but no network is the setup people get stuck on: the VM
 * wizard refuses at the last step with nothing to pick, and the reason is one
 * page away. Naming it on the dashboard is cheaper than diagnosing it later.
 */
export function NetworksWidget() {
  const { t } = useTranslation('compute');
  const { navigate } = useWidgetContext();
  const { data, isLoading } = useOverview();

  if (isLoading) {
    return <Skeleton className="h-full w-full" />;
  }

  const rows: { host: string; network: Network }[] = [];
  let hostsWithout = 0;
  for (const entry of data?.environments ?? []) {
    if (!entry.computeEnabled || !entry.available) continue;
    const networks = entry.networks ?? [];
    if (networks.length === 0) {
      hostsWithout += 1;
      continue;
    }
    for (const network of networks) {
      rows.push({ host: entry.name || entry.environmentId, network });
    }
  }

  if (rows.length === 0 && hostsWithout === 0) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
        <IconNetwork className="h-6 w-6 text-muted-foreground" aria-hidden="true" />
        <p className="text-sm text-muted-foreground">{t('overview.empty.title')}</p>
      </div>
    );
  }

  return (
    <Button
      type="button"
      variant="ghost"
      size="lg"
      onClick={() => navigate('/plugins/addon-compute/networks')}
      className="flex h-full w-full flex-col items-stretch justify-start gap-2 rounded p-1 text-left font-normal whitespace-normal hover:bg-muted/50"
    >
      <div className="flex items-baseline gap-2">
        <span className="text-2xl font-semibold text-foreground">{rows.length}</span>
        <span className="text-sm text-muted-foreground">{t('widgets.computeNetworks.total')}</span>
      </div>

      {hostsWithout > 0 && (
        <Badge variant="warning" className="w-fit">
          {t('widgets.computeNetworks.missing', { count: hostsWithout })}
        </Badge>
      )}

      <div className="min-h-0 flex-1 space-y-1 overflow-y-auto">
        {rows.slice(0, 6).map(({ host, network }) => (
          <div key={`${host}-${network.id}`} className="flex items-center justify-between gap-2">
            <span className="min-w-0 truncate text-sm text-foreground">{network.name}</span>
            <span className="shrink-0 font-mono text-xs text-muted-foreground">
              {network.subnetCidr || network.mode}
            </span>
          </div>
        ))}
      </div>
    </Button>
  );
}
