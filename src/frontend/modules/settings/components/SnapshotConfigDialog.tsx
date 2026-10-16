// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import {
  Dialog,
  DialogBody,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@resources/components/ui/Dialog';
import { Input } from '@resources/components/ui/Input';
import { Label } from '@resources/components/ui/Label';
import { Spinner } from '@resources/components/ui/Spinner';
import { Switch } from '@resources/components/ui/Switch';
import { Button } from '@resources/components/ui/Button';
import { Select } from '@resources/components/ui/Select';
import type { SnapshotConfig, SnapshotConfigWrite } from '../hooks/useSnapshotConfigs';
import { useSnapshotConfigForm } from '../hooks/useSnapshotConfigForm';
import { SchemaFormField } from './SchemaFormField';

interface Props {
  open: boolean;
  config?: SnapshotConfig | null;
  /**
   * Driver to select on open, set when the operator clicked "Set up" on
   * a detected driver. Saves them from picking a driver they were just
   * told they have.
   */
  presetDriver?: string;
  onOpenChange: (open: boolean) => void;
  onSave: (body: SnapshotConfigWrite) => void;
  saving?: boolean;
}

export function SnapshotConfigDialog({ open: isOpen, config, presetDriver, onOpenChange, onSave, saving }: Props) {
  const { t } = useTranslation('settings');
  const {
    adapters,
    adapterList,
    selectedAdapter,
    form,
    setForm,
    configJson,
    setConfigJson,
    configError,
    schemaValues,
    setSchemaValues,
    pickAdapter,
    handleSubmit,
  } = useSnapshotConfigForm({ open: isOpen, config, presetDriver, onSave });

  const schemaFields = selectedAdapter?.ConfigSchema ?? [];
  const storedSecrets = new Set(config?.secretsSet ?? []);

  return (
    <Dialog open={isOpen} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{config ? t('snapshots.configs.edit') : t('snapshots.configs.add')}</DialogTitle>
          <DialogDescription>{t('snapshots.configs.subtitle')}</DialogDescription>
        </DialogHeader>
        <form onSubmit={handleSubmit}>
          <DialogBody className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="snap-name">{t('snapshots.configs.nameLabel')}</Label>
              <Input
                id="snap-name"
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder={t('snapshots.configs.namePlaceholder')}
                required
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="snap-driver">{t('snapshots.configs.driverLabel')}</Label>
              <Select
                value={form.driver}
                onChange={pickAdapter}
                placeholder={
                  adapters.isLoading ? t('snapshots.configs.loading') : t('snapshots.configs.driverPlaceholder')
                }
                disabled={adapters.isLoading}
                ariaLabel={t('snapshots.configs.driverLabel')}
                options={adapterList.map((a) => ({
                  value: a.Driver,
                  label: a.DisplayName,
                }))}
              />
              {selectedAdapter?.Description && (
                <p className="text-xs text-muted-foreground">{selectedAdapter.Description}</p>
              )}
              {adapterList.length === 0 && !adapters.isLoading && (
                <p className="text-xs text-muted-foreground">{t('snapshots.configs.noAdaptersHint')}</p>
              )}
            </div>
            {/* Plugin-declared ConfigSchema (typed fields). Each
                adapter's Spec carries the form definition; the host
                stores the values inside the row's `config` JSON
                column keyed by field.key. An adapter with no fields
                sends null rather than [], so the list is normalised
                before it is measured. */}
            {schemaFields.length > 0 && (
              <div className="space-y-3 rounded-lg border border-border p-3">
                <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                  {t('snapshots.configs.pluginFieldsLabel')}
                </p>
                {schemaFields.map((field) => (
                  <SchemaFormField
                    key={field.key}
                    field={
                      field.type === 'secret' && storedSecrets.has(field.key)
                        ? { ...field, placeholder: t('logging.secretStored') }
                        : field
                    }
                    i18nNs={selectedAdapter?.Slug}
                    value={schemaValues[field.key] ?? ''}
                    onChange={(value) => setSchemaValues((prev) => ({ ...prev, [field.key]: value }))}
                  />
                ))}
              </div>
            )}
            <div className="flex items-center justify-between rounded-lg border border-border p-3">
              <div>
                <Label className="text-sm font-medium">{t('snapshots.configs.enabledLabel')}</Label>
              </div>
              <Switch checked={form.enabled} onCheckedChange={(checked) => setForm({ ...form, enabled: checked })} />
            </div>
            <details className="rounded-lg border border-border p-3">
              <summary className="cursor-pointer text-sm font-medium text-muted-foreground">
                {t('snapshots.configs.advancedLabel')}
              </summary>
              <div className="mt-3 space-y-2">
                <Label htmlFor="snap-config">{t('snapshots.configs.configLabel')}</Label>
                <textarea
                  id="snap-config"
                  value={configJson}
                  onChange={(e) => setConfigJson(e.target.value)}
                  placeholder={t('snapshots.configs.configPlaceholder')}
                  className="min-h-[120px] w-full rounded-md border border-border bg-background px-3 py-2 font-mono text-sm"
                />
                {configError && <p className="text-sm text-destructive">{configError}</p>}
              </div>
            </details>
          </DialogBody>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={saving}>
              {t('snapshots.configs.cancel')}
            </Button>
            <Button type="submit" disabled={saving || !form.name.trim() || !form.driver.trim()}>
              {saving ? <Spinner className="size-4" /> : null}
              {t('snapshots.configs.save')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
