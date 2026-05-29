// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import {
  IconPlayerPlay,
  IconPlayerStop,
  IconRotate,
  IconPlayerPause,
  IconSkull,
  IconTrash,
  IconArrowLeft,
  IconExternalLink,
  IconArrowsTransferUp,
  IconLink,
  IconPencil,
  IconCheck,
  IconX,
  IconLock,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { truncateId } from '@resources/utils/format';
import type { ContainerInspect } from '@core/types/docker';
import { isProtectedContainer } from '@core/utils/protection';

const STATE_VARIANTS: Record<string, 'success' | 'destructive' | 'warning' | 'secondary'> = {
  running: 'success',
  exited: 'destructive',
  paused: 'warning',
  restarting: 'warning',
  created: 'secondary',
  dead: 'destructive',
};

const WEB_PORTS = new Set([80, 443, 8080, 8443, 3000, 3001, 4200, 5000, 5173, 8000, 8888, 8880, 8181, 9000, 9090, 9443]);

export function getInspectWebUrl(ports: ContainerInspect['NetworkSettings']['Ports']): string | null {
  if (!ports) return null;

  let bestPort: { hostPort: string; containerPort: number } | null = null;

  for (const [containerPortKey, bindings] of Object.entries(ports)) {
    if (!bindings || bindings.length === 0) continue;
    if (!containerPortKey.endsWith('/tcp')) continue;

    const containerPort = parseInt(containerPortKey);
    if (isNaN(containerPort)) continue;

    const binding = bindings[0];
    if (!binding?.HostPort) continue;

    if (!bestPort || WEB_PORTS.has(containerPort)) {
      bestPort = { hostPort: binding.HostPort, containerPort };
      if (WEB_PORTS.has(containerPort)) break;
    }
  }

  if (!bestPort) return null;

  const scheme = bestPort.containerPort === 443 || bestPort.containerPort === 8443 || bestPort.containerPort === 9443 ? 'https' : 'http';
  return `${scheme}://${window.location.hostname}:${bestPort.hostPort}`;
}

type HeaderActionButtonProps = {
  tooltip: string;
  onClick: () => void;
  icon: React.ReactNode;
  variant?: 'default' | 'destructive' | 'outline' | 'secondary' | 'ghost' | 'link';
  className?: string;
  disabled?: boolean;
};

