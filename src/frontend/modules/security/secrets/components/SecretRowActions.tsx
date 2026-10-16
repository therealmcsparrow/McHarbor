// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconEye, IconListCheck, IconPencil, IconRefresh, IconTrash } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipContent, TooltipTrigger } from '@resources/components/ui/Tooltip';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { SecretActionTooltip } from './SecretActionTooltip';
import type { Secret } from '../types';

export type SecretRowHandlers = {
  onRotate: (secret: Secret) => void;
  onReveal: (secret: Secret) => void;
  onDelete: (secret: Secret) => void;
  onReviewImpact: (secret: Secret) => void;
  onEdit: (secret: Secret) => void;
};

type SecretRowActionsProps = SecretRowHandlers & { secret: Secret };

export function SecretRowActions({
  secret,
  onRotate,
  onReveal,
  onDelete,
  onReviewImpact,
  onEdit,
}: SecretRowActionsProps) {
  const { t } = useTranslation('secrets');
  const canReveal = usePermission(PERM.SECRETS_REVEAL);
  const canManage = usePermission(PERM.SECRETS_MANAGE);
  const canDelete = usePermission(PERM.SECRETS_DELETE);

  return (
    <div className="flex items-center justify-end gap-1">
      {/*
        Disabled with a tooltip, never hidden. A user who cannot reveal
        should learn that reveal exists and which permission unlocks it —
        and the action column keeps the same width across roles.
      */}
      <Tooltip>
        <TooltipTrigger asChild>
          <span>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label={t('actions.reveal')}
              disabled={!canReveal || secret.system}
              onClick={() => onReveal(secret)}
            >
              <IconEye className="size-4" />
            </Button>
          </span>
        </TooltipTrigger>
        <TooltipContent>
          {secret.system
            ? t('actions.revealSystemDenied')
            : canReveal
              ? t('actions.reveal')
              : t('actions.revealDenied')}
        </TooltipContent>
      </Tooltip>

      <SecretActionTooltip label={t('actions.reviewImpact')} allowed>
        <Button
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.reviewImpact')}
          onClick={() => onReviewImpact(secret)}
        >
          <IconListCheck className="size-4" />
        </Button>
      </SecretActionTooltip>

      <SecretActionTooltip
        label={secret.system ? t('actions.editSystemDenied') : t('actions.edit')}
        allowed={canManage}
      >
        <Button
          permission={PERM.SECRETS_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.edit')}
          disabled={secret.system}
          onClick={() => onEdit(secret)}
        >
          <IconPencil className="size-4" />
        </Button>
      </SecretActionTooltip>

      <SecretActionTooltip
        label={secret.system ? t('actions.rotateSystemDenied') : t('actions.rotate')}
        allowed={canManage}
      >
        <Button
          permission={PERM.SECRETS_MANAGE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.rotate')}
          disabled={secret.system}
          onClick={() => onRotate(secret)}
        >
          <IconRefresh className="size-4" />
        </Button>
      </SecretActionTooltip>

      <SecretActionTooltip
        label={secret.system ? t('actions.deleteSystemDenied') : t('actions.delete', { ns: 'common' })}
        allowed={canDelete}
      >
        <Button
          permission={PERM.SECRETS_DELETE}
          variant="ghost"
          size="icon-sm"
          aria-label={t('actions.delete', { ns: 'common' })}
          disabled={secret.system}
          onClick={() => onDelete(secret)}
        >
          <IconTrash className="size-4 text-destructive" />
        </Button>
      </SecretActionTooltip>
    </div>
  );
}
