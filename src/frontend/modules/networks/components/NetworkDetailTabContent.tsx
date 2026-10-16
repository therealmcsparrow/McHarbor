// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

// NetworkDetailTabContent — what the tab panel shows, one face at a time.
//
// The four panels this draws are the same four the page used to stack end to
// end; the split is the point. General and IPAM answer "what is this
// network", the container table answers "what is on it", options and labels
// answer "how was it configured" — three questions, three tabs, rather than
// one scroll you have to walk to find out which of them you are looking at.

import type { NetworkInfo } from '@core/types/docker';
import { NetworkDetailPanels } from './NetworkDetailPanels';
import { NetworkHostLinkPanel } from './NetworkHostLinkPanel';
import { HostLinkFields } from './HostLinkFields';
import { NetworkConnectedContainersSection } from './NetworkConnectedContainersSection';
import type { NetworkTabId } from './NetworkDetailTabs';
import type { NetworkEditData } from '../hooks/useNetworkDetailState';

type ConnectedContainer = {
  id: string;
  Name: string;
  EndpointID: string;
  IPv4Address: string;
  IPv6Address: string;
  MacAddress: string;
};

type NetworkDetailTabContentProps = {
  activeTab: NetworkTabId;
  network: NetworkInfo;
  editData: NetworkEditData | null;
  editing: boolean;
  connectedContainers: ConnectedContainer[];
  t: (key: string) => string;
  onUpdateEdit: <K extends keyof NetworkEditData>(key: K, value: NetworkEditData[K]) => void;
  onUpdateIPAMEntry: (index: number, field: 'subnet' | 'gateway' | 'ipRange', value: string) => void;
  onAddIPAMEntry: () => void;
  onRemoveIPAMEntry: (index: number) => void;
  onOpenConnect: () => void;
  onDisconnect: (containerID: string) => void;
  onContainerOpen: (containerID: string) => void;
};

export function NetworkDetailTabContent({
  activeTab,
  network,
  editData,
  editing,
  connectedContainers,
  t,
  onUpdateEdit,
  onUpdateIPAMEntry,
  onAddIPAMEntry,
  onRemoveIPAMEntry,
  onOpenConnect,
  onDisconnect,
  onContainerOpen,
}: NetworkDetailTabContentProps) {
  const panelProps = {
    network,
    editData,
    editing,
    t,
    onUpdateEdit,
    onUpdateIPAMEntry,
    onAddIPAMEntry,
    onRemoveIPAMEntry,
  };

  if (activeTab === 'containers') {
    return (
      <NetworkConnectedContainersSection
        editing={editing}
        connectedContainers={connectedContainers}
        t={t}
        onOpenConnect={onOpenConnect}
        onDisconnect={onDisconnect}
        onContainerOpen={onContainerOpen}
      />
    );
  }

  if (activeTab === 'configuration') {
    const hasConfiguration =
      Object.keys(network.Options ?? {}).length > 0 || Object.keys(network.Labels ?? {}).length > 0;

    // The editors always render while editing — that is where you *add* the
    // first option — but a read-only network with neither says so rather
    // than showing an empty tab.
    if (!editing && !hasConfiguration) {
      return (
        <div className="mc-glass rounded-lg p-6">
          <p className="py-8 text-center text-sm text-muted-foreground">{t('detail.noConfiguration')}</p>
        </div>
      );
    }

    return (
      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <NetworkDetailPanels {...panelProps} sections={['options', 'labels']} />
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
      <NetworkDetailPanels {...panelProps} sections={['general', 'ipam']} />
      {!editing && <NetworkHostLinkPanel network={network} />}
      {editing && editData?.driver === 'macvlan' && (
        <div className="mc-glass rounded-lg p-6">
          <HostLinkFields
            value={editData.hostLink}
            onChange={(value) => onUpdateEdit('hostLink', value)}
            enableIPv6={editData.enableIPv6}
            mode={editData.options.find((option) => option.key === 'macvlan_mode')?.value ?? ''}
          />
        </div>
      )}
    </div>
  );
}
