// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { ChannelSetupGuide } from './ChannelSetupGuide';

export type TelegramFormData = {
  name: string;
  token: string;
  chatId: string;
};

type TelegramFormProps = {
  data: TelegramFormData;
  onChange: (data: TelegramFormData) => void;
  isEdit?: boolean;
};

const TELEGRAM_GUIDE_STEPS = [
  { titleKey: 'communications.guide.telegram.step1Title', descriptionKey: 'communications.guide.telegram.step1Description' },
  { titleKey: 'communications.guide.telegram.step2Title', descriptionKey: 'communications.guide.telegram.step2Description' },
  { titleKey: 'communications.guide.telegram.step3Title', descriptionKey: 'communications.guide.telegram.step3Description' },
  { titleKey: 'communications.guide.telegram.step4Title', descriptionKey: 'communications.guide.telegram.step4Description' },
];

export function TelegramForm({ data, onChange, isEdit }: TelegramFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<TelegramFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="telegram-name" className="mb-1">{t('communications.nameLabel')}</Label>
        <Input
          variant="outline"
          id="telegram-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('communications.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="telegram-token" className="mb-1">{t('communications.botTokenLabel')}</Label>
        <Input
          variant="outline"
          id="telegram-token"
          type={isEdit ? 'password' : 'text'}
          value={data.token}
          onChange={(e) => update({ token: e.target.value })}
          placeholder={isEdit ? t('communications.secretHint') : t('communications.botTokenPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="telegram-chatid" className="mb-1">{t('communications.chatIdLabel')}</Label>
        <Input
          variant="outline"
          id="telegram-chatid"
          value={data.chatId}
          onChange={(e) => update({ chatId: e.target.value })}
          placeholder={t('communications.chatIdPlaceholder')}
        />
      </div>

      <ChannelSetupGuide titleKey="communications.guide.telegram.title" steps={TELEGRAM_GUIDE_STEPS} />
    </div>
  );
}
