// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import type { ContainerInfo } from '@core/types/docker';
import { ResourcePicker, type ResourcePickerItem } from './ResourcePicker';

interface StackServiceSelectProps {
  value: string;
  onChange: (value: string) => void;
  envId?: string;
  stackName?: string;
  allowCustom?: boolean;
}

const SERVICE_LABEL = 'com.docker.compose.service';

/**
 * Picks a Compose service name from the containers of the selected
 * stack. The service comes from the compose label on each container, so
 * a stack that has not been deployed yet lists nothing — `allowCustom`
 * covers that case with a typed name.
 */
export function StackServiceSelect({ value, onChange, envId, stackName, allowCustom }: StackServiceSelectProps) {
  const { t } = useTranslation('common');

  const { data: containers, isLoading } = useQuery({
    queryKey: ['stack-services-select', envId ?? '', stackName ?? ''],
    queryFn: () =>
      api
        .get<ContainerInfo[]>(
          `/stacks/${encodeURIComponent(stackName ?? '')}/containers`,
          envId ? { env: envId } : undefined,
        )
        .then((r) => r.data ?? []),
    staleTime: 30_000,
    enabled: !!envId && !!stackName,
  });

  const seen = new Map<string, number>();
  for (const container of containers ?? []) {
    const service = container.StackService || container.Labels?.[SERVICE_LABEL];
    if (!service) continue;
    seen.set(service, (seen.get(service) ?? 0) + 1);
  }
  const items: ResourcePickerItem[] = [...seen.entries()].map(([service, count]) => ({
    value: service,
    title: service,
    subtitle: t('workflows.serviceContainerCount', { count }),
  }));

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={isLoading}
      allowCustom={allowCustom}
      placeholder={t('workflows.selectService')}
      emptyLabel={t('workflows.noServicesFound')}
      blockedLabel={
        !envId ? t('workflows.selectEnvironmentFirst') : !stackName ? t('workflows.selectStackFirst') : undefined
      }
      items={items}
    />
  );
}
