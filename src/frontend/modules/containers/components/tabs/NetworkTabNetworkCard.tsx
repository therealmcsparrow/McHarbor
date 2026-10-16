// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import type { TFunction } from 'i18next';
import { IconCheck, IconPencil, IconUnlink, IconX } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { PERM } from '@core/auth/permissionKeys';
import { InfoRow } from '@resources/components/ui/InfoRow';
import type { ContainerInspect } from '@core/types/docker';
import { NetworkEndpointSettingsFields, type NetworkEndpointSettingsDraft } from './NetworkEndpointSettingsFields';

export type ContainerNetworkEntry = [string, NonNullable<ContainerInspect['NetworkSettings']>['Networks'][string]];

type NetworkTabNetworkCardProps = {
  networkName: string;
  network: ContainerNetworkEntry[1];
  liveActionsEnabled: boolean;
  isEditing: boolean;
  editDraft: NetworkEndpointSettingsDraft;
  connectPending: boolean;
  disconnectPending: boolean;
  onStartEdit: () => void;
  onCancelEdit: () => void;
  onApplyEdit: () => void;
  onEditDraftChange: (patch: Partial<NetworkEndpointSettingsDraft>) => void;
  onDisconnect: () => void;
  t: TFunction<'containers'>;
};

// One connected network on the container network tab: its endpoint details,
// or the endpoint-settings editor while the operator is changing them.
export function NetworkTabNetworkCard({
  networkName,
  network,
  liveActionsEnabled,
  isEditing,
  editDraft,
  connectPending,
  disconnectPending,
  onStartEdit,
  onCancelEdit,
  onApplyEdit,
  onEditDraftChange,
  onDisconnect,
  t,
}: NetworkTabNetworkCardProps) {
  return (
    <div className="rounded-md border border-border p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <Badge variant="secondary">{networkName}</Badge>
        {liveActionsEnabled && (
          <div className="flex gap-2">
            {isEditing ? (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  permission={PERM.CONTAINERS_MANAGE}
                  onClick={onApplyEdit}
                  disabled={connectPending}
                >
                  <IconCheck className="mr-1 size-3.5" />
                  {t('networkTab.applyNetworkSettings')}
                </Button>
                <Button variant="ghost" size="sm" onClick={onCancelEdit}>
                  <IconX className="mr-1 size-3.5" />
                  {t('networkTab.cancelNetworkEdit')}
                </Button>
              </>
            ) : (
              <Button variant="ghost" size="sm" permission={PERM.CONTAINERS_MANAGE} onClick={onStartEdit}>
                <IconPencil className="mr-1 size-3.5" />
                {t('networkTab.editNetworkSettings')}
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              permission={PERM.CONTAINERS_MANAGE}
              onClick={onDisconnect}
              disabled={disconnectPending}
              className="text-danger hover:text-danger"
            >
              <IconUnlink className="mr-1 size-3.5" />
              {t('actions.disconnect')}
            </Button>
          </div>
        )}
      </div>

      {isEditing ? (
        <div className="space-y-3">
          <NetworkEndpointSettingsFields
            draft={editDraft}
            disabled={connectPending}
            onChange={onEditDraftChange}
            t={t}
          />
          <p className="text-xs text-muted-foreground">{t('networkTab.reconnectNotice')}</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-x-8 sm:grid-cols-2">
          <InfoRow label={t('networkTab.ipAddress')}>{network?.IPAddress || '-'}</InfoRow>
          <InfoRow label={t('networkTab.gateway')}>{network?.Gateway || '-'}</InfoRow>
          <InfoRow label={t('networkTab.macAddress')}>{network?.MacAddress || '-'}</InfoRow>
          <InfoRow label={t('networkTab.prefixLength')}>{network?.IPPrefixLen ?? '-'}</InfoRow>
          {(network?.Aliases?.length ?? 0) > 0 && (
            <InfoRow label={t('networkTab.aliases')}>{network.Aliases?.join(', ')}</InfoRow>
          )}
          {network?.GlobalIPv6Address && (
            <>
              <InfoRow label={t('networkTab.ipv6Address')}>{network.GlobalIPv6Address}</InfoRow>
              <InfoRow label={t('networkTab.ipv6Gateway')}>{network.IPv6Gateway || '-'}</InfoRow>
            </>
          )}
        </div>
      )}
    </div>
  );
}
