// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.
//
// EditProviderDialog — the edit counterpart to CreateProviderDialog.
//
// Before migration 125 this dialog carried one hardcoded branch per
// provider type (`providerType === 'entra_id' ? tenantId : ...`), which
// meant a newly installed identity plugin was editable only if the host
// happened to have a column and a branch for each of its fields. It now
// renders the plugin's own ConfigForm through PluginFormOutlet, seeded
// from the row's `config` map, so a plugin with more or fewer fields
// than the host knows about edits correctly with no host change.
//
// The host still owns what the host stores in typed columns: the
// display name, auto-provisioning, and group mapping. Everything
// protocol-specific belongs to the plugin.
//
// Secrets are never sent back to the browser — the API redacts them out
// of `config`. A secret field left blank therefore means "keep what is
// stored"; Service.mergeConfig implements that side of the contract.

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { IconHelpCircle } from '@tabler/icons-react';
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
import { PluginFormOutlet } from '@core/plugins/PluginFormOutlet';
import { SetupGuidePanel } from '@resources/components/SetupGuidePanel';
import { useIdentityAdapters } from '@resources/hooks/useIdentityAdapters';
import {
  useUpdateProvider,
  type IdentityProvider as IdentityProviderValue,
  type UpdateProviderInput,
  type GroupMapping,
} from '../hooks/useIdentityProviders';
import { GroupMappingEditor } from './GroupMappingEditor';
import { ProviderEndpointUrls } from './identity/ProviderEndpointUrls';
import { MappingPreviewPanel } from './identity/MappingPreviewPanel';
import { MappingImpactConfirmDialog } from './identity/MappingImpactConfirmDialog';
import { PERM } from '@core/auth/permissionKeys';

type EditProviderDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  provider: IdentityProviderValue;
};

// seedConfig builds the map handed to the plugin's ConfigForm. Rows
// written before migration 125 have an empty `config` but populated
// typed columns, so the typed values go in first and the stored map
// wins on top. Secrets are absent from both — blank means "unchanged".
function seedConfig(provider: IdentityProviderValue): Record<string, string> {
  const seed: Record<string, string> = {};
  const put = (key: string, value?: string) => {
    if (value) seed[key] = value;
  };
  put('client_id', provider.clientId);
  put('tenant_id', provider.tenantId);
  put('domain', provider.domain);
  put('issuer_url', provider.issuerUrl);
  put('metadata_url', provider.metadataUrl);
  put('entity_id', provider.entityId);
  put('scopes', provider.scopes);
  return { ...seed, ...provider.config };
}

