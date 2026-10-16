// Copyright (c) 2026 McSparrow. All rights reserved.
// McHarbor is licensed under the McHarbor License. See LICENSE for details.

import { useTranslation } from 'react-i18next';
import { IconStack2 } from '@tabler/icons-react';
import { Button } from '@resources/components/ui/Button';
import { Text as TypographyText } from '@resources/components/ui/Typography';
import type { RolePreset } from '../hooks/useRoles';

type RolePresetPickerProps = {
  presets: RolePreset[];
  onApply: (preset: RolePreset) => void;
};

/**
 * Phase 5: preset selector. Replaces the current permission selection
 * with the preset's curated set. Rendered only when there are presets
 * to pick from (presets array is the server-side seed) and the dialog
 * is editable.
 */
export function RolePresetPicker({ presets, onApply }: RolePresetPickerProps) {
  const { t } = useTranslation('security');

  return (
    <div>
      <label className="mb-2 block text-sm font-medium">
        <span className="flex items-center gap-2">
          <IconStack2 className="size-3.5" />
          {t('roles.presets.title')}
        </span>
      </label>
      <TypographyText size="xs" tone="muted" className="mb-2">
        {t('roles.presets.description')}
      </TypographyText>
      <div className="grid gap-2 sm:grid-cols-2">
        {presets.map((p) => (
          <Button
            key={p.key}
            type="button"
            variant="outline"
            size="sm"
            onClick={() => onApply(p)}
            className="h-auto justify-start py-2 text-left whitespace-normal"
            data-testid={`apply-preset-${p.key}`}
          >
            <span className="flex flex-col items-start gap-0.5">
              <span className="font-medium">{p.name}</span>
              <TypographyText size="xs" tone="muted" className="font-normal">
                {p.description}
              </TypographyText>
              <TypographyText size="xs" tone="muted" className="font-normal">
                {t('roles.permissionCount', { count: p.permissions.length })}
              </TypographyText>
            </span>
          </Button>
        ))}
      </div>
    </div>
  );
}
