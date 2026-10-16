// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// The containers and configuration faces of the volume detail tab panel.
// Split out of VolumeDetailTabContent.tsx unchanged; the overview face and
// the dispatcher stay there.

import { IconDatabase } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { StatusIndicator } from '@resources/components/ui/StatusIndicator';
import { truncateId } from '@resources/utils/format';
import type { VolumeInfo } from '@core/types/docker';
import type { VolumeMount } from './VolumeDetailTabContent';

export function ContainersPanel({
  mounts,
  t,
  onContainerOpen,
}: {
  mounts: VolumeMount[];
  t: (key: string) => string;
  onContainerOpen: (containerID: string) => void;
}) {
  return (
    <div className="mc-glass rounded-lg p-6">
      <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
        {t('detail.mountedByContainers')} ({mounts.length})
      </h3>

      {mounts.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-8 text-center">
          <IconDatabase className="size-8 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">{t('detail.noContainers')}</p>
        </div>
      ) : (
        <div data-scroll-exempt className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted">
              <tr className="border-b border-border">
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.containerName')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.containerId')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.mountPath')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.access')}</th>
              </tr>
            </thead>
            <tbody>
              {mounts.map((mount) => (
                <tr
                  key={`${mount.containerId}-${mount.destination}`}
                  className="border-b border-border transition-colors last:border-0 hover:bg-muted/50"
                >
                  <td className="px-3 py-2">
                    <Button
                      variant="link"
                      className="h-auto gap-1.5 p-0 font-medium"
                      onClick={() => onContainerOpen(mount.containerId)}
                    >
                      <StatusIndicator
                        tone={mount.state === 'running' ? 'success' : 'neutral'}
                        variant="dot"
                        size="sm"
                        ariaLabel={mount.state}
                      />
                      {mount.containerName}
                    </Button>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{truncateId(mount.containerId)}</td>
                  <td className="px-3 py-2 font-mono text-xs">{mount.destination}</td>
                  <td className="px-3 py-2">
                    <Badge variant={mount.readWrite ? 'default' : 'secondary'} className="px-1.5 py-0 text-[10px]">
                      {mount.readWrite ? t('detail.readWrite') : t('detail.readOnly')}
                    </Badge>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

export function ConfigurationPanel({ volume, t }: { volume: VolumeInfo; t: (key: string) => string }) {
  const options = Object.entries(volume.Options ?? {});
  const labels = Object.entries(volume.Labels ?? {});

  if (options.length === 0 && labels.length === 0) {
    return (
      <div className="mc-glass rounded-lg p-6">
        <p className="py-8 text-center text-sm text-muted-foreground">{t('detail.noConfiguration')}</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      {options.length > 0 && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('detail.options')}
          </h3>
          {options.map(([key, value]) => (
            <InfoRow key={key} label={key} variant="spread" title={String(value)}>
              <span className="font-mono text-xs">{String(value)}</span>
            </InfoRow>
          ))}
        </div>
      )}

      {labels.length > 0 && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('detail.labels')}
          </h3>
          {labels.map(([key, value]) => (
            <InfoRow key={key} label={key} variant="spread" title={String(value)}>
              <span className="font-mono text-xs">{String(value)}</span>
            </InfoRow>
          ))}
        </div>
      )}
    </div>
  );
}
