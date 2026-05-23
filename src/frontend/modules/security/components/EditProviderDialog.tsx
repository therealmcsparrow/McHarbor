// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import { useUpdateProvider, type IdentityProvider, type UpdateProviderInput, type GroupMapping } from '../hooks/useIdentityProviders';
import { GroupMappingEditor } from './GroupMappingEditor';

type EditProviderDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: IdentityProvider;
};

export function EditProviderDialog({ open, onOpenChange, provider }: EditProviderDialogProps) {
  const { t } = useTranslation('security');
  const [name, setName] = useState(provider.name);
  const [clientId, setClientId] = useState(provider.clientId);
  const [clientSecret, setClientSecret] = useState('');
  const [tenantId, setTenantId] = useState(provider.tenantId ?? '');
  const [domain, setDomain] = useState(provider.domain ?? '');
  const [issuerUrl, setIssuerUrl] = useState(provider.issuerUrl ?? '');
  const [metadataUrl, setMetadataUrl] = useState(provider.metadataUrl ?? '');
  const [entityId, setEntityId] = useState(provider.entityId ?? '');
  const [scopes, setScopes] = useState(provider.scopes);
  const [autoProvision, setAutoProvision] = useState(provider.autoProvision);
  const [autoImportGroups, setAutoImportGroups] = useState(provider.autoImportGroups);
  const [groupMappingEnabled, setGroupMappingEnabled] = useState(provider.groupMappingEnabled);
  const [groupMappings, setGroupMappings] = useState<GroupMapping[]>(provider.groupMappings ?? []);
  const updateProvider = useUpdateProvider();

  const typeLabel = provider.providerType === 'entra_id'
    ? t('identity.entraId')
    : provider.providerType === 'google'
      ? t('identity.google')
      : provider.providerType === 'generic_oidc'
        ? t('identity.genericOidc')
        : t('identity.saml2');
  const providerBaseUrl = `${window.location.origin}/api/identity-providers/${provider.id}`;

  function handleSave() {
    const input: UpdateProviderInput & { id: string } = { id: provider.id };

    if (name !== provider.name) input.name = name;
    if (clientId !== provider.clientId) input.clientId = clientId;
    if (clientSecret) input.clientSecret = clientSecret;
    if (scopes !== provider.scopes) input.scopes = scopes;
    if (autoProvision !== provider.autoProvision) input.autoProvision = autoProvision;

    if (provider.providerType === 'entra_id' && tenantId !== (provider.tenantId ?? '')) {
      input.tenantId = tenantId;
    }
    if (provider.providerType === 'google' && domain !== (provider.domain ?? '')) {
      input.domain = domain;
    }
    if (provider.providerType === 'generic_oidc' && issuerUrl !== (provider.issuerUrl ?? '')) {
      input.issuerUrl = issuerUrl;
    }
    if (provider.providerType === 'saml_2_0' && metadataUrl !== (provider.metadataUrl ?? '')) {
      input.metadataUrl = metadataUrl;
    }
    if (provider.providerType === 'saml_2_0' && entityId !== (provider.entityId ?? '')) {
      input.entityId = entityId;
    }

    if (autoImportGroups !== provider.autoImportGroups) {
      input.autoImportGroups = autoImportGroups;
    }
    if (groupMappingEnabled !== provider.groupMappingEnabled) {
      input.groupMappingEnabled = groupMappingEnabled;
    }
    input.groupMappings = groupMappings;

    updateProvider.mutate(input, {
      onSuccess: () => onOpenChange(false),
    });
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('identity.editProvider')}</DialogTitle>
          <DialogDescription>{typeLabel}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <Label htmlFor="edit-name" className="mb-1">{t('identity.name')}</Label>
            <Input
              variant="outline"
              id="edit-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </div>

          {provider.providerType === 'entra_id' && (
            <div>
              <Label htmlFor="edit-tenant" className="mb-1">{t('identity.tenantId')}</Label>
              <Input
                variant="outline"
                id="edit-tenant"
                value={tenantId}
                onChange={(e) => setTenantId(e.target.value)}
              />
            </div>
          )}

          {provider.providerType === 'google' && (
            <div>
              <Label htmlFor="edit-domain" className="mb-1">{t('identity.domain')}</Label>
              <Input
                variant="outline"
                id="edit-domain"
                value={domain}
                onChange={(e) => setDomain(e.target.value)}
              />
            </div>
          )}

          {provider.providerType === 'generic_oidc' && (
            <div>
              <Label htmlFor="edit-issuer-url" className="mb-1">{t('identity.issuerUrl')}</Label>
              <Input
                variant="outline"
                id="edit-issuer-url"
                value={issuerUrl}
                onChange={(e) => setIssuerUrl(e.target.value)}
                placeholder={t('identity.issuerUrlPlaceholder')}
              />
              <p className="mt-1 text-xs text-muted-foreground">
                {t('identity.issuerUrlDescription')}
              </p>
            </div>
          )}

          {provider.providerType !== 'saml_2_0' && (
          <div>
            <Label htmlFor="edit-client-id" className="mb-1">{t('identity.clientId')}</Label>
            <Input
              variant="outline"
              id="edit-client-id"
              value={clientId}
              onChange={(e) => setClientId(e.target.value)}
            />
          </div>
          )}

          {provider.providerType !== 'saml_2_0' && (
          <div>
            <Label htmlFor="edit-secret" className="mb-1">{t('identity.clientSecret')}</Label>
            <Input
              variant="outline"
              id="edit-secret"
              type="password"
              value={clientSecret}
              onChange={(e) => setClientSecret(e.target.value)}
              placeholder={t('identity.clientSecretPlaceholder')}
            />
            <p className="mt-1 text-xs text-muted-foreground">
              {t('identity.callbackUrlHint', 'Leave blank to keep existing secret')}
            </p>
          </div>
          )}

          {provider.providerType === 'saml_2_0' && (
            <>
              <div>
                <Label htmlFor="edit-metadata-url" className="mb-1">{t('identity.metadataUrl')}</Label>
                <Input
                  variant="outline"
                  id="edit-metadata-url"
                  value={metadataUrl}
                  onChange={(e) => setMetadataUrl(e.target.value)}
                  placeholder={t('identity.metadataUrlPlaceholder')}
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('identity.metadataUrlDescription')}
                </p>
              </div>

              <div>
                <Label htmlFor="edit-entity-id" className="mb-1">{t('identity.entityId')}</Label>
                <Input
                  variant="outline"
                  id="edit-entity-id"
                  value={entityId}
                  onChange={(e) => setEntityId(e.target.value)}
                  placeholder={t('identity.entityIdPlaceholder')}
                />
                <p className="mt-1 text-xs text-muted-foreground">
                  {t('identity.entityIdDescription')}
                </p>
              </div>

              <div className="rounded-lg border border-border p-3">
                <div>
                  <p className="text-sm font-medium text-foreground">{t('identity.serviceProviderMetadataUrl')}</p>
                  <code className="mt-2 block rounded-md bg-muted px-3 py-2 text-xs text-foreground">
                    {`${providerBaseUrl}/metadata`}
                  </code>
                </div>
                <div className="mt-3">
                  <p className="text-sm font-medium text-foreground">{t('identity.acsUrl')}</p>
                  <code className="mt-2 block rounded-md bg-muted px-3 py-2 text-xs text-foreground">
                    {`${providerBaseUrl}/acs`}
                  </code>
                </div>
              </div>
            </>
          )}

          {provider.providerType !== 'saml_2_0' && (
          <div>
            <Label htmlFor="edit-scopes" className="mb-1">{t('identity.scopes')}</Label>
            <Input
              variant="outline"
              id="edit-scopes"
              value={scopes}
              onChange={(e) => setScopes(e.target.value)}
            />
          </div>
          )}

          <div className="flex items-center justify-between rounded-lg border border-border p-3">
            <div>
              <p className="text-sm font-medium text-foreground">{t('identity.autoProvision')}</p>
              <p className="text-xs text-muted-foreground">{t('identity.autoProvisionDescription')}</p>
            </div>
            <Switch
              checked={autoProvision}
              onCheckedChange={setAutoProvision}
            />
          </div>

          <GroupMappingEditor
            autoImport={autoImportGroups}
            enabled={groupMappingEnabled}
            mappings={groupMappings}
            providerId={provider.id}
            supportsFetchGroups={provider.providerType !== 'generic_oidc' && provider.providerType !== 'saml_2_0'}
            onAutoImportChange={setAutoImportGroups}
            onEnabledChange={setGroupMappingEnabled}
            onMappingsChange={setGroupMappings}
          />
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t('common:cancel', 'Cancel')}
          </Button>
          <Button
            onClick={handleSave}
            disabled={
              !name ||
              (
                provider.providerType !== 'saml_2_0' &&
                !clientId
              ) ||
              (provider.providerType === 'generic_oidc' && !issuerUrl) ||
              (provider.providerType === 'saml_2_0' && !metadataUrl) ||
              updateProvider.isPending
            }
          >
            {updateProvider.isPending ? '...' : t('common:save', 'Save')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
