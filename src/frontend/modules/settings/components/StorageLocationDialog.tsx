// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconExternalLink, IconHelpCircle } from '@tabler/icons-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Button } from '@resources/components/ui/Button';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { ProviderFormOutlet } from '@core/plugins/ProviderFormOutlet';
import type { StorageLocation } from '@resources/hooks/useStorageLocations';
import { StorageLocationFormFields } from './StorageLocationFormFields';
import { StorageLocationSetupGuide } from './StorageLocationSetupGuide';
import { SetupGuidePanel } from '@resources/components/SetupGuidePanel';
import { pluginFormPatch, pluginFormValue } from './storage-location-form';
import { useStorageLocationDialog } from './useStorageLocationDialog';
import { PERM } from '@core/auth/permissionKeys';

type StorageLocationDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  location?: StorageLocation | null;
};

export function StorageLocationDialog({
  open: isOpen,
  onOpenChange,
  location: storageLocation,
}: StorageLocationDialogProps) {
  const { t } = useTranslation('settings');
  const {
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
  } = useStorageLocationDialog({ isOpen, onOpenChange, storageLocation });

  return (
    <Dialog open={isOpen} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-3xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit
              ? t('storage.editLocation')
              : step === 'type'
                ? t('storage.selectType')
                : t('storage.configuration')}
          </DialogTitle>
          <DialogDescription>
            {step === 'type'
              ? t('storage.selectTypeDescription')
              : provider.descriptionIsKey
                ? t(provider.description)
                : provider.description}
          </DialogDescription>
        </DialogHeader>

        <div>
          {step === 'type' && (
            <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
              {providerOptions.map((option) => (
                <Button
                  key={option.type}
                  variant="outline"
                  onClick={() => handleSelectType(option.type)}
                  className="flex h-auto flex-col items-center gap-3 p-4 text-center"
                >
                  <div className="flex size-8 items-center justify-center text-primary">{option.icon}</div>
                  <span className="text-xs font-medium text-foreground">
                    {option.labelIsKey ? t(option.label) : option.label}
                  </span>
                </Button>
              ))}
            </div>
          )}

          {step === 'config' && (
            <div className="space-y-4">
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <Label htmlFor="location-name" className="mb-1 text-xs text-muted-foreground">
                    {t('storage.nameLabel')}
                  </Label>
                  <Input
                    id="location-name"
                    value={form.name}
                    onChange={(changeEvent) => update({ name: changeEvent.target.value })}
                    placeholder={t('storage.namePlaceholder')}
                    variant="outline"
                  />
                </div>
                <div>
                  <Label className="mb-1 text-xs text-muted-foreground">{t('storage.typeLabel')}</Label>
                  <div className="mc-control flex items-center gap-2 rounded-lg border border-input bg-background px-3 text-sm text-foreground">
                    <span className="flex size-4 items-center justify-center text-primary">{provider.icon}</span>
                    {provider.labelIsKey ? t(provider.label) : provider.label}
                  </div>
                </div>
              </div>

              {/*
                ProviderFormOutlet mounts the plugin's ConfigForm
                when one is registered for the selected locationType
                (any plugin-owned storage type). The
                host's StorageLocationFormFields remains the fallback
                for built-in locationTypes. Both read/write the same
                `form` state so the operator's typed save path
                persists the plugin's clientId / clientSecret /
                basePath keys to first-class columns automatically.
              */}
              <ProviderFormOutlet
                resource="storage_location"
                kind="config_form"
                keyName={form.locationType}
                value={pluginFormValue(form)}
                onChange={(next) => update(pluginFormPatch(next))}
              >
                <StorageLocationFormFields data={form} isEdit={isEdit} onChange={update} />
              </ProviderFormOutlet>

              {helpOpen && (
                <ProviderFormOutlet
                  resource="storage_location"
                  kind="setup_guide"
                  keyName={form.locationType}
                  value={pluginFormValue(form)}
                  onChange={(next) => update(pluginFormPatch(next))}
                >
                  <SetupGuidePanel>
                    <StorageLocationSetupGuide locationType={form.locationType} />
                  </SetupGuidePanel>
                </ProviderFormOutlet>
              )}

              <p className="text-xs text-muted-foreground">{t('storage.credentialsDescription')}</p>
            </div>
          )}
        </div>

        {step === 'config' && (
          <DialogFooter className="flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:justify-between">
            <Button variant="ghost" onClick={() => setHelpOpen((current) => !current)} aria-expanded={helpOpen}>
              <IconHelpCircle className="size-4" />
              {helpOpen ? t('storage.hideSetupGuide') : t('storage.showSetupGuide')}
            </Button>
            <div className="flex flex-wrap items-center justify-end gap-2">
              {!isEdit && (
                <Button variant="outline" onClick={handleBack}>
                  {t('actions.back', { ns: 'common' })}
                </Button>
              )}
              <Button variant="outline" onClick={() => handleOpenChange(false)}>
                {t('actions.cancel', { ns: 'common' })}
              </Button>
              {canSaveAndConnect && (
                <Button
                  permission={PERM.SETTINGS_MANAGE}
                  variant="outline"
                  onClick={() => handleSave(true)}
                  disabled={!valid || saving}
                >
                  <IconExternalLink className="size-4" />
                  {saving ? t('actions.processing', { ns: 'common' }) : t('storage.saveAndConnect')}
                </Button>
              )}
              <Button permission={PERM.SETTINGS_MANAGE} onClick={() => handleSave(false)} disabled={!valid || saving}>
                {saving ? t('actions.processing', { ns: 'common' }) : t('actions.save', { ns: 'common' })}
              </Button>
            </div>
          </DialogFooter>
        )}
      </DialogContent>
    </Dialog>
  );
}
