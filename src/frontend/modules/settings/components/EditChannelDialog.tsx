// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useUpdateChannel, type CommunicationChannel } from '../hooks/useNotificationChannels';
import { SlackForm, type SlackFormData } from './SlackForm';
import { DiscordForm, type DiscordFormData } from './DiscordForm';
import { TeamsForm, type TeamsFormData } from './TeamsForm';
import { GotifyForm, type GotifyFormData } from './GotifyForm';
import { NtfyForm, type NtfyFormData } from './NtfyForm';
import { TelegramForm, type TelegramFormData } from './TelegramForm';
import { SignalForm, type SignalFormData } from './SignalForm';
import { WhatsAppForm, type WhatsAppFormData } from './WhatsAppForm';

type EditChannelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  channel: CommunicationChannel;
};

export function EditChannelDialog({ open, onOpenChange, channel }: EditChannelDialogProps) {
  const { t } = useTranslation('settings');
  const updateChannel = useUpdateChannel();

  const [slackData, setSlackData] = useState<SlackFormData>({ name: '', webhookUrl: '' });
  const [discordData, setDiscordData] = useState<DiscordFormData>({ name: '', webhookUrl: '' });
  const [teamsData, setTeamsData] = useState<TeamsFormData>({ name: '', webhookUrl: '' });
  const [gotifyData, setGotifyData] = useState<GotifyFormData>({ name: '', serverUrl: '', token: '', priority: '' });
  const [ntfyData, setNtfyData] = useState<NtfyFormData>({ name: '', serverUrl: '', topic: '', token: '', username: '', password: '', priority: '' });
  const [telegramData, setTelegramData] = useState<TelegramFormData>({ name: '', token: '', chatId: '' });
  const [signalData, setSignalData] = useState<SignalFormData>({ name: '', method: 'rest_api', serverUrl: '', senderNumber: '', recipients: '', username: '', password: '', token: '' });
  const [whatsappData, setWhatsappData] = useState<WhatsAppFormData>({ name: '', method: 'cloud_api', serverUrl: '', phoneNumberId: '', token: '', recipientPhone: '' });

  useEffect(() => {
    if (!open) return;

    switch (channel.channelType) {
      case 'slack':
        setSlackData({ name: channel.name, webhookUrl: '' });
        break;
      case 'discord':
        setDiscordData({ name: channel.name, webhookUrl: '' });
        break;
      case 'teams':
        setTeamsData({ name: channel.name, webhookUrl: '' });
        break;
      case 'gotify':
        setGotifyData({ name: channel.name, serverUrl: channel.serverUrl ?? '', token: '', priority: channel.priority ?? '' });
        break;
      case 'ntfy':
        setNtfyData({ name: channel.name, serverUrl: channel.serverUrl ?? '', topic: channel.topic ?? '', token: '', username: channel.username ?? '', password: '', priority: channel.priority ?? '' });
        break;
      case 'telegram':
        setTelegramData({ name: channel.name, token: '', chatId: channel.chatId ?? '' });
        break;
      case 'signal':
        setSignalData({ name: channel.name, method: (channel.method || 'rest_api') as SignalFormData['method'], serverUrl: channel.serverUrl ?? '', senderNumber: channel.senderNumber ?? '', recipients: channel.recipients ?? '', username: channel.username ?? '', password: '', token: '' });
        break;
      case 'whatsapp':
        setWhatsappData({ name: channel.name, method: (channel.method || 'cloud_api') as WhatsAppFormData['method'], serverUrl: channel.serverUrl ?? '', phoneNumberId: channel.phoneNumberId ?? '', token: '', recipientPhone: channel.recipientPhone ?? '' });
        break;
    }
  }, [open, channel]);

  function handleSave() {
    const base = { id: channel.id };

    switch (channel.channelType) {
      case 'slack':
        updateChannel.mutate({ ...base, name: slackData.name, webhookUrl: slackData.webhookUrl || undefined }, { onSuccess: () => onOpenChange(false) });
        break;
      case 'discord':
        updateChannel.mutate({ ...base, name: discordData.name, webhookUrl: discordData.webhookUrl || undefined }, { onSuccess: () => onOpenChange(false) });
        break;
      case 'teams':
        updateChannel.mutate({ ...base, name: teamsData.name, webhookUrl: teamsData.webhookUrl || undefined }, { onSuccess: () => onOpenChange(false) });
        break;
      case 'gotify':
        updateChannel.mutate({ ...base, name: gotifyData.name, serverUrl: gotifyData.serverUrl, token: gotifyData.token || undefined, priority: gotifyData.priority }, { onSuccess: () => onOpenChange(false) });
        break;
      case 'ntfy':
        updateChannel.mutate({ ...base, name: ntfyData.name, serverUrl: ntfyData.serverUrl, topic: ntfyData.topic, token: ntfyData.token || undefined, username: ntfyData.username, password: ntfyData.password || undefined, priority: ntfyData.priority }, { onSuccess: () => onOpenChange(false) });
        break;
      case 'telegram':
        updateChannel.mutate({ ...base, name: telegramData.name, token: telegramData.token || undefined, chatId: telegramData.chatId }, { onSuccess: () => onOpenChange(false) });
        break;
      case 'signal':
        updateChannel.mutate({ ...base, name: signalData.name, method: signalData.method, serverUrl: signalData.serverUrl, senderNumber: signalData.senderNumber || undefined, recipients: signalData.recipients, username: signalData.username || undefined, password: signalData.password || undefined, token: signalData.token || undefined }, { onSuccess: () => onOpenChange(false) });
        break;
      case 'whatsapp':
        updateChannel.mutate({ ...base, name: whatsappData.name, method: whatsappData.method, serverUrl: whatsappData.serverUrl || undefined, phoneNumberId: whatsappData.phoneNumberId || undefined, token: whatsappData.token || undefined, recipientPhone: whatsappData.recipientPhone }, { onSuccess: () => onOpenChange(false) });
        break;
    }
  }

  const typeLabel = t(`communications.type${channel.channelType.charAt(0).toUpperCase() + channel.channelType.slice(1)}`);

  const isValid = (() => {
    switch (channel.channelType) {
      case 'slack': return !!slackData.name;
      case 'discord': return !!discordData.name;
      case 'teams': return !!teamsData.name;
      case 'gotify': return !!(gotifyData.name && gotifyData.serverUrl);
      case 'ntfy': return !!(ntfyData.name && ntfyData.serverUrl && ntfyData.topic);
      case 'telegram': return !!(telegramData.name && telegramData.chatId);
      case 'signal': {
        if (!signalData.name || !signalData.serverUrl || !signalData.recipients) return false;
        if (signalData.method === 'bot') return !!signalData.token;
        return !!signalData.senderNumber;
      }
      case 'whatsapp': {
        if (!whatsappData.name || !whatsappData.recipientPhone) return false;
        if (whatsappData.method === 'gateway' || whatsappData.method === 'saas') return !!whatsappData.serverUrl;
        if (whatsappData.method === 'business') return !!(whatsappData.serverUrl && whatsappData.phoneNumberId);
        return !!whatsappData.phoneNumberId; // cloud_api
      }
      default: return false;
    }
  })();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('communications.editChannel')}</DialogTitle>
          <DialogDescription>{typeLabel}</DialogDescription>
        </DialogHeader>

        <div>
          {channel.channelType === 'slack' && <SlackForm data={slackData} onChange={setSlackData} isEdit />}
          {channel.channelType === 'discord' && <DiscordForm data={discordData} onChange={setDiscordData} isEdit />}
          {channel.channelType === 'teams' && <TeamsForm data={teamsData} onChange={setTeamsData} isEdit />}
          {channel.channelType === 'gotify' && <GotifyForm data={gotifyData} onChange={setGotifyData} isEdit />}
          {channel.channelType === 'ntfy' && <NtfyForm data={ntfyData} onChange={setNtfyData} isEdit />}
          {channel.channelType === 'telegram' && <TelegramForm data={telegramData} onChange={setTelegramData} isEdit />}
          {channel.channelType === 'signal' && <SignalForm data={signalData} onChange={setSignalData} isEdit />}
          {channel.channelType === 'whatsapp' && <WhatsAppForm data={whatsappData} onChange={setWhatsappData} isEdit />}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common:cancel', 'Cancel')}
          </Button>
          <Button
            onClick={handleSave}
            disabled={!isValid || updateChannel.isPending}
          >
            {updateChannel.isPending ? '...' : t('common:save', 'Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
