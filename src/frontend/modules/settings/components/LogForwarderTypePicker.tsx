// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconPuzzle } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { PluginAssetImage } from '@core/plugins/PluginAssetImage';
import type { LogForwarderAdapter } from '../hooks/useLogForwarders';

type LogForwarderTypePickerProps = {
  adapters: LogForwarderAdapter[];
  onSelect: (adapter: LogForwarderAdapter) => void;
};

/**
 * Step one of adding a forwarder: which transport. Every installed adapter
 * stays on offer however many forwarders already use it — two SIEM
 * collectors or two syslog daemons is a normal setup, not an edge case.
 */
export function LogForwarderTypePicker({ adapters, onSelect }: LogForwarderTypePickerProps) {
  const { t } = useTranslation('settings');

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
      {adapters.map((item) => {
        return (
          <Button
            key={item.forwarderType}
            variant="outline"
            onClick={() => onSelect(item)}
            className="flex h-auto flex-col items-center gap-3 p-4 text-center"
          >
            <div className="flex size-8 items-center justify-center text-primary">
              {item.slug && item.icon ? (
                <PluginAssetImage slug={item.slug} icon={item.icon} className="size-6" />
              ) : (
                <IconPuzzle className="size-6" />
              )}
            </div>
            <span className="text-xs font-medium text-foreground">{item.displayName}</span>
          </Button>
        );
      })}
      {adapters.length === 0 && (
        <p className="col-span-full text-sm text-muted-foreground">{t('logging.noAdapters')}</p>
      )}
    </div>
  );
}
