// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Select } from '@resources/components/ui/Select';

export type NtfyFormData = {
  name: string;
  serverUrl: string;
  topic: string;
  token: string;
  username: string;
  password: string;
  priority: string;
};

type NtfyFormProps = {
  data: NtfyFormData;
  onChange: (data: NtfyFormData) => void;
  isEdit?: boolean;
};

const NTFY_PRIORITY_OPTIONS = [
  { value: '', label: 'Default' },
  { value: 'min', label: 'Min' },
  { value: 'low', label: 'Low' },
  { value: 'default', label: 'Default' },
  { value: 'high', label: 'High' },
  { value: 'urgent', label: 'Urgent' },
];

export function NtfyForm({ data, onChange, isEdit }: NtfyFormProps) {
  const { t } = useTranslation('settings');

  function update(partial: Partial<NtfyFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div>
        <Label htmlFor="ntfy-name" className="mb-1">{t('communications.nameLabel')}</Label>
        <Input
          variant="outline"
          id="ntfy-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('communications.namePlaceholder')}
        />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <div>
          <Label htmlFor="ntfy-server" className="mb-1">{t('communications.serverUrlLabel')}</Label>
          <Input
            variant="outline"
            id="ntfy-server"
            value={data.serverUrl}
            onChange={(e) => update({ serverUrl: e.target.value })}
            placeholder={t('communications.ntfyServerPlaceholder')}
          />
        </div>
        <div>
          <Label htmlFor="ntfy-topic" className="mb-1">{t('communications.topicLabel')}</Label>
          <Input
            variant="outline"
            id="ntfy-topic"
            value={data.topic}
            onChange={(e) => update({ topic: e.target.value })}
            placeholder={t('communications.topicPlaceholder')}
          />
        </div>
      </div>

      <div>
        <Label className="mb-1">{t('communications.priorityLabel')}</Label>
        <Select
          variant="outline"
          value={data.priority}
          onChange={(val) => update({ priority: val })}
          options={NTFY_PRIORITY_OPTIONS}
          searchable={false}
        />
      </div>

      <div className="rounded-lg border border-border p-3">
        <p className="mb-2 text-xs font-medium text-muted-foreground">{t('communications.optionalAuth')}</p>
        <div className="space-y-3">
          <div>
            <Label htmlFor="ntfy-token" className="mb-1">{t('communications.accessTokenLabel')}</Label>
            <Input
              variant="outline"
              id="ntfy-token"
              type={isEdit ? 'password' : 'text'}
              value={data.token}
              onChange={(e) => update({ token: e.target.value })}
              placeholder={isEdit ? t('communications.secretHint') : t('communications.accessTokenPlaceholder')}
            />
          </div>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Label htmlFor="ntfy-username" className="mb-1">{t('communications.usernameLabel')}</Label>
              <Input
                variant="outline"
                id="ntfy-username"
                value={data.username}
                onChange={(e) => update({ username: e.target.value })}
                placeholder={t('communications.usernamePlaceholder')}
              />
            </div>
            <div>
              <Label htmlFor="ntfy-password" className="mb-1">{t('communications.passwordLabel')}</Label>
              <Input
                variant="outline"
                id="ntfy-password"
                type="password"
                value={data.password}
                onChange={(e) => update({ password: e.target.value })}
                placeholder={isEdit ? t('communications.secretHint') : t('communications.passwordPlaceholder')}
              />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
