// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconLock, IconMessageCircle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Spinner } from '@resources/components/ui/Spinner';
import { ErrorState } from '@resources/components/ui/ErrorState';
import { aiProviderIconNode } from './ai-provider-icons';
import type { PickerEntry, ProviderType } from '../hooks/useAIProviderPicker';

type AIProviderTypePickerProps = {
  enabledTypes: PickerEntry[];
  disabledTypes: PickerEntry[];
  hasAnyAIPluginInstalled: boolean;
  loading: boolean;
  error: unknown;
  onRetry: () => void;
  onSelect: (type: ProviderType) => void;
};

const PRIMARY_ICON_CLASS = 'size-8 text-primary';
const MUTED_ICON_CLASS = 'size-8 text-muted-foreground';

/**
 * Step one of adding a connection: which provider.
 *
 * A tile whose plugin is installed but not loaded renders locked and links
 * to the plugin page rather than disappearing — "it is off" is a more
 * useful answer than silence when the admin knows they installed it.
 */
export function AIProviderTypePicker({
  enabledTypes,
  disabledTypes,
  hasAnyAIPluginInstalled,
  loading,
  error,
  onRetry,
  onSelect,
}: AIProviderTypePickerProps) {
  const { t } = useTranslation('settings');

  if (error) {
    return <ErrorState variant="card" error={error} onRetry={onRetry} />;
  }

  if (loading) {
    return (
      <output className="flex items-center justify-center py-12">
        <Spinner size="lg" />
        <span className="sr-only">{t('common:dataGrid.loading')}</span>
      </output>
    );
  }

  if (!hasAnyAIPluginInstalled) {
    return (
      <div className="rounded-lg border border-dashed border-border bg-card p-6 text-center">
        <IconMessageCircle className="mx-auto size-10 text-muted-foreground" />
        <h3 className="mt-3 text-sm font-medium text-foreground">{t('aiProviders.noPluginsInstalledTitle')}</h3>
        <p className="mt-1 text-sm text-muted-foreground">{t('aiProviders.noPluginsInstalledDescription')}</p>
        <Button asChild variant="outline" className="mt-4">
          <a href="/store?tab=plugins">{t('aiProviders.openAppStore')}</a>
        </Button>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
      {enabledTypes.map((entry) => (
        <Button
          key={entry.type}
          variant="outline"
          onClick={() => onSelect(entry.type)}
          className="flex h-auto flex-col items-center gap-3 p-4"
        >
          {aiProviderIconNode(entry.slug, entry.icon, PRIMARY_ICON_CLASS)}
          <span className="text-xs font-medium text-foreground">{entry.displayName}</span>
          <span className="text-[10px] text-muted-foreground">{entry.type}</span>
        </Button>
      ))}
      {disabledTypes.map((entry) => (
        <a
          key={entry.type}
          href="/store?tab=plugins"
          title={t('aiProviders.disabledTileHint')}
          aria-label={entry.displayName + ' — ' + t('aiProviders.disabledTileHint')}
          className="flex h-auto flex-col items-center gap-3 rounded-md border border-border bg-card p-4 opacity-60 transition-opacity hover:opacity-100"
        >
          <div className="relative">
            {aiProviderIconNode(entry.slug, entry.icon, MUTED_ICON_CLASS)}
            <IconLock className="absolute -right-1 -bottom-1 size-4 bg-card text-muted-foreground" aria-hidden="true" />
          </div>
          <span className="flex items-center gap-1 text-xs font-medium text-muted-foreground">{entry.displayName}</span>
          <span className="text-[10px] text-muted-foreground">{entry.type}</span>
        </a>
      ))}
    </div>
  );
}
