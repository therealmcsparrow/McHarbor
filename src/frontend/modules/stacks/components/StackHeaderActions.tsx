// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconArrowDown,
  IconTrash,
  IconArrowsTransferUp,
  IconLink,
  IconPencil,
} from '@tabler/icons-react';
import { HeaderActionButton } from '@resources/components/ui/HeaderActionButton';
import { PERM } from '@core/auth/permissionKeys';
import { useHostSelfKey } from '@resources/hooks/useHostSelfKey';

type StackHeaderActionsProps = {
  isManaged: boolean;
  isRunning: boolean;
  locked: boolean;
  onAction: (action: string) => void;
  onRemove: () => void;
  onTakeOver: () => void;
  onLinkContainer: () => void;
  onEdit?: () => void;
  /** The stack runs McHarbor itself, so its actions also need host_self.manage. */
  targetsSelf?: boolean;
};

export function StackHeaderActions({
  isManaged,
  isRunning,
  locked,
  onAction,
  onRemove,
  onTakeOver,
  onLinkContainer,
  onEdit,
  targetsSelf = false,
}: StackHeaderActionsProps) {
  const { t } = useTranslation('stacks');
  const selfKey = useHostSelfKey(targetsSelf);

  return (
    <>
      {isManaged && (
        <HeaderActionButton
          tooltip={t('editStack.editStack')}
          permission={selfKey(PERM.STACKS_MANAGE)}
          onClick={() => onEdit?.()}
          disabled={locked}
          icon={<IconPencil className="size-3.5" />}
        />
      )}
      {isRunning ? (
        <>
          <HeaderActionButton
            tooltip={t('actions.stop')}
            permission={selfKey(PERM.STACKS_MANAGE)}
            onClick={() => onAction('stop')}
            disabled={locked}
            icon={<IconPlayerStop className="size-3.5" />}
          />
          <HeaderActionButton
            tooltip={t('actions.restart')}
            permission={selfKey(PERM.STACKS_MANAGE)}
            onClick={() => onAction('restart')}
            disabled={locked}
            icon={<IconRotate className="size-3.5" />}
          />
        </>
      ) : (
        isManaged && (
          <HeaderActionButton
            tooltip={t('actions.up')}
            permission={selfKey(PERM.STACKS_MANAGE)}
            onClick={() => onAction('up')}
            variant="default"
            disabled={locked}
            icon={<IconPlayerPlay className="size-3.5" />}
          />
        )
      )}
      <HeaderActionButton
        tooltip={t('actions.down')}
        permission={selfKey(PERM.STACKS_MANAGE)}
        onClick={() => onAction('down')}
        disabled={locked}
        icon={<IconArrowDown className="size-3.5" />}
      />
      {!isManaged && (
        <HeaderActionButton
          tooltip={t('takeOver.adopt')}
          permission={selfKey(PERM.STACKS_MANAGE)}
          onClick={onTakeOver}
          variant="secondary"
          disabled={locked}
          icon={<IconArrowsTransferUp className="size-3.5" />}
        />
      )}
      <HeaderActionButton
        tooltip={t('link.linkContainer')}
        permission={selfKey(PERM.STACKS_MANAGE)}
        onClick={onLinkContainer}
        variant="secondary"
        disabled={locked}
        icon={<IconLink className="size-3.5" />}
      />
      <div className="mx-0.5 h-5 w-px bg-border" />
      <HeaderActionButton
        tooltip={t('actions.remove')}
        permission={selfKey(PERM.STACKS_DELETE)}
        onClick={onRemove}
        variant="destructive"
        disabled={locked}
        icon={<IconTrash className="size-3.5" />}
      />
    </>
  );
}
