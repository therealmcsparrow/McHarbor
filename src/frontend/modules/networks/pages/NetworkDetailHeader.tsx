// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconArrowLeft, IconTrash, IconPencil, IconCheck, IconX, IconLoader2 } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { truncateId } from '@resources/utils/format';

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
        <Button
          variant="outline"
          size="icon-sm"
          aria-label={tooltip}
          onClick={onClick}
          className={className}
        >
          {icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

type NetworkDetailHeaderProps = {
  name: string;
  id: string;
  driver: string;
  scope: string;
  editing: boolean;
  saving?: boolean;
  onEdit: () => void;
  onSave: () => void;
  onCancelEdit: () => void;
  onRemove: () => void;
};

export function NetworkDetailHeader({
  name,
  id,
  driver,
  scope,
  editing,
  saving,
  onEdit,
  onSave,
  onCancelEdit,
  onRemove,
}: NetworkDetailHeaderProps) {
  const navigate = useNavigate();
  const { t } = useTranslation('networks');

  return (
    <div className="flex flex-1 items-center justify-between">
      {/* Left: back + name + badges */}
      <div className="flex items-center gap-3">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="outline"
              size="icon-sm"
              aria-label={t('detail.backToNetworks')}
              onClick={() => navigate('/networks')}
            >
              <IconArrowLeft className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('detail.backToNetworks')}</TooltipContent>
        </Tooltip>
        <div className="h-5 w-px bg-border" />
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-sm font-semibold text-foreground">{name}</h1>
            <Badge variant="default" className="text-[10px] px-1.5 py-0">
              {driver}
            </Badge>
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
              {scope}
            </Badge>
          </div>
          <p className="font-mono text-xs text-muted-foreground">{truncateId(id)}</p>
        </div>
      </div>

      {/* Right: action buttons */}
      <div className="flex items-center gap-1.5">
        {editing ? (
          <>
            <HeaderActionButton
              tooltip={t('edit.saveChanges')}
              onClick={onSave}
              icon={
                saving ? (
                  <IconLoader2 className="size-3.5 animate-spin" />
                ) : (
                  <IconCheck className="size-3.5" />
                )
              }
              className="text-green-500 hover:bg-green-500/10 hover:border-green-500/30"
            />
            <HeaderActionButton
              tooltip={t('edit.cancelChanges')}
              onClick={onCancelEdit}
              icon={<IconX className="size-3.5" />}
              className="text-muted-foreground hover:bg-muted/50 hover:border-border"
            />
          </>
        ) : (
          <>
            <HeaderActionButton
              tooltip={t('actions.edit')}
              onClick={onEdit}
              icon={<IconPencil className="size-3.5" />}
              className="text-blue-500 hover:bg-blue-500/10 hover:border-blue-500/30"
            />
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
