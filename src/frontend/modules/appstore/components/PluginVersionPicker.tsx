// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { Select } from '@resources/components/ui/Select';
import type { PluginEntry, PluginVersion } from '../types/plugins';

interface PluginVersionPickerProps {
  entry: PluginEntry;
  versions: PluginVersion[];
  selectedVersion?: PluginVersion | null;
  onSelectedVersionChange?: (version: PluginVersion) => void;
}

// The pre-install picker body of PluginInstallDialog: a version selector
// with the latest catalog entry as the default.
export function PluginVersionPicker({
  entry,
  versions,
  selectedVersion,
  onSelectedVersionChange,
}: PluginVersionPickerProps) {
  const { t } = useTranslation(['common', 'plugins']);

  return (
    <div className="flex flex-col gap-4 px-4 py-4">
      <p className="text-sm text-muted-foreground">
        {t('plugins.catalog.pickerHint', {
          ns: 'plugins',
        })}
      </p>
      <div className="flex items-center gap-2">
        <span className="shrink-0 text-xs font-medium text-muted-foreground">
          {t('plugins.catalog.versionLabel', { ns: 'plugins' })}
        </span>
        <div className="min-w-0 flex-1">
          <Select
            value={selectedVersion?.version ?? versions[0]?.version ?? ''}
            onChange={(v) => {
              const next = versions.find((x) => x.version === v);
              if (next && onSelectedVersionChange) onSelectedVersionChange(next);
            }}
            options={versions.map((v, i) => ({
              value: v.version,
              label: i === 0 ? `${v.version} (${t('plugins.catalog.latestLabel', { ns: 'plugins' })})` : v.version,
            }))}
            ariaLabel={t('plugins.catalog.versionSelectorLabel', {
              name: entry.name,
              ns: 'plugins',
            })}
            searchable={false}
          />
        </div>
      </div>
    </div>
  );
}
