// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconTrash,
  IconFileText,
  IconPencil,
  IconArrowDown,
  IconArrowsTransferUp,
  IconCircleArrowUp,
  IconCircleCheck,
  IconAlertCircle,
} from '@tabler/icons-react';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { ActionButton } from '@resources/components/ui/ActionButton';
import { isProtectedStack } from '@core/utils/protection';
import type { StackInfo } from '@resources/hooks/useStacks';
import type { StackUpdateResult } from '../hooks/useStackUpdates';
import { PERM } from '@core/auth/permissionKeys';
import { useHostSelfKey } from '@resources/hooks/useHostSelfKey';

type StackUpdateCellProps = {
  result: StackUpdateResult;
};

export function StackUpdateCell({ result }: StackUpdateCellProps) {
  const { t } = useTranslation('stacks');
  const hasError = result.services.some((s) => s.error);

  if (result.updateAvailable) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <IconCircleArrowUp className="size-4 text-success" />
        </TooltipTrigger>
        <TooltipContent>{t('updates.updateAvailable')}</TooltipContent>
      </Tooltip>
    );
  }
  if (hasError) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>
          <IconAlertCircle className="size-4 text-warning" />
        </TooltipTrigger>
        <TooltipContent>{t('updates.checkFailed')}</TooltipContent>
      </Tooltip>
    );
  }
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <IconCircleCheck className="size-4 text-muted-foreground" />
      </TooltipTrigger>
      <TooltipContent>{t('updates.upToDate')}</TooltipContent>
    </Tooltip>
  );
}

type StackRowActionsProps = {
  stack: StackInfo;
  onEdit: (s: StackInfo) => void;
  onLogs: (s: StackInfo) => void;
  onRemove: (s: StackInfo) => void;
  onTakeOver: (s: StackInfo) => void;
  onAction: (name: string, action: string) => void;
};

export function StackRowActions({ stack: s, onEdit, onLogs, onRemove, onTakeOver, onAction }: StackRowActionsProps) {
  const { t } = useTranslation('stacks');
  const selfKey = useHostSelfKey(s.targetsSelf);
  const isRunning = s.status === 'running' || s.status === 'partial';
  const locked = isProtectedStack(s);
  return (
    <div className="flex items-center justify-end">
      {s.type === 'managed' && (
        <ActionButton
          label={t('actions.edit')}
          permission={selfKey(PERM.STACKS_MANAGE)}
          onClick={() => onEdit(s)}
          disabled={locked}
          icon={<IconPencil className="h-3.5 w-3.5 text-primary" />}
        />
      )}
      {s.type === 'discovered' && (
        <ActionButton
          label={t('takeOver.adopt')}
          permission={selfKey(PERM.STACKS_MANAGE)}
          onClick={() => onTakeOver(s)}
          disabled={locked}
          icon={<IconArrowsTransferUp className="h-3.5 w-3.5 text-chart-3" />}
        />
      )}
      <ActionButton
        label={t('actions.logs')}
        onClick={() => onLogs(s)}
        icon={<IconFileText className="h-3.5 w-3.5 text-chart-1" />}
      />
      {isRunning ? (
        <>
          <ActionButton
            label={t('actions.restart')}
            permission={selfKey(PERM.STACKS_MANAGE)}
            onClick={() => onAction(s.name, 'restart')}
            disabled={locked}
            icon={<IconRotate className="h-3.5 w-3.5 text-info" />}
          />
          <ActionButton
            label={t('actions.stop')}
            permission={selfKey(PERM.STACKS_MANAGE)}
            onClick={() => onAction(s.name, 'stop')}
            disabled={locked}
            icon={<IconPlayerStop className="h-3.5 w-3.5 text-warning" />}
          />
        </>
      ) : (
        s.type === 'managed' && (
          <ActionButton
            label={t('actions.up')}
            permission={selfKey(PERM.STACKS_MANAGE)}
            onClick={() => onAction(s.name, 'up')}
            disabled={locked}
            icon={<IconPlayerPlay className="h-3.5 w-3.5 text-success" />}
          />
        )
      )}
      <ActionButton
        label={t('actions.down')}
        permission={selfKey(PERM.STACKS_MANAGE)}
        onClick={() => onAction(s.name, 'down')}
        disabled={locked}
        icon={<IconArrowDown className="h-3.5 w-3.5 text-chart-2" />}
      />
      <ActionButton
        label={t('actions.remove')}
        permission={selfKey(PERM.STACKS_DELETE)}
        onClick={() => onRemove(s)}
        disabled={locked}
        icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
      />
    </div>
  );
}
