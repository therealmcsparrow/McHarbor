// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import type { TFunction } from 'i18next';
import { IconCloudUpload } from '@tabler/icons-react';
import type { ColumnDef } from '@resources/components/table-features';
import type { TableColumnMeta } from '@resources/layout/table-page/types';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { StatusError } from '@resources/components/ui/StatusError';
import { PERM } from '@core/auth/permissionKeys';
import {
  agentPlatform,
  agentState,
  inFlight,
  type AgentRow,
  type AgentState,
} from '@resources/hooks/useAgentInventory';

const DASH = '—';

export const AGENT_STATE_KEY: Record<AgentState, string> = {
  updating: 'about.agents.progress.updating',
  pending: 'about.agents.progress.pending',
  started: 'about.agents.progress.started',
  failed: 'about.agents.progress.failed',
  tooOld: 'about.agents.tooOld',
  excluded: 'about.agents.excluded',
  outdated: 'about.agents.outdated',
  current: 'about.agents.current',
};

const STATE_VARIANT: Record<AgentState, 'secondary' | 'success' | 'destructive' | 'warning' | 'outline'> = {
  updating: 'secondary',
  pending: 'secondary',
  started: 'success',
  failed: 'destructive',
  tooOld: 'warning',
  excluded: 'outline',
  outdated: 'warning',
  current: 'success',
};

type AgentColumnsArgs = {
  excluded: ReadonlySet<string>;
  sweeping: boolean;
  onUpdate: (agent: AgentRow) => void;
};

function kindLabel(agent: AgentRow, t: TFunction<'settings'>): string {
  return agent.installKind ? t(`about.agents.kind.${agent.installKind}`) : t('about.agents.kind.unknown');
}

function targetText(agent: AgentRow): string {
  if (!agent.targetVersion) return DASH;
  return agent.targetBuild ? `${agent.targetVersion} (${agent.targetBuild})` : agent.targetVersion;
}

export function useAgentInventoryColumns({
  excluded,
  sweeping,
  onUpdate,
}: AgentColumnsArgs): ColumnDef<AgentRow, unknown>[] {
  const { t } = useTranslation('settings');

  return useMemo<ColumnDef<AgentRow, unknown>[]>(
    () => [
      {
        id: 'name',
        accessorFn: (agent) => agent.name,
        header: t('about.agents.name'),
        cell: ({ row }) => (
          <span>
            <span className="font-medium text-foreground">{row.original.name}</span>
            {row.original.hostname && row.original.hostname !== row.original.name ? (
              <span className="ml-1.5 text-xs text-muted-foreground">{row.original.hostname}</span>
            ) : null}
          </span>
        ),
      },
      {
        id: 'type',
        accessorFn: (agent) => kindLabel(agent, t),
        header: t('about.agents.type'),
        meta: { width: 'sm' } satisfies TableColumnMeta,
        cell: ({ row }) => <span className="text-muted-foreground">{kindLabel(row.original, t)}</span>,
      },
      {
        id: 'address',
        accessorFn: (agent) => agent.remoteIp || DASH,
        header: t('about.agents.address'),
        cell: ({ row }) => <span className="font-mono text-muted-foreground">{row.original.remoteIp || DASH}</span>,
      },
      {
        id: 'platform',
        accessorFn: (agent) => agentPlatform(agent) || DASH,
        header: t('about.agents.platform'),
        meta: { defaultVisible: false } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <span className="font-mono text-muted-foreground">{agentPlatform(row.original) || DASH}</span>
        ),
      },
      {
        id: 'version',
        accessorFn: (agent) => agent.version || DASH,
        header: t('about.agents.version'),
        cell: ({ row }) => <span className="font-mono text-muted-foreground">{row.original.version || DASH}</span>,
      },
      {
        id: 'build',
        accessorFn: (agent) => agent.build || DASH,
        header: t('about.agents.build'),
        cell: ({ row }) => <span className="font-mono text-muted-foreground">{row.original.build || DASH}</span>,
      },
      {
        id: 'target',
        accessorFn: targetText,
        header: t('about.agents.target'),
        cell: ({ row }) => (
          <span className={row.original.outdated ? 'font-mono text-foreground' : 'font-mono text-muted-foreground'}>
            {targetText(row.original)}
          </span>
        ),
      },
      {
        id: 'updateAvailable',
        accessorFn: (agent) => (agent.outdated ? t('labels.yes', { ns: 'common' }) : t('labels.no', { ns: 'common' })),
        header: t('about.agents.updateAvailable'),
        meta: { width: 'sm', defaultVisible: false } satisfies TableColumnMeta,
      },
      {
        id: 'status',
        accessorFn: (agent) => t(AGENT_STATE_KEY[agentState(agent, excluded.has(agent.envId))]),
        header: t('about.agents.status'),
        cell: ({ row }) => <AgentStatusCell agent={row.original} excluded={excluded.has(row.original.envId)} />,
      },
      {
        id: 'actions',
        header: t('labels.actions', { ns: 'common' }),
        enableSorting: false,
        meta: {
          width: 'xs',
          locked: true,
          noExport: true,
          label: t('labels.actions', { ns: 'common' }),
        } satisfies TableColumnMeta,
        cell: ({ row }) => (
          <div className="flex justify-end">
            <Button
              variant="ghost"
              size="icon-sm"
              permission={PERM.ENVIRONMENTS_MANAGE}
              aria-label={t('about.agents.updateOne', { name: row.original.name })}
              disabled={!row.original.updatable || inFlight(row.original) || sweeping}
              onClick={() => onUpdate(row.original)}
            >
              <IconCloudUpload className="size-4" aria-hidden="true" />
            </Button>
          </div>
        ),
      },
    ],
    [t, excluded, sweeping, onUpdate],
  );
}

function AgentStatusCell({ agent, excluded }: { agent: AgentRow; excluded: boolean }) {
  const { t } = useTranslation('settings');
  const state = agentState(agent, excluded);
  const progress = agent.progress;

  if (state === 'updating') {
    return (
      <span className="flex items-center gap-1.5">
        <Spinner className="h-3.5 w-3.5" />
        <span className="text-xs text-foreground">
          {progress?.target
            ? t('about.agents.progress.updatingTo', { target: progress.target })
            : t('about.agents.progress.updating')}
        </span>
      </span>
    );
  }

  return (
    <span className="flex flex-wrap items-center gap-1.5">
      <Badge variant={STATE_VARIANT[state]}>{t(AGENT_STATE_KEY[state])}</Badge>
      {state === 'started' && progress?.mode ? (
        <span className="text-xs text-muted-foreground">{t(`about.agents.mode.${progress.mode}`)}</span>
      ) : null}
      {state === 'failed' ? <StatusError error={progress?.error} /> : null}
    </span>
  );
}
