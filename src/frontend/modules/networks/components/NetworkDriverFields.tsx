// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconInfoCircle } from '@tabler/icons-react';
import { Input } from '@resources/components/ui/Input';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import type { Driver, DriverConfig } from './networkDriverConfig';

interface NetworkDriverFieldsProps {
  driver: Driver;
  cfg: DriverConfig;
  parent: string;
  onParentChange: (value: string) => void;
  mode: string;
  onModeChange: (value: string) => void;
  internal: boolean;
  onInternalChange: (value: boolean) => void;
  attachable: boolean;
  onAttachableChange: (value: boolean) => void;
}

export function NetworkDriverFields({
  driver,
  cfg,
  parent,
  onParentChange,
  mode,
  onModeChange,
  internal,
  onInternalChange,
  attachable,
  onAttachableChange,
}: NetworkDriverFieldsProps) {
  const { t } = useTranslation('networks');

  return (
    <>
      {/* Host/None info */}
      {(driver === 'host' || driver === 'none') && (
        <div className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-sm text-muted-foreground">
          <IconInfoCircle className="h-4 w-4 shrink-0" />
          {t('create.noConfig', { driver })}
        </div>
      )}

      {/* Parent Interface */}
      {cfg.hasParent && (
        <div className="border-t border-border pt-4">
          <Label className="mb-1">
            {t('create.parentInterface')} <span className="text-destructive">*</span>
          </Label>
          <Input
            variant="outline"
            type="text"
            value={parent}
            onChange={(e) => onParentChange(e.target.value)}
            placeholder={t('create.parentPlaceholder')}
          />
          <p className="mt-1 text-xs text-muted-foreground">
            {t('create.parentHelp')}
          </p>
        </div>
      )}

      {/* Mode */}
      {cfg.hasMode && cfg.modes && (
        <div className="border-t border-border pt-4">
          <Label className="mb-1">{t('create.mode')}</Label>
          <Select
            variant="outline"
            value={mode}
            onChange={onModeChange}
            options={cfg.modes.map((m) => ({ value: m, label: m }))}
          />
        </div>
      )}

      {/* Toggles */}
      {cfg.hasToggles && (
        <div className="border-t border-border pt-4 space-y-2">
          <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
            <input
              type="checkbox"
              checked={internal}
              onChange={(e) => onInternalChange(e.target.checked)}
              className="rounded border-input"
            />
            {t('create.internal')}
            <span className="text-muted-foreground">— {t('create.internalHelp')}</span>
          </label>
          <label className="flex items-center gap-2 text-sm text-foreground cursor-pointer">
            <input
              type="checkbox"
              checked={attachable}
              onChange={(e) => onAttachableChange(e.target.checked)}
              className="rounded border-input"
            />
            {t('create.attachable')}
            <span className="text-muted-foreground">— {t('create.attachableHelp')}</span>
          </label>
        </div>
      )}
    </>
  );
}
