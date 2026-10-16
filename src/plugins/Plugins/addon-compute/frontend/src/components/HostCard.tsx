// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconDatabase, IconNetwork, IconPhoto, IconServer2, IconSettings } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { Switch } from '@resources/components/ui/Switch';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { usePermissionGate } from '@resources/hooks/usePermissionGate';
import { useUpdateEnvironment } from '../hooks/useCompute';
import { COMPUTE_PERM } from '../permissions';
import { formatBytes } from '../format';
import { hostStatus, worstPool } from '../overview-status';
import { PoolCapacityBar } from './PoolCapacityBar';
import { StatTile } from './StatTile';
import type { OverviewEntry } from '../types';

/**
 * One compute host, in the same shape as an environment card.
 *
 * The card answers the only question that matters before creating a guest — is
 * this host ready, and if not what is missing — so the blocking condition is
 * the first thing after the name, and an unreachable host says why instead of
 * showing zeroes that read as "empty".
 */

/** The image cache root every agent mounts the shared library on. */
const AGENT_LIBRARY_MOUNT = '/var/lib/mcharbor/compute/images';

interface HostCardProps {
  entry: OverviewEntry;
  onNavigate: (to: string) => void;
  onSetup: (environmentId: string) => void;
}

export function HostCard({ entry, onNavigate, onSetup }: HostCardProps) {
  const { t } = useTranslation('compute');
  const updateEnvironment = useUpdateEnvironment();
  const { denied, reason } = usePermissionGate(COMPUTE_PERM.MANAGE);

  const hostState = hostStatus(entry);
  const pools = entry.pools ?? [];
  const networks = entry.networks ?? [];
  const fullest = worstPool(entry);
  const totalCapacity = pools.reduce((sum, pool) => sum + pool.capacityBytes, 0);

  return (
    <Card className="min-h-[350px] transition-colors hover:border-primary/40">
      <CardContent className="flex flex-1 flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <h3 className="truncate text-base font-semibold text-foreground">{entry.name}</h3>
            <p className="truncate font-mono text-xs text-muted-foreground">{entry.environmentId}</p>
          </div>
          <Badge variant={hostState.tone} className="shrink-0 px-2 py-1 text-[10px]">
            {t(`overview.status.${hostState.key}`)}
          </Badge>
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="secondary">{t('nav.section.compute')}</Badge>
          {entry.capability ? (
            <>
              <Badge variant="outline" className="uppercase">
                {entry.capability.os}
              </Badge>
              <span className="truncate font-mono text-xs text-muted-foreground">{entry.capability.arch}</span>
            </>
          ) : (
            <span className="truncate text-xs text-muted-foreground">{t('capability.neverProbed')}</span>
          )}
        </div>

        {!entry.available && (
          <p className="rounded-lg border border-border/70 bg-warning/10 px-3 py-2 text-xs text-foreground">
            {entry.unavailableReason === 'not_agent_environment'
              ? t('agent.required.title')
              : entry.unavailableReason === 'agent_too_old'
                ? t('agent.tooOld.title')
                : t('agent.offline.title')}
          </p>
        )}

        {entry.available && (
          <div className="rounded-lg border border-border/70 bg-background/40 p-3">
            <div className="flex items-center justify-between gap-3">
              <span className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
                <IconDatabase className="size-4" aria-hidden="true" />
                {t('nav.pools')}
              </span>
              <span className="shrink-0 text-xs text-muted-foreground">
                {pools.length > 0 ? t('overview.poolCount', { count: pools.length }) : ''}
              </span>
            </div>
            <div className="mt-3">
              {fullest ? (
                <>
                  <PoolCapacityBar pool={fullest} compact />
                  <p className="mt-1.5 truncate text-xs text-muted-foreground">
                    {pools.length === 1 ? fullest.name : formatBytes(totalCapacity)}
                  </p>
                </>
              ) : (
                <p className="text-sm font-medium text-warning">{t('overview.needsPool')}</p>
              )}
            </div>
          </div>
        )}

        <div className="mt-auto grid grid-cols-2 gap-2">
          <StatTile
            label={t('nav.networks')}
            value={
              networks.length > 0 ? networks.map((network) => network.name).join(', ') : t('overview.needsNetwork')
            }
            tone={networks.length > 0 ? 'default' : 'warning'}
            icon={<IconNetwork className="size-3.5" aria-hidden="true" />}
          />
          <StatTile
            label={t('nav.images')}
            value={entry.cachedImages}
            icon={<IconPhoto className="size-3.5" aria-hidden="true" />}
          />
        </div>

        {/*
          Stated by the operator, not probed: McHarbor cannot see whether this
          host's agent really mounts the library, so the tooltip names the path
          that has to be mounted rather than claiming it is.
        */}
        <div className="flex items-start justify-between gap-3 rounded-lg border border-border p-2">
          <div className="min-w-0">
            <p className="flex items-center gap-1.5 text-xs font-medium text-foreground">
              <IconServer2 className="size-3.5" aria-hidden="true" />
              {t('overview.sharedLibrary')}
            </p>
            <p className="mt-0.5 text-xs text-muted-foreground">
              {entry.sharedLibrary ? t('overview.sharedLibraryOn') : t('overview.sharedLibraryOff')}
            </p>
          </div>
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="inline-flex">
                <Switch
                  aria-label={t('overview.sharedLibrary')}
                  checked={Boolean(entry.sharedLibrary)}
                  disabled={denied || updateEnvironment.isPending}
                  onCheckedChange={(checked) =>
                    updateEnvironment.mutate({
                      envId: entry.environmentId,
                      sharedLibrary: checked,
                    })
                  }
                />
              </span>
            </TooltipTrigger>
            <TooltipContent>
              {denied ? reason : t('overview.sharedLibraryHint', { path: AGENT_LIBRARY_MOUNT })}
            </TooltipContent>
          </Tooltip>
        </div>
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2">
        <Button variant="ghost" size="sm" onClick={() => onNavigate(`/hosts/${entry.environmentId}/overview`)}>
          {t('hosts.detail.open')}
        </Button>
        <Button variant="ghost" size="sm" onClick={() => onNavigate('/pools')}>
          {t('nav.pools')}
        </Button>
        <Button variant="ghost" size="sm" onClick={() => onNavigate('/images')}>
          {t('nav.images')}
        </Button>
        <div className="flex-1" />
        <Button
          permission={COMPUTE_PERM.MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.runWizard')}
          title={t('actions.runWizard')}
          onClick={() => onSetup(entry.environmentId)}
        >
          <IconSettings className="h-4 w-4" />
        </Button>
      </CardFooter>
    </Card>
  );
}
