// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  IconBrandSlack,
  IconBrandDiscord,
  IconBrandTeams,
  IconBell,
  IconSend2,
  IconBrandTelegram,
  IconMessageCircle,
  IconBrandWhatsapp,
  IconTrash,
  IconPencil,
  IconSend,
  IconStar,
  IconStarFilled,
} from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';
import { ConfirmDialog } from '@resources/components/ui/ConfirmDialog';
import {
  useUpdateChannel,
  useDeleteChannel,
  useSetDefaultChannel,
  type CommunicationChannel,
  type ChannelType,
} from '../hooks/useNotificationChannels';

type ChannelCardProps = {
  channel: CommunicationChannel;
  onEdit: (channel: CommunicationChannel) => void;
  onTest: (channel: CommunicationChannel) => void;
};

const TYPE_CONFIG: Record<ChannelType, { icon: typeof IconBrandSlack; color: string; label: string }> = {
  slack: { icon: IconBrandSlack, color: 'text-[#4A154B]', label: 'Slack' },
  discord: { icon: IconBrandDiscord, color: 'text-[#5865F2]', label: 'Discord' },
  teams: { icon: IconBrandTeams, color: 'text-[#6264A7]', label: 'Teams' },
  gotify: { icon: IconBell, color: 'text-primary', label: 'Gotify' },
  ntfy: { icon: IconSend2, color: 'text-orange-400', label: 'ntfy' },
  telegram: { icon: IconBrandTelegram, color: 'text-[#26A5E4]', label: 'Telegram' },
  signal: { icon: IconMessageCircle, color: 'text-[#3A76F0]', label: 'Signal' },
  whatsapp: { icon: IconBrandWhatsapp, color: 'text-[#25D366]', label: 'WhatsApp' },
};

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

  const config = TYPE_CONFIG[channel.channelType];
  const Icon = config.icon;

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
      <div className="flex items-center gap-4 rounded-lg border border-border bg-card p-4">
        <Icon className={`size-8 shrink-0 ${config.color}`} />

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h4 className="truncate text-sm font-medium text-foreground">
              {channel.name}
            </h4>
            {channel.isDefault && (
              <Badge variant="default">{t('communications.default')}</Badge>
            )}
            <Badge variant={channel.enabled ? 'default' : 'secondary'}>
              {channel.enabled ? t('communications.enabled') : t('communications.disabled')}
            </Badge>
          </div>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {config.label}
            {channel.method ? (() => {
              const key = METHOD_LABEL_KEYS[channel.method as string];
              return key ? <> &middot; {t(key)}</> : null;
            })() : null}
          </p>
        </div>

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={channel.isDefault ? t('communications.isDefault') : t('communications.setDefault')}
            onClick={handleSetDefault}
            disabled={channel.isDefault || setDefault.isPending}
          >
            {channel.isDefault ? (
              <IconStarFilled className="size-4 text-amber-400" />
            ) : (
              <IconStar className="size-4" />
            )}
          </Button>
          <Switch
            checked={channel.enabled}
            onCheckedChange={handleToggle}
            disabled={updateChannel.isPending}
          />
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('communications.testChannel')}
            onClick={() => onTest(channel)}
          >
            <IconSend className="size-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon-sm"
            aria-label={t('communications.editChannel')}
            onClick={() => onEdit(channel)}
          >
            <IconPencil className="size-4" />
          </Button>
          <Button
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
