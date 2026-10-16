// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconEraser } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { NumberInput } from '@resources/components/ui/NumberInput';
import { Spinner } from '@resources/components/ui/Spinner';
import { Tooltip, TooltipTrigger, TooltipContent } from '@resources/components/ui/Tooltip';
import { PERM } from '@core/auth/permissionKeys';
import { SettingsField } from '@resources/components/settings/SettingsGroup';
import type { PurgeMutationLike } from './retention-rows';

type RetentionLogRowProps = {
  labelKey: string;
  descriptionKey: string;
  /** Draft value in the form, which may differ from what is saved. */
  days: number;
  onDaysChange: (days: number) => void;
  /** The horizon the server currently has, which is what a purge acts on. */
  savedDays: number;
  /** Preset-driven rows lock their day count; the preset owns the number. */
  disabled?: boolean;
  purge: PurgeMutationLike;
  /** True while this row's own confirm dialog is open. */
  confirming: boolean;
  onRequestPurge: () => void;
};

/**
 * One purgeable log: how long it is kept, and a "clear now" escape hatch.
 *
 * "Clear now" acts on the *saved* horizon, not the draft one, which is why
 * both numbers are props. A user who types 7 into the box has not yet asked
 * for anything to be deleted, so the button and its tooltip keep quoting the
 * value the server would actually purge against until they press Save.
 *
 * A saved horizon of 0 means "keep forever", and there is then nothing a
 * purge would delete — the button disables and the tooltip says so rather
 * than letting it silently do nothing.
 */
export function RetentionLogRow({
  labelKey,
  descriptionKey,
  days,
  onDaysChange,
  savedDays,
  disabled,
  purge,
  confirming,
  onRequestPurge,
}: RetentionLogRowProps) {
  const { t } = useTranslation('settings');
  const eligible = savedDays > 0;

  return (
    <SettingsField
      htmlFor={`settings-retention-log-row-${labelKey}`}
      label={t(labelKey)}
      description={t(descriptionKey)}
    >
      <div className="flex flex-wrap items-center gap-3">
        <NumberInput
          id={`settings-retention-log-row-${labelKey}`}
          value={days}
          onChange={onDaysChange}
          min={0}
          max={3650}
          disabled={disabled}
          className="w-40"
        />
        <Tooltip>
          <TooltipTrigger asChild>
            {/* The span keeps the tooltip reachable while the button is
                disabled — a disabled button fires no pointer events, so the
                explanation for why it is disabled would be unreadable. */}
            <span className="inline-flex">
              <Button
                permission={PERM.SETTINGS_MANAGE}
                type="button"
                variant="destructive"
                size="sm"
                onClick={eligible ? onRequestPurge : undefined}
                aria-disabled={!eligible}
                className={eligible ? undefined : 'opacity-50'}
                aria-label={t('retention.clearNow')}
              >
                <IconEraser className="size-4" />
                {t('retention.clearNow')}
              </Button>
            </span>
          </TooltipTrigger>
          <TooltipContent>
            {eligible ? t('retention.clearNowHint', { days: savedDays }) : t('retention.keepForever')}
          </TooltipContent>
        </Tooltip>
        {/* Only when the purge is running without this row's dialog open —
            the dialog carries its own loading state. */}
        {purge.isPending && !confirming && <Spinner size="sm" className="text-muted-foreground" />}
      </div>
    </SettingsField>
  );
}
