// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconTrash,
  IconFileText,
  IconPencil,
  IconArrowDown,
  IconEye,
  IconArrowsTransferUp,
} from '@tabler/icons-react';
import type { StackInfo } from '@resources/hooks/useStacks';
import { CardFooter } from '@resources/components/ui/Card';
import { ActionButton } from '@resources/components/ui/ActionButton';
import { PERM } from '@core/auth/permissionKeys';
import { useHostSelfKey } from '@resources/hooks/useHostSelfKey';

type StackCardActionsProps = {
  stack: StackInfo;
  locked: boolean;
  isRunning: boolean;
  onAction: (name: string, action: string) => void;
  onEdit: (s: StackInfo) => void;
  onLogs: (s: StackInfo) => void;
  onRemove: (s: StackInfo) => void;
  onTakeOver?: (s: StackInfo) => void;
};

export function StackCardActions({
  stack: s,
  locked,
  isRunning,
  onAction,
  onEdit,
  onLogs,
  onRemove,
  onTakeOver,
}: StackCardActionsProps) {
  const { t } = useTranslation('stacks');
  const selfKey = useHostSelfKey(s.targetsSelf);
  const navigate = useNavigate();

  return (
    <CardFooter className="gap-1 px-3 py-2" onClick={(e) => e.stopPropagation()}>
      <ActionButton
        label={t('actions.view')}
        onClick={() => navigate(`/stacks/${s.name}`)}
        icon={<IconEye className="h-3.5 w-3.5 text-foreground" />}
      />
      {s.type === 'managed' && (
        <ActionButton
          label={t('actions.edit')}
          permission={selfKey(PERM.STACKS_MANAGE)}
          onClick={() => onEdit(s)}
          disabled={locked}
          icon={<IconPencil className="h-3.5 w-3.5 text-primary" />}
        />
      )}
      {s.type === 'discovered' && onTakeOver && (
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
      <div className="flex-1" />
      <ActionButton
        label={t('actions.remove')}
        permission={selfKey(PERM.STACKS_DELETE)}
        onClick={() => onRemove(s)}
        disabled={locked}
        icon={<IconTrash className="h-3.5 w-3.5 text-destructive" />}
      />
    </CardFooter>
  );
}
