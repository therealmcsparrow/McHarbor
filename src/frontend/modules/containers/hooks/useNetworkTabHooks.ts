// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useCallback, useMemo } from 'react';
import type { ContainerInfo } from '@core/types/docker';
import type { EditFormData, PortMappingEntry } from '../types/edit-form';
import { releaseMappingPort, removeExposedPort } from '../types/exposed-ports';

const NO_IMAGE_PORTS: ReadonlySet<string> = new Set();

type FieldChange = <K extends keyof EditFormData>(field: K, value: EditFormData[K]) => void;

export function useNetworkEditHandlers(
  editData: EditFormData | null | undefined,
  extraHosts: string[],
  onFieldChange: FieldChange | undefined,
  imagePorts: ReadonlySet<string> = NO_IMAGE_PORTS,
) {
  const handlePortMappingChange = useCallback(
    (index: number, field: keyof PortMappingEntry, value: string) => {
      const mappings = [...(editData?.portMappings ?? [])];
      const existing = mappings[index] ?? {
        containerPort: '',
        hostPort: '',
        hostIp: '0.0.0.0',
        protocol: 'tcp',
      };
      mappings[index] = { ...existing, [field]: value };
      onFieldChange?.('portMappings', mappings);
    },
    [editData?.portMappings, onFieldChange],
  );

  const handleAddPortMapping = useCallback(() => {
    onFieldChange?.('portMappings', [
      ...(editData?.portMappings ?? []),
      { containerPort: '', hostPort: '', hostIp: '0.0.0.0', protocol: 'tcp' },
    ]);
  }, [editData?.portMappings, onFieldChange]);

  const handleRemovePortMapping = useCallback(
    (index: number) => {
      const mappings = editData?.portMappings ?? [];
      const removed = mappings[index];
      const remaining = mappings.filter((_, i) => i !== index);
      onFieldChange?.('portMappings', remaining);
      // The save re-exposes every mapped port, so a mapping's port must leave with it or it
      // stays exposed for good.
      if (removed) {
        onFieldChange?.(
          'exposedPorts',
          releaseMappingPort(editData?.exposedPorts ?? [], removed, remaining, imagePorts),
        );
      }
    },
    [editData?.portMappings, editData?.exposedPorts, imagePorts, onFieldChange],
  );

  const handleRemoveExposedPort = useCallback(
    (port: string) => {
      onFieldChange?.('exposedPorts', removeExposedPort(editData?.exposedPorts ?? [], port, imagePorts));
    },
    [editData?.exposedPorts, imagePorts, onFieldChange],
  );

  const handleExtraHostChange = useCallback(
    (index: number, value: string) => {
      const hosts = [...extraHosts];
      hosts[index] = value;
      onFieldChange?.('extraHosts', hosts);
    },
    [extraHosts, onFieldChange],
  );

  const handleAddExtraHost = useCallback(() => {
    onFieldChange?.('extraHosts', [...extraHosts, '']);
  }, [extraHosts, onFieldChange]);

  const handleRemoveExtraHost = useCallback(
    (index: number) => {
      onFieldChange?.(
        'extraHosts',
        extraHosts.filter((_, i) => i !== index),
      );
    },
    [extraHosts, onFieldChange],
  );

  return {
    handlePortMappingChange,
    handleAddPortMapping,
    handleRemovePortMapping,
    handleRemoveExposedPort,
    handleExtraHostChange,
    handleAddExtraHost,
    handleRemoveExtraHost,
  };
}

export function useKnownAgentNetworks(
  isAgentEnvironment: boolean,
  connectedNetworkNames: Set<string>,
  containers: ContainerInfo[],
) {
  return useMemo(() => {
    if (!isAgentEnvironment) return [];

    const names = new Set<string>();
    for (const networkName of connectedNetworkNames) {
      names.add(networkName);
    }
    for (const item of containers) {
      for (const networkName of Object.keys(item.NetworkSettings?.Networks ?? {})) {
        names.add(networkName);
      }
    }

    return Array.from(names)
      .sort((a, b) => a.localeCompare(b))
      .map((networkName) => ({
        Id: networkName,
        Name: networkName,
        Created: '',
        Scope: '',
        Driver: 'bridge',
        EnableIPv6: false,
        IPAM: { Driver: '', Config: [] },
        Internal: false,
        Attachable: false,
        Containers: {},
        Options: null,
        Labels: null,
      }));
  }, [connectedNetworkNames, containers, isAgentEnvironment]);
}
