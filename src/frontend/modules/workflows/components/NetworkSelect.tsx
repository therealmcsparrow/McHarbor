// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { NetworkInfo } from '@core/types/docker';
import { ResourcePicker } from './ResourcePicker';

interface NetworkSelectProps {
  value: string;
  onChange: (value: string) => void;
  envId?: string;
}

/** Picks a Docker network by name from the selected environment. */
export function NetworkSelect({ value, onChange, envId }: NetworkSelectProps) {
  const { t } = useTranslation('common');

  const { data: networks, isLoading } = useQuery({
    queryKey: ['networks-select', envId ?? ''],
    queryFn: () => api.get<NetworkInfo[]>('/networks', envId ? { env: envId } : undefined).then((r) => r.data ?? []),
    staleTime: 30_000,
    enabled: !!envId,
  });

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={isLoading}
      placeholder={t('workflows.selectNetwork')}
      emptyLabel={t('workflows.noNetworksFound')}
      blockedLabel={envId ? undefined : t('workflows.selectEnvironmentFirst')}
      items={(networks ?? []).map((network) => ({
        value: network.Name,
        title: network.Name,
        subtitle: `${network.Driver} - ${network.Scope}${network.Internal ? ' - internal' : ''}`,
        badge: network.BuiltIn ? t('workflows.builtInBadge') : undefined,
      }))}
    />
  );
}
