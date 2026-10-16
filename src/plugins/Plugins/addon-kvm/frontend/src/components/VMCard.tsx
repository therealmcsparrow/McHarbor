// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { IconCpu, IconCpu2, IconServer } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Card, CardContent, CardFooter } from '@resources/components/ui/Card';
import { PlainButton } from '@resources/components/ui/PlainButton';
import { cn } from '@resources/utils/cn';
import { VMStateBadge } from './VMStateBadge';
import { GuestAddress, VMActions, type VMAction } from './VMActions';
import { formatRelative } from '../format';
import type { VM } from '../types';

interface VMCardProps {
  vm: VM;
  hostName: string;
  busy: boolean;
  diagnosePending: boolean;
  onOpen: (id: string) => void;
  onAction: (action: VMAction) => void;
  onDelete: (vm: VM) => void;
  onDiagnose: (vm: VM) => void;
}

function Tile({ label, value, icon }: { label: string; value: ReactNode; icon?: ReactNode }) {
  return (
    <div className="rounded-lg border border-border/70 bg-background/40 px-3 py-2">
      <div className="flex items-center gap-1.5 text-muted-foreground">
        {icon}
        <span className="truncate text-[11px] font-medium">{label}</span>
      </div>
      <p className="mt-1 truncate text-sm font-semibold text-foreground tabular-nums">{value}</p>
    </div>
  );
}

/**
 * One guest, in the same shape as an environment card.
 *
 * A guest whose host is unreachable is dimmed rather than hidden, and its
 * actions are disabled — the same treatment the table row gives it.
 */
export function VMCard({ vm, hostName, busy, diagnosePending, onOpen, onAction, onDelete, onDiagnose }: VMCardProps) {
  const { t } = useTranslation('vm');

  return (
    <Card
      className={cn(
        'min-h-[300px] transition-colors hover:border-primary/40',
        vm.hostAvailable ? undefined : 'opacity-60',
      )}
    >
      <CardContent className="flex flex-1 flex-col gap-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0 space-y-1">
            <div className="flex min-w-0 items-center gap-2">
              <PlainButton
                onClick={() => onOpen(vm.id)}
                className="truncate text-base font-semibold text-foreground hover:text-primary hover:underline"
              >
                {vm.name}
              </PlainButton>
              {vm.isTemplate && (
                <Badge variant="secondary" className="shrink-0 px-2 py-1 text-[10px]">
                  {t('nav.templates')}
                </Badge>
              )}
            </div>
            <p className="flex items-center gap-1.5 truncate text-xs text-muted-foreground">
              <IconServer className="size-3.5 shrink-0" aria-hidden="true" />
              {hostName}
            </p>
          </div>
          <VMStateBadge vm={vm} />
        </div>

        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline" className="uppercase">
            {vm.firmware}
          </Badge>
          {vm.guestOsVariant && <span className="truncate text-xs text-muted-foreground">{vm.guestOsVariant}</span>}
        </div>

        <div className="grid grid-cols-2 gap-2">
          <Tile
            label={t('list.columns.resources')}
            value={`${vm.vcpus} vCPU`}
            icon={<IconCpu className="size-3.5" aria-hidden="true" />}
          />
          <Tile
            label={t('list.columns.resources')}
            value={`${Math.round(vm.memoryMb / 1024)} GB`}
            icon={<IconCpu2 className="size-3.5" aria-hidden="true" />}
          />
        </div>

        {vm.lastError && <p className="text-xs break-words text-destructive">{vm.lastError}</p>}

        <div className="mt-auto space-y-2">
          <div className="flex items-center justify-between gap-3 rounded-lg border border-border/70 bg-background/40 px-3 py-2">
            <span className="text-xs font-medium text-muted-foreground">{t('list.columns.address')}</span>
            <GuestAddress vm={vm} />
          </div>
          <div className="flex items-center justify-between gap-3 rounded-lg border border-border/70 bg-background/40 px-3 py-2">
            <span className="text-xs font-medium text-muted-foreground">{t('list.columns.uptime')}</span>
            <span className="text-xs text-muted-foreground">
              {vm.state === 'running' && vm.lastStartedAt ? formatRelative(vm.lastStartedAt) : '—'}
            </span>
          </div>
        </div>
      </CardContent>

      <CardFooter className="gap-1 px-3 py-2">
        <VMActions
          vm={vm}
          busy={busy}
          diagnosePending={diagnosePending}
          onOpen={onOpen}
          onAction={onAction}
          onDelete={onDelete}
          onDiagnose={onDiagnose}
        />
      </CardFooter>
    </Card>
  );
}
