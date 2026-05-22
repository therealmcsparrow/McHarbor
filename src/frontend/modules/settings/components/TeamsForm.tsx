// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { ChannelSetupGuide } from './ChannelSetupGuide';

export type TeamsFormData = {
  name: string;
  webhookUrl: string;
};

type TeamsFormProps = {
  data: TeamsFormData;
  onChange: (data: TeamsFormData) => void;
  isEdit?: boolean;
};

const TEAMS_GUIDE_STEPS = [
  { titleKey: 'communications.guide.teams.step1Title', descriptionKey: 'communications.guide.teams.step1Description' },
  { titleKey: 'communications.guide.teams.step2Title', descriptionKey: 'communications.guide.teams.step2Description' },
  { titleKey: 'communications.guide.teams.step3Title', descriptionKey: 'communications.guide.teams.step3Description' },
  { titleKey: 'communications.guide.teams.step4Title', descriptionKey: 'communications.guide.teams.step4Description' },
];

export function TeamsForm({ data, onChange, isEdit }: TeamsFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<TeamsFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="teams-name" className="mb-1">{t('communications.nameLabel')}</Label>
        <Input
          variant="outline"
          id="teams-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('communications.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="teams-webhook" className="mb-1">{t('communications.webhookUrlLabel')}</Label>
        <Input
          variant="outline"
          id="teams-webhook"
          type={isEdit ? 'password' : 'text'}
          value={data.webhookUrl}
          onChange={(e) => update({ webhookUrl: e.target.value })}
          placeholder={isEdit ? t('communications.secretHint') : t('communications.webhookUrlPlaceholder')}
        />
      </div>

      <ChannelSetupGuide titleKey="communications.guide.teams.title" steps={TEAMS_GUIDE_STEPS} />
    </div>
  );
}
