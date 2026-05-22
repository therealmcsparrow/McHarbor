// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  IconChevronRight,
  IconDatabase,
  IconLink,
  IconServer,
} from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import type { ContainerMount } from '@core/types/docker';

type QuickAccessCardsProps = {
  mounts: ContainerMount[];
  currentPath: string;
  onNavigate: (path: string) => void;
};

export function QuickAccessCards({ mounts, currentPath, onNavigate }: QuickAccessCardsProps) {
  const { t } = useTranslation('containers');

  const browseableMounts = mounts.filter(
    (m) => !m.Destination.includes('.sock') && m.Type !== 'tmpfs',
  );

  return (
    <div className="space-y-2">
      <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wider">
        {t('files.quickAccess')}
      </h3>
      <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
        {/* Container filesystem card */}
        <Button
          variant="ghost"
          onClick={() => onNavigate('/')}
          className={`group flex h-auto items-start gap-3 whitespace-normal rounded-lg border p-3 text-left transition-colors ${
            currentPath === '/'
              ? 'border-primary/50 bg-primary/5'
              : 'border-border bg-card'
          } hover:border-primary/40 hover:bg-muted/50 cursor-pointer`}
        >
          <div className="mt-0.5 shrink-0">
            <IconServer className="h-4 w-4 text-primary" />
          </div>
          <div className="min-w-0 flex-1 space-y-1">
            <div className="flex items-center gap-1.5">
              <span className="truncate font-mono text-xs font-medium text-foreground">
                /
              </span>
              <Badge variant="outline" className="shrink-0 px-1 py-0 text-[10px]">
                {t('files.filesystem')}
              </Badge>
            </div>
            <p className="truncate font-mono text-[11px] text-muted-foreground">
              {t('files.fullFilesystem')}
            </p>
          </div>
          <IconChevronRight className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
        </Button>

        {/* Mount cards */}
        {mounts.map((mount) => {
          const isBrowseable = browseableMounts.includes(mount);
          const isActive = currentPath.replace(/\/$/, '').startsWith(mount.Destination.replace(/\/$/, ''));
          return (
            <Button
              variant="ghost"
              key={mount.Destination}
              onClick={() => isBrowseable && onNavigate(mount.Destination)}
              disabled={!isBrowseable}
              className={`group flex h-auto items-start gap-3 whitespace-normal rounded-lg border p-3 text-left transition-colors ${
                isActive
                  ? 'border-blue-400/50 bg-blue-400/5'
                  : 'border-border bg-card'
              } ${
                isBrowseable
                  ? 'hover:border-blue-400/40 hover:bg-muted/50 cursor-pointer'
                  : 'opacity-60 cursor-default'
              }`}
            >
              <div className="mt-0.5 shrink-0">
                {mount.Type === 'volume' ? (
                  <IconDatabase className="h-4 w-4 text-blue-400" />
                ) : (
                  <IconLink className="h-4 w-4 text-blue-400" />
                )}
              </div>
              <div className="min-w-0 flex-1 space-y-1">
                <div className="flex items-center gap-1.5">
                  <span className="truncate font-mono text-xs font-medium text-foreground">
                    {mount.Destination}
                  </span>
                  <Badge variant="outline" className="shrink-0 px-1 py-0 text-[10px]">
                    {mount.Type}
                  </Badge>
                  {!mount.RW && (
                    <Badge variant="secondary" className="shrink-0 px-1 py-0 text-[10px]">
                      ro
                    </Badge>
                  )}
                  {!isBrowseable && (
                    <Badge variant="secondary" className="shrink-0 px-1 py-0 text-[10px]">
                      {t('files.socket')}
                    </Badge>
                  )}
                </div>
                <p className="truncate font-mono text-[11px] text-muted-foreground">
                  {mount.Type === 'volume' ? mount.Name ?? mount.Source : mount.Source}
                </p>
              </div>
              {isBrowseable && (
                <IconChevronRight className="mt-0.5 h-3.5 w-3.5 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
              )}
            </Button>
          );
        })}
      </div>
    </div>
  );
}
