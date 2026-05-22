// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';

export type SlackFormData = {
  name: string;
  webhookUrl: string;
};

type SlackFormProps = {
  data: SlackFormData;
  onChange: (data: SlackFormData) => void;
  isEdit?: boolean;
};

export function SlackForm({ data, onChange, isEdit }: SlackFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<SlackFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="slack-name" className="mb-1">{t('communications.nameLabel')}</Label>
        <Input
          variant="outline"
          id="slack-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('communications.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="slack-webhook" className="mb-1">{t('communications.webhookUrlLabel')}</Label>
        <Input
          variant="outline"
          id="slack-webhook"
          type={isEdit ? 'password' : 'text'}
          value={data.webhookUrl}
          onChange={(e) => update({ webhookUrl: e.target.value })}
          placeholder={isEdit ? t('communications.secretHint') : t('communications.webhookUrlPlaceholder')}
        />
      </div>
    </div>
  );
}
