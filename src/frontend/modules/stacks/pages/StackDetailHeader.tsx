// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  IconArrowLeft,
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconArrowDown,
  IconTrash,
  IconArrowsTransferUp,
  IconLink,
  IconPencil,
  IconCheck,
  IconX,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';

const STATUS_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  running: 'success',
  stopped: 'destructive',
  partial: 'warning',
};

type HeaderActionButtonProps = {
  tooltip: string;
  onClick: () => void;
  icon: React.ReactNode;
  variant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link';
  className?: string;
};

function HeaderActionButton({
  tooltip,
  onClick,
  icon,
  variant = 'outline',
  className = '',
}: HeaderActionButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          onClick={onClick}
          variant={variant}
          size="icon-sm"
          aria-label={tooltip}
          className={className}
        >
          {icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

type StackDetailHeaderProps = {
  stackName: string;
  status: string;
  isManaged: boolean;
  isRunning: boolean;
  editing?: boolean;
  onAction: (action: string) => void;
  onRemove: () => void;
  onTakeOver: () => void;
  onLinkContainer: () => void;
  onEdit?: () => void;
  onSave?: () => void;
  onCancelEdit?: () => void;
  saving?: boolean;
};

export function StackDetailHeader({
  stackName,
  status,
  isManaged,
  isRunning,
  editing,
  onAction,
  onRemove,
  onTakeOver,
  onLinkContainer,
  onEdit,
  onSave,
  onCancelEdit,
  saving,
}: StackDetailHeaderProps) {
  const navigate = useNavigate();
  const { t } = useTranslation('stacks');

  return (
    <div className="flex flex-1 items-center justify-between">
      {/* Left: back + name + badges */}
      <div className="flex items-center gap-3">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              onClick={() => navigate('/stacks')}
              variant="outline"
              size="icon-sm"
              aria-label={t('detail.backToStacks')}
              className="text-muted-foreground hover:text-foreground"
            >
              <IconArrowLeft className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('detail.backToStacks')}</TooltipContent>
        </Tooltip>
        <div className="h-5 w-px bg-border" />
        <div className="flex items-center gap-2">
          <h1 className="text-sm font-semibold text-foreground">{stackName}</h1>
          <Badge
            variant={isManaged ? 'default' : 'outline'}
            className="text-[9px] px-1.5 py-0"
          >
            {isManaged ? t('badges.managed') : t('badges.discovered')}
          </Badge>
          <Badge
            variant={STATUS_VARIANTS[status] ?? 'secondary'}
            className="text-[10px] px-1.5 py-0"
          >
            {status}
          </Badge>
        </div>
      </div>

      {/* Right: action buttons */}
      <div className="flex items-center gap-1.5">
        {editing ? (
          <>
            <HeaderActionButton
              tooltip={t('editStack.saveStack')}
              onClick={() => onSave?.()}
              variant="default"
              icon={
                saving ? (
                  <span className="size-3.5 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                ) : (
                  <IconCheck className="size-3.5" />
                )
              }
            />
            <HeaderActionButton
              tooltip={t('editStack.cancel')}
              onClick={() => onCancelEdit?.()}
              icon={<IconX className="size-3.5" />}
              className="text-muted-foreground"
            />
          </>
        ) : (
          <>
            {isManaged && (
              <HeaderActionButton
                tooltip={t('editStack.editStack')}
                onClick={() => onEdit?.()}
                icon={<IconPencil className="size-3.5" />}
              />
            )}
            {isRunning ? (
              <>
                <HeaderActionButton
                  tooltip={t('actions.stop')}
                  onClick={() => onAction('stop')}
                  icon={<IconPlayerStop className="size-3.5" />}
                />
                <HeaderActionButton
                  tooltip={t('actions.restart')}
                  onClick={() => onAction('restart')}
                  icon={<IconRotate className="size-3.5" />}
                />
              </>
            ) : (
              isManaged && (
                <HeaderActionButton
                  tooltip={t('actions.up')}
                  onClick={() => onAction('up')}
                  variant="default"
                  icon={<IconPlayerPlay className="size-3.5" />}
                />
              )
            )}
            <HeaderActionButton
              tooltip={t('actions.down')}
              onClick={() => onAction('down')}
              icon={<IconArrowDown className="size-3.5" />}
            />
            {!isManaged && (
              <HeaderActionButton
                tooltip={t('takeOver.adopt')}
                onClick={onTakeOver}
                variant="secondary"
                icon={<IconArrowsTransferUp className="size-3.5" />}
              />
            )}
            <HeaderActionButton
              tooltip={t('link.linkContainer')}
              onClick={onLinkContainer}
              variant="secondary"
              icon={<IconLink className="size-3.5" />}
            />
            <div className="h-5 w-px bg-border mx-0.5" />
            <HeaderActionButton
              tooltip={t('actions.remove')}
              onClick={onRemove}
              variant="destructive"
              icon={<IconTrash className="size-3.5" />}
            />
          </>
        )}
      </div>
    </div>
  );
}
