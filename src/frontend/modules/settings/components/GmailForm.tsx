// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { EmailSetupGuide } from './EmailSetupGuide';

export type GmailFormData = {
  name: string;
  clientId: string;
  clientSecret: string;
  fromAddress: string;
  fromName: string;
};

type GmailFormProps = {
  data: GmailFormData;
  onChange: (data: GmailFormData) => void;
  isEdit?: boolean;
};

const GMAIL_STEPS = [
  { titleKey: 'email.guide.gmail.step1Title', descriptionKey: 'email.guide.gmail.step1Description' },
  { titleKey: 'email.guide.gmail.step2Title', descriptionKey: 'email.guide.gmail.step2Description' },
  { titleKey: 'email.guide.gmail.step3Title', descriptionKey: 'email.guide.gmail.step3Description' },
  { titleKey: 'email.guide.gmail.step4Title', descriptionKey: 'email.guide.gmail.step4Description' },
];

export function GmailForm({ data, onChange, isEdit }: GmailFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<GmailFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <EmailSetupGuide
        titleKey="email.guide.gmail.title"
        steps={GMAIL_STEPS}
      />

      <div>
        <Label htmlFor="gmail-name" className="mb-1">{t('email.nameLabel')}</Label>
        <Input
          variant="outline"
          id="gmail-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('email.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="gmail-client-id" className="mb-1">{t('email.clientIdLabel')}</Label>
        <Input
          variant="outline"
          id="gmail-client-id"
          value={data.clientId}
          onChange={(e) => update({ clientId: e.target.value })}
          placeholder={t('email.clientIdPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="gmail-secret" className="mb-1">{t('email.clientSecretLabel')}</Label>
        <Input
          variant="outline"
          id="gmail-secret"
          type="password"
          value={data.clientSecret}
          onChange={(e) => update({ clientSecret: e.target.value })}
          placeholder={isEdit ? t('email.passwordHint') : t('email.clientSecretPlaceholder')}
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label htmlFor="gmail-from-address" className="mb-1">{t('email.fromAddressLabel')}</Label>
          <Input
            variant="outline"
            id="gmail-from-address"
            type="email"
            value={data.fromAddress}
            onChange={(e) => update({ fromAddress: e.target.value })}
            placeholder={t('email.fromAddressPlaceholder')}
          />
        </div>
        <div>
          <Label htmlFor="gmail-from-name" className="mb-1">{t('email.fromNameLabel')}</Label>
          <Input
            variant="outline"
            id="gmail-from-name"
            value={data.fromName}
            onChange={(e) => update({ fromName: e.target.value })}
            placeholder={t('email.fromNamePlaceholder')}
          />
        </div>
      </div>
    </div>
  );
}
