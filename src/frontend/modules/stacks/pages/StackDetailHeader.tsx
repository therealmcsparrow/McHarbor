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
  IconPencil,
  IconCheck,
  IconX,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
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
  className?: string;
};

function HeaderActionButton({ tooltip, onClick, icon, className = '' }: HeaderActionButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          onClick={onClick}
          className={`flex size-8 items-center justify-center rounded-lg border border-border transition-colors ${className}`}
        >
          {icon}
        </button>
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
            <button
              onClick={() => navigate('/stacks')}
              className="flex size-8 items-center justify-center rounded-lg border border-border text-muted-foreground hover:bg-muted/50 hover:text-foreground transition-colors"
            >
              <IconArrowLeft className="size-4" />
            </button>
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
              icon={
                saving ? (
                  <span className="size-3.5 animate-spin rounded-full border-2 border-primary border-t-transparent" />
                ) : (
                  <IconCheck className="size-3.5" />
                )
              }
              className="text-green-500 hover:bg-green-500/10 hover:border-green-500/30"
            />
            <HeaderActionButton
              tooltip={t('editStack.cancel')}
              onClick={() => onCancelEdit?.()}
              icon={<IconX className="size-3.5" />}
              className="text-muted-foreground hover:bg-muted/50 hover:border-border"
            />
          </>
        ) : (
          <>
            {isManaged && (
              <HeaderActionButton
                tooltip={t('editStack.editStack')}
                onClick={() => onEdit?.()}
                icon={<IconPencil className="size-3.5" />}
                className="text-blue-500 hover:bg-blue-500/10 hover:border-blue-500/30"
              />
            )}
            {isRunning ? (
              <>
                <HeaderActionButton
                  tooltip={t('actions.stop')}
                  onClick={() => onAction('stop')}
                  icon={<IconPlayerStop className="size-3.5" />}
                  className="text-yellow-500 hover:bg-yellow-500/10 hover:border-yellow-500/30"
                />
                <HeaderActionButton
                  tooltip={t('actions.restart')}
                  onClick={() => onAction('restart')}
                  icon={<IconRotate className="size-3.5" />}
                  className="text-blue-500 hover:bg-blue-500/10 hover:border-blue-500/30"
                />
              </>
            ) : (
              isManaged && (
                <HeaderActionButton
                  tooltip={t('actions.up')}
                  onClick={() => onAction('up')}
                  icon={<IconPlayerPlay className="size-3.5" />}
                  className="text-green-500 hover:bg-green-500/10 hover:border-green-500/30"
                />
              )
            )}
            <HeaderActionButton
              tooltip={t('actions.down')}
              onClick={() => onAction('down')}
              icon={<IconArrowDown className="size-3.5" />}
              className="text-orange-500 hover:bg-orange-500/10 hover:border-orange-500/30"
            />
            {!isManaged && (
              <HeaderActionButton
                tooltip={t('takeOver.adopt')}
                onClick={onTakeOver}
                icon={<IconArrowsTransferUp className="size-3.5" />}
                className="text-violet-500 hover:bg-violet-500/10 hover:border-violet-500/30"
              />
            )}
            <div className="h-5 w-px bg-border mx-0.5" />
            <HeaderActionButton
              tooltip={t('actions.remove')}
              onClick={onRemove}
              icon={<IconTrash className="size-3.5" />}
              className="text-red-500 hover:bg-red-500/10 hover:border-red-500/30"
            />
          </>
        )}
      </div>
    </div>
  );
}
