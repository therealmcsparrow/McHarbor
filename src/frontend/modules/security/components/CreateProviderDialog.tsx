// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBrandAzure, IconBrandGoogle, IconShieldLock } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { useCreateProvider, type CreateProviderInput } from '../hooks/useIdentityProviders';
import { EntraIdForm } from './EntraIdForm';
import { GenericOIDCForm } from './GenericOIDCForm';
import { GoogleForm } from './GoogleForm';
import { SAMLForm } from './SAMLForm';

type CreateProviderDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

type ProviderType = 'entra_id' | 'google' | 'generic_oidc' | 'saml_2_0' | null;

const DEFAULT_ENTRA = {
  name: '',
  clientId: '',
  clientSecret: '',
  tenantId: '',
  scopes: 'openid profile email',
  autoProvision: true,
  autoImportGroups: false,
  groupMappingEnabled: false,
  groupMappings: [] as { providerGroup: string; mcharborGroupId: string }[],
};

const DEFAULT_GOOGLE = {
  name: '',
  clientId: '',
  clientSecret: '',
  domain: '',
  scopes: 'openid profile email',
  autoProvision: true,
  autoImportGroups: false,
  groupMappingEnabled: false,
  groupMappings: [] as { providerGroup: string; mcharborGroupId: string }[],
};

const DEFAULT_GENERIC = {
  name: '',
  issuerUrl: '',
  clientId: '',
  clientSecret: '',
  scopes: 'openid profile email',
  autoProvision: true,
  autoImportGroups: false,
  groupMappingEnabled: false,
  groupMappings: [] as { providerGroup: string; mcharborGroupId: string }[],
};

const DEFAULT_SAML = {
  name: '',
  metadataUrl: '',
  entityId: '',
  autoProvision: true,
  autoImportGroups: false,
  groupMappingEnabled: false,
  groupMappings: [] as { providerGroup: string; mcharborGroupId: string }[],
};

