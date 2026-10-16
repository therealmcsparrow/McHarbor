// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { apiClientPath, usePickerSource } from '@core/plugins/capabilities';
import type { PluginPickerSourceRef } from '@core/plugins/loader';
import { assertSuccess } from '@resources/utils/api-mutation';
import { usePermission } from '@core/auth/usePermission';
import { ResourcePicker } from './ResourcePicker';
import { pickerItems, pickerQuery } from '../utils/plugin-picker';

interface PluginSelectProps {
  value: string;
  onChange: (value: string) => void;
  source?: PluginPickerSourceRef;
  envId?: string;
  allowCustom?: boolean;
}

/**
 * The one picker behind every `plugin-select` field. The list, its item shape and
 * the key it is read with all come from the plugin's picker-source, so the host
 * never names the plugin's route; without the plugin the field stays visible,
 * disabled, and says why.
 */
export function PluginSelect({ value, onChange, source, envId, allowCustom }: PluginSelectProps) {
  const { t } = useTranslation('common');
  const capability = usePickerSource(source);
  const picker = capability?.picker;
  const allowed = usePermission(picker?.permission ?? []);

  const { data, isLoading } = useQuery({
    queryKey: ['plugin-select', capability?.plugin ?? '', capability?.target ?? '', envId ?? ''],
    enabled: Boolean(picker) && allowed,
    queryFn: async () => {
      const res = await api.get<unknown>(apiClientPath(picker!.path), pickerQuery(picker!, envId));
      return assertSuccess(res);
    },
    staleTime: 30_000,
  });

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={isLoading && Boolean(picker) && allowed}
      placeholder={t('workflows.pluginSelectPlaceholder')}
      emptyLabel={t('workflows.pluginSelectEmpty')}
      items={picker ? pickerItems(picker, data) : []}
      allowCustom={Boolean(picker) && allowCustom}
      permission={picker?.permission}
      unavailableReason={picker ? undefined : t('workflows.pluginSelectUnavailable')}
    />
  );
}
