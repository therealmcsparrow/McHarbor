// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { IconNetwork, IconPlus, IconTrash, IconX } from '@tabler/icons-react';
import type { NetworkInfo } from '@core/types/docker';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { InfoRow } from '@resources/components/ui/InfoRow';
import { Input } from '@resources/components/ui/Input';
import { KeyValueEditor } from '@resources/components/KeyValueEditor';
import { Select } from '@resources/components/ui/Select';
import { formatDate, truncateId } from '@resources/utils/format';
import { DRIVER_OPTIONS } from './networkDriverConfig';
import { ToggleField } from './ToggleField';
import type { NetworkEditData } from '../hooks/useNetworkDetailState';

type Translation = (key: string) => string;

type NetworkDetailPanelProps = {
  network: NetworkInfo;
  editData: NetworkEditData | null;
  editing: boolean;
  t: Translation;
  onUpdateEdit: <K extends keyof NetworkEditData>(key: K, value: NetworkEditData[K]) => void;
  onUpdateIPAMEntry: (index: number, field: 'subnet' | 'gateway' | 'ipRange', value: string) => void;
  onAddIPAMEntry: () => void;
  onRemoveIPAMEntry: (index: number) => void;
};

export function NetworkDetailPanels({
  network,
  editData,
  editing,
  t,
  onUpdateEdit,
  onUpdateIPAMEntry,
  onAddIPAMEntry,
  onRemoveIPAMEntry,
}: NetworkDetailPanelProps) {
  if (!editing || !editData) {
    return (
      <>
        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.general')}</h3>
          <InfoRow label={t('detail.name')}>{network.Name}</InfoRow>
          <InfoRow label={t('detail.networkId')}><span className="font-mono text-xs">{network.Id}</span></InfoRow>
          <InfoRow label={t('detail.driver')}><Badge variant="default" className="px-1.5 py-0 text-[10px]">{network.Driver}</Badge></InfoRow>
          <InfoRow label={t('detail.scope')}><Badge variant="secondary" className="px-1.5 py-0 text-[10px]">{network.Scope}</Badge></InfoRow>
          <InfoRow label={t('detail.created')}>{formatDate(network.Created)}</InfoRow>
          <InfoRow label={t('detail.internal')}>{network.Internal ? <Badge variant="warning">{t('badges.yes')}</Badge> : t('badges.no')}</InfoRow>
          <InfoRow label={t('detail.attachable')}>{network.Attachable ? <Badge variant="success">{t('badges.yes')}</Badge> : t('badges.no')}</InfoRow>
          <InfoRow label={t('detail.ipv6')}>{network.EnableIPv6 ? <Badge variant="success">{t('badges.yes')}</Badge> : t('badges.no')}</InfoRow>
        </div>

        <div className="rounded-lg border border-border bg-card p-6">
          <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.ipam')}</h3>
          <InfoRow label={t('detail.ipamDriver')}>{network.IPAM?.Driver || 'default'}</InfoRow>
          {network.IPAM?.Config?.length ? (
            network.IPAM.Config.map((config, index) => (
              <div key={`ipam-${index}`}>
                {config.Subnet && <InfoRow label={t('detail.subnet')}><span className="font-mono text-xs">{config.Subnet}</span></InfoRow>}
                {config.Gateway && <InfoRow label={t('detail.gateway')}><span className="font-mono text-xs">{config.Gateway}</span></InfoRow>}
                {config.IPRange && <InfoRow label={t('detail.ipRange')}><span className="font-mono text-xs">{config.IPRange}</span></InfoRow>}
              </div>
            ))
          ) : (
            <p className="text-sm text-muted-foreground">{t('detail.noIpamConfig')}</p>
          )}
        </div>

        {network.Options && Object.keys(network.Options).length > 0 && (
          <div className="rounded-lg border border-border bg-card p-6">
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.options')}</h3>
            {Object.entries(network.Options).map(([key, value]) => (
              <InfoRow key={key} label={key}><span className="font-mono text-xs">{String(value)}</span></InfoRow>
            ))}
          </div>
        )}

        {network.Labels && Object.keys(network.Labels).length > 0 && (
          <div className="rounded-lg border border-border bg-card p-6">
            <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.labels')}</h3>
            {Object.entries(network.Labels).map(([key, value]) => (
              <InfoRow key={key} label={key}><span className="font-mono text-xs">{String(value)}</span></InfoRow>
            ))}
          </div>
        )}
      </>
    );
  }

  return (
    <>
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.general')}</h3>
        <div className="space-y-3">
          <div>
            <label className="text-xs font-medium text-muted-foreground">{t('detail.name')}</label>
            <Input
              variant="outline"
              value={editData.name}
              onChange={(event) => onUpdateEdit('name', event.target.value)}
              placeholder={t('edit.namePlaceholder')}
              className="mt-1"
            />
          </div>
          <InfoRow label={t('detail.networkId')}><span className="font-mono text-xs">{network.Id}</span></InfoRow>
          <div>
            <label className="text-xs font-medium text-muted-foreground">{t('detail.driver')}</label>
            <Select
              variant="outline"
              value={editData.driver}
              onChange={(value) => onUpdateEdit('driver', value)}
              options={DRIVER_OPTIONS.map((option) => ({ value: option.value, label: option.label }))}
              className="mt-1"
            />
          </div>
          <InfoRow label={t('detail.scope')}><Badge variant="secondary" className="px-1.5 py-0 text-[10px]">{network.Scope}</Badge></InfoRow>
          <InfoRow label={t('detail.created')}>{formatDate(network.Created)}</InfoRow>
          <ToggleField label={t('detail.internal')} checked={editData.internal} onChange={(value) => onUpdateEdit('internal', value)} />
          <ToggleField label={t('detail.attachable')} checked={editData.attachable} onChange={(value) => onUpdateEdit('attachable', value)} />
          <ToggleField label={t('detail.ipv6')} checked={editData.enableIPv6} onChange={(value) => onUpdateEdit('enableIPv6', value)} />
        </div>
      </div>

      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.ipam')}</h3>
        <div className="space-y-4">
          <InfoRow label={t('detail.ipamDriver')}>{editData.ipamDriver}</InfoRow>
          {editData.ipamConfig.map((config, index) => (
            <div key={`ipam-edit-${index}`} className="space-y-2 rounded-md border border-border p-3">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-muted-foreground">{t('detail.subnet')} #{index + 1}</span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={t('edit.removeIpamConfig')}
                  onClick={() => onRemoveIPAMEntry(index)}
                  className="size-6 text-muted-foreground hover:text-red-500"
                >
                  <IconTrash className="size-3" />
                </Button>
              </div>
              <Input variant="outline" value={config.subnet} onChange={(event) => onUpdateIPAMEntry(index, 'subnet', event.target.value)} placeholder={t('create.subnetPlaceholder')} />
              <Input variant="outline" value={config.gateway} onChange={(event) => onUpdateIPAMEntry(index, 'gateway', event.target.value)} placeholder={t('create.gatewayPlaceholder')} />
              <Input variant="outline" value={config.ipRange} onChange={(event) => onUpdateIPAMEntry(index, 'ipRange', event.target.value)} placeholder={t('create.ipRangePlaceholder')} />
            </div>
          ))}
          <Button variant="outline" size="sm" onClick={onAddIPAMEntry}>
            <IconPlus className="mr-1 size-3.5" />
            {t('edit.addIpamConfig')}
          </Button>
        </div>
      </div>

      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.options')}</h3>
        <KeyValueEditor
          entries={editData.options}
          onChange={(entries) => onUpdateEdit('options', entries)}
          keyLabel={t('create.optionKey')}
          valueLabel={t('create.optionValue')}
          addLabel={t('create.add')}
        />
      </div>

      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wider text-muted-foreground">{t('detail.labels')}</h3>
        <KeyValueEditor
          entries={editData.labels}
          onChange={(entries) => onUpdateEdit('labels', entries)}
          keyLabel={t('create.labelKey')}
          valueLabel={t('create.labelValue')}
          addLabel={t('create.add')}
        />
      </div>
    </>
  );
}

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
    <div className="mt-6 rounded-lg border border-border bg-card p-6">
      <div className="mb-4 flex items-center justify-between">
        <h3 className="text-sm font-semibold uppercase tracking-wider text-muted-foreground">
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
            <thead>
              <tr className="border-b border-border">
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.containerName')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.endpointId')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.ipv4Address')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.ipv6Address')}</th>
                <th className="px-3 py-2 text-xs font-medium text-muted-foreground">{t('detail.macAddress')}</th>
                {!editing && <th className="px-3 py-2 text-right text-xs font-medium text-muted-foreground">{t('columns.actions')}</th>}
              </tr>
            </thead>
            <tbody>
              {connectedContainers.map((container) => (
                <tr key={container.id} className="border-b border-border transition-colors last:border-0 hover:bg-muted/50">
                  <td className="px-3 py-2">
                    <Button variant="link" className="h-auto p-0 font-medium" onClick={() => onContainerOpen(container.id)}>
                      {container.Name}
                    </Button>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{truncateId(container.EndpointID)}</td>
                  <td className="px-3 py-2 font-mono text-xs">{container.IPv4Address || '-'}</td>
                  <td className="px-3 py-2 font-mono text-xs">{container.IPv6Address || '-'}</td>
                  <td className="px-3 py-2 font-mono text-xs text-muted-foreground">{container.MacAddress || '-'}</td>
                  {!editing && (
                    <td className="px-3 py-2 text-right">
                      <Button variant="ghost" size="icon" aria-label={t('detail.disconnect')} onClick={() => onDisconnect(container.id)}>
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