export function CreateProviderDialog({ open, onOpenChange }: CreateProviderDialogProps) {
  const { t } = useTranslation('security');
  const [step, setStep] = useState<'type' | 'config'>('type');
  const [providerType, setProviderType] = useState<ProviderType>(null);
  const [entraData, setEntraData] = useState(DEFAULT_ENTRA);
  const [googleData, setGoogleData] = useState(DEFAULT_GOOGLE);
  const [genericData, setGenericData] = useState(DEFAULT_GENERIC);
  const [samlData, setSamlData] = useState(DEFAULT_SAML);
  const createProvider = useCreateProvider();

  function reset() {
    setStep('type');
    setProviderType(null);
    setEntraData(DEFAULT_ENTRA);
    setGoogleData(DEFAULT_GOOGLE);
    setGenericData(DEFAULT_GENERIC);
    setSamlData(DEFAULT_SAML);
  }

  function handleOpenChange(value: boolean) {
    if (!value) reset();
    onOpenChange(value);
  }

  function handleSelectType(type: Exclude<ProviderType, null>) {
    setProviderType(type);
    setStep('config');
  }

  function handleBack() {
    setProviderType(null);
    setStep('type');
  }

  function handleCreate() {
    let input: CreateProviderInput;

    if (providerType === 'entra_id') {
      input = {
        name: entraData.name,
        providerType: 'entra_id',
        clientId: entraData.clientId,
        clientSecret: entraData.clientSecret,
        tenantId: entraData.tenantId,
        scopes: entraData.scopes,
        autoProvision: entraData.autoProvision,
        autoImportGroups: entraData.autoImportGroups,
        groupMappingEnabled: entraData.groupMappingEnabled,
        groupMappings: entraData.groupMappings,
      };
    } else if (providerType === 'google') {
      input = {
        name: googleData.name,
        providerType: 'google',
        clientId: googleData.clientId,
        clientSecret: googleData.clientSecret,
        domain: googleData.domain,
        scopes: googleData.scopes,
        autoProvision: googleData.autoProvision,
        autoImportGroups: googleData.autoImportGroups,
        groupMappingEnabled: googleData.groupMappingEnabled,
        groupMappings: googleData.groupMappings,
      };
    } else if (providerType === 'generic_oidc') {
      input = {
        name: genericData.name,
        providerType: 'generic_oidc',
        issuerUrl: genericData.issuerUrl,
        clientId: genericData.clientId,
        clientSecret: genericData.clientSecret,
        scopes: genericData.scopes,
        autoProvision: genericData.autoProvision,
        autoImportGroups: genericData.autoImportGroups,
        groupMappingEnabled: genericData.groupMappingEnabled,
        groupMappings: genericData.groupMappings,
      };
    } else {
      input = {
        name: samlData.name,
        providerType: 'saml_2_0',
        clientId: '',
        clientSecret: '',
        metadataUrl: samlData.metadataUrl,
        entityId: samlData.entityId || undefined,
        autoProvision: samlData.autoProvision,
        autoImportGroups: samlData.autoImportGroups,
        groupMappingEnabled: samlData.groupMappingEnabled,
        groupMappings: samlData.groupMappings,
      };
    }

    createProvider.mutate(input, {
      onSuccess: () => handleOpenChange(false),
    });
  }

  const isValid =
    providerType === 'entra_id'
      ? entraData.name && entraData.clientId && entraData.clientSecret && entraData.tenantId
      : providerType === 'google'
        ? googleData.name && googleData.clientId && googleData.clientSecret
        : providerType === 'generic_oidc'
          ? genericData.name && genericData.issuerUrl && genericData.clientId && genericData.clientSecret
          : providerType === 'saml_2_0'
            ? samlData.name && samlData.metadataUrl
          : false;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>
            {step === 'type' ? t('identity.selectType') : t('identity.configuration')}
          </DialogTitle>
          <DialogDescription>
            {step === 'type'
              ? t('identity.selectTypeDescription')
              : providerType === 'entra_id'
                ? t('identity.entraId')
                : providerType === 'google'
                  ? t('identity.google')
                  : providerType === 'generic_oidc'
                    ? t('identity.genericOidc')
                    : t('identity.saml2')}
          </DialogDescription>
        </DialogHeader>

        <div>
          {step === 'type' && (
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <Button
                variant="outline"
                onClick={() => handleSelectType('entra_id')}
                className="flex h-auto flex-col items-center gap-3 p-6"
              >
                <IconBrandAzure className="size-10 text-[#0078D4]" />
                <span className="text-sm font-medium text-foreground">
                  {t('identity.entraId')}
                </span>
              </Button>
              <Button
                variant="outline"
                onClick={() => handleSelectType('google')}
                className="flex h-auto flex-col items-center gap-3 p-6"
              >
                <IconBrandGoogle className="size-10 text-[#4285F4]" />
                <span className="text-sm font-medium text-foreground">
                  {t('identity.google')}
                </span>
              </Button>
              <Button
                variant="outline"
                onClick={() => handleSelectType('generic_oidc')}
                className="flex h-auto flex-col items-center gap-3 p-6"
              >
                <IconShieldLock className="size-10 text-sky-500" />
                <span className="text-sm font-medium text-foreground">
                  {t('identity.genericOidc')}
                </span>
              </Button>
              <Button
                variant="outline"
                onClick={() => handleSelectType('saml_2_0')}
                className="flex h-auto flex-col items-center gap-3 p-6"
              >
                <IconShieldLock className="size-10 text-emerald-500" />
                <span className="text-sm font-medium text-foreground">
                  {t('identity.saml2')}
                </span>
              </Button>
            </div>
          )}

          {step === 'config' && providerType === 'entra_id' && (
            <EntraIdForm data={entraData} onChange={setEntraData} />
          )}

          {step === 'config' && providerType === 'google' && (
            <GoogleForm data={googleData} onChange={setGoogleData} />
          )}

          {step === 'config' && providerType === 'generic_oidc' && (
            <GenericOIDCForm data={genericData} onChange={setGenericData} />
          )}

          {step === 'config' && providerType === 'saml_2_0' && (
            <SAMLForm
              data={samlData}
              onChange={setSamlData}
              introTitleKey="identity.saml2"
              introDescriptionKey="identity.samlCreateHint"
            />
          )}
        </div>

        {step === 'config' && (
          <DialogFooter>
            <Button variant="outline" onClick={handleBack}>
              {t('common:back', 'Back')}
            </Button>
            <Button
              onClick={handleCreate}
              disabled={!isValid || createProvider.isPending}
            >
              {createProvider.isPending ? '...' : t('identity.addProvider')}
            </Button>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
