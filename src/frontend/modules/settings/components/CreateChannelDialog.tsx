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
} from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useCreateChannel, type ChannelType } from '../hooks/useNotificationChannels';
import { SlackForm, type SlackFormData } from './SlackForm';
import { DiscordForm, type DiscordFormData } from './DiscordForm';
import { TeamsForm, type TeamsFormData } from './TeamsForm';
import { GotifyForm, type GotifyFormData } from './GotifyForm';
import { NtfyForm, type NtfyFormData } from './NtfyForm';
import { TelegramForm, type TelegramFormData } from './TelegramForm';
import { SignalForm, type SignalFormData } from './SignalForm';
import { WhatsAppForm, type WhatsAppFormData } from './WhatsAppForm';

type CreateChannelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const CHANNEL_TYPES: { type: ChannelType; icon: typeof IconBrandSlack; color: string }[] = [
  { type: 'slack', icon: IconBrandSlack, color: 'text-[#4A154B]' },
  { type: 'discord', icon: IconBrandDiscord, color: 'text-[#5865F2]' },
  { type: 'teams', icon: IconBrandTeams, color: 'text-[#6264A7]' },
  { type: 'gotify', icon: IconBell, color: 'text-primary' },
  { type: 'ntfy', icon: IconSend2, color: 'text-orange-400' },
  { type: 'telegram', icon: IconBrandTelegram, color: 'text-[#26A5E4]' },
  { type: 'signal', icon: IconMessageCircle, color: 'text-[#3A76F0]' },
  { type: 'whatsapp', icon: IconBrandWhatsapp, color: 'text-[#25D366]' },
];

const DEFAULT_SLACK: SlackFormData = { name: '', webhookUrl: '' };
const DEFAULT_DISCORD: DiscordFormData = { name: '', webhookUrl: '' };
const DEFAULT_TEAMS: TeamsFormData = { name: '', webhookUrl: '' };
const DEFAULT_GOTIFY: GotifyFormData = { name: '', serverUrl: '', token: '', priority: '' };
const DEFAULT_NTFY: NtfyFormData = { name: '', serverUrl: 'https://ntfy.sh', topic: '', token: '', username: '', password: '', priority: '' };
const DEFAULT_TELEGRAM: TelegramFormData = { name: '', token: '', chatId: '' };
const DEFAULT_SIGNAL: SignalFormData = { name: '', method: 'rest_api', serverUrl: '', senderNumber: '', recipients: '', username: '', password: '', token: '' };
const DEFAULT_WHATSAPP: WhatsAppFormData = { name: '', method: 'cloud_api', serverUrl: '', phoneNumberId: '', token: '', recipientPhone: '' };

