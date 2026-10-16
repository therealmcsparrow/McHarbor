// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconTrash, IconPencil, IconSend, IconStar, IconStarFilled, IconPuzzle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useUpdateChannel,
  useDeleteChannel,
  useSetDefaultChannel,
  type CommunicationChannel,
} from '../hooks/useNotificationChannels';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import { useNotificationAdapters } from '../hooks/useCommunicationAdapters';
import { PERM } from '@core/auth/permissionKeys';

type ChannelCardProps = {
  channel: CommunicationChannel;
  onEdit: (channel: CommunicationChannel) => void;
  onTest: (channel: CommunicationChannel) => void;
};

// The icon and label a channel renders come from the owning plugin's
// adapter spec (its icons/plugin.svg and DisplayName) so a channel looks
// the same in the list as it did in the "Add Communication" picker. The
// host knows no channel brands itself: when the plugin is uninstalled or
// the adapter catalog has not loaded yet, the tile falls back to a
// generic icon and the capitalised channel type.
function fallbackLabel(channelType: string): string {
  return channelType.charAt(0).toUpperCase() + channelType.slice(1);
}

// The setup method is a host-stored field on the channel row, so the host has
// to be able to name it. The labels stay brand-free — "REST API" rather than a
// messenger's name in front of it — because the brand next to them already
// comes from the owning plugin's adapter DisplayName.
const METHOD_LABEL_KEYS: Record<string, string> = {
  cloud_api: 'communications.methodCloudApi',
  gateway: 'communications.methodGateway',
  business: 'communications.methodBusiness',
  saas: 'communications.methodSaas',
  rest_api: 'communications.methodRestApi',
  bot: 'communications.methodBot',
  signald: 'communications.methodSignald',
  simple: 'communications.methodSimple',
};

export function ChannelCard({ channel, onEdit, onTest }: ChannelCardProps) {
  const { t } = useTranslation('settings');
  const [deleteOpen, setDeleteOpen] = useState(false);
  const updateChannel = useUpdateChannel();
  const deleteChannel = useDeleteChannel();
  const setDefault = useSetDefaultChannel();

  // Adapter catalog is already in the query cache — the picker and
  // the edit dialog both read it, so this is a cache hit in practice.
  const { data: adapters } = useNotificationAdapters();
  const adapter = adapters?.[channel.channelType];

  const label = adapter?.DisplayName || fallbackLabel(channel.channelType);

  function handleToggle(enabled: boolean) {
    updateChannel.mutate({ id: channel.id, enabled });
  }

  function handleSetDefault() {
    setDefault.mutate(channel.id);
  }

  function handleDelete() {
    deleteChannel.mutate(channel.id, {
      onSuccess: () => setDeleteOpen(false),
    });
  }

  return (
    <>
      <div className="mc-glass flex items-center gap-4 rounded-lg p-4">
        {adapter?.Slug && adapter.Icon ? (
          <PluginAssetImage slug={adapter.Slug} icon={adapter.Icon} className="size-8 shrink-0 object-contain" />
        ) : (
          <IconPuzzle className="size-8 shrink-0 text-muted-foreground" />
        )}

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h4 className="truncate text-sm font-medium text-foreground">{channel.name}</h4>
            {channel.isDefault && <Badge variant="default">{t('communications.default')}</Badge>}
            <Badge variant={channel.enabled ? 'default' : 'secondary'}>
              {channel.enabled ? t('communications.enabled') : t('communications.disabled')}
            </Badge>
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {label}
            {channel.method
              ? (() => {
                  const key = METHOD_LABEL_KEYS[channel.method as string];
                  return key ? <> &middot; {t(key)}</> : null;
                })()
              : null}
          </p>
        </div>

        <div className="flex items-center gap-1">
          <Button
            permission={PERM.COMMUNICATIONS_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={channel.isDefault ? t('communications.isDefault') : t('communications.setDefault')}
            onClick={handleSetDefault}
            disabled={channel.isDefault || setDefault.isPending}
          >
            {channel.isDefault ? <IconStarFilled className="size-4 text-warning" /> : <IconStar className="size-4" />}
          </Button>
          <Switch checked={channel.enabled} onCheckedChange={handleToggle} disabled={updateChannel.isPending} />
          <Button
            permission={PERM.COMMUNICATIONS_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={t('communications.testChannel')}
            onClick={() => onTest(channel)}
          >
            <IconSend className="size-4" />
          </Button>
          <Button
            permission={PERM.COMMUNICATIONS_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={t('communications.editChannel')}
            onClick={() => onEdit(channel)}
          >
            <IconPencil className="size-4" />
          </Button>
          <Button
            permission={PERM.COMMUNICATIONS_MANAGE}
            variant="ghost"
            size="icon-sm"
            aria-label={t('communications.deleteChannel')}
            onClick={() => setDeleteOpen(true)}
          >
            <IconTrash className="size-4 text-destructive" />
          </Button>
        </div>
      </div>

      <ConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title={t('communications.confirmDeleteTitle')}
        description={t('communications.confirmDeleteDescription')}
        onConfirm={handleDelete}
        loading={deleteChannel.isPending}
        variant="destructive"
      />
    </>
  );
}
