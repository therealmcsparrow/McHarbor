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
  IconMail,
  IconBrandWindows,
  IconBrandGoogle,
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
import { useCreateEmailServer } from '../hooks/useEmailServers';
import { SlackForm, type SlackFormData } from './SlackForm';
import { DiscordForm, type DiscordFormData } from './DiscordForm';
import { TeamsForm, type TeamsFormData } from './TeamsForm';
import { GotifyForm, type GotifyFormData } from './GotifyForm';
import { NtfyForm, type NtfyFormData } from './NtfyForm';
import { TelegramForm, type TelegramFormData } from './TelegramForm';
import { SignalForm, type SignalFormData } from './SignalForm';
import { WhatsAppForm, type WhatsAppFormData } from './WhatsAppForm';
import { SMTPForm, type SMTPFormData } from './SMTPForm';
import { ExchangeForm, type ExchangeFormData } from './ExchangeForm';
import { GmailForm, type GmailFormData } from './GmailForm';

type CreateChannelDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

type EmailCreateType = 'email_smtp' | 'email_exchange' | 'email_google';
type CommunicationCreateType = ChannelType | EmailCreateType;

const COMMUNICATION_TYPES: {
  type: CommunicationCreateType;
  icon: typeof IconBrandSlack;
  color: string;
  labelKey: string;
  descriptionKey: string;
}[] = [
  { type: 'email_smtp', icon: IconMail, color: 'text-primary', labelKey: 'communications.typeSmtp', descriptionKey: 'communications.typeSmtpDescription' },
  { type: 'email_exchange', icon: IconBrandWindows, color: 'text-[#0078D4]', labelKey: 'communications.typeExchange', descriptionKey: 'communications.typeExchangeDescription' },
  { type: 'email_google', icon: IconBrandGoogle, color: 'text-[#4285F4]', labelKey: 'communications.typeGoogle', descriptionKey: 'communications.typeGoogleDescription' },
  { type: 'slack', icon: IconBrandSlack, color: 'text-[#4A154B]', labelKey: 'communications.typeSlack', descriptionKey: 'communications.typeSlackDescription' },
  { type: 'discord', icon: IconBrandDiscord, color: 'text-[#5865F2]', labelKey: 'communications.typeDiscord', descriptionKey: 'communications.typeDiscordDescription' },
  { type: 'teams', icon: IconBrandTeams, color: 'text-[#6264A7]', labelKey: 'communications.typeTeams', descriptionKey: 'communications.typeTeamsDescription' },
  { type: 'gotify', icon: IconBell, color: 'text-primary', labelKey: 'communications.typeGotify', descriptionKey: 'communications.typeGotifyDescription' },
  { type: 'ntfy', icon: IconSend2, color: 'text-orange-400', labelKey: 'communications.typeNtfy', descriptionKey: 'communications.typeNtfyDescription' },
  { type: 'telegram', icon: IconBrandTelegram, color: 'text-[#26A5E4]', labelKey: 'communications.typeTelegram', descriptionKey: 'communications.typeTelegramDescription' },
  { type: 'signal', icon: IconMessageCircle, color: 'text-[#3A76F0]', labelKey: 'communications.typeSignal', descriptionKey: 'communications.typeSignalDescription' },
  { type: 'whatsapp', icon: IconBrandWhatsapp, color: 'text-[#25D366]', labelKey: 'communications.typeWhatsapp', descriptionKey: 'communications.typeWhatsappDescription' },
];

