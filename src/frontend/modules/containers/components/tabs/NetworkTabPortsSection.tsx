// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconListDetails, IconLock, IconPlus, IconX } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import type { PortMappingEntry } from '../../types/edit-form';
import type { ExposedPortRow } from '../../types/exposed-ports';
import { PortContextDrawer } from '../PortContextDrawer';
import { PortMappingEditRow } from './PortMappingEditRow';

type NetworkTabPortsSectionProps = {
  containerId: string;
  allowConnectionTest: boolean;
  editing: boolean;
  exposedPorts: string[];
  /** Edit mode: what the save will expose, and why a port may not be removable. */
  exposedRows?: ExposedPortRow[];
  onRemoveExposedPort?: (port: string) => void;
  portMappingKeys: string[];
  portMappings: PortMappingEntry[];
  publishedPortMappings: PortMappingEntry[];
  onAddPortMapping: () => void;
  onPortMappingChange: (index: number, field: keyof PortMappingEntry, value: string) => void;
  onRemovePortMapping: (index: number) => void;
  /** Which card to render, so the configuration editor can place them in different columns. */
  part?: 'all' | 'mappings' | 'exposed';
};

export function NetworkTabPortsSection({
  containerId,
  allowConnectionTest,
  editing,
  exposedPorts,
  exposedRows = [],
  onRemoveExposedPort,
  portMappingKeys,
  portMappings,
  publishedPortMappings,
  onAddPortMapping,
  onPortMappingChange,
  onRemovePortMapping,
  part = 'all',
}: NetworkTabPortsSectionProps) {
  const { t } = useTranslation('containers');
  const [selectedPort, setSelectedPort] = useState<PortMappingEntry | null>(null);
  const portKey = `${containerId}|${editing}`;
  const [portFor, setPortFor] = useState(portKey);
  if (portFor !== portKey) {
    setPortFor(portKey);
    setSelectedPort(null);
  }
  return (
    <>
      {part !== 'exposed' && (
        <div className="mc-glass rounded-lg p-6 lg:col-span-2">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('networkTab.portMappings')}
          </h3>
          {editing ? (
            <div className="space-y-2">
              {portMappings.length > 0 && (
                <div className="hidden grid-cols-[1fr_1fr_100px_80px_auto] gap-2 text-xs font-medium text-muted-foreground sm:grid">
                  <span>{t('networkTab.hostPort')}</span>
                  <span>{t('networkTab.containerPort')}</span>
                  <span>{t('networkTab.hostIp')}</span>
                  <span>{t('networkTab.protocol')}</span>
                  <span className="w-8" />
                </div>
              )}
              {portMappings.map((mapping, index) => (
                <PortMappingEditRow
                  key={portMappingKeys[index]}
                  mapping={mapping}
                  index={index}
                  onChange={onPortMappingChange}
                  onRemove={onRemovePortMapping}
                />
              ))}
              <Button variant="outline" size="sm" onClick={onAddPortMapping} className="mt-1">
                <IconPlus className="mr-1 size-3.5" />
                {t('edit.addPort')}
              </Button>
            </div>
          ) : publishedPortMappings.length > 0 ? (
            <div className="space-y-2">
              {publishedPortMappings.map((mapping) => (
                <Button
                  key={`${mapping.hostIp}:${mapping.hostPort}:${mapping.containerPort}/${mapping.protocol}`}
                  variant="ghost"
                  onClick={() => setSelectedPort(mapping)}
                  aria-haspopup="dialog"
                  className="h-auto min-h-11 w-full justify-between rounded-md bg-muted px-3 py-2 font-mono text-xs sm:min-h-[1.875rem]"
                >
                  <span className="min-w-0 text-left break-all">
                    {mapping.hostIp}:{mapping.hostPort} → {mapping.containerPort}/{mapping.protocol}
                  </span>
                  <IconListDetails className="size-4 shrink-0 text-muted-foreground" />
                </Button>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t('networkTab.noPortMappings')}</p>
          )}
        </div>
      )}

      {part !== 'mappings' && (
        <div className="mc-glass rounded-lg p-6">
          <h3 className="mb-4 text-sm font-semibold tracking-wider text-muted-foreground uppercase">
            {t('networkTab.exposedPorts')}
          </h3>
          {editing ? (
            exposedRows.length > 0 ? (
              <div className="space-y-3">
                <div className="flex flex-wrap gap-2">
                  {exposedRows.map((row) => (
                    <Badge key={row.port} variant="secondary" className="gap-1 font-mono text-xs">
                      {row.port}
                      {row.lockedBy ? (
                        <IconLock
                          className="size-3 text-muted-foreground"
                          // eslint-disable-next-line jsx-a11y/prefer-tag-over-role -- inline SVG icon; an <img> would need a data URI
                          role="img"
                          aria-label={
                            row.lockedBy === 'image'
                              ? t('networkTab.exposedLockedImage')
                              : t('networkTab.exposedLockedMapping')
                          }
                        />
                      ) : (
                        <Button
                          variant="ghost"
                          size="icon"
                          className="size-4 p-0"
                          aria-label={t('networkTab.removeExposedPort', { port: row.port })}
                          onClick={() => onRemoveExposedPort?.(row.port)}
                        >
                          <IconX className="size-3" />
                        </Button>
                      )}
                    </Badge>
                  ))}
                </div>
                {exposedRows.some((row) => row.lockedBy) && (
                  <p className="text-xs text-muted-foreground">{t('networkTab.exposedLockedHint')}</p>
                )}
              </div>
            ) : (
              <p className="text-sm text-muted-foreground">{t('networkTab.noExposedPorts')}</p>
            )
          ) : exposedPorts.length > 0 ? (
            <div className="flex flex-wrap gap-2">
              {exposedPorts.map((port) => (
                <Badge key={port} variant="secondary" className="font-mono text-xs">
                  {port}
                </Badge>
              ))}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t('networkTab.noExposedPorts')}</p>
          )}
        </div>
      )}

      {part !== 'exposed' && (
        <PortContextDrawer
          mapping={selectedPort}
          allowConnectionTest={allowConnectionTest}
          onClose={() => setSelectedPort(null)}
        />
      )}
    </>
  );
}
