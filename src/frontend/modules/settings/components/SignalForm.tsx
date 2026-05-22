// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { ChannelSetupGuide } from './ChannelSetupGuide';
import { MethodSelector } from './MethodSelector';

export type SignalMethod = 'rest_api' | 'bot' | 'signald' | 'simple';

export type SignalFormData = {
  name: string;
  method: SignalMethod;
  serverUrl: string;
  senderNumber: string;
  recipients: string;
  username: string;
  password: string;
  token: string;
};

type SignalFormProps = {
  data: SignalFormData;
  onChange: (data: SignalFormData) => void;
  isEdit?: boolean;
};

const SIGNAL_METHODS: { key: SignalMethod; labelKey: string; descriptionKey: string }[] = [
  { key: 'rest_api', labelKey: 'communications.methodRestApi', descriptionKey: 'communications.methodRestApiDescription' },
  { key: 'bot', labelKey: 'communications.methodBot', descriptionKey: 'communications.methodBotDescription' },
  { key: 'signald', labelKey: 'communications.methodSignald', descriptionKey: 'communications.methodSignaldDescription' },
  { key: 'simple', labelKey: 'communications.methodSimple', descriptionKey: 'communications.methodSimpleDescription' },
];

const GUIDE_STEPS: Record<SignalMethod, { titleKey: string; descriptionKey: string }[]> = {
  rest_api: [
    { titleKey: 'communications.guide.signal.step1Title', descriptionKey: 'communications.guide.signal.step1Description' },
    { titleKey: 'communications.guide.signal.step2Title', descriptionKey: 'communications.guide.signal.step2Description' },
    { titleKey: 'communications.guide.signal.step3Title', descriptionKey: 'communications.guide.signal.step3Description' },
    { titleKey: 'communications.guide.signal.step4Title', descriptionKey: 'communications.guide.signal.step4Description' },
  ],
  bot: [
    { titleKey: 'communications.guide.signalBot.step1Title', descriptionKey: 'communications.guide.signalBot.step1Description' },
    { titleKey: 'communications.guide.signalBot.step2Title', descriptionKey: 'communications.guide.signalBot.step2Description' },
    { titleKey: 'communications.guide.signalBot.step3Title', descriptionKey: 'communications.guide.signalBot.step3Description' },
    { titleKey: 'communications.guide.signalBot.step4Title', descriptionKey: 'communications.guide.signalBot.step4Description' },
  ],
  signald: [
    { titleKey: 'communications.guide.signalD.step1Title', descriptionKey: 'communications.guide.signalD.step1Description' },
    { titleKey: 'communications.guide.signalD.step2Title', descriptionKey: 'communications.guide.signalD.step2Description' },
    { titleKey: 'communications.guide.signalD.step3Title', descriptionKey: 'communications.guide.signalD.step3Description' },
    { titleKey: 'communications.guide.signalD.step4Title', descriptionKey: 'communications.guide.signalD.step4Description' },
  ],
  simple: [
    { titleKey: 'communications.guide.signalSimple.step1Title', descriptionKey: 'communications.guide.signalSimple.step1Description' },
    { titleKey: 'communications.guide.signalSimple.step2Title', descriptionKey: 'communications.guide.signalSimple.step2Description' },
    { titleKey: 'communications.guide.signalSimple.step3Title', descriptionKey: 'communications.guide.signalSimple.step3Description' },
  ],
};

const GUIDE_TITLES: Record<SignalMethod, string> = {
  rest_api: 'communications.guide.signal.title',
  bot: 'communications.guide.signalBot.title',
  signald: 'communications.guide.signalD.title',
  simple: 'communications.guide.signalSimple.title',
};

const showSenderNumber = (m: SignalMethod) => m !== 'bot';
const showAuth = (m: SignalMethod) => m === 'rest_api';
const showToken = (m: SignalMethod) => m === 'bot';

export function SignalForm({ data, onChange, isEdit }: SignalFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<SignalFormData>) {
    onChange({ ...data, ...partial });
  }

  function handleMethodChange(method: SignalMethod) {
    onChange({ name: data.name, method, serverUrl: '', senderNumber: '', recipients: '', username: '', password: '', token: '' });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="signal-name" className="mb-1">{t('communications.nameLabel')}</Label>
        <Input
          variant="outline"
          id="signal-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('communications.namePlaceholder')}
        />
      </div>

      <MethodSelector
        methods={SIGNAL_METHODS}
        selected={data.method}
        onChange={handleMethodChange}
      />

      <div>
        <Label htmlFor="signal-server" className="mb-1">{t('communications.serverUrlLabel')}</Label>
        <Input
          variant="outline"
          id="signal-server"
          value={data.serverUrl}
          onChange={(e) => update({ serverUrl: e.target.value })}
          placeholder={t('communications.signalServerPlaceholder')}
        />
      </div>

      {showToken(data.method) && (
        <div>
          <Label htmlFor="signal-token" className="mb-1">{t('communications.accessTokenLabel')}</Label>
          <Input
            variant="outline"
            id="signal-token"
            type={isEdit ? 'password' : 'text'}
            value={data.token}
            onChange={(e) => update({ token: e.target.value })}
            placeholder={isEdit ? t('communications.secretHint') : t('communications.accessTokenPlaceholder')}
          />
        </div>
      )}

      <div className={showSenderNumber(data.method) ? 'grid grid-cols-2 gap-3' : ''}>
        {showSenderNumber(data.method) && (
          <div>
            <Label htmlFor="signal-sender" className="mb-1">{t('communications.senderNumberLabel')}</Label>
            <Input
              variant="outline"
              id="signal-sender"
              value={data.senderNumber}
              onChange={(e) => update({ senderNumber: e.target.value })}
              placeholder={t('communications.senderNumberPlaceholder')}
            />
          </div>
        )}
        <div>
          <Label htmlFor="signal-recipients" className="mb-1">{t('communications.recipientsLabel')}</Label>
          <Input
            variant="outline"
            id="signal-recipients"
            value={data.recipients}
            onChange={(e) => update({ recipients: e.target.value })}
            placeholder={t('communications.recipientsPlaceholder')}
          />
        </div>
      </div>

      {showAuth(data.method) && (
        <div className="rounded-lg border border-border p-3">
          <p className="mb-2 text-xs font-medium text-muted-foreground">{t('communications.optionalAuth')}</p>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="signal-username" className="mb-1">{t('communications.usernameLabel')}</Label>
              <Input
                variant="outline"
                id="signal-username"
                value={data.username}
                onChange={(e) => update({ username: e.target.value })}
                placeholder={t('communications.usernamePlaceholder')}
              />
            </div>
            <div>
              <Label htmlFor="signal-password" className="mb-1">{t('communications.passwordLabel')}</Label>
              <Input
                variant="outline"
                id="signal-password"
                type="password"
                value={data.password}
                onChange={(e) => update({ password: e.target.value })}
                placeholder={isEdit ? t('communications.secretHint') : t('communications.passwordPlaceholder')}
              />
            </div>
          </div>
        </div>
      )}

      <ChannelSetupGuide titleKey={GUIDE_TITLES[data.method]} steps={GUIDE_STEPS[data.method]} />
    </div>
  );
}