export function EditProviderDialog({ open: isOpen, onOpenChange, provider }: EditProviderDialogProps) {
  const { t } = useTranslation('security');
  const [itemName, setName] = useState(provider.name);
  const [config, setConfig] = useState<Record<string, string>>(() => seedConfig(provider));
  const [autoProvision, setAutoProvision] = useState(provider.autoProvision);
  const [autoImportGroups, setAutoImportGroups] = useState(provider.autoImportGroups);
  const [groupMappingEnabled, setGroupMappingEnabled] = useState(provider.groupMappingEnabled);
  const [groupMappings, setGroupMappings] = useState<GroupMapping[]>(provider.groupMappings ?? []);
  const [helpOpen, setHelpOpen] = useState(false);
  const updateProvider = useUpdateProvider();
  const { data: adapters } = useIdentityAdapters();

  const spec = adapters?.[provider.providerType];
  const capabilities = spec?.Capabilities;
  const [confirmOpen, setConfirmOpen] = useState(false);
  const mappingConfig = { groupMappingEnabled, groupMappings, autoImportGroups, autoProvision };
  const mappingChanged =
    groupMappingEnabled !== provider.groupMappingEnabled ||
    autoImportGroups !== provider.autoImportGroups ||
    JSON.stringify(groupMappings) !== JSON.stringify(provider.groupMappings ?? []);

  function handleSave() {
    if (mappingChanged && !confirmOpen) {
      updateProvider.reset();
      setConfirmOpen(true);
      return;
    }
    const input: UpdateProviderInput & { id: string } = { id: provider.id };

    if (itemName !== provider.name) input.name = itemName;
    if (autoProvision !== provider.autoProvision) input.autoProvision = autoProvision;
    if (autoImportGroups !== provider.autoImportGroups) input.autoImportGroups = autoImportGroups;
    if (groupMappingEnabled !== provider.groupMappingEnabled) {
      input.groupMappingEnabled = groupMappingEnabled;
    }
    input.groupMappings = groupMappings;
    // Always sent: the host merges rather than replaces, so an
    // untouched secret keeps its stored value.
    input.config = config;

    updateProvider.mutate(input, { onSuccess: () => onOpenChange(false) });
  }

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('identity.editProvider')}</DialogTitle>
          <DialogDescription>{spec?.DisplayName ?? provider.providerType}</DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div>
            <Label htmlFor="edit-name" className="mb-1">
              {t('identity.name')}
            </Label>
            <Input variant="outline" id="edit-name" value={itemName} onChange={(e) => setName(e.target.value)} />
          </div>

          <PluginFormOutlet
            resource="identity_provider"
            kind="config_form"
            keyName={provider.providerType}
            value={config}
            onChange={setConfig}
            i18nPrefix="plugin"
          />

          <p className="text-xs text-muted-foreground">{t('identity.secretsBlankHint')}</p>

          {helpOpen && (
            <SetupGuidePanel>
              <PluginFormOutlet
                resource="identity_provider"
                kind="setup_guide"
                keyName={provider.providerType}
                value={config}
                onChange={() => {
                  /* setup guides are read-only; no-op to satisfy the outlet API */
                }}
                i18nPrefix="plugin"
              />
            </SetupGuidePanel>
          )}

          <ProviderEndpointUrls providerId={provider.id} capabilities={capabilities} />

          <div className="flex items-center justify-between rounded-lg border border-border p-3">
            <div>
              <p className="text-sm font-medium text-foreground">{t('identity.autoProvision')}</p>
              <p className="text-xs text-muted-foreground">{t('identity.autoProvisionDescription')}</p>
            </div>
            <Switch checked={autoProvision} onCheckedChange={setAutoProvision} />
          </div>

          <GroupMappingEditor
            autoImport={autoImportGroups}
            enabled={groupMappingEnabled}
            mappings={groupMappings}
            providerId={provider.id}
            supportsFetchGroups={Boolean(capabilities?.SupportsGroupFetch)}
            onAutoImportChange={setAutoImportGroups}
            onEnabledChange={setGroupMappingEnabled}
            onMappingsChange={setGroupMappings}
          />
          <MappingPreviewPanel
            providerId={provider.id}
            providerType={provider.providerType}
            config={mappingConfig}
            canFetchGroups={Boolean(capabilities?.SupportsGroupFetch)}
          />
        </div>

        <DialogFooter className="flex-col items-stretch gap-2 sm:flex-row sm:items-center sm:justify-between">
          <Button variant="ghost" onClick={() => setHelpOpen((current) => !current)} aria-expanded={helpOpen}>
            <IconHelpCircle className="size-4" />
            {helpOpen ? t('identity.hideSetupGuide') : t('identity.showSetupGuide')}
          </Button>
          <div className="flex flex-wrap items-center justify-end gap-2">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              {t('common:actions.cancel')}
            </Button>
            <Button
              permission={PERM.IDENTITY_PROVIDERS_MANAGE}
              onClick={handleSave}
              disabled={!itemName || updateProvider.isPending}
            >
              {updateProvider.isPending ? '...' : t('common:actions.save')}
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
      <MappingImpactConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        providerId={provider.id}
        providerType={provider.providerType}
        config={mappingConfig}
        saving={updateProvider.isPending}
        saveError={updateProvider.isError ? updateProvider.error.message : undefined}
        onConfirm={handleSave}
      />
    </Dialog>
  );
}
