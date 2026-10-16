// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useId } from 'react';
import { useTranslation } from 'react-i18next';
import { Switch } from '@resources/components/ui/Switch';
import { Alert } from '@resources/components/ui/Alert';

type SecretFileModeToggleProps = {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  /** Shown under the switch when on, explaining the trade-off. */
  showDetail?: boolean;
};

/**
 * The per-container / per-stack opt-in for file-based secret injection.
 *
 * Shared across the three places it is offered — container create, App Store
 * install, and the stack settings — so the copy says the same thing in all
 * three. The honesty is the point here: this is a security control with a real
 * limit and a real compatibility cost, and a toggle that only said "more
 * secure" would be selling something.
 */
export function SecretFileModeToggle({
  checked,
  onCheckedChange,
  disabled,
  showDetail = true,
}: SecretFileModeToggleProps) {
  const { t } = useTranslation('secrets');
  const switchId = useId();

  return (
    <div className="space-y-2">
      <label htmlFor={switchId} className="flex items-start gap-3">
        <Switch id={switchId} checked={checked} onCheckedChange={onCheckedChange} disabled={disabled} />
        <span className="space-y-0.5">
          <span className="block text-sm font-medium">{t('fileMode.label')}</span>
          <span className="block text-xs text-muted-foreground">{t('fileMode.hint')}</span>
        </span>
      </label>

      {checked && showDetail && (
        <Alert variant="warning" title={t('fileMode.compatTitle')}>
          <div className="space-y-1.5 text-xs">
            <p>{t('fileMode.compatBody')}</p>
            <p>{t('fileMode.limitBody')}</p>
          </div>
        </Alert>
      )}
    </div>
  );
}
