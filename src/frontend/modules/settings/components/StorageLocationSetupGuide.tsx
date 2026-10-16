// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { useIsNative, useIsSandboxed } from '@resources/hooks/useInstallKind';
import { IconInfoCircle } from '@tabler/icons-react';
import type { StorageLocationType } from '@resources/hooks/useStorageLocations';
import { useStorageAdapters } from '../hooks/useStorageAdapters';
import { storageProvider } from './storage-location-options';

type StorageLocationSetupGuideProps = {
  locationType: StorageLocationType;
};

// The host only ships the `local` built-in setup guide. Every
// other location type is plugin-owned and renders via the
// plugin's SetupGuide.tsx bundle (mounted by ProviderFormOutlet
// in StorageLocationDialog.tsx). The plugin delivers its own
// localized strings; the host stays out of the way.
const LOCAL_STEPS: string[] = ['localMount', 'localPath', 'basePath'];
const LOCAL_STEPS_SANDBOXED: string[] = ['localMountNative', 'localPathNative', 'basePath'];
const LOCAL_STEPS_NATIVE_ROOT: string[] = ['localMountNativeRoot', 'basePath'];

export function StorageLocationSetupGuide({ locationType }: StorageLocationSetupGuideProps) {
  const { t } = useTranslation('settings');
  const native = useIsNative();
  const sandboxed = useIsSandboxed();
  const { data: pluginAdapters } = useStorageAdapters();
  const provider = storageProvider(locationType, pluginAdapters);

  // Plugin-owned destinations render their own setup guide via
  // ProviderFormOutlet in StorageLocationDialog.tsx. The host
  // never shows the redirect URI either — the plugin owns its
  // own OAuth endpoint and the operator configures the OAuth
  // client at the provider's developer console.
  const steps =
    locationType !== 'local' ? [] : sandboxed ? LOCAL_STEPS_SANDBOXED : native ? LOCAL_STEPS_NATIVE_ROOT : LOCAL_STEPS;

  return (
    <div className="rounded-lg border border-border bg-muted/30 p-4">
      <div className="flex items-start gap-3">
        <IconInfoCircle className="mt-0.5 size-5 shrink-0 text-primary" />
        <div className="min-w-0 flex-1 space-y-3">
          <div>
            <h3 className="text-sm font-medium text-foreground">
              {t('storage.setupGuideTitle', {
                provider: provider.labelIsKey ? t(provider.label) : provider.label,
              })}
            </h3>
            <p className="mt-1 text-xs text-muted-foreground">{t('storage.setupGuideDescription')}</p>
          </div>

          {steps.length > 0 && (
            <ol className="list-decimal space-y-1.5 pl-4 text-xs text-muted-foreground">
              {steps.map((step) => (
                <li key={step}>{t(`storage.setupGuide.${step}`)}</li>
              ))}
            </ol>
          )}
        </div>
      </div>
    </div>
  );
}
