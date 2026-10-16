// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPuzzle } from '@tabler/icons-react';
import { Badge } from '@resources/components/ui/Badge';
import { Button } from '@resources/components/ui/Button';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import type { ScannerAdapter } from '../hooks/useScanners';

type ScannerEnginePickerProps = {
  adapters: ScannerAdapter[];
  onSelect: (adapter: ScannerAdapter) => void;
};

/** Step one of the scanner dialog: the grid of installed engines. */
export function ScannerEnginePicker({ adapters, onSelect }: ScannerEnginePickerProps) {
  const { t } = useTranslation('security');

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
      {adapters.map((item) => {
        return (
          <Button
            key={item.scannerType}
            variant="outline"
            onClick={() => onSelect(item)}
            className="flex h-auto flex-col items-center gap-2 p-4 text-center"
          >
            <div className="flex size-8 items-center justify-center text-primary">
              {item.slug && item.icon ? (
                <PluginAssetImage slug={item.slug} icon={item.icon} className="size-6" />
              ) : (
                <IconPuzzle className="size-6" />
              )}
            </div>
            <span className="text-xs font-medium text-foreground">{item.displayName}</span>
            {/*
              An engine that cannot run is still offered, but
              labelled. Hiding it would leave an operator who just
              installed the plugin staring at a picker that does
              not list what they installed, with nothing to
              explain why.
            */}
            {!item.available && (
              <Badge variant="destructive" className="py-0.5 text-[10px]">
                {item.requiresLocalBinary ? t('scanning.binaryMissing') : t('scanning.notConfigured')}
              </Badge>
            )}
          </Button>
        );
      })}
      {adapters.length === 0 && (
        <p className="col-span-full text-sm text-muted-foreground">{t('scanning.noAdapters')}</p>
      )}
    </div>
  );
}
