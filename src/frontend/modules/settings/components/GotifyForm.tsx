// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { ChannelSetupGuide } from './ChannelSetupGuide';

export type GotifyFormData = {
  name: string;
  serverUrl: string;
  token: string;
  priority: string;
};

type GotifyFormProps = {
  data: GotifyFormData;
  onChange: (data: GotifyFormData) => void;
  isEdit?: boolean;
};

const GOTIFY_GUIDE_STEPS = [
  { titleKey: 'communications.guide.gotify.step1Title', descriptionKey: 'communications.guide.gotify.step1Description' },
  { titleKey: 'communications.guide.gotify.step2Title', descriptionKey: 'communications.guide.gotify.step2Description' },
  { titleKey: 'communications.guide.gotify.step3Title', descriptionKey: 'communications.guide.gotify.step3Description' },
  { titleKey: 'communications.guide.gotify.step4Title', descriptionKey: 'communications.guide.gotify.step4Description' },
];

export function GotifyForm({ data, onChange, isEdit }: GotifyFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<GotifyFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="gotify-name" className="mb-1">{t('communications.nameLabel')}</Label>
        <Input
          variant="outline"
          id="gotify-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('communications.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="gotify-server" className="mb-1">{t('communications.serverUrlLabel')}</Label>
        <Input
          variant="outline"
          id="gotify-server"
          value={data.serverUrl}
          onChange={(e) => update({ serverUrl: e.target.value })}
          placeholder={t('communications.serverUrlPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="gotify-token" className="mb-1">{t('communications.tokenLabel')}</Label>
        <Input
          variant="outline"
          id="gotify-token"
          type={isEdit ? 'password' : 'text'}
          value={data.token}
          onChange={(e) => update({ token: e.target.value })}
          placeholder={isEdit ? t('communications.secretHint') : t('communications.tokenPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="gotify-priority" className="mb-1">{t('communications.priorityLabel')}</Label>
        <Input
          variant="outline"
          id="gotify-priority"
          value={data.priority}
          onChange={(e) => update({ priority: e.target.value })}
          placeholder={t('communications.gotifyPriorityPlaceholder')}
        />
      </div>

      <ChannelSetupGuide titleKey="communications.guide.gotify.title" steps={GOTIFY_GUIDE_STEPS} />
    </div>
  );
}
