// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { SetupGuide } from './SetupGuide';
import { GroupMappingEditor } from './GroupMappingEditor';
import type { GroupMapping } from '../hooks/useIdentityProviders';

type GoogleFormData = {
  name: string;
  clientId: string;
  clientSecret: string;
  domain: string;
  scopes: string;
  autoProvision: boolean;
  autoImportGroups: boolean;
  groupMappingEnabled: boolean;
  groupMappings: GroupMapping[];
};

type GoogleFormProps = {
  data: GoogleFormData;
  onChange: (data: GoogleFormData) => void;
};

const GOOGLE_STEPS = [
  { titleKey: 'identity.guide.google.step1Title', descriptionKey: 'identity.guide.google.step1Description' },
  { titleKey: 'identity.guide.google.step2Title', descriptionKey: 'identity.guide.google.step2Description' },
  { titleKey: 'identity.guide.google.step3Title', descriptionKey: 'identity.guide.google.step3Description' },
  { titleKey: 'identity.guide.google.step4Title', descriptionKey: 'identity.guide.google.step4Description' },
  { titleKey: 'identity.guide.google.step5Title', descriptionKey: 'identity.guide.google.step5Description' },
];

export function GoogleForm({ data, onChange }: GoogleFormProps) {
  const { t } = useTranslation('security');
  const callbackUrl = `${window.location.origin}/api/identity-providers/callback`;

  function update(partial: Partial<GoogleFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <SetupGuide
        titleKey="identity.guide.google.title"
        steps={GOOGLE_STEPS}
        callbackUrl={callbackUrl}
      />

      <div>
        <Label htmlFor="google-name" className="mb-1">{t('identity.name')}</Label>
        <Input
          variant="outline"
          id="google-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('identity.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="google-client-id" className="mb-1">{t('identity.clientId')}</Label>
        <Input
          variant="outline"
          id="google-client-id"
          value={data.clientId}
          onChange={(e) => update({ clientId: e.target.value })}
          placeholder={t('identity.clientIdPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="google-secret" className="mb-1">{t('identity.clientSecret')}</Label>
        <Input
          variant="outline"
          id="google-secret"
          type="password"
          value={data.clientSecret}
          onChange={(e) => update({ clientSecret: e.target.value })}
          placeholder={t('identity.clientSecretPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="google-domain" className="mb-1">{t('identity.domain')}</Label>
        <Input
          variant="outline"
          id="google-domain"
          value={data.domain}
          onChange={(e) => update({ domain: e.target.value })}
          placeholder={t('identity.domainPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="google-scopes" className="mb-1">{t('identity.scopes')}</Label>
        <Input
          variant="outline"
          id="google-scopes"
          value={data.scopes}
          onChange={(e) => update({ scopes: e.target.value })}
          placeholder={t('identity.scopesPlaceholder')}
        />
      </div>

      <div className="flex items-center justify-between rounded-lg border border-border p-3">
        <div>
          <p className="text-sm font-medium text-foreground">{t('identity.autoProvision')}</p>
          <p className="text-xs text-muted-foreground">{t('identity.autoProvisionDescription')}</p>
        </div>
        <Switch
          checked={data.autoProvision}
          onCheckedChange={(checked) => update({ autoProvision: checked })}
        />
      </div>

      <GroupMappingEditor
        autoImport={data.autoImportGroups}
        enabled={data.groupMappingEnabled}
        mappings={data.groupMappings}
        onAutoImportChange={(autoImport) => update({ autoImportGroups: autoImport })}
        onEnabledChange={(enabled) => update({ groupMappingEnabled: enabled })}
        onMappingsChange={(mappings) => update({ groupMappings: mappings })}
      />
    </div>
  );
}
