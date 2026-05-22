// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { ChannelSetupGuide } from './ChannelSetupGuide';
import { MethodSelector } from './MethodSelector';

export type WhatsAppMethod = 'cloud_api' | 'gateway' | 'business' | 'saas';

export type WhatsAppFormData = {
  name: string;
  method: WhatsAppMethod;
  serverUrl: string;
  phoneNumberId: string;
  token: string;
  recipientPhone: string;
};

type WhatsAppFormProps = {
  data: WhatsAppFormData;
  onChange: (data: WhatsAppFormData) => void;
  isEdit?: boolean;
};

const WHATSAPP_METHODS: { key: WhatsAppMethod; labelKey: string; descriptionKey: string }[] = [
  { key: 'cloud_api', labelKey: 'communications.methodCloudApi', descriptionKey: 'communications.methodCloudApiDescription' },
  { key: 'gateway', labelKey: 'communications.methodGateway', descriptionKey: 'communications.methodGatewayDescription' },
  { key: 'business', labelKey: 'communications.methodBusiness', descriptionKey: 'communications.methodBusinessDescription' },
  { key: 'saas', labelKey: 'communications.methodSaas', descriptionKey: 'communications.methodSaasDescription' },
];

const GUIDE_STEPS: Record<WhatsAppMethod, { titleKey: string; descriptionKey: string }[]> = {
  cloud_api: [
    { titleKey: 'communications.guide.whatsapp.step1Title', descriptionKey: 'communications.guide.whatsapp.step1Description' },
    { titleKey: 'communications.guide.whatsapp.step2Title', descriptionKey: 'communications.guide.whatsapp.step2Description' },
    { titleKey: 'communications.guide.whatsapp.step3Title', descriptionKey: 'communications.guide.whatsapp.step3Description' },
    { titleKey: 'communications.guide.whatsapp.step4Title', descriptionKey: 'communications.guide.whatsapp.step4Description' },
    { titleKey: 'communications.guide.whatsapp.step5Title', descriptionKey: 'communications.guide.whatsapp.step5Description' },
  ],
  gateway: [
    { titleKey: 'communications.guide.whatsappGateway.step1Title', descriptionKey: 'communications.guide.whatsappGateway.step1Description' },
    { titleKey: 'communications.guide.whatsappGateway.step2Title', descriptionKey: 'communications.guide.whatsappGateway.step2Description' },
    { titleKey: 'communications.guide.whatsappGateway.step3Title', descriptionKey: 'communications.guide.whatsappGateway.step3Description' },
    { titleKey: 'communications.guide.whatsappGateway.step4Title', descriptionKey: 'communications.guide.whatsappGateway.step4Description' },
  ],
  business: [
    { titleKey: 'communications.guide.whatsappBusiness.step1Title', descriptionKey: 'communications.guide.whatsappBusiness.step1Description' },
    { titleKey: 'communications.guide.whatsappBusiness.step2Title', descriptionKey: 'communications.guide.whatsappBusiness.step2Description' },
    { titleKey: 'communications.guide.whatsappBusiness.step3Title', descriptionKey: 'communications.guide.whatsappBusiness.step3Description' },
    { titleKey: 'communications.guide.whatsappBusiness.step4Title', descriptionKey: 'communications.guide.whatsappBusiness.step4Description' },
  ],
  saas: [
    { titleKey: 'communications.guide.whatsappSaas.step1Title', descriptionKey: 'communications.guide.whatsappSaas.step1Description' },
    { titleKey: 'communications.guide.whatsappSaas.step2Title', descriptionKey: 'communications.guide.whatsappSaas.step2Description' },
    { titleKey: 'communications.guide.whatsappSaas.step3Title', descriptionKey: 'communications.guide.whatsappSaas.step3Description' },
    { titleKey: 'communications.guide.whatsappSaas.step4Title', descriptionKey: 'communications.guide.whatsappSaas.step4Description' },
  ],
};

const GUIDE_TITLES: Record<WhatsAppMethod, string> = {
  cloud_api: 'communications.guide.whatsapp.title',
  gateway: 'communications.guide.whatsappGateway.title',
  business: 'communications.guide.whatsappBusiness.title',
  saas: 'communications.guide.whatsappSaas.title',
};

const showServerUrl = (m: WhatsAppMethod) => m === 'gateway' || m === 'business' || m === 'saas';
const showPhoneNumberId = (m: WhatsAppMethod) => m === 'cloud_api' || m === 'business';

export function WhatsAppForm({ data, onChange, isEdit }: WhatsAppFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<WhatsAppFormData>) {
    onChange({ ...data, ...partial });
  }

  function handleMethodChange(method: WhatsAppMethod) {
    onChange({ name: data.name, method, serverUrl: '', phoneNumberId: '', token: '', recipientPhone: '' });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="whatsapp-name" className="mb-1">{t('communications.nameLabel')}</Label>
        <Input
          variant="outline"
          id="whatsapp-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('communications.namePlaceholder')}
        />
      </div>

      <MethodSelector
        methods={WHATSAPP_METHODS}
        selected={data.method}
        onChange={handleMethodChange}
      />

      {showServerUrl(data.method) && (
        <div>
          <Label htmlFor="whatsapp-server" className="mb-1">{t('communications.apiUrlLabel')}</Label>
          <Input
            variant="outline"
            id="whatsapp-server"
            value={data.serverUrl}
            onChange={(e) => update({ serverUrl: e.target.value })}
            placeholder={t('communications.apiUrlPlaceholder')}
          />
        </div>
      )}

      {showPhoneNumberId(data.method) && (
        <div>
          <Label htmlFor="whatsapp-phone-id" className="mb-1">{t('communications.phoneNumberIdLabel')}</Label>
          <Input
            variant="outline"
            id="whatsapp-phone-id"
            value={data.phoneNumberId}
            onChange={(e) => update({ phoneNumberId: e.target.value })}
            placeholder={t('communications.phoneNumberIdPlaceholder')}
          />
        </div>
      )}

      <div>
        <Label htmlFor="whatsapp-token" className="mb-1">{t('communications.accessTokenLabel')}</Label>
        <Input
          variant="outline"
          id="whatsapp-token"
          type={isEdit ? 'password' : 'text'}
          value={data.token}
          onChange={(e) => update({ token: e.target.value })}
          placeholder={isEdit ? t('communications.secretHint') : t('communications.accessTokenPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="whatsapp-recipient" className="mb-1">{t('communications.recipientPhoneLabel')}</Label>
        <Input
          variant="outline"
          id="whatsapp-recipient"
          value={data.recipientPhone}
          onChange={(e) => update({ recipientPhone: e.target.value })}
          placeholder={t('communications.recipientPhonePlaceholder')}
        />
      </div>

      <ChannelSetupGuide titleKey={GUIDE_TITLES[data.method]} steps={GUIDE_STEPS[data.method]} />
    </div>
  );
}