const DEFAULT_SMTP: SMTPFormData = { name: '', host: '', port: 587, encryption: 'starttls', authMethod: 'plain', username: '', password: '', fromAddress: '', fromName: '' };
const DEFAULT_EXCHANGE: ExchangeFormData = { name: '', clientId: '', clientSecret: '', tenantId: '', fromAddress: '', fromName: '' };
const DEFAULT_GMAIL: GmailFormData = { name: '', clientId: '', clientSecret: '', fromAddress: '', fromName: '' };
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
  const [communicationType, setCommunicationType] = useState<CommunicationCreateType | null>(null);
  const [smtpData, setSmtpData] = useState(DEFAULT_SMTP);
  const [exchangeData, setExchangeData] = useState(DEFAULT_EXCHANGE);
  const [gmailData, setGmailData] = useState(DEFAULT_GMAIL);
  const [slackData, setSlackData] = useState(DEFAULT_SLACK);
  const [discordData, setDiscordData] = useState(DEFAULT_DISCORD);
  const [teamsData, setTeamsData] = useState(DEFAULT_TEAMS);
  const [gotifyData, setGotifyData] = useState(DEFAULT_GOTIFY);
  const [ntfyData, setNtfyData] = useState(DEFAULT_NTFY);
  const [telegramData, setTelegramData] = useState(DEFAULT_TELEGRAM);
  const [signalData, setSignalData] = useState(DEFAULT_SIGNAL);
  const [whatsappData, setWhatsappData] = useState(DEFAULT_WHATSAPP);
  const createChannel = useCreateChannel();
  const createEmailServer = useCreateEmailServer();

  function reset() {
    setStep('type');
    setCommunicationType(null);
    setSmtpData(DEFAULT_SMTP);
    setExchangeData(DEFAULT_EXCHANGE);
    setGmailData(DEFAULT_GMAIL);
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

  function handleSelectType(type: CommunicationCreateType) {
    setCommunicationType(type);
    setStep('config');
  }

  function handleCreate() {
    if (!communicationType) return;

    if (communicationType === 'email_smtp') {
      createEmailServer.mutate({
        name: smtpData.name,
        serverType: 'smtp',
        host: smtpData.host,
        port: smtpData.port,
        encryption: smtpData.encryption,
        authMethod: smtpData.authMethod,
        username: smtpData.username,
        password: smtpData.password,
        fromAddress: smtpData.fromAddress,
        fromName: smtpData.fromName,
      }, { onSuccess: () => handleOpenChange(false) });
      return;
    }

    if (communicationType === 'email_exchange') {
      createEmailServer.mutate({
        name: exchangeData.name,
        serverType: 'exchange',
        clientId: exchangeData.clientId,
        clientSecret: exchangeData.clientSecret,
        tenantId: exchangeData.tenantId,
        fromAddress: exchangeData.fromAddress,
        fromName: exchangeData.fromName,
      }, { onSuccess: () => handleOpenChange(false) });
      return;
    }

    if (communicationType === 'email_google') {
      createEmailServer.mutate({
        name: gmailData.name,
        serverType: 'gmail',
        clientId: gmailData.clientId,
        clientSecret: gmailData.clientSecret,
        fromAddress: gmailData.fromAddress,
        fromName: gmailData.fromName,
      }, { onSuccess: () => handleOpenChange(false) });
      return;
    }

    const channelType = communicationType;
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

  const selectedOption = COMMUNICATION_TYPES.find((type) => type.type === communicationType);
  const isValid = communicationType ? getIsValid(communicationType) : false;
  const isPending = createChannel.isPending || createEmailServer.isPending;

  function getIsValid(type: CommunicationCreateType): boolean {
    switch (type) {
      case 'email_smtp': return !!(smtpData.name && smtpData.host && smtpData.port && smtpData.fromAddress);
      case 'email_exchange': return !!(exchangeData.name && exchangeData.clientId && exchangeData.clientSecret && exchangeData.tenantId && exchangeData.fromAddress);
      case 'email_google': return !!(gmailData.name && gmailData.clientId && gmailData.clientSecret && gmailData.fromAddress);
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
              : selectedOption
                ? t(selectedOption.descriptionKey)
                : ''}
          </DialogDescription>
        </DialogHeader>

        <div>
          {step === 'type' && (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
              {COMMUNICATION_TYPES.map(({ type, icon: Icon, color, labelKey }) => (
                <Button
                  key={type}
                  variant="outline"
                  onClick={() => handleSelectType(type)}
                  className="flex h-auto flex-col items-center gap-3 p-4"
                >
                  <Icon className={`size-8 ${color}`} />
                  <span className="text-xs font-medium text-foreground">
                    {t(labelKey)}
                  </span>
                </Button>
              ))}
            </div>
          )}

          {step === 'config' && communicationType === 'email_smtp' && <SMTPForm data={smtpData} onChange={setSmtpData} />}
          {step === 'config' && communicationType === 'email_exchange' && <ExchangeForm data={exchangeData} onChange={setExchangeData} />}
          {step === 'config' && communicationType === 'email_google' && <GmailForm data={gmailData} onChange={setGmailData} />}
          {step === 'config' && communicationType === 'slack' && <SlackForm data={slackData} onChange={setSlackData} />}
          {step === 'config' && communicationType === 'discord' && <DiscordForm data={discordData} onChange={setDiscordData} />}
          {step === 'config' && communicationType === 'teams' && <TeamsForm data={teamsData} onChange={setTeamsData} />}
          {step === 'config' && communicationType === 'gotify' && <GotifyForm data={gotifyData} onChange={setGotifyData} />}
          {step === 'config' && communicationType === 'ntfy' && <NtfyForm data={ntfyData} onChange={setNtfyData} />}
          {step === 'config' && communicationType === 'telegram' && <TelegramForm data={telegramData} onChange={setTelegramData} />}
          {step === 'config' && communicationType === 'signal' && <SignalForm data={signalData} onChange={setSignalData} />}
          {step === 'config' && communicationType === 'whatsapp' && <WhatsAppForm data={whatsappData} onChange={setWhatsappData} />}
        </div>

        {step === 'config' && (
          <DialogFooter>
            <Button variant="outline" onClick={() => setStep('type')}>
              {t('common:back', 'Back')}
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!isValid || isPending}
            >
              {isPending ? '...' : t('communications.addChannel')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
