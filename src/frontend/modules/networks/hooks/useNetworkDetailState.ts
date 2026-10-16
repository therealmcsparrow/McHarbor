// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { toastError } from '@resources/utils/toast';
import { api } from '@core/api/client';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useEnvironmentStore } from '@resources/stores/environment';
import { isBuiltInNetwork } from '@core/utils/network';
import type { NetworkInfo } from '@core/types/docker';
import { withoutHostLinkAddress, withoutHostLinkLabels } from '../utils/hostLinkLabels';
import { recreateNetworkBody } from '../utils/recreateNetworkBody';
import { hostLinkFormOf, type HostLinkFormValue } from '../utils/hostLinkForm';
import { reportRecreateOutcome, type RecreateNetworkResult } from '../utils/recreateOutcome';
import {
  useConnectNetwork,
  useContainersForNetwork,
  useDisconnectNetwork,
  useNetworkInspect,
  useNetworkRefusals,
  useRemoveNetwork,
} from './useNetworks';

export type NetworkEditData = {
  name: string;
  driver: string;
  internal: boolean;
  attachable: boolean;
  enableIPv6: boolean;
  /** The macvlan host interface; the server stores it in labels it owns, so it is edited here, not as a label. */
  hostLink: HostLinkFormValue;
  ipamDriver: string;
  ipamConfig: Array<{ subnet: string; gateway: string; ipRange: string; auxAddresses?: Record<string, string> }>;
  labels: Array<{ key: string; value: string }>;
  options: Array<{ key: string; value: string }>;
};

function editDataOf(network: NetworkInfo): NetworkEditData {
  return {
    name: network.Name,
    driver: network.Driver,
    internal: network.Internal,
    attachable: network.Attachable,
    enableIPv6: network.EnableIPv6,
    ipamDriver: network.IPAM?.Driver || 'default',
    ipamConfig: (network.IPAM?.Config ?? []).map((config) => ({
      subnet: config.Subnet || '',
      gateway: config.Gateway || '',
      ipRange: config.IPRange || '',
      auxAddresses: withoutHostLinkAddress(config.AuxiliaryAddresses),
    })),
    labels: withoutHostLinkLabels(Object.entries(network.Labels || {}).map(([key, value]) => ({ key, value }))),
    options: Object.entries(network.Options || {}).map(([key, value]) => ({ key, value })),
    hostLink: hostLinkFormOf(network.Labels),
  };
}