export function CreateChannelDialog({ open, onOpenChange }: CreateChannelDialogProps) {
  const { t } = useTranslation('settings');
  const [step, setStep] = useState<'type' | 'config'>('type');
  const [channelType, setChannelType] = useState<ChannelType | null>(null);
  const [slackData, setSlackData] = useState(DEFAULT_SLACK);
  const [discordData, setDiscordData] = useState(DEFAULT_DISCORD);
  const [teamsData, setTeamsData] = useState(DEFAULT_TEAMS);
  const [gotifyData, setGotifyData] = useState(DEFAULT_GOTIFY);
  const [ntfyData, setNtfyData] = useState(DEFAULT_NTFY);
  const [telegramData, setTelegramData] = useState(DEFAULT_TELEGRAM);
  const [signalData, setSignalData] = useState(DEFAULT_SIGNAL);
  const [whatsappData, setWhatsappData] = useState(DEFAULT_WHATSAPP);
  const createChannel = useCreateChannel();

  function reset() {
    setStep('type');
    setChannelType(null);
    setSlackData(DEFAULT_SLACK);
    setDiscordData(DEFAULT_DISCORD);
    setTeamsData(DEFAULT_TEAMS);
    setGotifyData(DEFAULT_GOTIFY);
    setNtfyData(DEFAULT_NTFY);
    setTelegramData(DEFAULT_TELEGRAM);
    setSignalData(DEFAULT_SIGNAL);
    setWhatsappData(DEFAULT_WHATSAPP);
  }

  function handleOpenChange(value: boolean) {
    if (!value) reset();
    onOpenChange(value);
  }

  function handleSelectType(type: ChannelType) {
    setChannelType(type);
    setStep('config');
  }

  function handleCreate() {
    if (!channelType) return;

    const base = { channelType };

    const inputMap: Record<ChannelType, Record<string, unknown>> = {
      slack: { name: slackData.name, webhookUrl: slackData.webhookUrl },
      discord: { name: discordData.name, webhookUrl: discordData.webhookUrl },
      teams: { name: teamsData.name, webhookUrl: teamsData.webhookUrl },
      gotify: { name: gotifyData.name, serverUrl: gotifyData.serverUrl, token: gotifyData.token, priority: gotifyData.priority || undefined },
      ntfy: { name: ntfyData.name, serverUrl: ntfyData.serverUrl, topic: ntfyData.topic, token: ntfyData.token || undefined, username: ntfyData.username || undefined, password: ntfyData.password || undefined, priority: ntfyData.priority || undefined },
      telegram: { name: telegramData.name, token: telegramData.token, chatId: telegramData.chatId },
      signal: { name: signalData.name, method: signalData.method, serverUrl: signalData.serverUrl, senderNumber: signalData.senderNumber || undefined, recipients: signalData.recipients, username: signalData.username || undefined, password: signalData.password || undefined, token: signalData.token || undefined },
      whatsapp: { name: whatsappData.name, method: whatsappData.method, serverUrl: whatsappData.serverUrl || undefined, phoneNumberId: whatsappData.phoneNumberId || undefined, token: whatsappData.token, recipientPhone: whatsappData.recipientPhone },
    };

    createChannel.mutate(
      { ...base, ...inputMap[channelType] } as Parameters<typeof createChannel.mutate>[0],
      { onSuccess: () => handleOpenChange(false) },
    );
  }

  const isValid = channelType ? getIsValid(channelType) : false;

  function getIsValid(type: ChannelType): boolean {
    switch (type) {
      case 'slack': return !!(slackData.name && slackData.webhookUrl);
      case 'discord': return !!(discordData.name && discordData.webhookUrl);
      case 'teams': return !!(teamsData.name && teamsData.webhookUrl);
      case 'gotify': return !!(gotifyData.name && gotifyData.serverUrl && gotifyData.token);
      case 'ntfy': return !!(ntfyData.name && ntfyData.serverUrl && ntfyData.topic);
      case 'telegram': return !!(telegramData.name && telegramData.token && telegramData.chatId);
      case 'signal': {
        if (!signalData.name || !signalData.serverUrl || !signalData.recipients) return false;
        if (signalData.method === 'bot') return !!signalData.token;
        return !!signalData.senderNumber;
      }
      case 'whatsapp': {
        if (!whatsappData.name || !whatsappData.token || !whatsappData.recipientPhone) return false;
        if (whatsappData.method === 'gateway' || whatsappData.method === 'saas') return !!whatsappData.serverUrl;
        if (whatsappData.method === 'business') return !!(whatsappData.serverUrl && whatsappData.phoneNumberId);
        return !!whatsappData.phoneNumberId; // cloud_api
      }
      default: return false;
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {step === 'type' ? t('communications.selectType') : t('communications.configuration')}
          </DialogTitle>
          <DialogDescription>
            {step === 'type'
              ? t('communications.selectTypeDescription')
              : channelType
                ? t(`communications.type${channelType.charAt(0).toUpperCase() + channelType.slice(1)}`)
                : ''}
          </DialogDescription>
        </DialogHeader>

        <div>
          {step === 'type' && (
            <div className="grid grid-cols-4 gap-3">
              {CHANNEL_TYPES.map(({ type, icon: Icon, color }) => (
                <Button
                  key={type}
                  variant="outline"
                  onClick={() => handleSelectType(type)}
                  className="flex h-auto flex-col items-center gap-3 p-4"
                >
                  <Icon className={`size-8 ${color}`} />
                  <span className="text-xs font-medium text-foreground">
                    {t(`communications.type${type.charAt(0).toUpperCase() + type.slice(1)}`)}
                  </span>
                </Button>
              ))}
            </div>
          )}

          {step === 'config' && channelType === 'slack' && <SlackForm data={slackData} onChange={setSlackData} />}
          {step === 'config' && channelType === 'discord' && <DiscordForm data={discordData} onChange={setDiscordData} />}
          {step === 'config' && channelType === 'teams' && <TeamsForm data={teamsData} onChange={setTeamsData} />}
          {step === 'config' && channelType === 'gotify' && <GotifyForm data={gotifyData} onChange={setGotifyData} />}
          {step === 'config' && channelType === 'ntfy' && <NtfyForm data={ntfyData} onChange={setNtfyData} />}
          {step === 'config' && channelType === 'telegram' && <TelegramForm data={telegramData} onChange={setTelegramData} />}
          {step === 'config' && channelType === 'signal' && <SignalForm data={signalData} onChange={setSignalData} />}
          {step === 'config' && channelType === 'whatsapp' && <WhatsAppForm data={whatsappData} onChange={setWhatsappData} />}
        </div>

        {step === 'config' && (
          <DialogFooter>
            <Button variant="outline" onClick={() => setStep('type')}>
              {t('common:back', 'Back')}
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!isValid || createChannel.isPending}
            >
              {createChannel.isPending ? '...' : t('communications.addChannel')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
