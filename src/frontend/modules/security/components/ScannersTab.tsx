// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useState } from 'react';
import { useNavigate } from 'react-router';
import { useTranslation } from 'react-i18next';
import { IconLoader2, IconPlus, IconShieldSearch } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Switch } from '@resources/components/ui/Switch';
import { PERM } from '@core/auth/permissionKeys';
import {
  useScannerSettings,
  useSaveScannerSettings,
  type ScannerSettingsData,
} from '@resources/hooks/useScannerSettings';
import { useScannerAdapters, useScannerConfigs, type ScannerConfig } from '../hooks/useScanners';
import { ScannerCard } from './ScannerCard';
import { ScannerDialog } from './ScannerDialog';

const DEFAULT_SETTINGS: ScannerSettingsData = {
  scanTimeout: 300,
  scanOnInstall: false,
  scanOnUpdate: false,
};

/**
 * Security → Scanners: which engines scan this estate, and when.
 *
 * Two halves, and the split is the point. The top half lists
 * *configured scanners* — named instances of whatever `scanner-`
 * plugins are installed, added through the same picker-then-form flow
 * the identity, communication, storage, AI and logging tabs use. The
 * bottom half holds the handful of settings that are the host's rather
 * than any engine's: how long a scan may take, and whether installs and
 * updates scan automatically.
 *
 * This replaced three hard-coded toggles (Trivy / Grype / Clair) plus a
 * single global Clair URL. That shape could not express two Clair
 * servers, could not accommodate a fourth engine, and put engine names
 * in the host's settings table — all three of which the plugin family
 * fixes.
 */
export function ScannersTab() {
  const { t } = useTranslation('security');
  const navigate = useNavigate();

  const { data: adapters = [], isLoading: adaptersLoading } = useScannerAdapters();
  const { data: scanners = [], isLoading: scannersLoading } = useScannerConfigs();
  const { data: settings, isLoading: settingsLoading } = useScannerSettings();
  const saveSettings = useSaveScannerSettings();

  const [createOpen, setCreateOpen] = useState(false);
  const [editScanner, setEditScanner] = useState<ScannerConfig | null>(null);
  const [form, setForm] = useState<ScannerSettingsData>(DEFAULT_SETTINGS);

  const [syncedSettings, setSyncedSettings] = useState<typeof settings>(undefined);
  if (settings !== syncedSettings) {
    setSyncedSettings(settings);
    if (settings) setForm(settings);
  }

  if (adaptersLoading || scannersLoading || settingsLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <IconLoader2 className="size-5 animate-spin text-muted-foreground" />
      </div>
    );
  }

  // No plugin installed at all: the Add button would open an empty
  // picker, so point at the store instead. Scanners configured before
  // the plugin was removed still list below, so an operator can see and
  // clean up what stopped.
  const nothingInstalled = adapters.length === 0 && scanners.length === 0;

  return (
    <div className="space-y-6">
      <div className="mc-glass rounded-lg p-6">
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <h3 className="mb-1 text-sm font-semibold text-foreground">{t('scanning.title')}</h3>
            <p className="text-xs text-muted-foreground">{t('scanning.description')}</p>
          </div>
          {!nothingInstalled && (
            <Button
              permission={PERM.SETTINGS_MANAGE}
              onClick={() => setCreateOpen(true)}
              disabled={adapters.length === 0}
              title={adapters.length === 0 ? t('scanning.noAdapters') : undefined}
            >
              <IconPlus className="size-4" />
              {t('scanning.addScanner')}
            </Button>
          )}
        </div>

        <div className="mt-6">
          {nothingInstalled ? (
            <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12 text-center">
              <IconShieldSearch className="size-10 text-muted-foreground" />
              <p className="mt-3 text-sm font-medium text-foreground">{t('scanning.emptyTitle')}</p>
              <p className="mt-1 max-w-md text-sm text-muted-foreground">{t('scanning.emptyDescription')}</p>
              <Button
                className="mt-4"
                variant="outline"
                onClick={() => navigate('/app-store/plugins?category=scanner')}
              >
                {t('scanning.browsePlugins')}
              </Button>
            </div>
          ) : scanners.length > 0 ? (
            <div className="space-y-3">
              {scanners.map((scanner) => (
                <ScannerCard key={scanner.id} scanner={scanner} onEdit={setEditScanner} />
              ))}
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center rounded-lg border border-dashed border-border py-12 text-center">
              <IconShieldSearch className="size-10 text-muted-foreground" />
              <p className="mt-3 text-sm text-muted-foreground">{t('scanning.noScanners')}</p>
            </div>
          )}
        </div>
      </div>

      <div className="mc-glass rounded-lg p-6">
        <h4 className="mb-1 text-sm font-semibold text-foreground">{t('scanning.autoScanTitle')}</h4>
        <p className="mb-4 text-xs text-muted-foreground">{t('scanning.autoScanDescription')}</p>

        <div className="space-y-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <p className="text-sm font-medium text-foreground">{t('scanning.scanOnInstall')}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t('scanning.scanOnInstallDescription')}</p>
            </div>
            <Switch
              checked={form.scanOnInstall}
              onCheckedChange={(value) => setForm((current) => ({ ...current, scanOnInstall: value }))}
              aria-label={t('scanning.scanOnInstall')}
            />
          </div>
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <p className="text-sm font-medium text-foreground">{t('scanning.scanOnUpdate')}</p>
              <p className="mt-1 text-xs text-muted-foreground">{t('scanning.scanOnUpdateDescription')}</p>
            </div>
            <Switch
              checked={form.scanOnUpdate}
              onCheckedChange={(value) => setForm((current) => ({ ...current, scanOnUpdate: value }))}
              aria-label={t('scanning.scanOnUpdate')}
            />
          </div>
        </div>

        <div className="mt-6 max-w-xs">
          <label htmlFor="scan-timeout-input" className="mb-1 block text-xs font-medium text-muted-foreground">
            {t('scanning.scanTimeout')}
          </label>
          <div className="flex items-center gap-2">
            <NumberInput
              id="scan-timeout-input"
              min={30}
              max={1800}
              value={form.scanTimeout}
              onChange={(scanTimeout) =>
                setForm((current) => ({
                  ...current,
                  scanTimeout: Math.trunc(scanTimeout),
                }))
              }
              className="w-44"
            />
            <span className="text-xs text-muted-foreground">{t('scanning.scanTimeoutSuffix')}</span>
          </div>
        </div>

        <div className="mt-6">
          <Button
            permission={PERM.SETTINGS_MANAGE}
            onClick={() => saveSettings.mutate(form)}
            disabled={saveSettings.isPending}
          >
            {saveSettings.isPending && <IconLoader2 className="mr-2 size-4 animate-spin" />}
            {t('scanning.save')}
          </Button>
        </div>
      </div>

      <ScannerDialog open={createOpen} onOpenChange={setCreateOpen} adapters={adapters} />
      {editScanner && (
        <ScannerDialog
          open={!!editScanner}
          onOpenChange={(isOpen) => {
            if (!isOpen) setEditScanner(null);
          }}
          adapters={adapters}
          scanner={editScanner}
        />
      )}
    </div>
  );
}
