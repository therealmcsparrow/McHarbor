// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { SetupGuide } from './SetupGuide';
import { GroupMappingEditor } from './GroupMappingEditor';
import type { GroupMapping } from '../hooks/useIdentityProviders';

type EntraIdFormData = {
  name: string;
  clientId: string;
  clientSecret: string;
  tenantId: string;
  scopes: string;
  autoProvision: boolean;
  autoImportGroups: boolean;
  groupMappingEnabled: boolean;
  groupMappings: GroupMapping[];
};

type EntraIdFormProps = {
  data: EntraIdFormData;
  onChange: (data: EntraIdFormData) => void;
};

const ENTRA_STEPS = [
  { titleKey: 'identity.guide.entraId.step1Title', descriptionKey: 'identity.guide.entraId.step1Description' },
  { titleKey: 'identity.guide.entraId.step2Title', descriptionKey: 'identity.guide.entraId.step2Description' },
  { titleKey: 'identity.guide.entraId.step3Title', descriptionKey: 'identity.guide.entraId.step3Description' },
  { titleKey: 'identity.guide.entraId.step4Title', descriptionKey: 'identity.guide.entraId.step4Description' },
  { titleKey: 'identity.guide.entraId.step5Title', descriptionKey: 'identity.guide.entraId.step5Description' },
];

export function EntraIdForm({ data, onChange }: EntraIdFormProps) {
  const { t } = useTranslation('security');
  const callbackUrl = `${window.location.origin}/api/identity-providers/callback`;

  function update(partial: Partial<EntraIdFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <SetupGuide
        titleKey="identity.guide.entraId.title"
        steps={ENTRA_STEPS}
        callbackUrl={callbackUrl}
      />

      <div>
        <Label htmlFor="entra-name" className="mb-1">{t('identity.name')}</Label>
        <Input
          variant="outline"
          id="entra-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('identity.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="entra-tenant" className="mb-1">{t('identity.tenantId')}</Label>
        <Input
          variant="outline"
          id="entra-tenant"
          value={data.tenantId}
          onChange={(e) => update({ tenantId: e.target.value })}
          placeholder={t('identity.tenantIdPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="entra-client-id" className="mb-1">{t('identity.clientId')}</Label>
        <Input
          variant="outline"
          id="entra-client-id"
          value={data.clientId}
          onChange={(e) => update({ clientId: e.target.value })}
          placeholder={t('identity.clientIdPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="entra-secret" className="mb-1">{t('identity.clientSecret')}</Label>
        <Input
          variant="outline"
          id="entra-secret"
          type="password"
          value={data.clientSecret}
          onChange={(e) => update({ clientSecret: e.target.value })}
          placeholder={t('identity.clientSecretPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="entra-scopes" className="mb-1">{t('identity.scopes')}</Label>
        <Input
          variant="outline"
          id="entra-scopes"
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