export function useNetworkDetailState(id: string, t: (key: string) => string) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const envID = useEnvironmentStore((store) => store.currentId);
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  const { data: network, isLoading } = useNetworkInspect(id);
  const removeNetwork = useRemoveNetwork();
  const refuseNetwork = useNetworkRefusals();
  const connectNetwork = useConnectNetwork();
  const disconnectNetwork = useDisconnectNetwork();
  const { data: allContainers = [] } = useContainersForNetwork();
  const [editing, setEditing] = useState(false);
  const [editData, setEditData] = useState<NetworkEditData | null>(null);
  const [saving, setSaving] = useState(false);
  const [removeDialogOpen, setRemoveDialogOpen] = useState(false);
  const [recreateDialogOpen, setRecreateDialogOpen] = useState(false);
  const [disconnectTarget, setDisconnectTarget] = useState<string | null>(null);
  const [connectDialogOpen, setConnectDialogOpen] = useState(false);
  const [selectedContainer, setSelectedContainer] = useState('');

  // The list's Edit lands here with ?edit=1; the flag is consumed once the network loads,
  // so a refresh or Back does not reopen the editor.
  const [searchParams, setSearchParams] = useSearchParams();
  const wantsEdit = searchParams.get('edit') === '1';
  const [editConsumed, setEditConsumed] = useState(false);
  if (wantsEdit && network && !editConsumed) {
    setEditConsumed(true);
    if (!isBuiltInNetwork(network)) {
      setEditData(editDataOf(network));
      setEditing(true);
    }
  } else if (!wantsEdit && editConsumed) {
    setEditConsumed(false);
  }
  useEffect(() => {
    if (!wantsEdit || !network) {
      return;
    }
    setSearchParams(
      (params) => {
        params.delete('edit');
        return params;
      },
      { replace: true },
    );
  }, [wantsEdit, network, setSearchParams]);

  useEffect(() => {
    setHeaderActive(true);
    return () => setHeaderActive(false);
  }, [setHeaderActive]);

  const connectedContainers = useMemo(() => {
    if (!network?.Containers) {
      return [];
    }
    return Object.entries(network.Containers).map(([containerID, info]) => ({
      id: containerID,
      ...info,
    }));
  }, [network]);

  const availableContainers = useMemo(() => {
    if (!network?.Containers) {
      return allContainers;
    }
    const connectedIDs = new Set(Object.keys(network.Containers));
    return allContainers.filter((container) => !connectedIDs.has(container.Id));
  }, [allContainers, network]);

  function startEditing() {
    if (!network) {
      return;
    }

    setEditData(editDataOf(network));
    setEditing(true);
  }

  function cancelEditing() {
    setEditing(false);
    setEditData(null);
  }

  async function confirmSave() {
    if (!network || !editData) {
      return;
    }

    setSaving(true);
    setRecreateDialogOpen(false);

    try {
      const envParam = envID ? `?env=${envID}` : '';
      // One server-side call: it snapshots each container's address, MAC and aliases, refuses a bad
      // definition before touching anything, and puts the containers back exactly as they were.
      const result = await api.post<RecreateNetworkResult>(
        `/networks/${id}/recreate${envParam}`,
        recreateNetworkBody(editData),
      );
      refuseNetwork(result);
      const outcome = assertSuccess(result);
      const newID = outcome?.Id;

      void queryClient.invalidateQueries({ queryKey: ['networks'] });
      void queryClient.invalidateQueries({ queryKey: ['network'] });
      reportRecreateOutcome(outcome?.reconnected ?? [], t);
      void navigate(newID ? `/networks/${newID}` : '/networks', { replace: !!newID });
      setEditing(false);
      setEditData(null);
    } catch (err) {
      toastError(err, t('toast.updateFailed'));
    } finally {
      setSaving(false);
    }
  }

  function updateEdit<K extends keyof NetworkEditData>(key: K, value: NetworkEditData[K]) {
    setEditData((current) => (current ? { ...current, [key]: value } : current));
  }

  function updateIPAMEntry(index: number, field: 'subnet' | 'gateway' | 'ipRange', value: string) {
    setEditData((current) =>
      current
        ? {
            ...current,
            ipamConfig: current.ipamConfig.map((config, configIndex) =>
              configIndex === index ? { ...config, [field]: value } : config,
            ),
          }
        : current,
    );
  }

  return {
    network,
    isLoading,
    removeNetwork,
    connectNetwork,
    disconnectNetwork,
    editing,
    editData,
    saving,
    removeDialogOpen,
    recreateDialogOpen,
    disconnectTarget,
    connectDialogOpen,
    selectedContainer,
    connectedContainers,
    availableContainers,
    startEditing,
    cancelEditing,
    confirmSave,
    updateEdit,
    updateIPAMEntry,
    setRemoveDialogOpen,
    setRecreateDialogOpen,
    setDisconnectTarget,
    setConnectDialogOpen,
    setSelectedContainer,
    addIPAMEntry: () =>
      setEditData((current) =>
        current
          ? { ...current, ipamConfig: [...current.ipamConfig, { subnet: '', gateway: '', ipRange: '' }] }
          : current,
      ),
    removeIPAMEntry: (index: number) =>
      setEditData((current) =>
        current
          ? { ...current, ipamConfig: current.ipamConfig.filter((_, currentIndex) => currentIndex !== index) }
          : current,
      ),
  };
}
