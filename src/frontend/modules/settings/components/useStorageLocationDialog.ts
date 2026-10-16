// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import {
  type StorageLocation,
  type StorageLocationInput,
  type StorageLocationType,
  useAuthorizeStorageLocation,
  useCreateStorageLocation,
  useStorageLocationSecrets,
  useUpdateStorageLocation,
} from '@resources/hooks/useStorageLocations';
import { useStorageAdapters } from '../hooks/useStorageAdapters';
import { storageProvider, storageProviderOptionsWithPlugins } from './storage-location-options';
import {
  DEFAULT_FORM,
  cleanInput,
  formFromLocation,
  secretsToFormPatch,
  supportsConsent,
} from './storage-location-form';

type Options = {
  isOpen: boolean;
  onOpenChange: (open: boolean) => void;
  storageLocation?: StorageLocation | null;
};

export function useStorageLocationDialog({ isOpen, onOpenChange, storageLocation }: Options) {
  const [step, setStep] = useState<'type' | 'config'>('type');
  const [form, setForm] = useState<StorageLocationInput>(DEFAULT_FORM);
  const [helpOpen, setHelpOpen] = useState(false);
  const createLocation = useCreateStorageLocation();
  const updateLocation = useUpdateStorageLocation();
  const authorizeLocation = useAuthorizeStorageLocation();
  const { data: pluginAdapters } = useStorageAdapters();
  // The GET /api/storage-locations/{id} endpoint deliberately omits
  // credentials, so opening an existing destination for edit
  // wouldn't show the populated clientId / clientSecret / etc. We
  // fetch the decrypted credentials from a dedicated endpoint and
  // merge them into the form once they arrive. The query is only
  // armed when editing an existing location — creating a new one
  // has no secrets to fetch.
  const secretsQuery = useStorageLocationSecrets(storageLocation?.id, !!storageLocation);
  const isEdit = !!storageLocation;
  const provider = storageProvider(form.locationType, pluginAdapters);
  const providerOptions = storageProviderOptionsWithPlugins(pluginAdapters);

  const [seeded, setSeeded] = useState<{ storageLocation: StorageLocation | null | undefined; isOpen: boolean }>({
    storageLocation: undefined,
    isOpen: false,
  });
  if (seeded.storageLocation !== storageLocation || seeded.isOpen !== isOpen) {
    setSeeded({ storageLocation, isOpen });
    if (isOpen) {
      setForm(formFromLocation(storageLocation));
      setStep(storageLocation ? 'config' : 'type');
      setHelpOpen(false);
    }
  }

  // Merge decrypted secrets into the form once the dedicated
  // endpoint returns. Plugin-owned destinations read their values
  // through the typed `form` state (clientId, clientSecret,
  // basePath) so the plugin's ConfigForm populates without any
  // additional plumbing.
  const [mergedSecrets, setMergedSecrets] = useState<typeof secretsQuery.data>(undefined);
  if (secretsQuery.data !== mergedSecrets) {
    setMergedSecrets(secretsQuery.data);
    if (secretsQuery.data) {
      setForm((current) => ({ ...current, ...secretsToFormPatch(secretsQuery.data) }));
    }
  }

  function update(patch: Partial<StorageLocationInput>) {
    setForm((current) => ({ ...current, ...patch }));
  }

  function handleOpenChange(value: boolean) {
    if (!value) {
      setStep('type');
      setForm(DEFAULT_FORM);
      setHelpOpen(false);
    }
    onOpenChange(value);
  }

  function handleSelectType(locationType: StorageLocationType) {
    if (locationType === 'local') {
      update({
        locationType,
        basePath: form.basePath,
        authMethod: undefined,
        port: undefined,
        tlsMode: undefined,
      });
      setStep('config');
      return;
    }
    if (locationType === 'ftp') {
      update({
        locationType,
        authMethod: 'password',
        passiveMode: true,
        port: 21,
        tlsMode: undefined,
      });
      setStep('config');
      return;
    }
    // Every other type is plugin-owned: the plugin's ConfigForm applies
    // its own defaults, so the host does not special-case it here.
    update({ locationType });
    setStep('config');
  }

  function handleBack() {
    setStep('type');
    setHelpOpen(false);
  }

  function connectLocation(id: string) {
    authorizeLocation.mutate(id, {
      onSuccess: (data) => {
        window.location.assign(data.authorizationUrl);
      },
    });
  }

  function handleSave(connectAfterSave = false) {
    const body = cleanInput(form);
    if (isEdit && storageLocation) {
      updateLocation.mutate(
        { id: storageLocation.id, ...body },
        {
          onSuccess: (updated) => {
            if (connectAfterSave && supportsConsent(updated.locationType)) {
              connectLocation(updated.id);
              return;
            }
            onOpenChange(false);
          },
        },
      );
      return;
    }

    createLocation.mutate(body, {
      onSuccess: (created) => {
        if (connectAfterSave && supportsConsent(created.locationType)) {
          connectLocation(created.id);
          return;
        }
        onOpenChange(false);
      },
    });
  }

  const saving = createLocation.isPending || updateLocation.isPending || authorizeLocation.isPending;
  const valid = form.name.trim() !== '' && form.locationType !== undefined;
  const canSaveAndConnect = supportsConsent(form.locationType);

  return {
    step,
    form,
    helpOpen,
    setHelpOpen,
    isEdit,
    provider,
    providerOptions,
    update,
    handleOpenChange,
    handleSelectType,
    handleBack,
    handleSave,
    saving,
    valid,
    canSaveAndConnect,
  };
}
