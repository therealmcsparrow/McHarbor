// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { GroupMappingEditor } from './GroupMappingEditor';
import type { GroupMapping } from '../hooks/useIdentityProviders';

type GenericOIDCFormData = {
  name: string;
  issuerUrl: string;
  clientId: string;
  clientSecret: string;
  scopes: string;
  autoProvision: boolean;
  autoImportGroups: boolean;
  groupMappingEnabled: boolean;
  groupMappings: GroupMapping[];
};

type GenericOIDCFormProps = {
  data: GenericOIDCFormData;
  onChange: (data: GenericOIDCFormData) => void;
};

export function GenericOIDCForm({ data, onChange }: GenericOIDCFormProps) {
  const { t } = useTranslation('security');
  const callbackUrl = `${window.location.origin}/api/identity-providers/callback`;

  function update(partial: Partial<GenericOIDCFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-border p-3">
        <p className="text-sm font-medium text-foreground">{t('identity.callbackUrl')}</p>
        <p className="mt-1 text-xs text-muted-foreground">{t('identity.callbackUrlHint')}</p>
        <code className="mt-3 block rounded-md bg-muted px-3 py-2 text-xs text-foreground">
          {callbackUrl}
        </code>
      </div>

      <div>
        <Label htmlFor="oidc-name" className="mb-1">{t('identity.name')}</Label>
        <Input
          variant="outline"
          id="oidc-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('identity.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="oidc-issuer" className="mb-1">{t('identity.issuerUrl')}</Label>
        <Input
          variant="outline"
          id="oidc-issuer"
          value={data.issuerUrl}
          onChange={(e) => update({ issuerUrl: e.target.value })}
          placeholder={t('identity.issuerUrlPlaceholder')}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('identity.issuerUrlDescription')}</p>
      </div>

      <div>
        <Label htmlFor="oidc-client-id" className="mb-1">{t('identity.clientId')}</Label>
        <Input
          variant="outline"
          id="oidc-client-id"
          value={data.clientId}
          onChange={(e) => update({ clientId: e.target.value })}
          placeholder={t('identity.clientIdPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="oidc-secret" className="mb-1">{t('identity.clientSecret')}</Label>
        <Input
          variant="outline"
          id="oidc-secret"
          type="password"
          value={data.clientSecret}
          onChange={(e) => update({ clientSecret: e.target.value })}
          placeholder={t('identity.clientSecretPlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="oidc-scopes" className="mb-1">{t('identity.scopes')}</Label>
        <Input
          variant="outline"
          id="oidc-scopes"
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
        supportsFetchGroups={false}
        onAutoImportChange={(autoImport) => update({ autoImportGroups: autoImport })}
        onEnabledChange={(enabled) => update({ groupMappingEnabled: enabled })}
        onMappingsChange={(mappings) => update({ groupMappings: mappings })}
      />
    </div>
  );
}
