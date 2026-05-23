// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { GroupMappingEditor } from './GroupMappingEditor';
import type { GroupMapping } from '../hooks/useIdentityProviders';

type SAMLFormData = {
  name: string;
  metadataUrl: string;
  entityId: string;
  autoProvision: boolean;
  autoImportGroups: boolean;
  groupMappingEnabled: boolean;
  groupMappings: GroupMapping[];
};

type SAMLFormProps = {
  data: SAMLFormData;
  onChange: (data: SAMLFormData) => void;
  introTitleKey?: string;
  introDescriptionKey?: string;
};

export function SAMLForm({
  data,
  onChange,
  introTitleKey = 'identity.saml2',
  introDescriptionKey = 'identity.samlCreateHint',
}: SAMLFormProps) {
  const { t } = useTranslation('security');
  const urlPrefix = `${window.location.origin}/api/identity-providers/<provider-id>`;

  function update(partial: Partial<SAMLFormData>) {
    onChange({ ...data, ...partial });
  }

  return (
    <div className="space-y-4">
      <div className="rounded-lg border border-border p-3">
        <p className="text-sm font-medium text-foreground">{t(introTitleKey)}</p>
        <p className="mt-1 text-xs text-muted-foreground">
          {t(introDescriptionKey)}
        </p>
        <div className="mt-3 space-y-2">
          <div>
            <p className="text-xs text-muted-foreground">{t('identity.serviceProviderMetadataUrl')}</p>
            <code className="mt-1 block rounded-md bg-muted px-3 py-2 text-xs text-foreground">
              {`${urlPrefix}/metadata`}
            </code>
          </div>
          <div>
            <p className="text-xs text-muted-foreground">{t('identity.acsUrl')}</p>
            <code className="mt-1 block rounded-md bg-muted px-3 py-2 text-xs text-foreground">
              {`${urlPrefix}/acs`}
            </code>
          </div>
        </div>
      </div>

      <div>
        <Label htmlFor="saml-name" className="mb-1">{t('identity.name')}</Label>
        <Input
          variant="outline"
          id="saml-name"
          value={data.name}
          onChange={(e) => update({ name: e.target.value })}
          placeholder={t('identity.namePlaceholder')}
        />
      </div>

      <div>
        <Label htmlFor="saml-metadata-url" className="mb-1">{t('identity.metadataUrl')}</Label>
        <Input
          variant="outline"
          id="saml-metadata-url"
          value={data.metadataUrl}
          onChange={(e) => update({ metadataUrl: e.target.value })}
          placeholder={t('identity.metadataUrlPlaceholder')}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('identity.metadataUrlDescription')}</p>
      </div>

      <div>
        <Label htmlFor="saml-entity-id" className="mb-1">{t('identity.entityId')}</Label>
        <Input
          variant="outline"
          id="saml-entity-id"
          value={data.entityId}
          onChange={(e) => update({ entityId: e.target.value })}
          placeholder={t('identity.entityIdPlaceholder')}
        />
        <p className="mt-1 text-xs text-muted-foreground">{t('identity.entityIdDescription')}</p>
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
