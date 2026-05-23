// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { IconLoader2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Badge } from '@resources/components/ui/Badge';
import { Switch } from '@resources/components/ui/Switch';
import {
  useScannerSettings,
  useSaveScannerSettings,
  useAvailableScanners,
  type ScannerSettingsData,
} from '@resources/hooks/useScannerSettings';

export function ScannersTab() {
  const { t } = useTranslation('security');
  const { data: settings, isLoading } = useScannerSettings();
  const { data: scanners } = useAvailableScanners();
  const saveMutation = useSaveScannerSettings();

  const [form, setForm] = useState<ScannerSettingsData>({
    trivyEnabled: true,
    grypeEnabled: false,
    clairEnabled: false,
    clairUrl: '',
    defaultScanner: 'trivy',
    scanTimeout: 300,
    scanOnInstall: false,
    scanOnUpdate: false,
  });

  useEffect(() => {
    if (settings) {
      setForm(settings);
    }
  }, [settings]);

  const availabilityMap = new Map(
    (scanners?.scanners ?? []).map((s) => [s.name, s.available])
  );

  function handleSave() {
    saveMutation.mutate(form);
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <IconLoader2 className="size-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  const enabledScanners = [
    form.trivyEnabled && 'trivy',
    form.grypeEnabled && 'grype',
    form.clairEnabled && 'clair',
  ].filter(Boolean) as string[];

  return (
    <div className="space-y-6">
      <div className="rounded-lg border border-border bg-card p-6">
        <h3 className="mb-1 text-sm font-semibold text-foreground">
          {t('scanning.title')}
        </h3>
        <p className="mb-6 text-xs text-muted-foreground">
          {t('scanning.description')}
        </p>

        <div className="space-y-4">
          {/* Trivy */}
          <ScannerToggle
            label={t('scanning.trivyEnabled')}
            description={t('scanning.trivyDescription')}
            checked={form.trivyEnabled}
            available={availabilityMap.get('trivy')}
            onChange={(v) => setForm((f) => ({ ...f, trivyEnabled: v }))}
            t={t}
          />

          {/* Grype */}
          <ScannerToggle
            label={t('scanning.grypeEnabled')}
            description={t('scanning.grypeDescription')}
            checked={form.grypeEnabled}
            available={availabilityMap.get('grype')}
            onChange={(v) => setForm((f) => ({ ...f, grypeEnabled: v }))}
            t={t}
          />

          {/* Clair */}
          <ScannerToggle
            label={t('scanning.clairEnabled')}
            description={t('scanning.clairDescription')}
            checked={form.clairEnabled}
            available={availabilityMap.get('clair')}
            onChange={(v) => setForm((f) => ({ ...f, clairEnabled: v }))}
            t={t}
          />

          {/* Clair URL */}
          {form.clairEnabled && (
            <div className="ml-12">
              <label className="mb-1 block text-xs font-medium text-muted-foreground">
                {t('scanning.clairUrl')}
              </label>
              <input
                type="url"
                value={form.clairUrl}
                onChange={(e) => setForm((f) => ({ ...f, clairUrl: e.target.value }))}
                placeholder={t('scanning.clairUrlPlaceholder')}
                className="w-full max-w-md rounded-md border border-border bg-muted px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none"
              />
            </div>
          )}
        </div>

        {/* Default scanner + timeout */}
        <div className="mt-6 grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              {t('scanning.defaultScanner')}
            </label>
            <select
              value={form.defaultScanner}
              onChange={(e) => setForm((f) => ({ ...f, defaultScanner: e.target.value }))}
              className="w-full rounded-md border border-border bg-muted px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none"
            >
              {enabledScanners.map((name) => (
                <option key={name} value={name}>
                  {name.charAt(0).toUpperCase() + name.slice(1)}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-xs font-medium text-muted-foreground">
              {t('scanning.scanTimeout')}
            </label>
            <div className="flex items-center gap-2">
              <input
                type="number"
                min={30}
                max={1800}
                value={form.scanTimeout}
                onChange={(e) =>
                  setForm((f) => ({ ...f, scanTimeout: parseInt(e.target.value) || 300 }))
                }
                className="w-32 rounded-md border border-border bg-muted px-3 py-1.5 text-sm text-foreground focus:border-primary focus:outline-none"
              />
              <span className="text-xs text-muted-foreground">
                {t('scanning.scanTimeoutSuffix')}
              </span>
            </div>
          </div>
        </div>

        {/* Automatic scanning */}
        <div className="mt-6 border-t border-border pt-6">
          <h4 className="mb-1 text-sm font-semibold text-foreground">
            {t('scanning.autoScanTitle')}
          </h4>
          <p className="mb-4 text-xs text-muted-foreground">
            {t('scanning.autoScanDescription')}
          </p>
          <div className="space-y-4">
            <ScannerToggle
              label={t('scanning.scanOnInstall')}
              description={t('scanning.scanOnInstallDescription')}
              checked={form.scanOnInstall}
              onChange={(v) => setForm((f) => ({ ...f, scanOnInstall: v }))}
              t={t}
            />
            <ScannerToggle
              label={t('scanning.scanOnUpdate')}
              description={t('scanning.scanOnUpdateDescription')}
              checked={form.scanOnUpdate}
              onChange={(v) => setForm((f) => ({ ...f, scanOnUpdate: v }))}
              t={t}
            />
          </div>
        </div>

        <div className="mt-6">
          <Button onClick={handleSave} disabled={saveMutation.isPending}>
            {saveMutation.isPending && (
              <IconLoader2 className="mr-2 size-4 animate-spin" />
            )}
            {t('scanning.save')}
          </Button>
        </div>
      </div>
    </div>
  );
}

function ScannerToggle({
  label,
  description,
  checked,
  available,
  onChange,
  t,
}: {
  label: string;
  description: string;
  checked: boolean;
  available?: boolean;
  onChange: (val: boolean) => void;
  t: (key: string) => string;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="flex items-start gap-3">
        <Switch
          checked={checked}
          aria-label={label}
          className="mt-0.5"
          onCheckedChange={onChange}
        />
        <div>
          <span className="text-sm font-medium text-foreground">{label}</span>
          <p className="text-xs text-muted-foreground">{description}</p>
        </div>
      </div>
      {available !== undefined && (
        <Badge variant={available ? 'success' : 'secondary'} className="shrink-0">
          {available ? t('scanning.available') : t('scanning.unavailable')}
        </Badge>
      )}
    </div>
  );
}
