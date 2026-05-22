// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconBrandAzure, IconBrandGoogle } from '@tabler/icons-react';
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
import { GoogleForm } from './GoogleForm';

type CreateProviderDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

type ProviderType = 'entra_id' | 'google' | null;

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

export function CreateProviderDialog({ open, onOpenChange }: CreateProviderDialogProps) {
  const { t } = useTranslation('security');
  const [step, setStep] = useState<'type' | 'config'>('type');
  const [providerType, setProviderType] = useState<ProviderType>(null);
  const [entraData, setEntraData] = useState(DEFAULT_ENTRA);
  const [googleData, setGoogleData] = useState(DEFAULT_GOOGLE);
  const createProvider = useCreateProvider();

  function reset() {
    setStep('type');
    setProviderType(null);
    setEntraData(DEFAULT_ENTRA);
    setGoogleData(DEFAULT_GOOGLE);
  }

  function handleOpenChange(value: boolean) {
    if (!value) reset();
    onOpenChange(value);
  }

  function handleSelectType(type: ProviderType) {
    setProviderType(type);
    setStep('config');
  }

  function handleBack() {
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
    } else {
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
                : t('identity.google')}
          </DialogDescription>
        </DialogHeader>

        <div>
          {step === 'type' && (
            <div className="grid grid-cols-2 gap-3">
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
            </div>
          )}

          {step === 'config' && providerType === 'entra_id' && (
            <EntraIdForm data={entraData} onChange={setEntraData} />
          )}

          {step === 'config' && providerType === 'google' && (
            <GoogleForm data={googleData} onChange={setGoogleData} />
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
