// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { assertSuccess } from '@resources/utils/api-mutation';
import { useEnvironmentStore } from '@resources/stores/environment';

export type HostDeviceCategory =
  | 'serial'
  | 'gpu'
  | 'audio'
  | 'video'
  | 'input'
  | 'network'
  | 'usb'
  | 'system'
  | 'block';

export type HostDevice = {
  path: string;
  type: 'char' | 'block';
  major: number;
  minor: number;
  category: HostDeviceCategory;
  label?: string;
  stablePath?: string;
  links?: string[];
  driver?: string;
  dangerous: boolean;
};

export type HostDeviceList = {
  devices: HostDevice[];
  truncated: boolean;
  collectedAt: string;
};

export function useHostDevices() {
  const envId = useEnvironmentStore((s) => s.currentId);
  const allowed = usePermission(PERM.HOST_VIEW);
  const query = useQuery({
    queryKey: ['host-devices', envId],
    queryFn: () =>
      api.get<HostDeviceList>('/system/host-devices', envId ? { env: envId } : undefined).then(assertSuccess),
    enabled: allowed,
    staleTime: 60_000,
  });
  return { ...query, allowed };
}
