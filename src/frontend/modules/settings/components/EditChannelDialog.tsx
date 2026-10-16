// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconHelpCircle } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { PluginFormOutlet } from '@core/plugins/PluginFormOutlet';
import { SetupGuidePanel } from '@resources/components/SetupGuidePanel';
import { useUpdateChannel, type CommunicationChannel } from '../hooks/useNotificationChannels';
import { PERM } from '@core/auth/permissionKeys';
import { useItsmChannels } from '../hooks/useItsm';
import { ChannelWebhookPanel } from './ChannelWebhookPanel';

type EditChannelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: CommunicationChannel;
};

export function EditChannelDialog({ open: isOpen, onOpenChange, channel }: EditChannelDialogProps) {
  const { t } = useTranslation('settings');
  const itsmChannels = useItsmChannels(isOpen);
  const updateChannel = useUpdateChannel();
  const [helpOpen, setHelpOpen] = useState(false);
  const [pluginConfig, setPluginConfig] = useState<Record<string, string>>({});
  const [pluginErrors, setPluginErrors] = useState<Record<string, string>>({});

  const [seededFor, setSeededFor] = useState<{ isOpen: boolean; channel: CommunicationChannel | null }>({
    isOpen: false,
    channel: null,
  });
  if (seededFor.isOpen !== isOpen || seededFor.channel !== channel) {
    setSeededFor({ isOpen, channel });
    if (isOpen) seedFromChannel();
  }

  function seedFromChannel() {
    setHelpOpen(false);
    setPluginErrors({});
    // The host returns non-secret config keys in `config` and promotes a
    // few well-known ones to top-level fields; seed the snake_case key
    // from the top-level field when `config` lacks it so plugin forms
    // that read `value.chat_id` etc. start populated. Secrets are never
    // returned — the plugin form renders them optional in edit mode and
    // the backend keeps the stored value when the key is blank.
    const seeded: Record<string, string> = { ...channel.config };
    const promoted: Array<[string, string | undefined]> = [
      ['server_url', channel.serverUrl],
      ['topic', channel.topic],
      ['chat_id', channel.chatId],
      ['phone_number_id', channel.phoneNumberId],
      ['recipient_phone', channel.recipientPhone],
      ['sender_number', channel.senderNumber],
      ['recipients', channel.recipients],
      ['username', channel.username],
      ['priority', channel.priority],
    ];
    for (const [key, val] of promoted) {
      if (!seeded[key] && val) seeded[key] = val;
    }
    setPluginConfig({ name: channel.name, ...seeded });
  }

  function handleSave() {
    const base = { id: channel.id };

    const { name: channelName, ...config } = pluginConfig;
    const cleanConfig = { ...config };
    if (!cleanConfig.password) {
      delete cleanConfig.password;
    }
    updateChannel.mutate({ ...base, name: channelName, config: cleanConfig }, { onSuccess: () => onOpenChange(false) });
  }

  const typeLabel = channel.channelType;

  function handlePluginError(field: string, message: string | null) {
    setPluginErrors((current) => {
      const next = { ...current };
      if (message) {
        next[field] = message;
      } else {
        delete next[field];
      }
      return next;
    });
  }

  function renderGuidePanel() {
    return (
      <SetupGuidePanel>
        <PluginFormOutlet
          resource="channel"
          kind="setup_guide"
          keyName={channel.channelType}
          value={pluginConfig}
          onChange={setPluginConfig}
          i18nPrefix="commSmtp"
        />
      </SetupGuidePanel>
    );
  }

  const isValid = !!pluginConfig.name && Object.keys(pluginErrors).length === 0;
  const itsmChannel = itsmChannels.data?.find((c) => c.id === channel.id);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('communications.editChannel')}</DialogTitle>
          <DialogDescription>{typeLabel}</DialogDescription>
        </DialogHeader>

        <div>
          <PluginFormOutlet
            resource="channel"
            kind="config_form"
            keyName={channel.channelType}
            value={pluginConfig}
            onChange={setPluginConfig}
            onError={handlePluginError}
            mode="edit"
            i18nPrefix="commSmtp"
          />

          {helpOpen && renderGuidePanel()}
          {itsmChannel && <ChannelWebhookPanel channel={itsmChannel} />}
        </div>

        <DialogFooter className="flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:justify-between">
          <Button variant="ghost" onClick={() => setHelpOpen((current) => !current)} aria-expanded={helpOpen}>
            <IconHelpCircle className="size-4" />
            {helpOpen ? t('communications.hideSetupGuide') : t('communications.showSetupGuide')}
          </Button>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('common:actions.cancel')}
            </Button>
            <Button
              permission={PERM.COMMUNICATIONS_MANAGE}
              onClick={handleSave}
              disabled={!isValid || updateChannel.isPending}
            >
              {updateChannel.isPending ? '...' : t('common:actions.save')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
