// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { ResourcePicker } from './ResourcePicker';

type StackItem = {
  id: string;
  name: string;
  status: string;
  type: string;
  services?: unknown[];
};

interface StackSelectProps {
  value: string;
  onChange: (value: string) => void;
  envId?: string;
  allowCustom?: boolean;
}

/** Picks a Compose stack by name from the selected environment. */
export function StackSelect({ value, onChange, envId, allowCustom }: StackSelectProps) {
  const { t } = useTranslation('common');

  const { data: stacks, isLoading } = useQuery({
    queryKey: ['stacks-select', envId ?? ''],
    queryFn: () => api.get<StackItem[]>('/stacks', envId ? { env: envId } : undefined).then((r) => r.data ?? []),
    staleTime: 30_000,
    enabled: !!envId,
  });

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={isLoading}
      allowCustom={allowCustom}
      placeholder={t('workflows.selectStack')}
      emptyLabel={t('workflows.noStacksFound')}
      blockedLabel={envId ? undefined : t('workflows.selectEnvironmentFirst')}
      items={(stacks ?? []).map((stack) => ({
        value: stack.name,
        title: stack.name,
        subtitle: `${stack.type} - ${stack.services?.length ?? 0} ${t('workflows.services')}`,
        badge: stack.status,
        tone: stack.status === 'running' ? 'success' : 'default',
      }))}
    />
  );
}
