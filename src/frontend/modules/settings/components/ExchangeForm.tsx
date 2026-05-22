// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { EmailSetupGuide } from './EmailSetupGuide';

export type ExchangeFormData = {
  name: string;
  clientId: string;
  clientSecret: string;
  tenantId: string;
  fromAddress: string;
  fromName: string;
};

type ExchangeFormProps = {
  data: ExchangeFormData;
  onChange: (data: ExchangeFormData) => void;
  isEdit?: boolean;
};

const EXCHANGE_STEPS = [
  { titleKey: 'email.guide.exchange.step1Title', descriptionKey: 'email.guide.exchange.step1Description' },
  { titleKey: 'email.guide.exchange.step2Title', descriptionKey: 'email.guide.exchange.step2Description' },
  { titleKey: 'email.guide.exchange.step3Title', descriptionKey: 'email.guide.exchange.step3Description' },
  { titleKey: 'email.guide.exchange.step4Title', descriptionKey: 'email.guide.exchange.step4Description' },
  { titleKey: 'email.guide.exchange.step5Title', descriptionKey: 'email.guide.exchange.step5Description' },
];

export function ExchangeForm({ data, onChange, isEdit }: ExchangeFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<ExchangeFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <EmailSetupGuide
        titleKey="email.guide.exchange.title"
        steps={EXCHANGE_STEPS}
      />

      <div>
        <Label htmlFor="exchange-name" className="mb-1">{t('email.nameLabel')}</Label>
        <Input
          variant="outline"
          id="exchange-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('email.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="exchange-tenant" className="mb-1">{t('email.tenantIdLabel')}</Label>
        <Input
          variant="outline"
          id="exchange-tenant"
          value={data.tenantId}
          onChange={(e) => update({ tenantId: e.target.value })}
          placeholder={t('email.tenantIdPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="exchange-client-id" className="mb-1">{t('email.clientIdLabel')}</Label>
        <Input
          variant="outline"
          id="exchange-client-id"
          value={data.clientId}
          onChange={(e) => update({ clientId: e.target.value })}
          placeholder={t('email.clientIdPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="exchange-secret" className="mb-1">{t('email.clientSecretLabel')}</Label>
        <Input
          variant="outline"
          id="exchange-secret"
          type="password"
          value={data.clientSecret}
          onChange={(e) => update({ clientSecret: e.target.value })}
          placeholder={isEdit ? t('email.passwordHint') : t('email.clientSecretPlaceholder')}
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label htmlFor="exchange-from-address" className="mb-1">{t('email.fromAddressLabel')}</Label>
          <Input
            variant="outline"
            id="exchange-from-address"
            type="email"
            value={data.fromAddress}
            onChange={(e) => update({ fromAddress: e.target.value })}
            placeholder={t('email.fromAddressPlaceholder')}
          />
        </div>
        <div>
          <Label htmlFor="exchange-from-name" className="mb-1">{t('email.fromNameLabel')}</Label>
          <Input
            variant="outline"
            id="exchange-from-name"
            value={data.fromName}
            onChange={(e) => update({ fromName: e.target.value })}
            placeholder={t('email.fromNamePlaceholder')}
          />
        </div>
      </div>
    </div>
  );
}
