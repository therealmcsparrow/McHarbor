// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconInfoCircle } from '@tabler/icons-react';
import { Select } from '@resources/components/ui/Select';
import { Label } from '@resources/components/ui/Label';
import { Switch } from '@resources/components/ui/Switch';
import type { Driver, DriverConfig } from './networkDriverConfig';
import { ParentInterfaceField } from './ParentInterfaceField';

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
  parent: parentInterface,
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
      {cfg.hasParent && <ParentInterfaceField value={parentInterface} onChange={onParentChange} />}

      {/* Mode */}
      {cfg.hasMode && cfg.modes && (
        <div className="border-t border-border pt-4">
          <Label className="mb-1">{t('create.mode')}</Label>
          <Select
            ariaLabel={t('create.mode')}
            variant="outline"
            value={mode}
            onChange={onModeChange}
            options={cfg.modes.map((m) => ({
              value: m,
              label: m,
              description: t(`create.modeHelp.${driver}.${m}`),
            }))}
          />
          <p className="mt-1 text-xs text-muted-foreground">{t(`create.modeHelp.${driver}.${mode}`)}</p>
        </div>
      )}

      {/* Toggles */}
      {cfg.hasToggles && (
        <div className="space-y-2 border-t border-border pt-4">
          <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
            <Switch checked={internal} onCheckedChange={onInternalChange} />
            {t('create.internal')}
            <span className="text-muted-foreground">— {t('create.internalHelp')}</span>
          </label>
          <label className="flex cursor-pointer items-center gap-2 text-sm text-foreground">
            <Switch checked={attachable} onCheckedChange={onAttachableChange} />
            {t('create.attachable')}
            <span className="text-muted-foreground">— {t('create.attachableHelp')}</span>
          </label>
        </div>
      )}
    </>
  );
}
