// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconListDetails } from '@tabler/icons-react';
import type { ContainerMount } from '@core/types/docker';
import { Button } from '@resources/components/ui/Button';

type MountsMobileListProps = {
  mounts: ContainerMount[];
  onSelect: (mount: ContainerMount) => void;
};

export function MountsMobileList({ mounts, onSelect }: MountsMobileListProps) {
  const { t } = useTranslation('containers');
  return (
    <div className="space-y-3 md:hidden">
      {mounts.map((mount) => (
        <div
          key={`${mount.Type}:${mount.Source ?? ''}:${mount.Destination}`}
          className="rounded-md border border-border bg-muted/30 p-3"
        >
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0 space-y-2">
              <p className="text-xs font-medium text-muted-foreground">{mount.Type}</p>
              <div>
                <p className="text-xs text-muted-foreground">{t('mounts.destination')}</p>
                <p className="font-mono text-xs break-all text-foreground">{mount.Destination}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">{t('mounts.source')}</p>
                <p className="font-mono text-xs break-all text-foreground">{mount.Source || '-'}</p>
              </div>
            </div>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => onSelect(mount)}
              aria-label={t('mounts.viewDetails', {
                destination: mount.Destination,
              })}
              aria-haspopup="dialog"
              className="shrink-0"
            >
              <IconListDetails className="size-4" />
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}
