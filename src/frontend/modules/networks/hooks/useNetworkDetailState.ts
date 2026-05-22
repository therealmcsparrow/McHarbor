// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router';
import { useQueryClient } from '@tanstack/react-query';
import { toast } from 'sonner';
import { api } from '@core/api/client';
import { useHeaderSlot } from '@resources/stores/headerSlot';
import { useEnvironmentStore } from '@resources/stores/environment';
import {
  useConnectNetwork,
  useContainersForNetwork,
  useDisconnectNetwork,
  useNetworkInspect,
  useRemoveNetwork,
} from './useNetworks';

export type NetworkEditData = {
  name: string;
  driver: string;
  internal: boolean;
  attachable: boolean;
  enableIPv6: boolean;
  ipamDriver: string;
  ipamConfig: Array<{ subnet: string; gateway: string; ipRange: string }>;
  labels: Array<{ key: string; value: string }>;
  options: Array<{ key: string; value: string }>;
};

export function useNetworkDetailState(id: string, t: (key: string) => string) {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const envID = useEnvironmentStore((store) => store.currentId);
  const setHeaderActive = useHeaderSlot((store) => store.setActive);
  const { data: network, isLoading } = useNetworkInspect(id);
  const removeNetwork = useRemoveNetwork();
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

    setEditData({
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
      })),
      labels: Object.entries(network.Labels || {}).map(([key, value]) => ({ key, value })),
      options: Object.entries(network.Options || {}).map(([key, value]) => ({ key, value })),
    });
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
      const envQuery: Record<string, string> = envID ? { env: envID } : {};
      const containersToReconnect = connectedContainers.map((container) => container.id);

      for (const containerID of containersToReconnect) {
        await api.post(`/networks/${id}/disconnect${envParam}`, { container: containerID });
      }

      await api.del(`/networks/${id}`, envQuery);

      const result = await api.post<{ Id: string }>(`/networks${envParam}`, {
        name: editData.name,
        driver: editData.driver,
        internal: editData.internal,
        attachable: editData.attachable,
        ipam: {
          Driver: editData.ipamDriver,
          Config: editData.ipamConfig
            .filter((config) => config.subnet)
            .map((config) => ({
              Subnet: config.subnet,
              Gateway: config.gateway || undefined,
              IPRange: config.ipRange || undefined,
            })),
        },
        options: Object.fromEntries(editData.options.filter((option) => option.key).map((option) => [option.key, option.value])),
        labels: Object.fromEntries(editData.labels.filter((label) => label.key).map((label) => [label.key, label.value])),
      });

      const newID = result?.data?.Id;
      if (newID && containersToReconnect.length > 0) {
        for (const containerID of containersToReconnect) {
          try {
            await api.post(`/networks/${newID}/connect${envParam}`, { container: containerID });
          } catch {
            // Container may have been removed while the network was recreated.
          }
        }
      }

      queryClient.invalidateQueries({ queryKey: ['networks'] });
      queryClient.invalidateQueries({ queryKey: ['network'] });
      toast.success(t('toast.updated'));
      navigate(newID ? `/networks/${newID}` : '/networks', { replace: !!newID });
      setEditing(false);
      setEditData(null);
    } catch {
      toast.error(t('toast.updateFailed'));
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
