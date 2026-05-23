// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPlus, IconTrash } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import type { PortMappingEntry } from '../../types/edit-form';

type NetworkTabPortsSectionProps = {
  editing: boolean;
  exposedPorts: string[];
  portMappingKeys: string[];
  portMappings: PortMappingEntry[];
  ports: string[];
  onAddPortMapping: () => void;
  onPortMappingChange: (index: number, field: keyof PortMappingEntry, value: string) => void;
  onRemovePortMapping: (index: number) => void;
};

export function NetworkTabPortsSection({
  editing,
  exposedPorts,
  portMappingKeys,
  portMappings,
  ports,
  onAddPortMapping,
  onPortMappingChange,
  onRemovePortMapping,
}: NetworkTabPortsSectionProps) {
  const { t } = useTranslation('containers');

  return (
    <>
      <div className="rounded-lg border border-border bg-card p-6 lg:col-span-2">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.portMappings')}</h3>
        {editing ? (
          <div className="space-y-2">
            {portMappings.length > 0 && (
              <div className="grid grid-cols-[1fr_1fr_100px_80px_auto] gap-2 text-xs font-medium text-muted-foreground">
                <span>{t('networkTab.hostPort')}</span>
                <span>{t('networkTab.containerPort')}</span>
                <span>{t('networkTab.hostIp')}</span>
                <span>{t('networkTab.protocol')}</span>
                <span className="w-8" />
              </div>
            )}
            {portMappings.map((mapping, index) => (
              <div key={portMappingKeys[index]} className="grid grid-cols-[1fr_1fr_100px_80px_auto] gap-2">
                <input
                  type="text"
                  value={mapping.hostPort}
                  onChange={(event) => onPortMappingChange(index, 'hostPort', event.target.value)}
                  placeholder="8080"
                  className="rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
                />
                <input
                  type="text"
                  value={mapping.containerPort}
                  onChange={(event) => onPortMappingChange(index, 'containerPort', event.target.value)}
                  placeholder="80"
                  className="rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
                />
                <input
                  type="text"
                  value={mapping.hostIp}
                  onChange={(event) => onPortMappingChange(index, 'hostIp', event.target.value)}
                  placeholder="0.0.0.0"
                  className="rounded-md border border-border bg-muted px-2 py-1.5 font-mono text-xs text-foreground focus:border-primary focus:outline-none"
                />
                <select
                  value={mapping.protocol}
                  onChange={(event) => onPortMappingChange(index, 'protocol', event.target.value)}
                  className="rounded-md border border-border bg-muted px-2 py-1.5 text-xs text-foreground focus:border-primary focus:outline-none"
                >
                  <option value="tcp">TCP</option>
                  <option value="udp">UDP</option>
                </select>
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => onRemovePortMapping(index)}
                  aria-label="Remove port"
                  className="size-7 text-muted-foreground hover:text-red-500"
                >
                  <IconTrash className="size-3.5" />
                </Button>
              </div>
            ))}
            <Button variant="outline" size="sm" onClick={onAddPortMapping} className="mt-1">
              <IconPlus className="mr-1 size-3.5" />
              {t('edit.addPort')}
            </Button>
          </div>
        ) : ports.length > 0 ? (
          <div className="space-y-2">
            {ports.map((port) => (
              <div key={port} className="rounded-md bg-muted px-3 py-2 font-mono text-xs text-foreground">
                {port}
              </div>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('networkTab.noPortMappings')}</p>
        )}
      </div>

      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('networkTab.exposedPorts')}</h3>
        {exposedPorts.length > 0 ? (
          <div className="flex flex-wrap gap-2">
            {exposedPorts.map((port) => (
              <Badge key={port} variant="secondary" className="font-mono text-xs">{port}</Badge>
            ))}
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">{t('networkTab.noExposedPorts')}</p>
        )}
      </div>
    </>
  );
}
