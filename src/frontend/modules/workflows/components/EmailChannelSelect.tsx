// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useQuery } from '@tanstack/react-query';
import { api } from '@core/api/client';
import { PERM } from '@core/auth/permissionKeys';
import { usePermission } from '@core/auth/usePermission';
import { useEmailChannelTypes } from '@core/plugins/capabilities';
import { assertSuccess } from '@resources/utils/api-mutation';
import { ResourcePicker } from './ResourcePicker';

type ChannelRow = {
  id: string;
  name: string;
  channelType: string;
  isDefault: boolean;
  enabled: boolean;
};

interface EmailChannelSelectProps {
  value: string;
  onChange: (value: string) => void;
}

/**
 * The server field of the send-email node. Which channels can carry an email is each comm
 * plugin's own email-channel declaration, so the host names no provider; with no such
 * plugin loaded the field stays visible, disabled, and says why.
 */
export function EmailChannelSelect({ value, onChange }: EmailChannelSelectProps) {
  const { t } = useTranslation('common');
  const emailTypes = useEmailChannelTypes();
  const allowed = usePermission(PERM.COMMUNICATIONS_VIEW);
  const { data, isLoading } = useQuery({
    queryKey: ['communication-channels', 'workflow-email'],
    enabled: allowed && emailTypes.length > 0,
    queryFn: () => api.get<ChannelRow[]>('/communication-channels').then(assertSuccess),
    staleTime: 30_000,
  });

  const items = (data ?? [])
    .filter((channel) => channel.enabled && emailTypes.includes(channel.channelType))
    .map((channel) => ({
      value: channel.id,
      title: channel.name,
      subtitle: channel.channelType,
      badge: channel.isDefault ? t('workflows.defaultTransport') : undefined,
    }));

  return (
    <ResourcePicker
      value={value}
      onChange={onChange}
      isLoading={allowed && isLoading && emailTypes.length > 0}
      placeholder={t('workflows.selectCommunicationChannel')}
      emptyLabel={t('workflows.noCommunicationChannelsFound')}
      clearLabel={t('workflows.useDefaultEmailChannel')}
      items={items}
      permission={PERM.COMMUNICATIONS_VIEW}
      unavailableReason={emailTypes.length === 0 ? t('workflows.emailChannelUnavailable') : undefined}
    />
  );
}
