// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { ComponentType } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconPencil,
  IconPlayerPlay,
  IconPlayerStop,
  IconRefresh,
  IconToggleLeft,
  IconToggleRight,
} from '@tabler/icons-react';
import { hostPermission, type HostTarget } from '@resources/hooks/useHostUpdates';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { TooltipFocusTarget } from '@resources/components/ui/TooltipFocusTarget';
import {
  isHostServiceEnabled,
  isHostServiceRunning,
  type HostService,
  type HostServiceAction,
} from '@resources/hooks/useHostServices';

type HostServiceActionsCellProps = {
  host: HostTarget;
  unit: HostService;
  busy: boolean;
  onAction: (unit: HostService, action: HostServiceAction) => void;
  onEdit: (unit: HostService) => void;
};

type RowButtonProps = {
  label: string;
  icon: ComponentType<{ className?: string }>;
  onClick: () => void;
  disabled: boolean;
  blockedReason?: string;
  permission: string;
};

function RowButton({ label, icon: Icon, onClick, disabled, blockedReason, permission }: RowButtonProps) {
  if (blockedReason) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <TooltipFocusTarget>
            <Button variant="ghost" size="icon-sm" aria-label={label} disabled aria-disabled="true">
              <Icon className="size-4" />
            </Button>
          </TooltipFocusTarget>
        </TooltipTrigger>
        <TooltipContent>{blockedReason}</TooltipContent>
      </Tooltip>
    );
  }
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      title={label}
      permission={permission}
      disabled={disabled}
      onClick={onClick}
    >
      <Icon className="size-4" />
    </Button>
  );
}

export function HostServiceActionsCell({ host, unit, busy, onAction, onEdit }: HostServiceActionsCellProps) {
  const { t } = useTranslation('environments');
  const running = isHostServiceRunning(unit);
  const enabled = isHostServiceEnabled(unit);
  const lifecycleOff = busy || Boolean(unit.template);
  const protectedReason = unit.protected ? t(`hostServices.protected.${unit.protected}`) : undefined;
  const itemName = unit.name;
  const manage = hostPermission(host, 'servicesManage');

  return (
    <div className="flex items-center justify-end gap-0.5">
      <RowButton
        permission={manage}
        label={t('hostServices.row.start', { unit: itemName })}
        icon={IconPlayerPlay}
        disabled={lifecycleOff || running}
        onClick={() => onAction(unit, 'start')}
      />
      <RowButton
        permission={manage}
        label={t('hostServices.row.stop', { unit: itemName })}
        icon={IconPlayerStop}
        disabled={lifecycleOff || !running}
        blockedReason={protectedReason}
        onClick={() => onAction(unit, 'stop')}
      />
      <RowButton
        permission={manage}
        label={t('hostServices.row.restart', { unit: itemName })}
        icon={IconRefresh}
        disabled={lifecycleOff}
        onClick={() => onAction(unit, 'restart')}
      />
      <RowButton
        permission={manage}
        label={t('hostServices.row.enable', { unit: itemName })}
        icon={IconToggleRight}
        disabled={lifecycleOff || enabled || unit.unitFileState === 'static' || unit.unitFileState === 'masked'}
        onClick={() => onAction(unit, 'enable')}
      />
      <RowButton
        permission={manage}
        label={t('hostServices.row.disable', { unit: itemName })}
        icon={IconToggleLeft}
        disabled={lifecycleOff || !enabled}
        blockedReason={protectedReason}
        onClick={() => onAction(unit, 'disable')}
      />
      <RowButton
        label={t('hostServices.row.edit', { unit: itemName })}
        icon={IconPencil}
        permission={hostPermission(host, 'servicesView')}
        disabled={busy}
        onClick={() => onEdit(unit)}
      />
    </div>
  );
}