function HeaderActionButton({
  tooltip,
  onClick,
  icon,
  variant = 'outline',
  className = '',
  disabled = false,
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
          disabled={disabled}
        >
          {icon}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

type ContainerDetailHeaderProps = {
  container: ContainerInspect;
  isRunning: boolean;
  name: string;
  webUrl: string | null;
  editing: boolean;
  onEdit: () => void;
  onSave: () => void;
  onCancelEdit: () => void;
  saving?: boolean;
  stackName?: string | null;
  onAction: (action: string) => void;
  onKill: () => void;
  onRemove: () => void;
  onTakeOver: () => void;
  onRelink: () => void;
};

export function ContainerDetailHeader({
  container,
  isRunning,
  name,
  webUrl,
  editing,
  onEdit,
  onSave,
  onCancelEdit,
  saving,
  stackName,
  onAction,
  onKill,
  onRemove,
  onTakeOver,
  onRelink,
}: ContainerDetailHeaderProps) {
  const navigate = useNavigate();
  const { t } = useTranslation('containers');
  const { t: tc } = useTranslation('common');
  const state = container.State?.Status ?? 'unknown';
  const resolvedStackName = stackName ?? container.Config?.Labels?.['com.docker.compose.project'] ?? null;
  const locked = isProtectedContainer(container);

  return (
    <div className="flex flex-1 items-center justify-between">
      {/* Left: back + name + id + state */}
      <div className="flex items-center gap-3">
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              onClick={() => navigate('/containers')}
              variant="outline"
              size="icon-sm"
              aria-label={t('actions.backToContainers')}
              className="text-muted-foreground hover:text-foreground"
            >
              <IconArrowLeft className="size-4" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>{t('actions.backToContainers')}</TooltipContent>
        </Tooltip>
        <div className="h-5 w-px bg-border" />
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-sm font-semibold text-foreground">{name}</h1>
            <Badge variant={STATE_VARIANTS[state] ?? 'secondary'} className="text-[10px] px-1.5 py-0">{state}</Badge>
            {locked && (
              <Badge variant="secondary" className="gap-1 text-[10px] px-1.5 py-0">
                <IconLock className="size-3" />
                {tc('actions.locked')}
              </Badge>
            )}
            {resolvedStackName ? (
              <Badge
                variant="default"
                className="text-[10px] px-1.5 py-0 cursor-pointer"
                onClick={() => navigate(`/stacks/${resolvedStackName}`)}
              >
                {resolvedStackName}
              </Badge>
            ) : (
              <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
                {t('badges.standalone')}
              </Badge>
            )}
          </div>
          <p className="font-mono text-xs text-muted-foreground">{truncateId(container.Id)}</p>
        </div>
      </div>

      {/* Right: action buttons */}
      <div className="flex items-center gap-1.5">
        {editing ? (
          <>
            <HeaderActionButton
              tooltip={t('edit.saveChanges')}
              onClick={onSave}
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
              tooltip={t('edit.cancelChanges')}
              onClick={onCancelEdit}
              icon={<IconX className="size-3.5" />}
              className="text-muted-foreground"
            />
          </>
        ) : (
          <>
            <HeaderActionButton
              tooltip={t('actions.edit')}
              onClick={onEdit}
              disabled={locked}
              icon={<IconPencil className="size-3.5" />}
            />
            {isRunning ? (
              <HeaderActionButton
                tooltip={t('actions.stop')}
                onClick={() => onAction('stop')}
                disabled={locked}
                icon={<IconPlayerStop className="size-3.5" />}
              />
            ) : (
              <HeaderActionButton
                tooltip={t('actions.start')}
                onClick={() => onAction('start')}
                variant="default"
                disabled={locked}
                icon={<IconPlayerPlay className="size-3.5" />}
              />
            )}
            <HeaderActionButton
              tooltip={t('actions.restart')}
              onClick={() => onAction('restart')}
              disabled={locked}
              icon={<IconRotate className="size-3.5" />}
            />
            <HeaderActionButton
              tooltip={t('actions.pause')}
              onClick={() => onAction('pause')}
              disabled={locked}
              icon={<IconPlayerPause className="size-3.5" />}
            />
            {webUrl && (
              <HeaderActionButton
                tooltip={t('actions.openWebsite')}
                onClick={() => window.open(webUrl, '_blank', 'noopener,noreferrer')}
                variant="secondary"
                icon={<IconExternalLink className="size-3.5" />}
              />
            )}
            <HeaderActionButton
              tooltip={t('actions.relinkStack')}
              onClick={onRelink}
              variant="secondary"
              disabled={locked}
              icon={<IconLink className="size-3.5" />}
            />
            {!container.Config?.Labels?.['com.docker.compose.project'] && (
              <HeaderActionButton
                tooltip={t('actions.takeOver')}
                onClick={onTakeOver}
                variant="secondary"
                disabled={locked}
                icon={<IconArrowsTransferUp className="size-3.5" />}
              />
            )}
            <div className="h-5 w-px bg-border mx-0.5" />
            <HeaderActionButton
              tooltip={t('actions.kill')}
              onClick={onKill}
              variant="destructive"
              disabled={locked}
              icon={<IconSkull className="size-3.5" />}
            />
            <HeaderActionButton
              tooltip={t('actions.remove')}
              onClick={onRemove}
              variant="destructive"
              disabled={locked}
              icon={<IconTrash className="size-3.5" />}
            />
          </>
        )}
      </div>
    </div>
  );
}
