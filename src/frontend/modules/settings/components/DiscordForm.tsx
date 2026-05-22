// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';

export type DiscordFormData = {
  name: string;
  webhookUrl: string;
};

type DiscordFormProps = {
  data: DiscordFormData;
  onChange: (data: DiscordFormData) => void;
  isEdit?: boolean;
};

export function DiscordForm({ data, onChange, isEdit }: DiscordFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<DiscordFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="discord-name" className="mb-1">{t('communications.nameLabel')}</Label>
        <Input
          variant="outline"
          id="discord-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('communications.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="discord-webhook" className="mb-1">{t('communications.webhookUrlLabel')}</Label>
        <Input
          variant="outline"
          id="discord-webhook"
          type={isEdit ? 'password' : 'text'}
          value={data.webhookUrl}
          onChange={(e) => update({ webhookUrl: e.target.value })}
          placeholder={isEdit ? t('communications.secretHint') : t('communications.webhookUrlPlaceholder')}
        />
      </div>
    </div>
  );
}
