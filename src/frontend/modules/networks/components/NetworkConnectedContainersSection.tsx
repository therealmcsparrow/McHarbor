// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconNetwork, IconPlus, IconX } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { truncateId } from '@resources/utils/format';

type Translation = (key: string) => string;

type ConnectedContainer = {
  id: string;
  Name: string;
  EndpointID: string;
  IPv4Address: string;
  IPv6Address: string;
  MacAddress: string;
};

type NetworkConnectedContainersProps = {
  editing: boolean;
  connectedContainers: ConnectedContainer[];
  t: Translation;
  onOpenConnect: () => void;
  onDisconnect: (containerID: string) => void;
  onContainerOpen: (containerID: string) => void;
};

export function NetworkConnectedContainersSection({
  editing,
  connectedContainers,
  t,
  onOpenConnect,
  onDisconnect,
  onContainerOpen,
}: NetworkConnectedContainersProps) {
  return (
    <div className="mc-glass rounded-lg p-6">
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-sm font-semibold tracking-wider text-muted-foreground uppercase">
          {t('detail.connectedContainers')} ({connectedContainers.length})
        </h3>
        {!editing && (
          <Button variant="outline" size="sm" onClick={onOpenConnect}>
            <IconPlus className="size-3.5" />
            {t('detail.connectContainer')}
          </Button>
        )}
      </div>

      {connectedContainers.length === 0 ? (
        <div className="flex flex-col items-center gap-2 py-8 text-center">
          <IconNetwork className="size-8 text-muted-foreground/50" />
          <p className="text-sm text-muted-foreground">{t('detail.noContainers')}</p>
        </div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead className="bg-muted">
              <tr className="border-b border-border">
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.containerName')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.endpointId')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.ipv4Address')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.ipv6Address')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.macAddress')}</th>
                {!editing && (
                  <th className="px-3 py-2 text-right text-xs font-medium text-muted-foreground">
                    {t('columns.actions')}
                  </th>
                )}
              </tr>
            </thead>
            <tbody>
              {connectedContainers.map((container) => (
                <tr
                  key={container.id}
                  className="border-b border-border transition-colors last:border-0 hover:bg-muted/50"
                >
                  <td className="px-3 py-2">
                    <Button
                      variant="link"
                      className="h-auto p-0 font-medium"
                      onClick={() => onContainerOpen(container.id)}
                    >
                      {container.Name}
                    </Button>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">
                    {truncateId(container.EndpointID)}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs">{container.IPv4Address || '-'}</td>
                  <td className="px-3 py-2 font-mono text-xs">{container.IPv6Address || '-'}</td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{container.MacAddress || '-'}</td>
                  {!editing && (
                    <td className="px-3 py-2 text-right">
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t('detail.disconnect')}
                        onClick={() => onDisconnect(container.id)}
                      >
                        <IconX className="size-3.5 text-destructive" />
                      </Button>
                    </td>
                  )}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
